package serveur

import (
	"bytes"
	"crypto/ecdh"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Cybertrist/CyberSAS/internal/base"
	"github.com/Cybertrist/CyberSAS/internal/dns"
	"github.com/Cybertrist/CyberSAS/internal/noise"
	"github.com/Cybertrist/CyberSAS/internal/protocole"
	"github.com/Cybertrist/CyberSAS/internal/tunnel"
	mdns "github.com/miekg/dns"
)

// Ces tests font tourner l'API pour de vrai, sans réseau ni noyau : une
// base SQLite dans un dossier temporaire, un moteur qui ne tourne pas, et
// un pare-feu remplacé par une fonction qui ne fait rien. Ceux nommés
// TestAudit reproduisent un constat de l'audit (docs/audit.md).

type tunMuet struct{}

func (tunMuet) Read([]byte) (int, error)    { select {} }
func (tunMuet) Write(b []byte) (int, error) { return len(b), nil }
func (tunMuet) Close() error                { return nil }

type banc struct {
	t       *testing.T
	srv     *Serveur
	http    *httptest.Server
	base    *base.Base
	prive   *ecdh.PrivateKey
	dossier string
	dns     *dns.Serveur
}

func nouveauBanc(t *testing.T) *banc {
	appliquerPareFeu = func(string) error { return nil }
	dossier := t.TempDir()
	ecrire := func(nom, contenu string) string {
		chemin := filepath.Join(dossier, nom)
		os.WriteFile(chemin, []byte(contenu), 0o600)
		return chemin
	}
	cfg := Config{
		Point: "vpn.essai:51820", Reseau: netip.MustParsePrefix("10.77.0.0/24"), Serveur: netip.MustParseAddr("10.77.0.1"),
		Interface: "sas0",
		Equipe:    ecrire("equipe.txt", "alice@x.fr equipe\nbob@x.fr equipe\ncarole@x.fr equipe\n"),
		Politique: ecrire("politique.json", `{"version": 1, "regles": [
			{"de": ["groupe:equipe"], "vers": ["etiquette:maison"], "ports": ["tcp:80"]}
		]}`),
		ClientsGoogle: ecrire("clients", "a-remplir.apps.googleusercontent.com\n"),
		DureeAppareil: time.Hour,
	}
	b, err := base.Ouvrir(filepath.Join(dossier, "sas.db"))
	if err != nil {
		t.Fatal(err)
	}
	prive, _ := noise.GenererCle()
	m := tunnel.Nouveau(tunnel.Config{Prive: prive, Tun: tunMuet{}})
	d := dns.Nouveau("sas.internal", "127.0.0.1:1")
	srv := Nouveau(cfg, b, m, d, prive, nil)
	h := httptest.NewServer(srv.Routes())
	t.Cleanup(h.Close)
	return &banc{t: t, srv: srv, http: h, base: b, prive: prive, dossier: dossier, dns: d}
}

func (b *banc) appel(methode, chemin, jeton string, corps any, reponse any) int {
	var c []byte
	if corps != nil {
		c, _ = json.Marshal(corps)
	}
	req, _ := http.NewRequest(methode, b.http.URL+chemin, bytes.NewReader(c))
	if jeton != "" {
		req.Header.Set("Authorization", "Bearer "+jeton)
	}
	rep, err := http.DefaultClient.Do(req)
	if err != nil {
		b.t.Fatal(err)
	}
	defer rep.Body.Close()
	if reponse != nil {
		json.NewDecoder(rep.Body).Decode(reponse)
	}
	return rep.StatusCode
}

// demande : une inscription correcte pour cette clé, avec sa preuve.
func (b *banc) demande(k *ecdh.PrivateKey, cle, nom string) protocole.DemandeConnexion {
	d := protocole.DemandeConnexion{CleInscription: cle, Nom: nom, Systeme: "linux",
		ClePublique: base64.StdEncoding.EncodeToString(k.PublicKey().Bytes()), Horodatage: time.Now().Unix()}
	if err := d.Prouver(k, b.prive.PublicKey().Bytes()); err != nil {
		b.t.Fatal(err)
	}
	return d
}

var compteur uint64

func (b *banc) cle(etiquette, utilisateur, nom string) string {
	compteur++
	c := "sas-essai-" + base64.RawURLEncoding.EncodeToString(noise.Nonce(compteur))
	if err := b.base.CreerCle(c, etiquette, utilisateur, nom, time.Now().Add(time.Minute)); err != nil {
		b.t.Fatal(err)
	}
	return c
}

func (b *banc) inscrire(etiquette, utilisateur, nom string) protocole.ReponseConnexion {
	b.t.Helper()
	k, _ := noise.GenererCle()
	var r protocole.ReponseConnexion
	if code := b.appel("POST", protocole.CheminConnexion, "", b.demande(k, b.cle(etiquette, utilisateur, nom), nom), &r); code != http.StatusOK {
		b.t.Fatalf("inscription de %s : %d", nom, code)
	}
	return r
}

