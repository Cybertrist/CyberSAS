// Package client : ce que tout client CyberSAS fait, quelle que soit la
// plateforme. Le client Linux (cmd/sas) s'en sert, et les applis Android et
// Windows s'en serviront à l'identique, compilées depuis ce même code.
//
// Trois choses :
//   - s'inscrire, en prouvant qu'on détient sa clé privée ;
//   - traduire l'état du réseau, donné par le serveur, en pairs et en règles
//     pour le moteur du tunnel ;
//   - ne croire le serveur que sur ce qu'il ne peut pas falsifier : sa
//     propre clé, retenue à l'inscription, et, quand le réseau a un verrou,
//     ce que l'admin a signé.
package client

import (
	"bytes"
	"crypto/ecdh"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/Cybertrist/CyberSAS/internal/b64"

	"github.com/Cybertrist/CyberSAS/internal/politique"
	"github.com/Cybertrist/CyberSAS/internal/protocole"
	"github.com/Cybertrist/CyberSAS/internal/tunnel"
	"github.com/Cybertrist/CyberSAS/internal/verrou"
)

// ErrDesinscrit : le serveur ne connaît plus cet appareil (retiré, expiré).
var ErrDesinscrit = errors.New("appareil désinscrit")

// tailleMaxReponse : un serveur hostile ne doit pas pouvoir saturer la
// mémoire du client avec une réponse sans fin.
const tailleMaxReponse = 1 << 20

// API : le client HTTPS de l'API.
type API struct {
	Base string
	http *http.Client
}

// NouvelleAPI refuse tout ce qui n'est pas https : la clé d'inscription et
// la clé du serveur ne doivent jamais voyager en clair. autoriteEnPlus
// ajoute une autorité de certification aux racines du système, celle du
// labo par exemple.
func NouvelleAPI(base string, autoriteEnPlus []byte) (*API, error) {
	u, err := url.Parse(base)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return nil, errors.New("l'adresse du serveur doit commencer par https://")
	}
	racines, _ := x509.SystemCertPool()
	if racines == nil {
		racines = x509.NewCertPool()
	}
	if len(autoriteEnPlus) > 0 && !racines.AppendCertsFromPEM(autoriteEnPlus) {
		return nil, errors.New("autorité de certification illisible")
	}
	return &API{Base: strings.TrimRight(base, "/"), http: &http.Client{Timeout: 20 * time.Second,
		// Une redirection pourrait emporter le jeton ailleurs : on n'en
		// suit aucune, l'API n'en fait jamais.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Transport:     &http.Transport{TLSClientConfig: &tls.Config{RootCAs: racines, MinVersion: tls.VersionTLS12}}}}, nil
}

func (a *API) Appel(methode, chemin, jeton string, corps, reponse any) error {
	var b []byte
	if corps != nil {
		b, _ = json.Marshal(corps)
	}
	req, err := http.NewRequest(methode, a.Base+chemin, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if jeton != "" {
		req.Header.Set("Authorization", "Bearer "+jeton)
	}
	rep, err := a.http.Do(req)
	if err != nil {
		return err
	}
	defer rep.Body.Close()
	corpsLu := io.LimitReader(rep.Body, tailleMaxReponse)
	if rep.StatusCode == http.StatusUnauthorized && jeton != "" {
		return ErrDesinscrit
	}
	if rep.StatusCode != http.StatusOK {
		var e protocole.Erreur
		json.NewDecoder(corpsLu).Decode(&e)
		return fmt.Errorf("%s (%d)", Propre(e.Erreur), rep.StatusCode)
	}
	if reponse != nil {
		return json.NewDecoder(corpsLu).Decode(reponse)
	}
	return nil
}

// Propre : un texte venu du serveur, prêt à afficher. Sans caractère de
// contrôle, un serveur hostile ne peut pas maquiller un terminal.
func Propre(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsPrint(r) {
			return r
		}
		return '?'
	}, s)
}

// Justificatif : un jeton Google, ou une clé d'inscription.
type Justificatif struct {
	JetonGoogle, CleInscription string
}

