package serveur

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Cybertrist/CyberSAS/internal/b64"
	"github.com/Cybertrist/CyberSAS/internal/noise"
	"github.com/Cybertrist/CyberSAS/internal/protocole"
	"github.com/Cybertrist/CyberSAS/internal/verrou"
)

// Le libellé : pris du nom proposé à l'inscription, changé par l'appareil
// lui-même, jamais par un autre membre de l'équipe.
func TestLibelle(t *testing.T) {
	b := nouveauBanc(t)
	alice := b.inscrire("", "alice@x.fr", "Portable d'Alice")
	bob := b.inscrire("", "bob@x.fr", "tel")
	if alice.Appareil.Libelle != "Portable d'Alice" || alice.Appareil.Nom != "portable-d-alice-alice" {
		t.Fatalf("inscrit : nom %q, libellé %q", alice.Appareil.Nom, alice.Appareil.Libelle)
	}
	if code := b.appel("POST", protocole.CheminLibelle, alice.Jeton, protocole.DemandeLibelle{Libelle: "  Z Fold8 Alice \x1b "}, nil); code != http.StatusOK {
		t.Fatalf("renommer le sien : %d", code)
	}
	var r protocole.EtatReseau
	b.appel("GET", protocole.CheminReseau, alice.Jeton, nil, &r)
	if r.Moi.Libelle != "Z Fold8 Alice" {
		t.Errorf("libellé relu : %q", r.Moi.Libelle)
	}
	// Bob n'est pas admin : il ne renomme pas l'appareil d'Alice.
	autre := protocole.DemandeLibelle{ClePublique: alice.Appareil.ClePublique, Libelle: "pirate"}
	if code := b.appel("POST", protocole.CheminLibelle, bob.Jeton, autre, nil); code != http.StatusForbidden {
		t.Errorf("renommer celui d'un autre : %d, 403 attendu", code)
	}
	if code := b.appel("POST", protocole.CheminLibelle, alice.Jeton, protocole.DemandeLibelle{Libelle: " \t "}, nil); code != http.StatusBadRequest {
		t.Errorf("nom vide : %d, 400 attendu", code)
	}
}

