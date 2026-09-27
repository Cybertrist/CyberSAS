package serveur

import (
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/netip"
	"strings"
	"time"
	"unicode"

	"github.com/Cybertrist/CyberSAS/internal/b64"

	"github.com/Cybertrist/CyberSAS/internal/base"
	"github.com/Cybertrist/CyberSAS/internal/politique"
	"github.com/Cybertrist/CyberSAS/internal/protocole"
	"github.com/Cybertrist/CyberSAS/internal/verrou"
)

func (s *Serveur) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST "+protocole.CheminConnexion, s.connexion)
	mux.HandleFunc("GET "+protocole.CheminReseau, s.reseau)
	mux.HandleFunc("POST "+protocole.CheminDeconnexion, s.deconnexion)
	mux.HandleFunc("POST "+protocole.CheminLibelle, s.libelle)
	mux.HandleFunc("GET "+protocole.CheminAppareils, s.appareils)
	mux.HandleFunc("POST "+protocole.CheminSignatures, s.signatures)
	mux.HandleFunc("POST "+protocole.CheminRetrait, s.retrait)
	mux.HandleFunc("POST "+protocole.CheminInvitation, s.invitation)
	mux.HandleFunc("POST "+protocole.CheminRevocations, s.revocations)
	mux.HandleFunc("GET "+protocole.CheminServeur, func(w http.ResponseWriter, _ *http.Request) {
		repondre(w, http.StatusOK, s.infoServeur())
	})
	mux.HandleFunc("GET "+protocole.CheminSante, func(w http.ResponseWriter, _ *http.Request) {
		repondre(w, http.StatusOK, map[string]string{"etat": "ok"})
	})
	return mux
}

func (s *Serveur) infoServeur() protocole.Serveur {
	return protocole.Serveur{ClePublique: s.publique, Point: s.cfg.Point, Adresse: s.cfg.Serveur.String()}
}

func repondre(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	// L'en-tête est parti : si le client a raccroché, il n'y a plus
	// personne à prévenir.
	_ = json.NewEncoder(w).Encode(v)
}

func refuser(w http.ResponseWriter, code int, message string) {
	repondre(w, code, protocole.Erreur{Erreur: message})
}

// clePubliqueValide écarte les points de petit ordre de X25519, comme la
// clé nulle : avec eux, le secret partagé ne dépend plus de notre clé.
// L'essai d'un échange avec une clé jetable suffit à les reconnaître,
// crypto/ecdh refusant tout secret nul.
func clePubliqueValide(b []byte) bool {
	pub, err := ecdh.X25519().NewPublicKey(b)
	if err != nil {
		return false
	}
	jetable, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return false
	}
	_, err = jetable.ECDH(pub)
	return err == nil
}

// texteCourt : ce qu'un appareil dit de lui-même (nom, système) finit
// affiché chez les autres. Caractères imprimables seulement, et court :
// pas de séquence de terminal, pas de roman.
func texteCourt(s string, max int) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsPrint(r) {
			return r
		}
		return -1
	}, s)
	if r := []rune(s); len(r) > max {
		s = string(r[:max])
	}
	return strings.TrimSpace(s)
}

// ip : Nginx a remplacé X-Real-IP par l'adresse qu'il a vue.
func ip(r *http.Request) string {
	if v := r.Header.Get("X-Real-IP"); v != "" {
		return v
	}
	return r.RemoteAddr
}

var errHorsEquipe = errors.New("cet utilisateur n'a pas accès à ce réseau")

