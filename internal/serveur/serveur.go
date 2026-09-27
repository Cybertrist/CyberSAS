// Package serveur assemble le serveur CyberSAS : l'API où les appareils
// s'inscrivent, le moteur du tunnel, le pare-feu et le DNS du VPN.
//
// Une seule fonction décide de l'état du réseau : Synchroniser. Elle relit
// l'équipe, la politique et les appareils, retire ce qui n'a plus lieu
// d'être, puis pousse le résultat au pare-feu, au moteur et au DNS. Elle
// tourne à chaque inscription, et toutes les cinq secondes, pour prendre
// en compte une modification faite à la main.
package serveur

import (
	"context"
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/netip"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Cybertrist/CyberSAS/internal/b64"

	"github.com/Cybertrist/CyberSAS/internal/base"
	"github.com/Cybertrist/CyberSAS/internal/dns"
	"github.com/Cybertrist/CyberSAS/internal/politique"
	"github.com/Cybertrist/CyberSAS/internal/protocole"
	"github.com/Cybertrist/CyberSAS/internal/tunnel"
	"github.com/Cybertrist/CyberSAS/internal/verrou"
	"github.com/coreos/go-oidc/v3/oidc"
)

type Config struct {
	Domaine       string       // pour le point du tunnel : vpn.<domaine>
	Point         string       // hôte:port UDP annoncé aux appareils
	Reseau        netip.Prefix // 10.77.0.0/24
	Serveur       netip.Addr   // 10.77.0.1
	Interface     string       // sas0
	Equipe        string       // fichier « adresse groupe »
	Politique     string       // fichier JSON
	ClientsGoogle string       // fichier : un identifiant client OAuth par ligne
	// Verrou : clé publique du verrou, en base64. Absent sans verrou.
	Verrou string
	// SignaturePolitique : {"version": N, "signature": "..."}, produit par
	// « sas verrou politique ». Revocations : la liste signée des clés
	// bannies. Tous deux facultatifs, et relus à chaque demande.
	SignaturePolitique string
	Revocations        string
	// RevocationsAppli : la liste signée sur le téléphone d'un admin, que
	// le serveur garde dans son dossier de données (la configuration est en
	// lecture seule). On sert la plus récente des deux.
	RevocationsAppli string
	DureeAppareil    time.Duration
}

type Serveur struct {
	cfg      Config
	base     *base.Base
	moteur   *tunnel.Moteur
	dns      *dns.Serveur
	prive    *ecdh.PrivateKey
	publique string
	journal  *slog.Logger
	rejeux   protocole.Rejeux

	mu        sync.Mutex
	flux      []politique.Flux
	signature string
	// Dernières versions lues avec succès : une équipe ou une politique
	// momentanément illisible ne doit ni tout effacer, ni tout bloquer.
	equipe politique.Equipe
	pol    *politique.Politique
	actifs map[[32]byte]bool // pairs donnés au moteur au dernier passage

	// relations : les paires de numéros d'appareils que le moteur a le
	// droit de relayer, et les mêmes en adresses pour le DNS. Lues à chaque
	// trame, d'où un verrou à part.
	// muRevocations tient une liste reçue de l'appli de la comparaison
	// avec celle en vigueur jusqu'à son écriture : deux envois simultanés
	// ne peuvent ni faire reculer la liste, ni mêler leurs octets.
	muRevocations sync.Mutex

	muRelations sync.RWMutex
	relations   map[[2]uint32]bool
	voit        map[[2]netip.Addr]bool

	// La vérification Google a son propre verrou : aller chercher les clés
	// de Google peut prendre du temps, et ne doit pas bloquer le reste.
	muGoogle sync.Mutex
	verif    *oidc.IDTokenVerifier
}

func Nouveau(cfg Config, b *base.Base, m *tunnel.Moteur, d *dns.Serveur, prive *ecdh.PrivateKey, journal *slog.Logger) *Serveur {
	if journal == nil {
		journal = slog.New(slog.DiscardHandler)
	}
	s := &Serveur{cfg: cfg, base: b, moteur: m, dns: d, journal: journal, prive: prive,
		publique: base64.StdEncoding.EncodeToString(prive.PublicKey().Bytes())}
	d.DefinirFiltre(s.Voit)
	return s
}

// Relie dit si le moteur peut relayer une trame de l'appareil de vers
// l'appareil vers. Appelée pour chaque trame relayée.
func (s *Serveur) Relie(de, vers uint32) bool {
	s.muRelations.RLock()
	defer s.muRelations.RUnlock()
	return s.relations[[2]uint32{de, vers}]
}