// Les routes des admins : fermées aux autres ; ouvertes, elles n'acceptent
// que des certificats vraiment signés par le verrou.
func TestRoutesAdmin(t *testing.T) {
	b := nouveauBanc(t)
	pub, prive, _ := verrou.Generer()
	cheminVerrou := filepath.Join(b.dossier, "verrou")
	os.WriteFile(cheminVerrou, []byte(base64.StdEncoding.EncodeToString(pub)), 0o600)
	b.srv.cfg.Verrou = cheminVerrou
	os.WriteFile(b.srv.cfg.Equipe, []byte("admin@x.fr admins\nalice@x.fr equipe\n"), 0o600)

	admin := b.inscrire("", "admin@x.fr", "fold")
	alice := b.inscrire("", "alice@x.fr", "portable")

	// Avec un verrou, un compte d'admin ne suffit pas : son appareil doit
	// être signé pour le groupe « admins ».
	if code := b.appel("GET", protocole.CheminAppareils, admin.Jeton, nil, nil); code != http.StatusForbidden {
		t.Errorf("admin pas encore signé : %d, 403 attendu", code)
	}
	b.certifier(t, prive, admin, "equipe", time.Hour)
	if code := b.appel("GET", protocole.CheminAppareils, admin.Jeton, nil, nil); code != http.StatusForbidden {
		t.Errorf("admin signé pour « equipe » : %d, 403 attendu", code)
	}
	b.certifier(t, prive, admin, "admins", -time.Hour)
	if code := b.appel("GET", protocole.CheminAppareils, admin.Jeton, nil, nil); code != http.StatusForbidden {
		t.Errorf("certificat d'admin expiré : %d, 403 attendu", code)
	}
	b.certifier(t, prive, admin, "admins", time.Hour)

	for _, c := range []struct{ methode, chemin string }{
		{"GET", protocole.CheminAppareils}, {"POST", protocole.CheminSignatures}, {"POST", protocole.CheminRetrait},
	} {
		if code := b.appel(c.methode, c.chemin, alice.Jeton, []protocole.Appareil{}, nil); code != http.StatusForbidden {
			t.Errorf("%s sans être admin : %d, 403 attendu", c.chemin, code)
		}
	}

	var liste []protocole.Appareil
	if code := b.appel("GET", protocole.CheminAppareils, admin.Jeton, nil, &liste); code != http.StatusOK || len(liste) != 2 {
		t.Fatalf("liste : %d, %d appareils", code, len(liste))
	}
	var fiche protocole.Appareil
	for _, a := range liste {
		if a.ClePublique == alice.Appareil.ClePublique {
			fiche = a
		}
	}
	if fiche.Signature != "" || fiche.Groupe != "equipe" {
		t.Fatalf("fiche d'Alice : %+v", fiche)
	}

	// Un certificat signé par une autre clé : refusé.
	_, faux, _ := verrou.Generer()
	signer := func(cle []byte) protocole.Appareil {
		k, _ := b64.Cle32(fiche.ClePublique)
		c := verrou.Certificat{Cle: k, Adresse: netip.MustParseAddr(fiche.Adresse), Proprietaire: fiche.Proprietaire,
			Groupe: fiche.Groupe, Expire: time.Now().Add(24 * time.Hour).Truncate(time.Second)}
		sig, err := c.Signer(cle)
		if err != nil {
			t.Fatal(err)
		}
		a := fiche
		a.Signature, a.SignatureExpire = base64.StdEncoding.EncodeToString(sig), c.Expire
		return a
	}
	if code := b.appel("POST", protocole.CheminSignatures, admin.Jeton, []protocole.Appareil{signer(faux)}, nil); code != http.StatusBadRequest {
		t.Errorf("certificat d'une autre clé : %d, 400 attendu", code)
	}
	// Signé par le verrou : accepté, et Alice le voit.
	if code := b.appel("POST", protocole.CheminSignatures, admin.Jeton, []protocole.Appareil{signer(prive)}, nil); code != http.StatusOK {
		t.Fatalf("vrai certificat : %d", code)
	}
	var r protocole.EtatReseau
	b.appel("GET", protocole.CheminReseau, alice.Jeton, nil, &r)
	if r.Moi.Signature == "" {
		t.Error("Alice ne voit pas son certificat")
	}

	// Retrait : pas de soi-même ; celui d'Alice, oui.
	if code := b.appel("POST", protocole.CheminRetrait, admin.Jeton, protocole.DemandeRetrait{ClePublique: admin.Appareil.ClePublique}, nil); code != http.StatusBadRequest {
		t.Errorf("se retirer soi-même : %d, 400 attendu", code)
	}
	if code := b.appel("POST", protocole.CheminRetrait, admin.Jeton, protocole.DemandeRetrait{ClePublique: alice.Appareil.ClePublique}, nil); code != http.StatusOK {
		t.Errorf("retirer Alice : %d", code)
	}
	if code := b.appel("GET", protocole.CheminReseau, alice.Jeton, nil, nil); code != http.StatusUnauthorized {
		t.Errorf("Alice retirée répond encore : %d", code)
	}
}