func (s *Serveur) connexion(w http.ResponseWriter, r *http.Request) {
	var d protocole.DemandeConnexion
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&d); err != nil {
		refuser(w, http.StatusBadRequest, "demande illisible")
		return
	}
	cle, err := b64.Decoder(d.ClePublique)
	if err != nil || len(cle) != 32 || !clePubliqueValide(cle) {
		refuser(w, http.StatusBadRequest, "clé publique invalide")
		return
	}
	journal := s.journal.With("evenement", "connexion", "ip", ip(r), "appareil", texteCourt(d.Nom, 30))
	// D'abord la preuve de possession : elle ne coûte rien, et sans elle on
	// ne consomme ni jeton Google ni clé d'inscription. Elle couvre toute la
	// demande, et ne sert qu'une fois.
	now := time.Now()
	if !d.VerifierPreuve(s.prive, cle, now) {
		journal.Warn("preuve de possession invalide")
		refuser(w, http.StatusUnauthorized, "preuve de possession de la clé invalide (horloge décalée ?)")
		return
	}
	if s.rejeux.DejaVue(d.Preuve, now) {
		journal.Warn("demande d'inscription rejouée")
		refuser(w, http.StatusUnauthorized, "demande déjà reçue")
		return
	}

	// Le nom proposé devient le libellé tel quel ; le nom sur le réseau en
	// est tiré (minuscules, tirets), voir base.NomPropre.
	a := base.Appareil{Nom: texteCourt(d.Nom, 30), Libelle: texteCourt(d.Nom, longueurLibelle), ClePublique: d.ClePublique,
		Systeme: texteCourt(d.Systeme, 32)}
	ins := base.Inscription{Jeton: jetonAleatoire()}
	switch {
	case d.JetonGoogle != "" && d.CleInscription != "":
		refuser(w, http.StatusBadRequest, "un seul justificatif à la fois")
		return
	case d.JetonGoogle != "":
		email, sub, err := s.verifierGoogle(r.Context(), d.JetonGoogle)
		if err != nil {
			journal.Warn("jeton Google refusé", "erreur", err)
			refuser(w, http.StatusUnauthorized, "connexion Google refusée")
			return
		}
		if s.chargerEquipe()[email] == "" {
			journal.Warn("adresse hors de l'équipe", "email", email)
			refuser(w, http.StatusForbidden, "ce compte Google n'a pas accès à ce réseau")
			return
		}
		if err := s.base.LierCompte(email, sub); err != nil {
			journal.Warn("adresse liée à un autre compte Google", "email", email)
			refuser(w, http.StatusForbidden, "cette adresse est liée à un autre compte Google")
			return
		}
		a.Proprietaire = email
	case d.CleInscription != "":
		ins.CleInscription = d.CleInscription
	default:
		refuser(w, http.StatusBadRequest, "il faut un jeton Google ou une clé d'inscription")
		return
	}
	// Vérifié dans la transaction, avant d'écrire : un refus ne gaspille pas
	// la clé d'inscription.
	ins.Accepter = func(etiquette, utilisateur string) error {
		if utilisateur != "" && s.chargerEquipe()[utilisateur] == "" {
			return errHorsEquipe
		}
		return nil
	}
	ins.Appareil, ins.DureePersonnel = a, s.cfg.DureeAppareil

	a, err = s.base.Enregistrer(ins, s.cfg.Reseau, s.cfg.Serveur)
	switch {
	case errors.Is(err, base.ErrIntrouvable):
		journal.Warn("clé d'inscription refusée")
		refuser(w, http.StatusUnauthorized, "clé d'inscription inconnue, déjà utilisée ou expirée")
		return
	case errors.Is(err, errHorsEquipe):
		refuser(w, http.StatusForbidden, err.Error())
		return
	case errors.Is(err, base.ErrAutreProprietaire), errors.Is(err, base.ErrNomPris):
		journal.Warn("inscription en conflit", "erreur", err)
		refuser(w, http.StatusConflict, err.Error())
		return
	case errors.Is(err, base.ErrReseauPlein):
		refuser(w, http.StatusServiceUnavailable, err.Error())
		return
	case err != nil:
		journal.Error("inscription impossible", "erreur", err)
		refuser(w, http.StatusInternalServerError, "inscription impossible")
		return
	}
	s.rejeux.Retenir(d.Preuve, now)
	if err := s.Synchroniser(); err != nil {
		journal.Error("synchronisation", "erreur", err)
	}
	journal.Info("appareil inscrit", "nom", a.Nom, "adresse", a.Adresse, "proprietaire", a.Proprietaire, "etiquette", a.Etiquette)
	_, texteVerrou := s.verrou()
	repondre(w, http.StatusOK, protocole.ReponseConnexion{
		Appareil: s.vers(a, a, time.Time{}),
		Reseau:   s.cfg.Reseau.String(),
		DNS:      s.cfg.Serveur.String(),
		Domaine:  "sas.internal",
		Serveur:  s.infoServeur(),
		Verrou:   texteVerrou,
		Jeton:    ins.Jeton,
	})
}

