// Package appareil : la vie d'un appareil CyberSAS, commune au client
// Linux (cmd/sas) et à l'appli Android (pont/) :
//   - son état sur disque (clé privée, inscription, ce qu'il a retenu) ;
//   - l'inscription, qui garde la même clé d'une fois sur l'autre ;
//   - la boucle qui tient le tunnel ouvert et suit l'état du réseau.
//
// Seules changent l'interface réseau (/dev/net/tun sous Linux, le
// descripteur de VpnService sous Android) et la façon de la configurer.
package appareil

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Cybertrist/CyberSAS/internal/b64"
	"github.com/Cybertrist/CyberSAS/internal/client"
	"github.com/Cybertrist/CyberSAS/internal/noise"
	"github.com/Cybertrist/CyberSAS/internal/protocole"
	"github.com/Cybertrist/CyberSAS/internal/tunnel"
)

// Etat : tout ce que l'appareil garde entre deux lancements.
type Etat struct {
	Serveur     string                     `json:"serveur"`
	ClePrivee   string                     `json:"cle_privee"`
	Inscription protocole.ReponseConnexion `json:"inscription"`
	// Retenu : ce que l'appareil a appris et ne laisse plus changer (clé du
	// serveur, verrou, versions signées déjà vues, clés révoquées).
	Retenu client.Retenu `json:"retenu"`
	// Autorite : l'autorité qui a signé le certificat HTTPS du serveur, en
	// PEM, quand ce n'est pas une autorité publique (le labo). Gardée ici,
	// avec le reste, pour survivre à une coupure au pire moment.
	Autorite string `json:"autorite,omitempty"`
}

// Stockage : le dossier où vit l'état, lisible par l'appareil seul
// (/var/lib/sas sous Linux, le dossier privé de l'appli sous Android).
type Stockage struct{ Dossier string }

func (s Stockage) Fichier() string { return filepath.Join(s.Dossier, "etat.json") }

func (s Stockage) Lire() (Etat, error) {
	var e Etat
	b, err := os.ReadFile(s.Fichier())
	if err != nil {
		return e, err
	}
	return e, json.Unmarshal(b, &e)
}

// Ecrire : fichier temporaire, synchronisé sur disque, puis renommé. Une
// coupure de courant ne laisse jamais un état à moitié écrit, ce qui ferait
// perdre la clé privée.
func (s Stockage) Ecrire(e Etat) error {
	if err := os.MkdirAll(s.Dossier, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(s.Dossier, 0o700); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(e, "", "  ")
	tmp := s.Fichier() + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.Fichier()); err != nil {
		return err
	}
	// Synchroniser le dossier fixe le renommage sur disque. Le fichier est
	// déjà complet : un échec ici ne peut que ramener l'ancien état entier.
	if d, err := os.Open(s.Dossier); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}

