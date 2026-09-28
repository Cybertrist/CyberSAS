// Package pont : le moteur CyberSAS vu depuis l'appli Android, compilé par
// gomobile (gomobile bind ./pont). Des types simples seulement : chaînes,
// entiers, erreurs, et une interface que Kotlin implémente.
//
// Tout le travail est fait par internal/appareil, le même code que le
// client Linux : l'appli ne fait que lui passer le descripteur de
// l'interface que VpnService a créée, et lire l'état du réseau en JSON.
package pont

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"os"
	"runtime"
	"sync"
	"time"

	"golang.org/x/sys/unix"

	"github.com/Cybertrist/CyberSAS/internal/appareil"
	"github.com/Cybertrist/CyberSAS/internal/b64"
	"github.com/Cybertrist/CyberSAS/internal/client"
	"github.com/Cybertrist/CyberSAS/internal/protocole"
	"github.com/Cybertrist/CyberSAS/internal/tunnel"
)

// Protecteur : VpnService.protect. La prise UDP du tunnel doit sortir par
// le vrai réseau, jamais par le tunnel lui-même.
type Protecteur interface {
	Proteger(fd int) bool
}

// api : l'autorité du labo, en PEM, s'ajoute aux racines du système. Vide
// avec un vrai certificat (Let's Encrypt).
func api(serveur, autorite string) (*client.API, error) {
	return client.NouvelleAPI(serveur, []byte(autorite))
}

// Infos : ce que l'appli affiche d'une inscription.
type Infos struct {
	Nom          string `json:"nom"`
	Libelle      string `json:"libelle"`
	Adresse      string `json:"adresse"`
	Reseau       string `json:"reseau"`
	Serveur      string `json:"serveur"`
	Domaine      string `json:"domaine"`
	Proprietaire string `json:"proprietaire"`
	Groupe       string `json:"groupe"`
	// Empreinte : celle de la clé de cet appareil, que l'admin compare
	// avant de signer.
	Empreinte string `json:"empreinte"`
	Verrou    string `json:"verrou"`
}

func infos(e appareil.Etat) string {
	a := e.Inscription.Appareil
	b, _ := json.Marshal(Infos{Nom: client.Propre(a.Nom), Libelle: client.Propre(a.Libelle), Adresse: e.Retenu.Moi.String(), Reseau: e.Retenu.Reseau.String(),
		Serveur: e.Serveur, Domaine: e.Inscription.Domaine, Proprietaire: client.Propre(a.Proprietaire), Groupe: client.Propre(a.Groupe),
		Empreinte: client.EmpreinteCle(e.Retenu.MaCle), Verrou: client.EmpreinteVerrou(e.Retenu.Verrou)})
	return string(b)
}

// Rejoindre inscrit l'appareil, avec une clé d'inscription ou un jeton
// Google. verrou : la clé publique du verrou donnée par l'admin. Rend les
// Infos en JSON.
func Rejoindre(dossier, serveur, verrou, cleInscription, jetonGoogle, nom, autorite string) (string, error) {
	a, err := api(serveur, autorite)
	if err != nil {
		return "", err
	}
	e, err := appareil.Rejoindre(appareil.Stockage{Dossier: dossier}, a, serveur,
		client.Justificatif{CleInscription: cleInscription, JetonGoogle: jetonGoogle},
		appareil.Options{Nom: nom, Systeme: runtime.GOOS, VerrouAttendu: verrou, Autorite: autorite})
	if err != nil {
		return "", err
	}
	return infos(e), nil
}

// Inscription : les Infos de l'inscription en cours, ou "" s'il n'y en a
// pas.
func Inscription(dossier string) string {
	e, err := appareil.Stockage{Dossier: dossier}.Lire()
	if err != nil {
		return ""
	}
	return infos(e)
}

// Quitter : se désinscrit auprès du serveur, puis oublie tout. L'état
// local est effacé même si le serveur ne répond pas.
func Quitter(dossier string) error {
	// Attendre vraiment la fin du moteur : une synchronisation en cours
	// réécrirait sinon etat.json, clé privée comprise, juste après qu'on
	// l'a effacé. Une requête à l'API dure 20 secondes au plus.
	arreter(25 * time.Second)
	s := appareil.Stockage{Dossier: dossier}
	e, err := s.Lire()
	if err != nil {
		// Illisible n'est pas absent : la clé est peut-être encore dedans.
		return s.Effacer()
	}
	if a, err := api(e.Serveur, e.Autorite); err == nil {
		_ = a.Appel("POST", protocole.CheminDeconnexion, e.Inscription.Jeton, nil, nil)
	}
	return s.Effacer()
}