// vers : un appareil tel que le voit demandeur. Chacun voit tout de
// lui-même ; des autres, le nécessaire au tunnel et au verrou. Une
// machine ne voit pas les adresses email des personnes.
func (s *Serveur) vers(a, demandeur base.Appareil, poignee time.Time) protocole.Appareil {
	moi := a.ID == demandeur.ID
	// #nosec G115 -- a.ID est borné par base.Enregistrer.
	r := protocole.Appareil{Numero: uint32(a.ID), Nom: a.Nom, Libelle: a.Libelle, Adresse: a.Adresse.String(), ClePublique: a.ClePublique,
		Etiquette: a.Etiquette, Moi: moi, EnLigne: !poignee.IsZero() && time.Since(poignee) < 3*time.Minute,
		Groupe: a.SignatureGroupe, SignatureExpire: a.SignatureExpire}
	if len(a.Signature) > 0 {
		r.Signature = base64.StdEncoding.EncodeToString(a.Signature)
	}
	// Le propriétaire fait partie du certificat : il faut le donner à qui
	// doit le vérifier. On le tait seulement quand le demandeur est une
	// machine et qu'il n'y a pas de certificat à vérifier.
	if moi || demandeur.Etiquette == "" || len(a.Signature) > 0 {
		r.Proprietaire = a.Proprietaire
	}
	// Le système (android, linux) dit quelle icône montrer, aux appareils
	// des personnes seulement : une machine n'a pas à savoir ce que les
	// gens utilisent. L'expiration ne regarde que l'appareil lui-même.
	if moi || demandeur.Etiquette == "" {
		r.Systeme = a.Systeme
	}
	if moi {
		r.Expire = a.Expire
	}
	return r
}

func (s *Serveur) authentifier(w http.ResponseWriter, r *http.Request) (base.Appareil, bool) {
	jeton, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || jeton == "" {
		refuser(w, http.StatusUnauthorized, "jeton manquant")
		return base.Appareil{}, false
	}
	a, err := s.base.ParJeton(jeton)
	if errors.Is(err, base.ErrIntrouvable) {
		refuser(w, http.StatusUnauthorized, "appareil inconnu : il faut se reconnecter")
		return a, false
	}
	if err != nil {
		refuser(w, http.StatusInternalServerError, "base indisponible")
		return a, false
	}
	// Coupé par une purge refusée (voir Synchroniser) : il reste en base,
	// mais son propriétaire n'est plus dans l'équipe.
	if a.Proprietaire != "" && s.chargerEquipe()[a.Proprietaire] == "" {
		refuser(w, http.StatusUnauthorized, "appareil inconnu : il faut se reconnecter")
		return a, false
	}
	return a, true
}