// Inscrire : demande la clé du serveur, calcule la preuve de possession,
// puis s'inscrit. Si verrouAttendu est donné, l'inscription échoue si le
// serveur annonce un autre verrou : c'est ainsi qu'un admin prudent
// installe un appareil, en lui donnant la clé du verrou d'avance.
func (a *API) Inscrire(prive *ecdh.PrivateKey, nom, systeme string, j Justificatif, verrouAttendu string) (protocole.ReponseConnexion, error) {
	var r protocole.ReponseConnexion
	var srv protocole.Serveur
	if err := a.Appel("GET", protocole.CheminServeur, "", nil, &srv); err != nil {
		return r, fmt.Errorf("clé du serveur : %w", err)
	}
	cleServeur, err := b64.Decoder(srv.ClePublique)
	if err != nil || len(cleServeur) != 32 {
		return r, errors.New("clé du serveur invalide")
	}
	d := protocole.DemandeConnexion{JetonGoogle: j.JetonGoogle, CleInscription: j.CleInscription, Nom: nom, Systeme: systeme,
		ClePublique: base64.StdEncoding.EncodeToString(prive.PublicKey().Bytes()), Horodatage: time.Now().Unix()}
	if err := d.Prouver(prive, cleServeur); err != nil {
		return r, err
	}
	if err := a.Appel("POST", protocole.CheminConnexion, "", d, &r); err != nil {
		return r, err
	}
	// La réponse doit parler de la clé que l'on vient d'utiliser pour la
	// preuve : sinon, le serveur a changé en cours de route.
	if r.Serveur.ClePublique != srv.ClePublique {
		return r, errors.New("le serveur a changé de clé pendant l'inscription")
	}
	if verrouAttendu != "" && r.Verrou != verrouAttendu {
		return r, fmt.Errorf("le serveur annonce un autre verrou (%s) que celui attendu (%s)",
			EmpreinteVerrou(r.Verrou), EmpreinteVerrou(verrouAttendu))
	}
	return r, nil
}

func EmpreinteVerrou(texte string) string {
	if texte == "" {
		return "aucun"
	}
	pub, err := verrou.LirePublique(texte)
	if err != nil {
		return "illisible"
	}
	return verrou.Empreinte(pub)
}

// EmpreinteCle : l'empreinte d'une clé d'appareil, que l'admin compare avec
// celle affichée par l'appareil lui-même avant de le signer.
func EmpreinteCle(texte string) string {
	b, err := b64.Decoder(texte)
	if err != nil {
		return "illisible"
	}
	return verrou.Empreinte(b)
}

// Retenu : ce que l'appareil a appris et ne laisse plus changer. Le client
// le garde sur disque, et Construire le met à jour.
type Retenu struct {
	CleServeur string       `json:"cle_serveur"`
	Verrou     string       `json:"verrou,omitempty"`
	MaCle      string       `json:"ma_cle"`
	Moi        netip.Addr   `json:"moi"`
	Reseau     netip.Prefix `json:"reseau"`
	// Plus hautes versions signées déjà vues : jamais de retour en arrière.
	VersionPolitique   uint64   `json:"version_politique,omitempty"`
	VersionRevocations uint64   `json:"version_revocations,omitempty"`
	Revoquees          []string `json:"revoquees,omitempty"`
}

// Ecarte : un pair ou un document que le serveur annonce, mais que ce
// client refuse, et pourquoi.
type Ecarte struct {
	Nom, Adresse, Raison string
}