func TestInscriptionEtPreuve(t *testing.T) {
	b := nouveauBanc(t)
	k, _ := noise.GenererCle()

	// Sans preuve, ou avec la preuve d'une autre clé : refusé, et la clé
	// d'inscription n'est pas consommée.
	c := b.cle("maison", "", "maison")
	d := b.demande(k, c, "maison")
	sansPreuve := d
	sansPreuve.Preuve = ""
	if code := b.appel("POST", protocole.CheminConnexion, "", sansPreuve, nil); code != http.StatusUnauthorized {
		t.Fatalf("inscription sans preuve : %d, 401 attendu", code)
	}
	autre, _ := noise.GenererCle()
	volee := b.demande(autre, c, "maison")
	volee.ClePublique = d.ClePublique // la clé publique d'un autre, sans sa clé privée
	if code := b.appel("POST", protocole.CheminConnexion, "", volee, nil); code != http.StatusUnauthorized {
		t.Fatalf("inscription d'une clé qu'on ne détient pas : %d, 401 attendu", code)
	}

	var r protocole.ReponseConnexion
	if code := b.appel("POST", protocole.CheminConnexion, "", d, &r); code != http.StatusOK {
		t.Fatalf("inscription correcte : %d", code)
	}
	if r.Appareil.Nom != "maison" || r.Serveur.ClePublique != b.srv.publique || r.Jeton == "" {
		t.Fatalf("réponse inattendue : %+v", r)
	}
	// La même demande, rejouée : refusée.
	if code := b.appel("POST", protocole.CheminConnexion, "", d, nil); code != http.StatusUnauthorized {
		t.Fatalf("demande rejouée : %d, 401 attendu", code)
	}
	// La clé d'inscription ne sert qu'une fois.
	if code := b.appel("POST", protocole.CheminConnexion, "", b.demande(k, c, "maison"), nil); code != http.StatusUnauthorized {
		t.Fatalf("clé réutilisée : %d, 401 attendu", code)
	}
}

func TestClePubliqueFaible(t *testing.T) {
	b := nouveauBanc(t)
	k, _ := noise.GenererCle()
	d := b.demande(k, b.cle("maison", "", ""), "x")
	d.ClePublique = base64.StdEncoding.EncodeToString(make([]byte, 32))
	if code := b.appel("POST", protocole.CheminConnexion, "", d, nil); code != http.StatusBadRequest {
		t.Fatalf("clé nulle : %d, 400 attendu", code)
	}
}

func TestPasDeRepriseDUneCleInscrite(t *testing.T) {
	b := nouveauBanc(t)
	k, _ := noise.GenererCle()
	if code := b.appel("POST", protocole.CheminConnexion, "", b.demande(k, b.cle("maison", "", ""), "maison"), nil); code != http.StatusOK {
		t.Fatalf("première inscription : %d", code)
	}
	if code := b.appel("POST", protocole.CheminConnexion, "", b.demande(k, b.cle("", "alice@x.fr", ""), "pirate"), nil); code != http.StatusConflict {
		t.Fatalf("reprise d'une clé inscrite : %d, 409 attendu", code)
	}
}

func TestReseauNeMontreQueLesPairsRelies(t *testing.T) {
	b := nouveauBanc(t)
	maison := b.inscrire("maison", "", "maison")
	alice := b.inscrire("", "alice@x.fr", "portable")
	nas := b.inscrire("nas", "", "nas")

	if code := b.appel("GET", protocole.CheminReseau, "", nil, nil); code != http.StatusUnauthorized {
		t.Fatalf("état du réseau sans jeton : %d, 401 attendu", code)
	}
	var etat protocole.EtatReseau
	b.appel("GET", protocole.CheminReseau, alice.Jeton, nil, &etat)
	if len(etat.Pairs) != 1 || etat.Pairs[0].Nom != "maison" {
		t.Fatalf("alice devrait voir la maison, et seulement elle : %+v", etat.Pairs)
	}
	// Une variable neuve : décoder dans l'ancienne garderait les champs
	// absents de la nouvelle réponse.
	etat = protocole.EtatReseau{}
	b.appel("GET", protocole.CheminReseau, maison.Jeton, nil, &etat)
	if len(etat.Pairs) != 1 || etat.Pairs[0].Nom != "portable-alice" {
		t.Fatalf("la maison devrait voir le portable d'alice : %+v", etat.Pairs)
	}
	if etat.Pairs[0].Proprietaire != "" || etat.Pairs[0].Systeme != "" {
		t.Fatal("une machine ne doit voir ni l'adresse email ni le système des personnes")
	}
	b.appel("GET", protocole.CheminReseau, nas.Jeton, nil, &etat)
	if len(etat.Pairs) != 0 {
		t.Fatalf("le nas ne devrait voir personne : %+v", etat.Pairs)
	}
	if !b.srv.Relie(maison.Appareil.Numero, alice.Appareil.Numero) || b.srv.Relie(nas.Appareil.Numero, alice.Appareil.Numero) {
		t.Fatal("les relations du relais ne suivent pas la politique")
	}
}