// reseau : ce qu'un appareil doit savoir pour configurer son tunnel. Ses
// pairs sont les seuls appareils avec qui la politique le relie ; les
// autres n'existent pas pour lui.
func (s *Serveur) reseau(w http.ResponseWriter, r *http.Request) {
	moi, ok := s.authentifier(w, r)
	if !ok {
		return
	}
	s.mu.Lock()
	flux := s.flux
	s.mu.Unlock()
	relies := map[netip.Addr]bool{}
	for paire := range politique.Relations(flux, s.cfg.Serveur) {
		if paire[0] == moi.Adresse {
			relies[paire[1]] = true
		}
	}
	poignees := map[string]time.Time{}
	for _, e := range s.moteur.Etat() {
		poignees[base64.StdEncoding.EncodeToString(e.Publique[:])] = e.DernierePoignee
	}
	tous, err := s.base.Appareils()
	if err != nil {
		refuser(w, http.StatusInternalServerError, "base indisponible")
		return
	}
	_, texteVerrou := s.verrou()
	etat := protocole.EtatReseau{Serveur: s.infoServeur(), Verrou: texteVerrou, Pairs: []protocole.Appareil{}, Entrant: []protocole.RegleEntrante{}}
	for _, a := range tous {
		switch {
		case a.ID == moi.ID:
			etat.Moi = s.vers(a, moi, poignees[a.ClePublique])
		case relies[a.Adresse]:
			etat.Pairs = append(etat.Pairs, s.vers(a, moi, poignees[a.ClePublique]))
		}
	}
	for _, e := range politique.Entrant(flux, moi.Adresse) {
		var ports []string
		for _, p := range e.Ports {
			ports = append(ports, p.String())
		}
		etat.Entrant = append(etat.Entrant, protocole.RegleEntrante{Sources: []string{e.Source.String()}, Ports: ports})
	}
	if texteVerrou != "" {
		s.documentsSignes(&etat)
	}
	repondre(w, http.StatusOK, etat)
}

func (s *Serveur) deconnexion(w http.ResponseWriter, r *http.Request) {
	moi, ok := s.authentifier(w, r)
	if !ok {
		return
	}
	// Sans effacement, l'appareil croirait être parti alors que sa clé
	// ouvrirait encore le tunnel : on le lui dit, il pourra réessayer.
	if err := s.base.Supprimer(moi.ID); err != nil {
		s.journal.Error("déconnexion impossible", "evenement", "deconnexion", "appareil", moi.Nom, "erreur", err)
		refuser(w, http.StatusInternalServerError, "déconnexion impossible, réessayer")
		return
	}
	// L'appareil est effacé de la base : si ce tour échoue, le suivant le
	// retirera du tunnel quoi qu'il arrive.
	if err := s.Synchroniser(); err != nil {
		s.journal.Error("synchronisation après déconnexion", "evenement", "synchronisation", "erreur", err)
	}
	s.journal.Info("appareil déconnecté", "evenement", "deconnexion", "appareil", moi.Nom, "ip", ip(r))
	repondre(w, http.StatusOK, map[string]string{"etat": "déconnecté"})
}

// ImporterCertificats enregistre des certificats signés par le verrou,
// après les avoir vérifiés avec sa clé publique et avec la fiche de chaque
// appareil : étiquette et propriétaire doivent être ceux de la base. Un
// certificat faux, périmé ou qui ne correspond plus est refusé. La clé
// privée du verrou n'est jamais passée par le serveur : il ne peut que
// transmettre ce que l'admin a signé.
func ImporterCertificats(b *base.Base, cleVerrou ed25519.PublicKey, liste []protocole.Appareil) (acceptes int, refuses []string) {
	fiches, err := b.Appareils()
	if err != nil {
		return 0, []string{"base indisponible"}
	}
	parCle := map[string]base.Appareil{}
	for _, f := range fiches {
		parCle[f.ClePublique] = f
	}
	for _, a := range liste {
		f, connu := parCle[a.ClePublique]
		k, err1 := b64.Decoder(a.ClePublique)
		sig, err2 := b64.Decoder(a.Signature)
		if !connu || err1 != nil || err2 != nil || len(k) != 32 {
			refuses = append(refuses, a.Nom)
			continue
		}
		c := verrou.Certificat{Adresse: f.Adresse, Etiquette: f.Etiquette, Proprietaire: f.Proprietaire,
			Groupe: a.Groupe, Expire: a.SignatureExpire}
		copy(c.Cle[:], k)
		if !c.Verifier(cleVerrou, sig, time.Now()) ||
			b.DefinirCertificat(a.ClePublique, f.Adresse, sig, a.Groupe, a.SignatureExpire) != nil {
			refuses = append(refuses, a.Nom)
			continue
		}
		acceptes++
	}
	return
}