// Voit dit si l'appareil à l'adresse de peut connaître le nom de celui à
// l'adresse vers : lui-même, le serveur, et ceux avec qui il est relié.
// Les autres n'existent pas pour lui, pas même dans le DNS.
func (s *Serveur) Voit(de, vers netip.Addr) bool {
	if de == vers || vers == s.cfg.Serveur || de == s.cfg.Serveur {
		return true
	}
	s.muRelations.RLock()
	defer s.muRelations.RUnlock()
	return s.voit[[2]netip.Addr{de, vers}]
}

func jetonAleatoire() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// equipeCourante : la liste de l'équipe, ou la dernière lue avec succès.
// Le booléen dit si la lecture vient de réussir. Appelée avec s.mu tenu.
func (s *Serveur) equipeCourante() (politique.Equipe, bool) {
	e, err := politique.ChargerEquipe(s.cfg.Equipe)
	if err != nil {
		if s.equipe == nil {
			s.journal.Error("équipe illisible, et aucune version précédente : personne n'entre", "erreur", err)
			return politique.Equipe{}, false
		}
		s.journal.Error("équipe illisible : on garde la dernière version lue", "erreur", err)
		return s.equipe, false
	}
	s.equipe = e
	return e, true
}

func (s *Serveur) chargerEquipe() politique.Equipe {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, _ := s.equipeCourante()
	return e
}

// retirer efface un appareil de la base, et le dit au journal. Un
// effacement raté est journalisé comme tel : l'appareil n'est alors plus
// servi, mais sa fiche reste, et le prochain tour réessaiera.
func (s *Serveur) retirer(a base.Appareil, raison string) {
	if err := s.base.Supprimer(a.ID); err != nil {
		s.journal.Error("effacement impossible", "evenement", "retrait", "appareil", a.Nom, "proprietaire", a.Proprietaire,
			"raison", raison, "erreur", err)
		return
	}
	s.journal.Warn("appareil retiré", "evenement", "retrait", "appareil", a.Nom, "proprietaire", a.Proprietaire, "raison", raison)
}

