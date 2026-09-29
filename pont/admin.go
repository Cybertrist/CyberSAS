package pont

import (
	"cmp"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/Cybertrist/CyberSAS/internal/appareil"
	"github.com/Cybertrist/CyberSAS/internal/b64"
	"github.com/Cybertrist/CyberSAS/internal/client"
	"github.com/Cybertrist/CyberSAS/internal/protocole"
	"github.com/Cybertrist/CyberSAS/internal/verrou"
)

// dureeCertificat : comme « sas verrou signer » par défaut.
const dureeCertificat = 90 * 24 * time.Hour

// appel : un appel à l'API, au nom de cet appareil.
func appel(dossier, methode, chemin string, corps, reponse any) error {
	e, err := appareil.Stockage{Dossier: dossier}.Lire()
	if err != nil {
		return errors.New("pas inscrit")
	}
	a, err := api(e.Serveur, e.Autorite)
	if err != nil {
		return err
	}
	return a.Appel(methode, chemin, e.Inscription.Jeton, corps, reponse)
}

// Libeller change le nom affiché d'un appareil : le sien si cle est vide.
func Libeller(dossier, cle, libelle string) error {
	return appel(dossier, "POST", protocole.CheminLibelle, protocole.DemandeLibelle{ClePublique: cle, Libelle: libelle}, nil)
}

// Fiche : un appareil vu par l'admin, pour l'écran des demandes.
type Fiche struct {
	Nom          string `json:"nom"`
	Libelle      string `json:"libelle"`
	Adresse      string `json:"adresse"`
	Proprietaire string `json:"proprietaire"`
	Etiquette    string `json:"etiquette"`
	Groupe       string `json:"groupe"`
	Systeme      string `json:"systeme"`
	Cle          string `json:"cle"`
	Empreinte    string `json:"empreinte"`
	Signe        bool   `json:"signe"`
}

// Appareils : tous les appareils du réseau, signés ou non, en JSON
// ([]Fiche). Réservé aux admins.
func Appareils(dossier string) (string, error) {
	var liste []protocole.Appareil
	if err := appel(dossier, "GET", protocole.CheminAppareils, nil, &liste); err != nil {
		return "", err
	}
	r := []Fiche{}
	for _, a := range liste {
		r = append(r, Fiche{Nom: client.Propre(a.Nom), Libelle: client.Propre(a.Libelle), Adresse: a.Adresse,
			Proprietaire: client.Propre(a.Proprietaire), Etiquette: client.Propre(a.Etiquette), Groupe: client.Propre(a.Groupe),
			Systeme: client.Propre(a.Systeme), Cle: a.ClePublique, Empreinte: client.EmpreinteCle(a.ClePublique), Signe: a.Signature != ""})
	}
	b, _ := json.Marshal(r)
	return string(b), nil
}

// lireVerrou : la graine de la clé privée du verrou, en base64 (le contenu
// de etat/verrou/cle). Elle doit aller avec le verrou que cet appareil a
// retenu à l'inscription : sinon, ses signatures ne vaudraient rien.
func lireVerrou(dossier, graine string) (ed25519.PrivateKey, error) {
	// Un copier-coller emporte souvent un espace, un retour à la ligne ou
	// des guillemets : on ne garde que l'alphabet du base64.
	propre := strings.Map(func(r rune) rune {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '+' || r == '/' || r == '=' {
			return r
		}
		return -1
	}, graine)
	g, err := b64.Decoder(propre)
	if err != nil || len(g) != ed25519.SeedSize {
		return nil, fmt.Errorf("clé du verrou illisible : %d caractères reçus, 44 attendus", len(propre))
	}
	prive := ed25519.NewKeyFromSeed(g)
	e, err := appareil.Stockage{Dossier: dossier}.Lire()
	if err != nil {
		return nil, errors.New("pas inscrit")
	}
	attendu, err := verrou.LirePublique(e.Retenu.Verrou)
	if err != nil {
		return nil, errors.New("ce réseau n'a pas de verrou")
	}
	if !prive.Public().(ed25519.PublicKey).Equal(attendu) {
		return nil, errors.New("cette clé n'est pas celle du verrou de ce réseau")
	}
	return prive, nil
}

