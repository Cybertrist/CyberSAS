package pont

import (
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"math"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Cybertrist/CyberSAS/internal/appareil"
	"github.com/Cybertrist/CyberSAS/internal/b64"
	"github.com/Cybertrist/CyberSAS/internal/client"
	"github.com/Cybertrist/CyberSAS/internal/protocole"
	"github.com/Cybertrist/CyberSAS/internal/verrou"
)

// fauxServeur : un serveur piraté, qui sert ce qu'on lui dit et garde ce
// que l'appli lui envoie.
type fauxServeur struct {
	mu          sync.Mutex
	appareils   [][]protocole.Appareil // une réponse par lecture ; la dernière se répète
	lectures    int
	revocations *protocole.ListeRevocations
	signes      []protocole.Appareil
	liste       *protocole.ListeRevocations
	invitations []protocole.Invitation
	annulee     string
}

func (f *fauxServeur) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch r.URL.Path {
	case protocole.CheminAppareils:
		json.NewEncoder(w).Encode(f.appareils[min(f.lectures, len(f.appareils)-1)])
		f.lectures++
	case protocole.CheminReseau:
		json.NewEncoder(w).Encode(protocole.EtatReseau{Revocations: f.revocations})
	case protocole.CheminSignatures:
		json.NewDecoder(r.Body).Decode(&f.signes)
		w.Write([]byte("{}"))
	case protocole.CheminInvitations:
		json.NewEncoder(w).Encode(f.invitations)
	case protocole.CheminAnnulation:
		var d protocole.DemandeAnnulation
		json.NewDecoder(r.Body).Decode(&d)
		f.annulee = d.ID
		w.Write([]byte("{}"))
	case protocole.CheminRevocations:
		f.liste = &protocole.ListeRevocations{}
		json.NewDecoder(r.Body).Decode(f.liste)
		w.Write([]byte("{}"))
	default:
		http.NotFound(w, r)
	}
}

// banc : un appareil d'admin inscrit auprès du faux serveur, avec le verrou
// retenu. Rend le dossier, la graine du verrou et le faux serveur.
func banc(t *testing.T) (string, string, *fauxServeur, []byte) {
	t.Helper()
	f := &fauxServeur{}
	srv := httptest.NewTLSServer(f)
	t.Cleanup(srv.Close)
	autorite := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	pub, prive, _ := verrou.Generer()
	dossier := t.TempDir()
	e := appareil.Etat{Serveur: srv.URL, Autorite: string(autorite),
		Retenu: client.Retenu{Verrou: base64.StdEncoding.EncodeToString(pub), Reseau: netip.MustParsePrefix("10.77.0.0/24")}}
	if err := (appareil.Stockage{Dossier: dossier}).Ecrire(e); err != nil {
		t.Fatal(err)
	}
	return dossier, base64.StdEncoding.EncodeToString(prive.Seed()), f, pub
}

func cleAuHasard(n byte) string {
	var k [32]byte
	k[0], k[31] = n, 9
	return base64.StdEncoding.EncodeToString(k[:])
}

// L'admin signe ce qu'on lui a montré. Si le serveur change le groupe ou
// l'adresse entre l'affichage et la signature, rien n'est signé.
func TestSignerCeQuiEstMontre(t *testing.T) {
	dossier, graine, f, pub := banc(t)
	lea := protocole.Appareil{Nom: "laptop-lea", Adresse: "10.77.0.20", ClePublique: cleAuHasard(1), Proprietaire: "lea@x.fr", Groupe: "equipe"}
	f.appareils = [][]protocole.Appareil{{lea}}
	montre, err := Appareils(dossier)
	if err != nil {
		t.Fatal(err)
	}

	// Le serveur piraté change le groupe et l'adresse au moment de signer.
	pire := lea
	pire.Groupe, pire.Adresse = "admins", "10.77.0.2"
	f.appareils = [][]protocole.Appareil{{pire}}
	f.lectures = 0
	if n, err := Signer(dossier, graine, montre); err == nil || n != 0 || f.signes != nil {
		t.Fatalf("fiche changée : %d signés, erreur %v", n, err)
	}

	// Rien n'a changé : la fiche montrée est signée, telle quelle.
	f.appareils = [][]protocole.Appareil{{lea}}
	n, err := Signer(dossier, graine, montre)
	if err != nil || n != 1 || len(f.signes) != 1 {
		t.Fatalf("signature : %d, %v", n, err)
	}
	s := f.signes[0]
	k, _ := b64.Cle32(s.ClePublique)
	sig, _ := b64.Decoder(s.Signature)
	c := verrou.Certificat{Cle: k, Adresse: netip.MustParseAddr("10.77.0.20"), Proprietaire: "lea@x.fr", Groupe: "equipe", Expire: s.SignatureExpire}
	if !c.Verifier(pub, sig, time.Now()) {
		t.Error("le certificat envoyé ne porte pas ce qui a été montré")
	}
}