// Synchroniser remet le réseau en accord avec l'équipe, la politique et la
// base. Elle ne touche au noyau que si quelque chose a changé.
func (s *Serveur) Synchroniser() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	equipe, equipeFraiche := s.equipeCourante()
	appareils, err := s.base.Appareils()
	if err != nil {
		return err
	}

	// Les retraits d'abord, et quoi qu'il arrive à la politique : une faute
	// de frappe dans politique.json ne doit pas maintenir un accès révoqué.
	now := time.Now()
	revoquees := s.clesRevoquees()
	var gardes, sortis []base.Appareil
	personnels := 0
	for _, a := range appareils {
		if a.Proprietaire != "" {
			personnels++
		}
		switch {
		case revoquee(revoquees, a.ClePublique):
			// Révoqué par « sas.sh revoquer » : les autres appareils le
			// refusent déjà, le serveur cesse aussi de lui parler.
			s.retirer(a, "révoqué par le verrou")
		case !a.Expire.IsZero() && now.After(a.Expire):
			// Même si l'effacement échoue, l'appareil n'est pas gardé : son
			// accès est coupé, et l'effacement sera retenté au prochain tour.
			s.retirer(a, "inscription expirée")
		case a.Proprietaire != "" && equipe[a.Proprietaire] == "":
			sortis = append(sortis, a)
		default:
			gardes = append(gardes, a)
		}
	}
	// Une équipe qui perd d'un coup plus de la moitié de ses appareils, c'est
	// plus souvent un fichier tronqué qu'une décision : on refuse d'effacer,
	// et l'on prévient. Leur accès est coupé quand même, puisqu'ils ne sont
	// pas dans gardes.
	massive := len(sortis) >= 3 && 2*len(sortis) > personnels
	if len(sortis) > 0 && equipeFraiche && !massive {
		for _, a := range sortis {
			s.retirer(a, "propriétaire sorti de l'équipe")
		}
	} else if len(sortis) > 0 {
		s.journal.Error("purge refusée : appareils coupés mais gardés en base, vérifier equipe.txt",
			"evenement", "purge_refusee", "appareils", len(sortis), "sur", personnels)
	}

	pol, err := politique.Charger(s.cfg.Politique)
	switch {
	case err == nil:
		s.pol = &pol
	case s.pol != nil:
		s.journal.Error("politique illisible : on garde la dernière version lue", "erreur", err)
		pol = *s.pol
	default:
		s.journal.Error("politique illisible, et aucune version précédente : tout est fermé", "erreur", err)
		pol = politique.Politique{}
	}

	var pairs []tunnel.Pair
	var apps []politique.Appareil
	numeros := map[netip.Addr]uint32{}
	noms := map[string]netip.Addr{}
	var sig strings.Builder
	for _, a := range gardes {
		cle, err := b64.Decoder(a.ClePublique)
		if err != nil || len(cle) != 32 {
			continue
		}
		var pub [32]byte
		copy(pub[:], cle)
		// Côté serveur, aucun filtre dans le moteur : ce qui s'adresse au
		// serveur lui-même passe par le pare-feu du noyau.
		pairs = append(pairs, tunnel.Pair{Publique: pub, Adresses: []netip.Prefix{netip.PrefixFrom(a.Adresse, 32)},
			Numero: uint32(a.ID), ToutEntrant: true}) // #nosec G115 -- borné par base.Enregistrer
		apps = append(apps, politique.Appareil{Adresse: a.Adresse, Proprietaire: a.Proprietaire, Etiquette: a.Etiquette})
		numeros[a.Adresse] = uint32(a.ID) // #nosec G115 -- borné par base.Enregistrer
		noms[a.Nom] = a.Adresse
		fmt.Fprintf(&sig, "%d %s %s %s %s %s|", a.ID, a.Nom, a.ClePublique, a.Adresse, a.Proprietaire, a.Etiquette)
	}
	// Les noms du serveur en dernier : aucun appareil ne peut les écraser.
	for n := range base.NomsReserves {
		noms[n] = s.cfg.Serveur
	}
	flux := pol.Compiler(equipe, apps, s.cfg.Serveur)
	regles := politique.Nft(flux, s.cfg.Interface, s.cfg.Serveur, s.cfg.Reseau)
	sig.WriteString(regles)
	if sig.String() == s.signature {
		return nil
	}

	relations := map[[2]uint32]bool{}
	voit := map[[2]netip.Addr]bool{}
	for paire := range politique.Relations(flux, s.cfg.Serveur) {
		de, vers := numeros[paire[0]], numeros[paire[1]]
		if de != 0 && vers != 0 {
			relations[[2]uint32{de, vers}] = true
			voit[paire] = true
		}
	}

	// Le pare-feu d'abord. S'il échoue, le noyau garde ses anciennes règles :
	// on n'ajoute alors aucun nouveau pair, on n'applique que les retraits.
	actifs := map[[32]byte]bool{}
	if err := appliquerPareFeu(regles); err != nil {
		var restreints []tunnel.Pair
		for _, p := range pairs {
			if s.actifs[p.Publique] {
				restreints = append(restreints, p)
				actifs[p.Publique] = true
			}
		}
		s.moteur.DefinirPairs(restreints)
		s.muRelations.Lock()
		for k := range s.relations {
			if !relations[k] {
				delete(s.relations, k)
			}
		}
		s.muRelations.Unlock()
		s.actifs = actifs
		return fmt.Errorf("pare-feu : %w (seuls les retraits sont appliqués)", err)
	}
	for _, p := range pairs {
		actifs[p.Publique] = true
	}
	s.muRelations.Lock()
	s.relations, s.voit = relations, voit
	s.muRelations.Unlock()
	s.moteur.DefinirPairs(pairs)
	s.dns.Definir(noms)
	s.flux, s.signature, s.actifs = flux, sig.String(), actifs
	s.journal.Info("réseau synchronisé", "evenement", "synchronisation", "appareils", len(gardes),
		"flux", len(flux), "relations", len(relations)/2)
	return nil
}

// Boucle : une synchronisation toutes les cinq secondes.
func (s *Serveur) Boucle(ctx context.Context) {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := s.Synchroniser(); err != nil {
				s.journal.Error("synchronisation", "erreur", err)
			}
		}
	}
}

// --- le verrou -------------------------------------------------------------------

// verrou : la clé publique du verrou, si l'admin en a mis un.
func (s *Serveur) verrou() (ed25519.PublicKey, string) {
	if s.cfg.Verrou == "" {
		return nil, ""
	}
	b, err := os.ReadFile(s.cfg.Verrou)
	if err != nil {
		return nil, ""
	}
	texte := strings.TrimSpace(string(b))
	pub, err := verrou.LirePublique(texte)
	if err != nil {
		s.journal.Error("clé du verrou illisible", "erreur", err)
		return nil, ""
	}
	return pub, texte
}

type signatureFichier struct {
	Version   uint64 `json:"version"`
	Signature string `json:"signature"`
}

// documentsSignes ajoute à l'état du réseau la politique signée et la liste
// de révocation, telles que l'admin les a produites. Le serveur ne fait que
// les transmettre : il ne peut pas les modifier sans que les appareils le
// voient.
func (s *Serveur) documentsSignes(e *protocole.EtatReseau) {
	if pol, err := os.ReadFile(s.cfg.Politique); err == nil {
		var sig signatureFichier
		if b, err := os.ReadFile(s.cfg.SignaturePolitique); err == nil && json.Unmarshal(b, &sig) == nil {
			e.Politique = base64.StdEncoding.EncodeToString(pol)
			e.PolitiqueVersion, e.PolitiqueSignature = sig.Version, sig.Signature
		}
	}
	e.Revocations = s.Revocations()
}