// --- le tunnel ---------------------------------------------------------------

var (
	mu       sync.Mutex
	arret    context.CancelFunc
	fini     chan struct{}
	tour     int // le numéro du dernier Demarrer
	derniere appareil.Vue
	enMarche bool
	erreur   string
)

// iface : le descripteur que VpnService a créé. Il est déjà configuré
// (adresse, route, MTU) : Configurer ne fait rien.
type iface struct{ *os.File }

func (iface) Configurer(netip.Prefix, int) error { return nil }

// Demarrer : tient le tunnel sur l'interface fd, jusqu'à Arreter. Rend la
// main tout de suite. Le descripteur appartient désormais au moteur.
func Demarrer(fd int, dossier string, p Protecteur) error {
	Arreter()
	// Non bloquant : Go passe par son ordonnanceur réseau, et fermer le
	// fichier débloque une lecture en cours.
	if err := unix.SetNonblock(fd, true); err != nil {
		unix.Close(fd)
		return err
	}
	f := os.NewFile(uintptr(fd), "tun")
	conn, err := net.ListenUDP("udp", &net.UDPAddr{})
	if err != nil {
		f.Close()
		return err
	}
	if p != nil {
		protege := false
		if brut, err := conn.SyscallConn(); err == nil {
			_ = brut.Control(func(d uintptr) { protege = p.Proteger(int(d)) })
		}
		if !protege {
			conn.Close()
			f.Close()
			return errors.New("la prise du tunnel n'a pas pu être protégée")
		}
	}
	ctx, annuler := context.WithCancel(context.Background())
	termine := make(chan struct{})
	mu.Lock()
	tour++
	montour := tour
	arret, fini, enMarche, erreur, derniere = annuler, termine, true, "", appareil.Vue{}
	mu.Unlock()

	journal := slog.New(slog.NewTextHandler(os.Stderr, nil))
	go func() {
		defer close(termine)
		var err error
		// Sous gomobile, une panique tue tout le processus, interface
		// comprise : on la change en erreur affichée.
		defer func() {
			if p := recover(); p != nil {
				conn.Close()
				f.Close()
				mu.Lock()
				if tour == montour {
					enMarche, erreur = false, fmt.Sprint("moteur arrêté : ", p)
				}
				mu.Unlock()
			}
		}()
		err = appareil.Tenir(ctx, appareil.Tenue{
			Stockage: appareil.Stockage{Dossier: dossier}, Interface: iface{f}, Conn: conn, Journal: journal,
			// L'autorité est relue à chaque appel : elle suit une réinscription.
			API: func(s string) (*client.API, error) {
				e, _ := appareil.Stockage{Dossier: dossier}.Lire()
				return api(s, e.Autorite)
			},
			SurVue:  func(v appareil.Vue) { mu.Lock(); derniere = v; mu.Unlock() },
			Periode: 5 * time.Second,
		})
		conn.Close()
		f.Close()
		mu.Lock()
		// Un Demarrer plus récent a pu prendre la suite : son état n'est
		// pas le nôtre.
		if tour == montour {
			enMarche = false
			if err != nil {
				erreur = err.Error()
			}
		}
		mu.Unlock()
	}()
	return nil
}

// Arreter coupe le tunnel et attend que le moteur ait rendu l'interface.
func Arreter() { arreter(3 * time.Second) }

func arreter(attente time.Duration) {
	mu.Lock()
	a, f := arret, fini
	arret, fini = nil, nil
	mu.Unlock()
	if a != nil {
		a()
		select {
		case <-f:
		case <-time.After(attente):
		}
	}
}

// --- l'état, pour l'appli ------------------------------------------------------

// Pair : un appareil du réseau, tel que l'appli l'affiche.
type Pair struct {
	Nom          string    `json:"nom"`
	Libelle      string    `json:"libelle,omitempty"`
	Adresse      string    `json:"adresse"`
	Proprietaire string    `json:"proprietaire"`
	Etiquette    string    `json:"etiquette"`
	Groupe       string    `json:"groupe"`
	Systeme      string    `json:"systeme"`
	EnLigne      bool      `json:"en_ligne"`
	Moi          bool      `json:"moi"`
	Serveur      bool      `json:"serveur"`
	Empreinte    string    `json:"empreinte"`
	Cle          string    `json:"cle"`
	Expire       time.Time `json:"expire,omitzero"`
	// Signe : son certificat est valide pour ce client. Sinon, Raison dit
	// pourquoi il est écarté.
	Signe  bool   `json:"signe"`
	Raison string `json:"raison,omitempty"`
	// Session : une poignée de main a réussi récemment avec lui.
	Session bool `json:"session"`
	// Ports : ce que ce téléphone a le droit d'ouvrir chez lui (« tcp:80 »,
	// « * »), d'après la politique signée. Sans objet si Vue.PortsConnus
	// est faux.
	Ports []string `json:"ports,omitempty"`
}