// VerifierVerrou : l'empreinte du verrou si la graine est bien la sienne.
func VerifierVerrou(dossier, graine string) (string, error) {
	prive, err := lireVerrou(dossier, graine)
	if err != nil {
		return "", err
	}
	return verrou.Empreinte(prive.Public().(ed25519.PublicKey)), nil
}

// Signer signe, avec la clé du verrou, les appareils de fiches : un
// tableau JSON de Fiche, exactement celles que l'écran des demandes a
// montrées à l'admin (Appareils). On ne signe que ce qu'il a vu :
//   - la clé, dont il a comparé l'empreinte avec celle de l'appareil ;
//   - l'adresse, le propriétaire ou l'étiquette, et le groupe affichés.
//
// Le serveur est relu au moment de signer : si un seul de ces champs a
// changé entre-temps, on refuse. Sinon un serveur piraté montrerait
// « groupe equipe » et ferait signer « groupe admins », ou l'adresse d'un
// autre appareil. Rend le nombre d'appareils signés.
func Signer(dossier, graine, fiches string) (int, error) {
	prive, err := lireVerrou(dossier, graine)
	if err != nil {
		return 0, err
	}
	var vues []Fiche
	if err := json.Unmarshal([]byte(fiches), &vues); err != nil || len(vues) == 0 {
		return 0, errors.New("aucun appareil à signer")
	}
	e, err := appareil.Stockage{Dossier: dossier}.Lire()
	if err != nil {
		return 0, errors.New("pas inscrit")
	}
	var liste []protocole.Appareil
	if err := appel(dossier, "GET", protocole.CheminAppareils, nil, &liste); err != nil {
		return 0, err
	}
	expire := time.Now().Add(dureeCertificat).Truncate(time.Second)
	var sortie []protocole.Appareil
	for _, v := range vues {
		i := slices.IndexFunc(liste, func(a protocole.Appareil) bool { return a.ClePublique == v.Cle })
		if i < 0 {
			return 0, fmt.Errorf("%s n'est plus sur le serveur", client.Propre(v.Nom))
		}
		a := liste[i]
		if a.Adresse != v.Adresse || client.Propre(a.Proprietaire) != v.Proprietaire ||
			client.Propre(a.Etiquette) != v.Etiquette || client.Propre(a.Groupe) != v.Groupe ||
			a.Proprietaire != client.Propre(a.Proprietaire) || a.Etiquette != client.Propre(a.Etiquette) || a.Groupe != client.Propre(a.Groupe) {
			return 0, fmt.Errorf("la fiche de %s a changé sur le serveur depuis l'affichage : rien n'est signé", client.Propre(v.Nom))
		}
		k, err1 := b64.Cle32(a.ClePublique)
		adresse, err2 := netip.ParseAddr(a.Adresse)
		if err1 != nil || err2 != nil || !adresse.Is4() {
			return 0, fmt.Errorf("fiche illisible : %s", client.Propre(v.Nom))
		}
		if err := adresseSignable(adresse, a.ClePublique, e.Retenu.Reseau, liste); err != nil {
			return 0, fmt.Errorf("%s : %w", client.Propre(v.Nom), err)
		}
		c := verrou.Certificat{Cle: k, Adresse: adresse, Etiquette: a.Etiquette, Proprietaire: a.Proprietaire,
			Groupe: a.Groupe, Expire: expire}
		sig, err := c.Signer(prive)
		if err != nil {
			return 0, fmt.Errorf("%s : %w", client.Propre(v.Nom), err)
		}
		a.Signature, a.SignatureExpire = base64.StdEncoding.EncodeToString(sig), expire
		sortie = append(sortie, a)
	}
	if err := appel(dossier, "POST", protocole.CheminSignatures, sortie, nil); err != nil {
		return 0, err
	}
	return len(sortie), nil
}

