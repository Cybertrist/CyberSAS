package serveur

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/Cybertrist/CyberSAS/internal/politique"
	"github.com/Cybertrist/CyberSAS/internal/protocole"
)

// L'équipe depuis l'appli : réservée aux admins ; ajouter, changer de
// groupe et retirer réécrivent equipe.txt sans perdre ses commentaires, et
// la liste des services web suit.
func TestEquipe(t *testing.T) {
	b := nouveauBanc(t)
	b.srv.cfg.EquipeWeb = filepath.Join(b.dossier, "emails.txt")
	os.WriteFile(b.srv.cfg.Equipe, []byte("# adresse Google        groupe (admins ou equipe)\nadmin@x.fr admins\nalice@x.fr equipe\n\n# les amis\nbob@x.fr equipe\n"), 0o600)
	admin := b.inscrire("", "admin@x.fr", "fold")
	alice := b.inscrire("", "alice@x.fr", "portable")
	bob := b.inscrire("", "bob@x.fr", "tel")

	if code := b.appel("GET", protocole.CheminEquipe, alice.Jeton, nil, nil); code != http.StatusForbidden {
		t.Errorf("liste sans être admin : %d, 403 attendu", code)
	}
	if code := b.appel("POST", protocole.CheminEquipe, alice.Jeton, protocole.DemandeMembre{Adresse: "alice@x.fr", Groupe: "admins"}, nil); code != http.StatusForbidden {
		t.Errorf("se nommer admin sans l'être : %d, 403 attendu", code)
	}
	if code := b.appel("GET", protocole.CheminEquipe, "", nil, nil); code != http.StatusUnauthorized {
		t.Errorf("liste sans jeton : %d, 401 attendu", code)
	}

	var liste []protocole.Membre
	if code := b.appel("GET", protocole.CheminEquipe, admin.Jeton, nil, &liste); code != http.StatusOK || len(liste) != 3 {
		t.Fatalf("liste : %d, %+v", code, liste)
	}
	if liste[0] != (protocole.Membre{Adresse: "admin@x.fr", Groupe: "admins", Moi: true}) || liste[1].Adresse != "alice@x.fr" {
		t.Errorf("ordre ou champs : %+v", liste)
	}

	for nom, c := range map[string]struct {
		d    protocole.DemandeMembre
		code int
	}{
		"adresse vide":       {protocole.DemandeMembre{Groupe: "equipe"}, http.StatusBadRequest},
		"pas une adresse":    {protocole.DemandeMembre{Adresse: "dave", Groupe: "equipe"}, http.StatusBadRequest},
		"retour à la ligne":  {protocole.DemandeMembre{Adresse: "dave@x.fr\npirate@x.fr admins", Groupe: "equipe"}, http.StatusBadRequest},
		"espace dedans":      {protocole.DemandeMembre{Adresse: "dave@x.fr admins", Groupe: "equipe"}, http.StatusBadRequest},
		"groupe inconnu":     {protocole.DemandeMembre{Adresse: "dave@x.fr", Groupe: "root"}, http.StatusBadRequest},
		"se retirer":         {protocole.DemandeMembre{Adresse: "admin@x.fr"}, http.StatusBadRequest},
		"se rétrograder":     {protocole.DemandeMembre{Adresse: "Admin@X.fr", Groupe: "equipe"}, http.StatusBadRequest},
		"retirer un inconnu": {protocole.DemandeMembre{Adresse: "inconnu@x.fr"}, http.StatusNotFound},
	} {
		if code := b.appel("POST", protocole.CheminEquipe, admin.Jeton, c.d, nil); code != c.code {
			t.Errorf("%s : %d, %d attendu", nom, code, c.code)
		}
	}

	// Ajout, en majuscules et avec des espaces : rangé en minuscules.
	if code := b.appel("POST", protocole.CheminEquipe, admin.Jeton, protocole.DemandeMembre{Adresse: "  Dave@X.fr ", Groupe: "equipe"}, &liste); code != http.StatusOK || len(liste) != 4 {
		t.Fatalf("ajout : %d, %+v", code, liste)
	}
	// Alice devient admin, Bob sort.
	if code := b.appel("POST", protocole.CheminEquipe, admin.Jeton, protocole.DemandeMembre{Adresse: "alice@x.fr", Groupe: "admins"}, nil); code != http.StatusOK {
		t.Fatalf("changement de groupe : %d", code)
	}
	if code := b.appel("POST", protocole.CheminEquipe, admin.Jeton, protocole.DemandeMembre{Adresse: "bob@x.fr"}, &liste); code != http.StatusOK || len(liste) != 3 {
		t.Fatalf("retrait : %d, %+v", code, liste)
	}

	brut, _ := os.ReadFile(b.srv.cfg.Equipe)
	attendu := "# adresse Google        groupe (admins ou equipe)\nadmin@x.fr admins\nalice@x.fr admins\n\n# les amis\ndave@x.fr equipe\n"
	if string(brut) != attendu {
		t.Errorf("equipe.txt réécrit :\n%s\nattendu :\n%s", brut, attendu)
	}
	if web, _ := os.ReadFile(b.srv.cfg.EquipeWeb); string(web) != "admin@x.fr\nalice@x.fr\ndave@x.fr\n" {
		t.Errorf("liste web : %q", web)
	}
	if st, err := os.Stat(b.srv.cfg.Equipe); err != nil || st.Size() == 0 {
		t.Fatalf("equipe.txt : %v", err)
	}
	// Bob est sorti : ses appareils sont coupés à la synchronisation.
	if code := b.appel("GET", protocole.CheminReseau, bob.Jeton, nil, nil); code != http.StatusUnauthorized {
		t.Errorf("Bob retiré de l'équipe répond encore : %d", code)
	}
	// Refaire la même chose ne change rien.
	if code := b.appel("POST", protocole.CheminEquipe, admin.Jeton, protocole.DemandeMembre{Adresse: "dave@x.fr", Groupe: "equipe"}, nil); code != http.StatusOK {
		t.Errorf("ajout répété : %d", code)
	}
	if encore, _ := os.ReadFile(b.srv.cfg.Equipe); string(encore) != attendu {
		t.Errorf("ajout répété a réécrit le fichier :\n%s", encore)
	}
}