// Revocations : la liste de révocation en vigueur, la plus récente de
// celle de « sas.sh revoquer » et de celle venue de l'appli. Aucune : nil.
func (s *Serveur) Revocations() *protocole.ListeRevocations {
	return RevocationsEnVigueur(s.cfg.Revocations, s.cfg.RevocationsAppli)
}

// clesRevoquees : les clés de la liste de révocation en vigueur, seulement
// si elle est bien signée par le verrou (un fichier forgé ne doit pas
// permettre d'effacer des appareils).
func (s *Serveur) clesRevoquees() map[[32]byte]bool {
	l := s.Revocations()
	pub, _ := s.verrou()
	if l == nil || pub == nil {
		return nil
	}
	var cles [][32]byte
	for _, c := range l.Cles {
		k, err := b64.Cle32(c)
		if err != nil {
			return nil
		}
		cles = append(cles, k)
	}
	sig, err := b64.Decoder(l.Signature)
	if err != nil || !verrou.VerifierRevocations(pub, l.Version, cles, sig) {
		s.journal.Error("liste de révocation mal signée, ignorée", "evenement", "revocation")
		return nil
	}
	r := make(map[[32]byte]bool, len(cles))
	for _, k := range cles {
		r[k] = true
	}
	return r
}

// revoquee compare sur les octets de la clé, jamais sur son écriture en
// base64 (voir client.adopterRevocations).
func revoquee(revoquees map[[32]byte]bool, cle string) bool {
	k, err := b64.Cle32(cle)
	return err == nil && revoquees[k]
}

// RevocationsEnVigueur : la plus récente des listes lisibles parmi ces
// fichiers (les absents sont ignorés).
func RevocationsEnVigueur(chemins ...string) *protocole.ListeRevocations {
	var r *protocole.ListeRevocations
	for _, chemin := range chemins {
		if chemin == "" {
			continue
		}
		b, err := os.ReadFile(chemin)
		if err != nil {
			continue
		}
		var l protocole.ListeRevocations
		if json.Unmarshal(b, &l) == nil && (r == nil || l.Version > r.Version) {
			r = &l
		}
	}
	return r
}

// --- Google ------------------------------------------------------------------

func (s *Serveur) clientsGoogle() []string {
	b, err := os.ReadFile(s.cfg.ClientsGoogle)
	if err != nil {
		return nil
	}
	var r []string
	for _, l := range strings.Fields(string(b)) {
		if strings.HasSuffix(l, ".apps.googleusercontent.com") && !strings.HasPrefix(l, "a-remplir") {
			r = append(r, l)
		}
	}
	return r
}

// verifierGoogle valide un jeton d'identité Google : signature par les
// clés publiées par Google, émetteur, expiration, destinataire et partie
// autorisée (azp). Un jeton émis pour une autre application ne passe pas.
// Rend l'adresse et l'identifiant permanent du compte.
func (s *Serveur) verifierGoogle(ctx context.Context, jeton string) (email, sub string, err error) {
	clients := s.clientsGoogle()
	if len(clients) == 0 {
		return "", "", fmt.Errorf("aucun client Google configuré")
	}
	s.muGoogle.Lock()
	if s.verif == nil {
		p, err := oidc.NewProvider(ctx, "https://accounts.google.com")
		if err != nil {
			s.muGoogle.Unlock()
			return "", "", fmt.Errorf("google injoignable : %w", err)
		}
		s.verif = p.Verifier(&oidc.Config{SkipClientIDCheck: true})
	}
	v := s.verif
	s.muGoogle.Unlock()

	tok, err := v.Verify(ctx, jeton)
	if err != nil {
		return "", "", err
	}
	if !slices.ContainsFunc(tok.Audience, func(a string) bool { return slices.Contains(clients, a) }) {
		return "", "", fmt.Errorf("jeton émis pour une autre application")
	}
	var c struct {
		Email   string `json:"email"`
		Verifie bool   `json:"email_verified"`
		Azp     string `json:"azp"`
	}
	if err := tok.Claims(&c); err != nil {
		return "", "", err
	}
	if c.Azp != "" && !slices.Contains(clients, c.Azp) {
		return "", "", fmt.Errorf("jeton obtenu par une autre application (azp)")
	}
	if !c.Verifie || c.Email == "" || tok.Subject == "" {
		return "", "", fmt.Errorf("adresse non vérifiée par Google")
	}
	return strings.ToLower(c.Email), tok.Subject, nil
}

// appliquerPareFeu charge les règles dans le noyau. Variable pour que les
// tests, qui n'ont pas de noyau à configurer, la remplacent.
var appliquerPareFeu = politique.Appliquer