// adresseSignable : une adresse du réseau retenu, qui n'est ni celle du
// serveur (la première) ni celle d'un autre appareil déjà signé.
func adresseSignable(a netip.Addr, cle string, reseau netip.Prefix, liste []protocole.Appareil) error {
	if !reseau.IsValid() || !reseau.Contains(a) || a == reseau.Masked().Addr() || a == reseau.Masked().Addr().Next() {
		return fmt.Errorf("adresse %s hors du réseau", a)
	}
	for _, b := range liste {
		if b.ClePublique != cle && b.Signature != "" && b.Adresse == a.String() {
			return fmt.Errorf("adresse %s déjà signée pour %s", a, client.Propre(b.Nom))
		}
	}
	return nil
}

// Retirer : un admin retire un appareil (une demande refusée).
func Retirer(dossier, cle string) error {
	return appel(dossier, "POST", protocole.CheminRetrait, protocole.DemandeRetrait{ClePublique: cle}, nil)
}

// Reseau : l'état du réseau lu directement sur l'API, sans tunnel (Vue en
// JSON). L'appli s'en sert quand le tunnel est coupé : on sait alors qui
// est admin, quand expire son certificat, et qui est en ligne. Les pairs
// sont vérifiés comme par le tunnel (client.Construire).
func Reseau(dossier string) (string, error) {
	s := appareil.Stockage{Dossier: dossier}
	e, err := s.Lire()
	if err != nil {
		return "", errors.New("pas inscrit")
	}
	a, err := api(e.Serveur, e.Autorite)
	if err != nil {
		return "", err
	}
	r, err := appareil.LireReseau(a, e)
	if err != nil {
		return "", err
	}
	ret := e.Retenu
	_, ecartes, sortant, err := client.ConstruireAvecSortant(r, &ret, netip.AddrPort{}, time.Now())
	if err != nil {
		return "", err
	}
	return vueJSON(appareil.Vue{Reseau: r, Ecartes: ecartes, Sortant: sortant}, false, "", time.Now()), nil
}

// Inviter : un lien d'invitation pour un membre de l'équipe, comme
// « sas.sh invitation » : https://vpn.…/rejoindre#serveur=…&cle=…&verrou=…
// (et l'autorité du labo, s'il y en a une). La clé vaut minutes minutes,
// une seule fois.
func Inviter(dossier, utilisateur string, minutes int) (string, error) {
	e, err := appareil.Stockage{Dossier: dossier}.Lire()
	if err != nil {
		return "", errors.New("pas inscrit")
	}
	var r protocole.ReponseInvitation
	if err := appel(dossier, "POST", protocole.CheminInvitation, protocole.DemandeInvitation{Utilisateur: utilisateur, Minutes: minutes}, &r); err != nil {
		return "", err
	}
	return lienRejoindre(e, r.Cle)
}

// EnCours : une invitation pas encore utilisée, pour l'écran de l'admin.
// Pour : l'adresse invitée, ou le nom d'une machine (Machine). Cree et
// Expire en secondes Unix ; Cree vaut 0 quand le serveur ne le sait pas.
type EnCours struct {
	ID       string `json:"id"`
	Pour     string `json:"pour"`
	Machine  bool   `json:"machine"`
	Createur string `json:"createur"`
	Cree     int64  `json:"cree"`
	Expire   int64  `json:"expire"`
}

// idInvitation : 16 chiffres hexadécimaux, ce que rend le serveur.
var idInvitation = regexp.MustCompile(`^[0-9a-f]{16}$`)