func TestSortieDeLEquipe(t *testing.T) {
	b := nouveauBanc(t)
	r := b.inscrire("", "alice@x.fr", "portable")
	os.WriteFile(b.srv.cfg.Equipe, []byte("bob@x.fr equipe\ncarole@x.fr equipe\n"), 0o600)
	if err := b.srv.Synchroniser(); err != nil {
		t.Fatal(err)
	}
	if code := b.appel("GET", protocole.CheminReseau, r.Jeton, nil, nil); code != http.StatusUnauthorized {
		t.Fatalf("appareil d'une personne sortie : %d, 401 attendu", code)
	}
}

// Audit serveur n°2 : une politique cassée ne bloque pas les retraits.
func TestAuditPolitiqueCasseeNeBloquePasLesRetraits(t *testing.T) {
	b := nouveauBanc(t)
	r := b.inscrire("", "alice@x.fr", "portable")
	os.WriteFile(b.srv.cfg.Politique, []byte(`{"regles": [`), 0o600) // faute de frappe
	os.WriteFile(b.srv.cfg.Equipe, []byte("bob@x.fr equipe\ncarole@x.fr equipe\n"), 0o600)
	b.srv.Synchroniser()
	if code := b.appel("GET", protocole.CheminReseau, r.Jeton, nil, nil); code != http.StatusUnauthorized {
		t.Fatalf("alice sortie garde son accès pendant que la politique est cassée : %d", code)
	}
	if n := len(b.srv.moteur.Etat()); n != 0 {
		t.Fatalf("le moteur garde %d pair(s) d'une personne sortie", n)
	}
}

// Audit serveur n°4 : une équipe illisible un instant n'efface rien.
func TestAuditEquipeIllisibleNEfacePas(t *testing.T) {
	b := nouveauBanc(t)
	r := b.inscrire("", "alice@x.fr", "portable")
	b.srv.Synchroniser()
	contenu, _ := os.ReadFile(b.srv.cfg.Equipe)
	os.Remove(b.srv.cfg.Equipe)
	b.srv.Synchroniser()
	os.WriteFile(b.srv.cfg.Equipe, contenu, 0o600)
	b.srv.Synchroniser()
	if code := b.appel("GET", protocole.CheminReseau, r.Jeton, nil, nil); code != http.StatusOK {
		t.Fatalf("l'appareil d'alice a été effacé pendant que l'équipe était illisible : %d", code)
	}
}

// Audit serveur n°4 bis : une purge massive (fichier tronqué) est refusée.
func TestAuditPurgeMassiveRefusee(t *testing.T) {
	b := nouveauBanc(t)
	var jetons []string
	for _, u := range []string{"alice@x.fr", "bob@x.fr", "carole@x.fr"} {
		jetons = append(jetons, b.inscrire("", u, "portable").Jeton)
	}
	os.WriteFile(b.srv.cfg.Equipe, []byte("# fichier tronqué\n"), 0o600)
	b.srv.Synchroniser()
	os.WriteFile(b.srv.cfg.Equipe, []byte("alice@x.fr equipe\nbob@x.fr equipe\ncarole@x.fr equipe\n"), 0o600)
	b.srv.Synchroniser()
	for _, j := range jetons {
		if code := b.appel("GET", protocole.CheminReseau, j, nil, nil); code != http.StatusOK {
			t.Fatalf("un fichier tronqué un instant a effacé des appareils : %d", code)
		}
	}
}

// Audit serveur n°3 : personne ne prend le nom « serveur » ni celui d'une
// machine, et les noms du serveur restent au serveur dans le DNS.
func TestAuditNomsReserves(t *testing.T) {
	b := nouveauBanc(t)
	r := b.inscrire("", "alice@x.fr", "Serveur")
	if r.Appareil.Nom == "serveur" || !strings.HasSuffix(r.Appareil.Nom, "-alice") {
		t.Fatalf("un appareil personnel a obtenu le nom %q", r.Appareil.Nom)
	}
	b.inscrire("maison", "", "maison")
	// Une deuxième machine « maison » est refusée, pas renommée.
	k, _ := noise.GenererCle()
	if code := b.appel("POST", protocole.CheminConnexion, "", b.demande(k, b.cle("maison", "", "maison"), "maison"), nil); code != http.StatusConflict {
		t.Fatalf("deuxième machine « maison » : %d, 409 attendu", code)
	}
	// Une machine ne peut pas s'appeler « serveur » non plus.
	k2, _ := noise.GenererCle()
	if code := b.appel("POST", protocole.CheminConnexion, "", b.demande(k2, b.cle("nas", "", "serveur"), "x"), nil); code != http.StatusConflict {
		t.Fatalf("machine nommée « serveur » : %d, 409 attendu", code)
	}
	b.srv.Synchroniser()
	if a := resoudre(t, b.dns, "10.77.0.1", "serveur.sas.internal."); a != "10.77.0.1" {
		t.Fatalf("serveur.sas.internal vaut %s", a)
	}
}