// Les garde-fous, vérifiés sur l'équipe relue : ils tiennent même quand
// deux admins agissent en même temps.
func TestRefusChangement(t *testing.T) {
	seul := politique.Equipe{"admin@x.fr": "admins", "alice@x.fr": "equipe"}
	deux := politique.Equipe{"admin@x.fr": "admins", "bea@x.fr": "admins"}
	for nom, c := range map[string]struct {
		e                    politique.Equipe
		moi, adresse, groupe string
		code                 int
	}{
		"ajout":                    {seul, "admin@x.fr", "dave@x.fr", "equipe", 0},
		"promotion":                {seul, "admin@x.fr", "alice@x.fr", "admins", 0},
		"retrait d'un autre admin": {deux, "admin@x.fr", "bea@x.fr", "", 0},
		"soi-même":                 {deux, "admin@x.fr", "admin@x.fr", "", http.StatusBadRequest},
		"inconnu":                  {seul, "admin@x.fr", "zoe@x.fr", "", http.StatusNotFound},
		"plus admin entre-temps":   {seul, "bea@x.fr", "alice@x.fr", "", http.StatusForbidden},
		"dernier admin retiré":     {politique.Equipe{"admin@x.fr": "admins"}, "admin@x.fr", "admin@x.fr", "", http.StatusBadRequest},
		"dernier admin (course)":   {politique.Equipe{"bea@x.fr": "admins", "admin@x.fr": "equipe"}, "admin@x.fr", "bea@x.fr", "", http.StatusConflict},
		"dernier admin rétrogradé": {politique.Equipe{"bea@x.fr": "admins", "admin@x.fr": "equipe"}, "admin@x.fr", "bea@x.fr", "equipe", http.StatusConflict},
	} {
		if code, _ := refusChangement(c.e, c.moi, c.adresse, c.groupe); code != c.code {
			t.Errorf("%s : %d, %d attendu", nom, code, c.code)
		}
	}
}

// reecrireEquipe garde tout ce qui n'est pas la ligne de la personne, et
// fait disparaître ses doublons.
func TestReecrireEquipe(t *testing.T) {
	for _, c := range []struct{ avant, adresse, groupe, apres string }{
		{"", "a@x.fr", "equipe", "a@x.fr equipe\n"},
		{"# seul\n", "a@x.fr", "admins", "# seul\na@x.fr admins\n"},
		{"A@x.fr equipe\n# a@x.fr admins\na@x.fr admins\n", "a@x.fr", "admins", "a@x.fr admins\n# a@x.fr admins\n"},
		{"b@x.fr equipe\na@x.fr equipe\n", "a@x.fr", "", "b@x.fr equipe\n"},
		{"a@x.fr equipe", "a@x.fr", "", ""},
		{"b@x.fr equipe\r\nc@x.fr equipe\r\n", "a@x.fr", "equipe", "b@x.fr equipe\r\nc@x.fr equipe\na@x.fr equipe\n"},
	} {
		if r := reecrireEquipe(c.avant, c.adresse, c.groupe); r != c.apres {
			t.Errorf("%q, %s %s : %q, %q attendu", c.avant, c.adresse, c.groupe, r, c.apres)
		}
	}
}