// Effacer : la clé privée est dans ce fichier. S'il n'a pas pu partir,
// l'appelant doit le dire.
func (s Stockage) Effacer() error {
	if err := os.Remove(s.Fichier()); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// Options d'une inscription.
type Options struct {
	Nom, Systeme string
	// VerrouAttendu : la clé publique du verrou donnée par l'admin. Vide,
	// c'est celle déjà retenue qui compte, s'il y en a une.
	VerrouAttendu string
	// Oublier : accepter un autre serveur ou un autre verrou que ceux déjà
	// retenus.
	Oublier bool
	// Autorite : voir Etat.Autorite.
	Autorite string
}

// Rejoindre inscrit l'appareil. On garde la clé déjà générée : se
// réinscrire ne change pas d'adresse.
func Rejoindre(s Stockage, api *client.API, serveur string, j client.Justificatif, o Options) (Etat, error) {
	ancien, dejaInscrit := s.Lire()
	var brut []byte
	if k, err := b64.Decoder(ancien.ClePrivee); err == nil && len(k) == 32 {
		brut = k
	} else {
		k, err := noise.GenererCle()
		if err != nil {
			return Etat{}, err
		}
		brut = k.Bytes()
	}
	prive, _ := noise.ClePrivee(brut)
	verrouAttendu := o.VerrouAttendu
	if verrouAttendu == "" && dejaInscrit == nil {
		verrouAttendu = ancien.Retenu.Verrou
	}
	r, err := api.Inscrire(prive, o.Nom, o.Systeme, j, verrouAttendu)
	if err != nil {
		return Etat{}, fmt.Errorf("inscription refusée : %w", err)
	}
	// Ce qu'on a retenu ne change pas sans qu'on le demande : un serveur
	// piraté qui ferait se réinscrire l'appareil ne ferait pas disparaître le
	// verrou pour autant.
	if dejaInscrit == nil && !o.Oublier {
		if ancien.Retenu.CleServeur != "" && ancien.Retenu.CleServeur != r.Serveur.ClePublique {
			return Etat{}, fmt.Errorf("le serveur a changé de clé (%s, avant %s) : refus",
				client.EmpreinteCle(r.Serveur.ClePublique), client.EmpreinteCle(ancien.Retenu.CleServeur))
		}
		if ancien.Retenu.Verrou != r.Verrou {
			return Etat{}, fmt.Errorf("le verrou a changé (%s, avant %s) : refus",
				client.EmpreinteVerrou(r.Verrou), client.EmpreinteVerrou(ancien.Retenu.Verrou))
		}
	}
	reseau, err1 := netip.ParsePrefix(r.Reseau)
	moi, err2 := netip.ParseAddr(r.Appareil.Adresse)
	if err1 != nil || err2 != nil || !moi.Is4() || !reseau.Contains(moi) {
		return Etat{}, fmt.Errorf("réponse du serveur incohérente : adresse %q dans %q", r.Appareil.Adresse, r.Reseau)
	}
	if err := ReseauAcceptable(reseau, r.Domaine); err != nil {
		return Etat{}, err
	}
	ret := client.Retenu{CleServeur: r.Serveur.ClePublique, Verrou: r.Verrou, Moi: moi, Reseau: reseau,
		MaCle: base64.StdEncoding.EncodeToString(prive.PublicKey().Bytes())}
	if dejaInscrit == nil && ancien.Retenu.Verrou == r.Verrou {
		ret.VersionPolitique, ret.VersionRevocations, ret.Revoquees =
			ancien.Retenu.VersionPolitique, ancien.Retenu.VersionRevocations, ancien.Retenu.Revoquees
	}
	e := Etat{Serveur: serveur, ClePrivee: base64.StdEncoding.EncodeToString(brut), Inscription: r, Retenu: ret, Autorite: o.Autorite}
	if err := s.Ecrire(e); err != nil {
		return Etat{}, err
	}
	return e, nil
}

// LireReseau : l'état du réseau, tel que le serveur l'annonce. Il n'est
// pas encore vérifié : c'est le travail de client.Construire.
func LireReseau(api *client.API, e Etat) (protocole.EtatReseau, error) {
	var r protocole.EtatReseau
	err := api.Appel("GET", protocole.CheminReseau, e.Inscription.Jeton, nil, &r)
	return r, err
}

// Interface : l'interface réseau du tunnel, et la façon de lui donner son
// adresse. Sous Android, VpnService la configure lui-même : Configurer ne
// fait rien.
type Interface interface {
	tunnel.Tun
	Configurer(adresse netip.Prefix, mtu int) error
}

// Vue : ce que la boucle sait du réseau après chaque synchronisation,
// pour l'afficher.
type Vue struct {
	Reseau  protocole.EtatReseau
	Ecartes []client.Ecarte
	Moteur  *tunnel.Moteur
	Erreur  error
}

// Tenue : de quoi faire tourner la boucle.
type Tenue struct {
	Stockage  Stockage
	Interface Interface
	Conn      *net.UDPConn
	Journal   *slog.Logger
	// API : le client HTTPS pour l'adresse d'un serveur.
	API func(serveur string) (*client.API, error)
	// SurVue, s'il est donné, reçoit l'état du réseau après chaque
	// synchronisation, réussie ou non.
	SurVue func(Vue)
	// Periode : l'intervalle entre deux demandes au serveur. Zéro vaut dix
	// secondes.
	Periode time.Duration
}

// Tenir ouvre le tunnel dès qu'il y a une inscription, et le tient jusqu'à
// l'annulation du contexte. Toutes les deux secondes elle relit l'état
// local ; à chaque période, elle redemande au serveur l'état du réseau.
func Tenir(ctx context.Context, t Tenue) error {
	ctx, arret := context.WithCancel(ctx)
	defer arret()
	periode := t.Periode
	if periode == 0 {
		periode = 10 * time.Second
	}
	var (
		moteur      *tunnel.Moteur
		dernier     []byte
		cle         []byte
		courant     Etat
		demande     time.Time
		derniersRef string
		finMoteur   = make(chan error, 1)
	)
	tic := time.NewTicker(2 * time.Second)
	defer tic.Stop()
	for {
		b, err := os.ReadFile(t.Stockage.Fichier())
		switch {
		case err == nil && !bytes.Equal(b, dernier):
			var e Etat
			if err := json.Unmarshal(b, &e); err != nil {
				t.Journal.Error("état illisible", "erreur", err)
				break
			}
			brut, err1 := b64.Decoder(e.ClePrivee)
			k, err2 := noise.ClePrivee(brut)
			if err1 != nil || err2 != nil {
				t.Journal.Error("clé privée illisible")
				break
			}
			switch {
			case moteur == nil:
				moteur = tunnel.Nouveau(tunnel.Config{Prive: k, Tun: t.Interface, Conn: t.Conn, Journal: t.Journal, Adresse: e.Retenu.Moi})
				m := moteur
				go func() { finMoteur <- m.Lancer(ctx) }()
			case !bytes.Equal(brut, cle):
				// Nouvelle inscription, nouvelle clé : les sessions de
				// l'ancienne ne valent plus rien.
				moteur.DefinirCle(k)
			}
			moteur.DefinirAdresse(e.Retenu.Moi)
			cle, dernier, courant, demande = brut, b, e, time.Time{}
			if err := t.Interface.Configurer(netip.PrefixFrom(e.Retenu.Moi, e.Retenu.Reseau.Bits()), tunnel.MTU); err != nil {
				t.Journal.Error("interface", "erreur", err)
			}
		case errors.Is(err, os.ErrNotExist) && moteur != nil && dernier != nil:
			moteur.DefinirPairs(nil)
			dernier = nil
		}

		if dernier != nil && time.Since(demande) >= periode {
			demande = time.Now()
			vue := Vue{Moteur: moteur}
			var refus string
			refus, vue.Reseau, vue.Ecartes, vue.Erreur = t.appliquer(moteur, &courant)
			switch {
			case errors.Is(vue.Erreur, client.ErrDesinscrit):
				t.Journal.Warn("le serveur ne connaît plus cet appareil : tunnel coupé")
				moteur.DefinirPairs(nil)
			case vue.Erreur != nil:
				t.Journal.Error("état du réseau", "erreur", vue.Erreur)
			case refus != derniersRef:
				if refus != "" {
					t.Journal.Warn("refusé par ce client", "detail", refus)
				}
				derniersRef = refus
			}
			if t.SurVue != nil {
				t.SurVue(vue)
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case err := <-finMoteur:
			if err != nil && ctx.Err() == nil {
				return fmt.Errorf("tunnel arrêté : %w", err)
			}
			return nil
		case <-tic.C:
		}
	}
}

// appliquer : l'état du réseau, vérifié, devient la liste des pairs. Ce
// que l'appareil apprend en chemin (versions signées, révocations) est
// écrit sur disque : il ne l'oublie pas en redémarrant.
func (t Tenue) appliquer(m *tunnel.Moteur, e *Etat) (string, protocole.EtatReseau, []client.Ecarte, error) {
	api, err := t.API(e.Serveur)
	if err != nil {
		return "", protocole.EtatReseau{}, nil, err
	}
	r, err := LireReseau(api, *e)
	if err != nil {
		return "", r, nil, err
	}
	point, err := net.ResolveUDPAddr("udp", e.Inscription.Serveur.Point)
	if err != nil {
		return "", r, nil, fmt.Errorf("point %s : %w", e.Inscription.Serveur.Point, err)
	}
	ret := e.Retenu
	pairs, ecartes, err := client.Construire(r, &ret, point.AddrPort(), time.Now())
	if err != nil {
		return "", r, nil, err
	}
	if ret.VersionPolitique != e.Retenu.VersionPolitique || ret.VersionRevocations != e.Retenu.VersionRevocations {
		nouveau := *e
		nouveau.Retenu = ret
		if err := t.Stockage.Ecrire(nouveau); err != nil {
			return "", r, ecartes, err
		}
		*e = nouveau
	}
	m.DefinirPairs(pairs)
	var refus []string
	for _, x := range ecartes {
		refus = append(refus, fmt.Sprintf("%s (%s) : %s", x.Nom, x.Adresse, x.Raison))
	}
	return strings.Join(refus, " ; "), r, ecartes, nil
}

// ReseauAcceptable : le serveur choisit le réseau que le téléphone route
// dans le tunnel et le domaine qu'il y cherche. Un lien d'invitation forgé
// mènerait à un serveur qui répondrait 0.0.0.0/0 : tout l'Internet du
// téléphone partirait chez lui. On n'accepte donc qu'une plage privée
// d'au plus 65 536 adresses, et un domaine sous .internal.
func ReseauAcceptable(reseau netip.Prefix, domaine string) error {
	prive := false
	for _, p := range []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "100.64.0.0/10"} {
		plage := netip.MustParsePrefix(p)
		if plage.Bits() <= reseau.Bits() && plage.Contains(reseau.Addr()) {
			prive = true
		}
	}
	if !reseau.Addr().Is4() || !prive || reseau.Bits() < 16 {
		return fmt.Errorf("réseau %s refusé : il faut une plage privée, /16 au plus large", reseau)
	}
	if domaine != "" && domaine != "internal" && !strings.HasSuffix(domaine, ".internal") {
		return fmt.Errorf("domaine %q refusé : il doit finir par .internal", domaine)
	}
	return nil
}