// Construire traduit l'état du réseau en pairs pour le moteur.
//
// Sans verrou, le client croit le serveur sur les pairs et les règles : il
// ne se protège que d'un serveur qui changerait de clé.
//
// Avec verrou, il ne croit que ce que l'admin a signé :
//   - les pairs sans certificat valide, expiré ou révoqué sont écartés ;
//   - ses propres règles d'entrée, il les calcule lui-même à partir de la
//     politique signée et des certificats : le serveur ne peut pas lui
//     ouvrir un port ;
//   - une politique ou une liste de révocation plus ancienne que la
//     dernière vue est refusée.
func Construire(r protocole.EtatReseau, ret *Retenu, point netip.AddrPort, maintenant time.Time) ([]tunnel.Pair, []Ecarte, error) {
	if r.Serveur.ClePublique != ret.CleServeur {
		return nil, nil, errors.New("le serveur annonce une autre clé que celle retenue à l'inscription : refus")
	}
	if r.Verrou != ret.Verrou {
		return nil, nil, fmt.Errorf("le serveur annonce un autre verrou (%s) que celui retenu (%s) : refus",
			EmpreinteVerrou(r.Verrou), EmpreinteVerrou(ret.Verrou))
	}
	adresseServeur, err := netip.ParseAddr(r.Serveur.Adresse)
	if err != nil || !adresseServeur.Is4() || !ret.Reseau.Contains(adresseServeur) {
		return nil, nil, errors.New("adresse du serveur invalide")
	}
	pubServeur, err := cle32(ret.CleServeur)
	if err != nil {
		return nil, nil, err
	}

	var ecartes []Ecarte
	// valide : une adresse de pair doit être IPv4, dans le réseau, et ni la
	// nôtre ni celle du serveur.
	valide := func(p protocole.Appareil) ([32]byte, netip.Addr, bool) {
		pub, err1 := cle32(p.ClePublique)
		a, err2 := netip.ParseAddr(p.Adresse)
		ok := err1 == nil && err2 == nil && a.Is4() && ret.Reseau.Contains(a) && a != ret.Moi &&
			a != adresseServeur && p.Numero != 0 && p.ClePublique != ret.MaCle
		if !ok {
			ecartes = append(ecartes, Ecarte{Propre(p.Nom), Propre(p.Adresse), "annonce malformée"})
		}
		return pub, a, ok
	}

	type retenuPair struct {
		p   protocole.Appareil
		pub [32]byte
		a   netip.Addr
	}
	var retenus []retenuPair
	var entrant map[netip.Addr][]tunnel.Regle

	if ret.Verrou == "" {
		for _, p := range r.Pairs {
			if pub, a, ok := valide(p); ok {
				retenus = append(retenus, retenuPair{p, pub, a})
			}
		}
		if entrant, err = entrantAnnonce(r.Entrant); err != nil {
			return nil, nil, err
		}
	} else {
		cleVerrou, err := verrou.LirePublique(ret.Verrou)
		if err != nil {
			return nil, nil, err
		}
		ecartes = append(ecartes, adopterRevocations(r.Revocations, cleVerrou, ret)...)
		// Les révocations se comparent sur les octets des clés, jamais sur
		// leur écriture : une même clé peut s'écrire de plusieurs façons en
		// base64, et un serveur piraté en profiterait pour faire passer une
		// clé révoquée.
		revoquees := map[[32]byte]bool{}
		for _, c := range ret.Revoquees {
			if k, err := cle32(c); err == nil {
				revoquees[k] = true
			}
		}
		pol, note := adopterPolitique(r, cleVerrou, ret)
		if note != "" {
			ecartes = append(ecartes, Ecarte{"politique", "", note})
		}
		certifie := func(p protocole.Appareil, pub [32]byte, a netip.Addr) bool {
			sig, _ := b64.Decoder(p.Signature)
			c := verrou.Certificat{Cle: pub, Adresse: a, Etiquette: p.Etiquette, Proprietaire: p.Proprietaire,
				Groupe: p.Groupe, Expire: p.SignatureExpire}
			return c.Verifier(cleVerrou, sig, maintenant)
		}
		// Nos propres attributs doivent aussi être signés : ce sont eux qui
		// disent quelles règles nous concernent.
		equipe := politique.Equipe{}
		var apps []politique.Appareil
		if maCle, err := cle32(ret.MaCle); err == nil && r.Moi.ClePublique == ret.MaCle && certifie(r.Moi, maCle, ret.Moi) {
			apps = append(apps, politique.Appareil{Adresse: ret.Moi, Proprietaire: r.Moi.Proprietaire, Etiquette: r.Moi.Etiquette})
			if r.Moi.Proprietaire != "" {
				equipe[strings.ToLower(r.Moi.Proprietaire)] = r.Moi.Groupe
			}
		} else {
			ecartes = append(ecartes, Ecarte{"cet appareil", ret.Moi.String(), "pas encore signé par le verrou : rien ne peut entrer"})
		}
		for _, p := range r.Pairs {
			pub, a, ok := valide(p)
			switch {
			case !ok:
			case revoquees[pub]:
				ecartes = append(ecartes, Ecarte{Propre(p.Nom), a.String(), "révoqué par le verrou"})
			case !certifie(p, pub, a):
				ecartes = append(ecartes, Ecarte{Propre(p.Nom), a.String(), "pas signé par le verrou (ou certificat expiré)"})
			default:
				retenus = append(retenus, retenuPair{p, pub, a})
				apps = append(apps, politique.Appareil{Adresse: a, Proprietaire: p.Proprietaire, Etiquette: p.Etiquette})
				if p.Proprietaire != "" {
					equipe[strings.ToLower(p.Proprietaire)] = p.Groupe
				}
			}
		}
		entrant = map[netip.Addr][]tunnel.Regle{}
		if pol != nil && len(apps) > 0 && apps[0].Adresse == ret.Moi {
			flux := pol.Compiler(equipe, apps, adresseServeur)
			for _, e := range politique.Entrant(flux, ret.Moi) {
				regles, err := versRegles(e.Ports)
				if err != nil {
					return nil, nil, err
				}
				entrant[e.Source] = append(entrant[e.Source], regles...)
			}
		}
	}

	pairs := []tunnel.Pair{{Publique: pubServeur, Adresses: []netip.Prefix{netip.PrefixFrom(adresseServeur, 32)},
		Point: point, Maintien: 25 * time.Second, Entrant: entrant[adresseServeur]}}
	for _, x := range retenus {
		pairs = append(pairs, tunnel.Pair{Publique: x.pub, Adresses: []netip.Prefix{netip.PrefixFrom(x.a, 32)},
			Numero: x.p.Numero, ParRelais: true, Entrant: entrant[x.a]})
	}
	return pairs, ecartes, nil
}