// L'invitation depuis l'appli : réservée aux admins, et seulement pour un
// membre de l'équipe ; la clé rendue sert une fois.
func TestInvitation(t *testing.T) {
	b := nouveauBanc(t)
	os.WriteFile(b.srv.cfg.Equipe, []byte("admin@x.fr admins\nalice@x.fr equipe\n"), 0o600)
	admin := b.inscrire("", "admin@x.fr", "fold")
	alice := b.inscrire("", "alice@x.fr", "portable")

	demande := protocole.DemandeInvitation{Utilisateur: " Alice@X.fr ", Minutes: 5}
	if code := b.appel("POST", protocole.CheminInvitation, alice.Jeton, demande, nil); code != http.StatusForbidden {
		t.Errorf("invitation sans être admin : %d, 403 attendu", code)
	}
	if code := b.appel("POST", protocole.CheminInvitation, admin.Jeton, protocole.DemandeInvitation{Utilisateur: "inconnu@x.fr"}, nil); code != http.StatusBadRequest {
		t.Errorf("invitation hors de l'équipe : %d, 400 attendu", code)
	}
	var inv protocole.ReponseInvitation
	if code := b.appel("POST", protocole.CheminInvitation, admin.Jeton, demande, &inv); code != http.StatusOK || inv.Cle == "" {
		t.Fatalf("invitation : %d, %+v", code, inv)
	}
	if d := time.Until(inv.Expire); d < 4*time.Minute || d > 5*time.Minute {
		t.Errorf("expire dans %v, 5 min attendues", d)
	}
	k, _ := noise.GenererCle()
	var r protocole.ReponseConnexion
	if code := b.appel("POST", protocole.CheminConnexion, "", b.demande(k, inv.Cle, "tablette"), &r); code != http.StatusOK {
		t.Fatalf("inscription avec l'invitation : %d", code)
	}
	if r.Appareil.Proprietaire != "alice@x.fr" {
		t.Errorf("propriétaire : %q", r.Appareil.Proprietaire)
	}
	k2, _ := noise.GenererCle()
	if code := b.appel("POST", protocole.CheminConnexion, "", b.demande(k2, inv.Cle, "autre"), nil); code == http.StatusOK {
		t.Error("l'invitation a servi deux fois")
	}
}

// La révocation depuis l'appli : signée par le verrou, plus récente que la
// liste en vigueur, sans en oublier une clé ; l'appareil révoqué disparaît.
func TestRevocations(t *testing.T) {
	b := nouveauBanc(t)
	pub, prive, _ := verrou.Generer()
	cheminVerrou := filepath.Join(b.dossier, "verrou")
	os.WriteFile(cheminVerrou, []byte(base64.StdEncoding.EncodeToString(pub)), 0o600)
	b.srv.cfg.Verrou = cheminVerrou
	b.srv.cfg.Revocations = filepath.Join(b.dossier, "revocations.json")
	b.srv.cfg.RevocationsAppli = filepath.Join(b.dossier, "revocations-appli.json")
	os.WriteFile(b.srv.cfg.Equipe, []byte("admin@x.fr admins\nalice@x.fr equipe\nbob@x.fr equipe\n"), 0o600)
	admin := b.inscrire("", "admin@x.fr", "fold")
	alice := b.inscrire("", "alice@x.fr", "portable")
	bob := b.inscrire("", "bob@x.fr", "tel")
	b.certifier(t, prive, admin, "admins", time.Hour)

	liste := func(cle []byte, version uint64, cles ...string) protocole.ListeRevocations {
		var brutes [][32]byte
		for _, c := range cles {
			k, _ := b64.Cle32(c)
			brutes = append(brutes, k)
		}
		return protocole.ListeRevocations{Version: version, Cles: cles,
			Signature: base64.StdEncoding.EncodeToString(verrou.SignerRevocations(cle, version, brutes))}
	}
	ka, kb := alice.Appareil.ClePublique, bob.Appareil.ClePublique

	// La liste de « sas.sh revoquer », version 3 : bob.
	brut, _ := json.Marshal(liste(prive, 3, kb))
	os.WriteFile(b.srv.cfg.Revocations, brut, 0o600)

	if code := b.appel("POST", protocole.CheminRevocations, alice.Jeton, liste(prive, 4, kb, ka), nil); code != http.StatusForbidden {
		t.Errorf("révoquer sans être admin : %d, 403 attendu", code)
	}
	_, faux, _ := verrou.Generer()
	for nom, c := range map[string]struct {
		l    protocole.ListeRevocations
		code int
	}{
		"autre clé":      {liste(faux, 4, kb, ka), http.StatusBadRequest},
		"pas plus neuve": {liste(prive, 3, kb, ka), http.StatusConflict},
		"oublie bob":     {liste(prive, 4, ka), http.StatusBadRequest},
		"soi-même":       {liste(prive, 4, kb, admin.Appareil.ClePublique), http.StatusBadRequest},
	} {
		if code := b.appel("POST", protocole.CheminRevocations, admin.Jeton, c.l, nil); code != c.code {
			t.Errorf("%s : %d, %d attendu", nom, code, c.code)
		}
	}
	if code := b.appel("POST", protocole.CheminRevocations, admin.Jeton, liste(prive, 4, kb, ka), nil); code != http.StatusOK {
		t.Fatalf("vraie liste : %d", code)
	}
	var r protocole.EtatReseau
	b.appel("GET", protocole.CheminReseau, admin.Jeton, nil, &r)
	if r.Revocations == nil || r.Revocations.Version != 4 || len(r.Revocations.Cles) != 2 {
		t.Errorf("liste servie : %+v", r.Revocations)
	}
	if code := b.appel("GET", protocole.CheminReseau, alice.Jeton, nil, nil); code != http.StatusUnauthorized {
		t.Errorf("Alice révoquée répond encore : %d", code)
	}
}