// Vue : l'état complet, en JSON.
type Vue struct {
	EnMarche bool     `json:"en_marche"`
	Connecte bool     `json:"connecte"`
	Erreur   string   `json:"erreur,omitempty"`
	Pairs    []Pair   `json:"pairs"`
	Notes    []string `json:"notes,omitempty"`
	// PortsConnus : la politique signée a été vérifiée et cet appareil est
	// signé ; les Ports des pairs disent alors vraiment ce qui est ouvert.
	PortsConnus bool `json:"ports_connus"`
}

// Etat : l'état du tunnel et du réseau, en JSON (Vue). L'appli le lit
// toutes les secondes ou deux pendant qu'elle est ouverte.
func Etat() string {
	mu.Lock()
	v, marche, err := derniere, enMarche, erreur
	mu.Unlock()
	return vueJSON(v, marche, err, time.Now())
}

// recente : une session sert tant que ses clés ont moins de trois minutes
// (le moteur les renouvelle toutes les deux).
const recente = 3 * time.Minute

func vueJSON(v appareil.Vue, marche bool, errMarche string, maintenant time.Time) string {
	out := Vue{EnMarche: marche, Erreur: errMarche, Pairs: []Pair{}, PortsConnus: v.Sortant != nil}
	ports := func(adresse string) []string {
		a, err := netip.ParseAddr(adresse)
		if err != nil {
			return nil
		}
		var s []string
		for _, p := range v.Sortant[a] {
			s = append(s, p.String())
		}
		return s
	}
	if v.Erreur != nil {
		out.Erreur = client.Propre(v.Erreur.Error())
	}
	sessions := map[[32]byte]bool{}
	if v.Moteur != nil {
		for _, p := range v.Moteur.Etat() {
			sessions[p.Publique] = !p.DernierePoignee.IsZero() && maintenant.Sub(p.DernierePoignee) < recente
		}
	}
	r := v.Reseau
	if r.Serveur.ClePublique != "" {
		pub, _ := cle32(r.Serveur.ClePublique)
		out.Connecte = sessions[pub]
		out.Pairs = append(out.Pairs, Pair{Nom: "serveur", Adresse: r.Serveur.Adresse, Etiquette: "serveur", Serveur: true,
			EnLigne: out.Connecte, Signe: true, Session: out.Connecte, Empreinte: client.EmpreinteCle(r.Serveur.ClePublique),
			Ports: ports(r.Serveur.Adresse)})
	}
	refus := map[string]string{}
	for _, x := range v.Ecartes {
		if x.Adresse != "" {
			refus[x.Adresse] = x.Raison
		} else {
			out.Notes = append(out.Notes, x.Nom+" : "+x.Raison)
		}
	}
	vers := func(a protocole.Appareil, moi bool) Pair {
		pub, _ := cle32(a.ClePublique)
		raison, ecarte := refus[a.Adresse]
		return Pair{Nom: client.Propre(a.Nom), Libelle: client.Propre(a.Libelle), Adresse: a.Adresse, Proprietaire: client.Propre(a.Proprietaire),
			Etiquette: client.Propre(a.Etiquette), Groupe: client.Propre(a.Groupe), Systeme: client.Propre(a.Systeme),
			EnLigne: a.EnLigne || moi, Moi: moi, Empreinte: client.EmpreinteCle(a.ClePublique), Cle: a.ClePublique, Expire: a.SignatureExpire,
			Signe: !ecarte, Raison: raison, Session: sessions[pub], Ports: ports(a.Adresse)}
	}
	if r.Moi.Adresse != "" {
		out.Pairs = append(out.Pairs, vers(r.Moi, true))
	}
	for _, a := range r.Pairs {
		out.Pairs = append(out.Pairs, vers(a, false))
	}
	b, _ := json.Marshal(out)
	return string(b)
}

func cle32(t string) ([32]byte, error) { return b64.Cle32(t) }

var _ tunnel.Tun = iface{}