// adopterRevocations : une liste signée plus récente que la nôtre la
// remplace. Plus ancienne, ou mal signée, elle est ignorée : on garde la
// nôtre, qui ne peut que s'allonger au fil des versions.
func adopterRevocations(l *protocole.ListeRevocations, cleVerrou []byte, ret *Retenu) []Ecarte {
	if l == nil || l.Version <= ret.VersionRevocations {
		return nil
	}
	if _, ok := RevocationsSignees(l, cleVerrou); !ok {
		return []Ecarte{{"révocations", "", "liste malformée ou mal signée, ignorée"}}
	}
	ret.VersionRevocations, ret.Revoquees = l.Version, slices.Clone(l.Cles)
	return nil
}

// adopterPolitique : la politique signée, si elle est valide et au moins
// aussi récente que la dernière vue. Sinon, aucune : rien n'entre, sauf
// les réponses à ce que l'on a ouvert.
func adopterPolitique(r protocole.EtatReseau, cleVerrou []byte, ret *Retenu) (*politique.Politique, string) {
	if r.Politique == "" {
		return nil, "aucune politique signée : rien ne peut entrer"
	}
	brut, err1 := b64.Decoder(r.Politique)
	sig, err2 := b64.Decoder(r.PolitiqueSignature)
	if err1 != nil || err2 != nil || !verrou.VerifierPolitique(cleVerrou, r.PolitiqueVersion, brut, sig) {
		return nil, "politique mal signée : refusée"
	}
	if r.PolitiqueVersion < ret.VersionPolitique {
		return nil, fmt.Sprintf("politique plus ancienne (%d) que la dernière vue (%d) : refusée", r.PolitiqueVersion, ret.VersionPolitique)
	}
	var p politique.Politique
	if err := json.Unmarshal(brut, &p); err != nil || p.Version != r.PolitiqueVersion {
		return nil, "politique illisible ou version incohérente : refusée"
	}
	if _, err := politique.Verifier(p); err != nil {
		return nil, "politique invalide : " + err.Error()
	}
	ret.VersionPolitique = r.PolitiqueVersion
	return &p, ""
}

func entrantAnnonce(liste []protocole.RegleEntrante) (map[netip.Addr][]tunnel.Regle, error) {
	entrant := map[netip.Addr][]tunnel.Regle{}
	for _, e := range liste {
		ports, err := politique.LirePorts(e.Ports)
		if err != nil {
			return nil, err
		}
		regles, _ := versRegles(ports)
		for _, s := range e.Sources {
			if a, err := netip.ParseAddr(s); err == nil && a.Is4() {
				entrant[a] = append(entrant[a], regles...)
			}
		}
	}
	return entrant, nil
}

// cle32 : une clé de 32 octets, en écriture base64 canonique (voir b64).
func cle32(texte string) ([32]byte, error) { return b64.Cle32(texte) }

// versRegles traduit des ports de la politique en règles du moteur.
func versRegles(ports []politique.Port) ([]tunnel.Regle, error) {
	var r []tunnel.Regle
	for _, p := range ports {
		switch p.Proto {
		case "*":
			r = append(r, tunnel.Regle{})
		case "icmp":
			r = append(r, tunnel.Regle{Proto: 1})
		case "tcp":
			r = append(r, tunnel.Regle{Proto: 6, Debut: p.Debut, Fin: p.Fin})
		case "udp":
			r = append(r, tunnel.Regle{Proto: 17, Debut: p.Debut, Fin: p.Fin})
		default:
			return nil, fmt.Errorf("protocole inconnu : %s", p.Proto)
		}
	}
	return r, nil
}