// certifier : l'appareil reçoit son certificat, comme par « sas verrou
// signer » en ligne de commande. expire peut être négatif.
func (b *banc) certifier(t *testing.T, prive []byte, r protocole.ReponseConnexion, groupe string, expire time.Duration) {
	t.Helper()
	k, _ := b64.Cle32(r.Appareil.ClePublique)
	c := verrou.Certificat{Cle: k, Adresse: netip.MustParseAddr(r.Appareil.Adresse), Proprietaire: r.Appareil.Proprietaire,
		Groupe: groupe, Expire: time.Now().Add(expire).Truncate(time.Second)}
	sig, err := c.Signer(prive)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.srv.base.DefinirCertificat(r.Appareil.ClePublique, c.Adresse, sig, groupe, c.Expire); err != nil {
		t.Fatal(err)
	}
}

// Deux listes envoyées en même temps : la plus récente gagne toujours, et
// le fichier reste lisible. Chaque liste contient les précédentes.
func TestRevocationsSimultanees(t *testing.T) {
	b := nouveauBanc(t)
	pub, prive, _ := verrou.Generer()
	cheminVerrou := filepath.Join(b.dossier, "verrou")
	os.WriteFile(cheminVerrou, []byte(base64.StdEncoding.EncodeToString(pub)), 0o600)
	b.srv.cfg.Verrou = cheminVerrou
	b.srv.cfg.RevocationsAppli = filepath.Join(b.dossier, "revocations-appli.json")
	os.WriteFile(b.srv.cfg.Equipe, []byte("admin@x.fr admins\n"), 0o600)
	admin := b.inscrire("", "admin@x.fr", "fold")
	b.certifier(t, prive, admin, "admins", time.Hour)
	liste := func(version uint64) protocole.ListeRevocations {
		var cles []string
		var brutes [][32]byte
		for i := range version {
			var k [32]byte
			k[0], k[31] = byte(i), 7
			cles = append(cles, base64.StdEncoding.EncodeToString(k[:]))
			brutes = append(brutes, k)
		}
		return protocole.ListeRevocations{Version: version, Cles: cles,
			Signature: base64.StdEncoding.EncodeToString(verrou.SignerRevocations(prive, version, brutes))}
	}
	for v := uint64(1); v <= 40; v += 2 {
		fini := make(chan struct{})
		go func() { b.appel("POST", protocole.CheminRevocations, admin.Jeton, liste(v+1), nil); close(fini) }()
		b.appel("POST", protocole.CheminRevocations, admin.Jeton, liste(v), nil)
		<-fini
		if l := b.srv.Revocations(); l == nil || l.Version != v+1 {
			t.Fatalf("après v%d et v%d en même temps : %+v", v, v+1, l)
		}
	}
}