// Invitations : les invitations en cours, en JSON ([]EnCours). Le serveur
// n'en donne jamais la clé. Réservé aux admins.
func Invitations(dossier string) (string, error) {
	var liste []protocole.Invitation
	if err := appel(dossier, "GET", protocole.CheminInvitations, nil, &liste); err != nil {
		return "", err
	}
	r := []EnCours{}
	for _, i := range liste {
		if !idInvitation.MatchString(i.ID) {
			continue
		}
		c := EnCours{ID: i.ID, Pour: client.Propre(i.Utilisateur), Createur: client.Propre(i.Createur), Expire: i.Expire.Unix()}
		if i.Utilisateur == "" {
			c.Pour, c.Machine = client.Propre(cmp.Or(i.Nom, i.Etiquette)), true
		}
		if !i.Cree.IsZero() {
			c.Cree = i.Cree.Unix()
		}
		r = append(r, c)
	}
	b, _ := json.Marshal(r)
	return string(b), nil
}

// AnnulerInvitation : l'invitation id ne permet plus de s'inscrire.
func AnnulerInvitation(dossier, id string) error {
	return appel(dossier, "POST", protocole.CheminAnnulation, protocole.DemandeAnnulation{ID: id}, nil)
}

// LienReseau : le lien permanent du réseau, sans clé d'inscription. Il
// donne l'adresse du serveur et le verrou à retenir ; la personne se
// connecte ensuite avec son compte Google de l'équipe. Il ne fait entrer
// personne à lui seul : il n'a donc pas à rester secret.
func LienReseau(dossier string) (string, error) {
	e, err := appareil.Stockage{Dossier: dossier}.Lire()
	if err != nil {
		return "", errors.New("pas inscrit")
	}
	return lienRejoindre(e, "")
}

// lienRejoindre : https://<hôte du serveur>/rejoindre#serveur=…&verrou=…,
// avec ou sans clé d'inscription. Un lien https, pour que WhatsApp, Gmail
// ou les SMS le rendent cliquable, et qu'Android l'ouvre dans l'appli (App
// Link vérifié par /.well-known/assetlinks.json). Tout est dans le
// fragment : un navigateur ne l'envoie jamais au serveur, qui ne peut donc
// ni journaliser la clé ni la voir passer. L'hôte est celui du serveur
// annoncé : l'appli refuse un lien dont l'hôte diffère.
func lienRejoindre(e appareil.Etat, cle string) (string, error) {
	s, err := url.Parse(e.Serveur)
	if err != nil || s.Scheme != "https" || s.Host == "" || s.User != nil {
		return "", errors.New("adresse du serveur illisible")
	}
	v := url.Values{"serveur": {e.Serveur}, "verrou": {e.Retenu.Verrou}}
	if cle != "" {
		v.Set("cle", cle)
	}
	if e.Autorite != "" {
		v.Set("autorite", base64.StdEncoding.EncodeToString([]byte(e.Autorite)))
	}
	return "https://" + s.Host + "/rejoindre#" + v.Encode(), nil
}

// Revoquer ajoute les clés (séparées par des virgules) à la liste de
// révocation, la signe avec la clé du verrou et l'envoie au serveur. On
// part de la liste que cet appareil a retenue, et de celle que sert le
// serveur si, et seulement si, elle est signée par le verrou
// (client.AllongerRevocations). Rend la nouvelle version.
func Revoquer(dossier, graine, cles string) (int, error) {
	prive, err := lireVerrou(dossier, graine)
	if err != nil {
		return 0, err
	}
	e, err := appareil.Stockage{Dossier: dossier}.Lire()
	if err != nil {
		return 0, errors.New("pas inscrit")
	}
	a, err := api(e.Serveur, e.Autorite)
	if err != nil {
		return 0, err
	}
	r, err := appareil.LireReseau(a, e)
	if err != nil {
		return 0, err
	}
	l, _, err := client.AllongerRevocations(prive, e.Retenu.VersionRevocations, e.Retenu.Revoquees, r.Revocations, strings.Split(strings.ReplaceAll(cles, " ", ""), ","))
	if err != nil {
		return 0, err
	}
	if err := appel(dossier, "POST", protocole.CheminRevocations, l, nil); err != nil {
		return 0, err
	}
	return int(min(l.Version, 1<<31)), nil
}