// Une adresse hors du réseau, celle du serveur, ou celle d'un autre
// appareil déjà signé : refusée.
func TestSignerAdresse(t *testing.T) {
	dossier, graine, f, _ := banc(t)
	for _, adresse := range []string{"10.99.0.5", "10.77.0.1", "10.77.0.0", "10.77.0.2"} {
		x := protocole.Appareil{Nom: "x", Adresse: adresse, ClePublique: cleAuHasard(3), Proprietaire: "x@x.fr", Groupe: "equipe"}
		maison := protocole.Appareil{Nom: "maison", Adresse: "10.77.0.2", ClePublique: cleAuHasard(4), Etiquette: "maison", Signature: "c2ln"}
		f.appareils, f.lectures, f.signes = [][]protocole.Appareil{{x, maison}}, 0, nil
		b, _ := json.Marshal([]Fiche{{Nom: "x", Adresse: adresse, Cle: x.ClePublique, Proprietaire: "x@x.fr", Groupe: "equipe"}})
		if n, err := Signer(dossier, graine, string(b)); err == nil || n != 0 {
			t.Errorf("adresse %s : signée", adresse)
		}
	}
}

// Une liste de révocation servie mais pas signée par le verrou n'entre pas
// dans celle que l'admin signe : ni ses clés, ni sa version au plafond.
func TestRevoquerIgnoreUneListeForgee(t *testing.T) {
	dossier, graine, f, pub := banc(t)
	victime := cleAuHasard(5)
	f.revocations = &protocole.ListeRevocations{Version: math.MaxUint64 - 1, Cles: []string{victime}, Signature: "Zm9yZ2U="}
	vole := cleAuHasard(6)
	v, err := Revoquer(dossier, graine, vole)
	if err != nil || v != 1 || f.liste == nil {
		t.Fatalf("révocation : version %d, %v", v, err)
	}
	if len(f.liste.Cles) != 1 || f.liste.Cles[0] != vole || f.liste.Version != 1 {
		t.Errorf("liste signée : %+v", f.liste)
	}
	if _, ok := client.RevocationsSignees(f.liste, pub); !ok {
		t.Error("la liste envoyée n'est pas signée par le verrou")
	}

	// Une liste servie bien signée, elle, est reprise.
	f.revocations = f.liste
	autre := cleAuHasard(7)
	if v, err := Revoquer(dossier, graine, autre); err != nil || v != 2 || len(f.liste.Cles) != 2 {
		t.Errorf("deuxième révocation : version %d, %v, %+v", v, err, f.liste)
	}
	f.revocations = f.liste
	if _, err := Revoquer(dossier, graine, autre); err == nil || !strings.Contains(err.Error(), "déjà") {
		t.Errorf("révoquer deux fois : %v", err)
	}
}

// Le lien du réseau porte le serveur, le verrou et l'autorité, jamais de
// clé d'inscription : il peut circuler sans faire entrer personne.
func TestLienReseau(t *testing.T) {
	dossier, _, _, pub := banc(t)
	lien, err := LienReseau(dossier)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(lien)
	if err != nil || u.Scheme != "cybersas" || u.Host != "rejoindre" {
		t.Fatalf("lien illisible : %q", lien)
	}
	q := u.Query()
	if q.Has("cle") {
		t.Fatal("le lien du réseau ne doit pas porter de clé d'inscription")
	}
	if q.Get("verrou") != base64.StdEncoding.EncodeToString(pub) || !strings.HasPrefix(q.Get("serveur"), "https://") || q.Get("autorite") == "" {
		t.Fatalf("lien incomplet : %q", lien)
	}
}

// Les invitations en cours : une machine se montre par son nom, un
// identifiant qui n'est pas celui d'une invitation est écarté, les textes
// du serveur sont nettoyés ; l'annulation envoie l'identifiant tel quel.
func TestInvitations(t *testing.T) {
	dossier, _, f, _ := banc(t)
	expire := time.Now().Add(time.Hour).Truncate(time.Second)
	f.invitations = []protocole.Invitation{
		{ID: "0123456789abcdef", Utilisateur: "alice@x.fr", Createur: "admin@x.fr", Cree: expire.Add(-time.Hour), Expire: expire},
		{ID: "fedcba9876543210", Etiquette: "maison", Nom: "nas\u202e", Expire: expire},
		{ID: "../retrait", Utilisateur: "bob@x.fr", Expire: expire},
	}
	j, err := Invitations(dossier)
	if err != nil {
		t.Fatal(err)
	}
	var liste []EnCours
	json.Unmarshal([]byte(j), &liste)
	if len(liste) != 2 {
		t.Fatalf("%d invitations, 2 attendues : %s", len(liste), j)
	}
	if a := liste[0]; a.Pour != "alice@x.fr" || a.Machine || a.Createur != "admin@x.fr" || a.Expire != expire.Unix() || a.Cree != expire.Unix()-3600 {
		t.Errorf("invitation d'Alice : %+v", a)
	}
	if m := liste[1]; !m.Machine || m.Pour != client.Propre("nas\u202e") || m.Cree != 0 {
		t.Errorf("machine : %+v", m)
	}
	if err := AnnulerInvitation(dossier, "0123456789abcdef"); err != nil || f.annulee != "0123456789abcdef" {
		t.Fatalf("annulation : %v, %q", err, f.annulee)
	}
}