// Audit serveur, durcissement a : le DNS ne répond qu'avec les noms que le
// demandeur a le droit de connaître.
func TestAuditDNSFiltre(t *testing.T) {
	b := nouveauBanc(t)
	maison := b.inscrire("maison", "", "maison")
	nas := b.inscrire("nas", "", "nas")
	alice := b.inscrire("", "alice@x.fr", "portable")
	b.srv.Synchroniser()
	if a := resoudre(t, b.dns, alice.Appareil.Adresse, "maison.sas.internal."); a != maison.Appareil.Adresse {
		t.Fatalf("alice doit trouver la maison : %q", a)
	}
	if a := resoudre(t, b.dns, alice.Appareil.Adresse, "nas.sas.internal."); a != "" {
		t.Fatalf("alice ne doit pas trouver le nas : %q", a)
	}
	if a := resoudre(t, b.dns, nas.Appareil.Adresse, "maison.sas.internal."); a != "" {
		t.Fatalf("le nas ne doit pas trouver la maison : %q", a)
	}
}

// Audit serveur n°7 : un numéro et une adresse libérés ne sont pas redonnés
// au suivant.
func TestAuditNumeroEtAdresseNonReutilises(t *testing.T) {
	b := nouveauBanc(t)
	un := b.inscrire("maison", "", "maison")
	b.base.SupprimerParNom("maison")
	deux := b.inscrire("nas", "", "nas")
	if deux.Appareil.Numero == un.Appareil.Numero || deux.Appareil.Adresse == un.Appareil.Adresse {
		t.Fatalf("numéro ou adresse réutilisé : %d %s, puis %d %s",
			un.Appareil.Numero, un.Appareil.Adresse, deux.Appareil.Numero, deux.Appareil.Adresse)
	}
}

// Audit serveur n°9 : une clé d'inscription n'est pas perdue quand
// l'inscription est refusée.
func TestAuditCleNonConsommeeSurRefus(t *testing.T) {
	b := nouveauBanc(t)
	c := b.cle("", "dave@x.fr", "")
	k, _ := noise.GenererCle()
	if code := b.appel("POST", protocole.CheminConnexion, "", b.demande(k, c, "portable"), nil); code != http.StatusForbidden {
		t.Fatalf("dave hors de l'équipe : %d, 403 attendu", code)
	}
	os.WriteFile(b.srv.cfg.Equipe, []byte("dave@x.fr equipe\n"), 0o600)
	if code := b.appel("POST", protocole.CheminConnexion, "", b.demande(k, c, "portable"), nil); code != http.StatusOK {
		t.Fatalf("la clé de dave a été gaspillée par le premier refus : %d", code)
	}
}

// Audit serveur n°6 : un pare-feu en échec n'active aucun nouvel appareil.
func TestAuditPareFeuEnEchec(t *testing.T) {
	b := nouveauBanc(t)
	appliquerPareFeu = func(string) error { return os.ErrPermission }
	defer func() { appliquerPareFeu = func(string) error { return nil } }()
	b.inscrire("maison", "", "maison")
	if n := len(b.srv.moteur.Etat()); n != 0 {
		t.Fatalf("%d pair(s) activé(s) alors que le pare-feu a échoué", n)
	}
}

// resoudre interroge le DNS du serveur comme le ferait l'appareil à
// l'adresse de.
func resoudre(t *testing.T, d *dns.Serveur, de, nom string) string {
	t.Helper()
	q := new(mdns.Msg)
	q.SetQuestion(nom, mdns.TypeA)
	w := &reponseDNS{de: net.UDPAddrFromAddrPort(netip.AddrPortFrom(netip.MustParseAddr(de), 5353))}
	d.ServeDNS(w, q)
	if w.m == nil || len(w.m.Answer) == 0 {
		return ""
	}
	return w.m.Answer[0].(*mdns.A).A.String()
}

type reponseDNS struct {
	mdns.ResponseWriter
	de net.Addr
	m  *mdns.Msg
}

func (r *reponseDNS) RemoteAddr() net.Addr       { return r.de }
func (r *reponseDNS) WriteMsg(m *mdns.Msg) error { r.m = m; return nil }
