package pont

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/Cybertrist/CyberSAS/internal/appareil"
	"github.com/Cybertrist/CyberSAS/internal/client"
	"github.com/Cybertrist/CyberSAS/internal/politique"
	"github.com/Cybertrist/CyberSAS/internal/protocole"
	"github.com/Cybertrist/CyberSAS/internal/verrou"
)

func lireVue(t *testing.T, s string) Vue {
	t.Helper()
	var v Vue
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("JSON illisible : %v\n%s", err, s)
	}
	return v
}

// Sans rien : une vue vide mais valide, jamais « null ».
func TestVueVide(t *testing.T) {
	v := lireVue(t, vueJSON(appareil.Vue{}, false, "", time.Now()))
	if v.EnMarche || v.Connecte || v.Pairs == nil || len(v.Pairs) != 0 {
		t.Errorf("vue vide inattendue : %+v", v)
	}
}

// Le serveur passe en tête, cet appareil est marqué, et un pair écarté par
// le client est « non signé » avec sa raison. Sans moteur, personne n'a de
// session.
func TestVueReseau(t *testing.T) {
	cle := "zKaLpkfwgqY3dEfBoAqXd+bsVVrXxGkRCq13XRRO/EY="
	r := protocole.EtatReseau{
		Serveur: protocole.Serveur{ClePublique: cle, Adresse: "10.77.0.1"},
		Moi:     protocole.Appareil{Nom: "fold8-tristan", Adresse: "10.77.0.18"},
		Pairs: []protocole.Appareil{
			{Nom: "maison", Adresse: "10.77.0.2", Etiquette: "maison", EnLigne: true},
			{Nom: "intrus", Adresse: "10.77.0.9"},
		},
	}
	vue := appareil.Vue{Reseau: r, Ecartes: []client.Ecarte{{Nom: "intrus", Adresse: "10.77.0.9", Raison: "pas signé par le verrou"}}}
	v := lireVue(t, vueJSON(vue, true, "", time.Now()))
	if !v.EnMarche || v.Connecte {
		t.Errorf("en marche %v, connecté %v", v.EnMarche, v.Connecte)
	}
	if len(v.Pairs) != 4 || !v.Pairs[0].Serveur || !v.Pairs[1].Moi {
		t.Fatalf("ordre inattendu : %+v", v.Pairs)
	}
	for _, p := range v.Pairs {
		switch p.Nom {
		case "maison":
			if !p.Signe || !p.EnLigne {
				t.Errorf("maison : %+v", p)
			}
		case "intrus":
			if p.Signe || p.Raison == "" {
				t.Errorf("intrus accepté : %+v", p)
			}
		}
		if p.Session {
			t.Errorf("%s a une session sans moteur", p.Nom)
		}
	}
}

// Les ports viennent de la politique vérifiée : connus, chaque pair porte
// ce qu'on peut lui ouvrir ; inconnus, l'appli ne doit rien en conclure.
func TestVuePorts(t *testing.T) {
	r := protocole.EtatReseau{
		Serveur: protocole.Serveur{ClePublique: "zKaLpkfwgqY3dEfBoAqXd+bsVVrXxGkRCq13XRRO/EY=", Adresse: "10.77.0.1"},
		Moi:     protocole.Appareil{Nom: "fold8-tristan", Adresse: "10.77.0.18"},
		Pairs:   []protocole.Appareil{{Nom: "maison", Adresse: "10.77.0.2"}, {Nom: "portable", Adresse: "10.77.0.3"}},
	}
	sortant := map[netip.Addr][]politique.Port{
		netip.MustParseAddr("10.77.0.2"): {{Proto: "tcp", Debut: 80, Fin: 80}, {Proto: "tcp", Debut: 443, Fin: 443}},
	}
	v := lireVue(t, vueJSON(appareil.Vue{Reseau: r, Sortant: sortant}, true, "", time.Now()))
	if !v.PortsConnus {
		t.Error("ports connus attendus")
	}
	for _, p := range v.Pairs {
		attendu := map[string]string{"maison": "tcp:80 tcp:443"}[p.Nom]
		if got := strings.Join(p.Ports, " "); got != attendu {
			t.Errorf("%s : ports %q, attendu %q", p.Nom, got, attendu)
		}
	}
	if v := lireVue(t, vueJSON(appareil.Vue{Reseau: r}, true, "", time.Now())); v.PortsConnus {
		t.Error("sans politique vérifiée, les ports ne sont pas connus")
	}
}

// Une erreur de synchronisation remonte, texte nettoyé.
func TestVueErreur(t *testing.T) {
	v := lireVue(t, vueJSON(appareil.Vue{Erreur: errors.New("serveur\x1b[31m injoignable")}, true, "", time.Now()))
	if v.Erreur != "serveur?[31m injoignable" {
		t.Errorf("erreur : %q", v.Erreur)
	}
}

// Une clé collée avec des espaces, un retour à la ligne ou des guillemets
// passe ; une clé tronquée dit combien de caractères sont arrivés.
func TestVerrouColle(t *testing.T) {
	dossier := t.TempDir()
	pub, prive, _ := verrou.Generer()
	if err := (appareil.Stockage{Dossier: dossier}).Ecrire(appareil.Etat{Retenu: client.Retenu{Verrou: base64.StdEncoding.EncodeToString(pub)}}); err != nil {
		t.Fatal(err)
	}
	graine := base64.StdEncoding.EncodeToString(prive.Seed())
	if _, err := VerifierVerrou(dossier, " \"\n"+graine[:20]+" "+graine[20:]+"\r\n\" "); err != nil {
		t.Errorf("clé collée avec des espaces : %v", err)
	}
	if _, err := VerifierVerrou(dossier, graine[:30]); err == nil || !strings.Contains(err.Error(), "30 caractères") {
		t.Errorf("clé tronquée : %v", err)
	}
	_, autre, _ := verrou.Generer()
	if _, err := VerifierVerrou(dossier, base64.StdEncoding.EncodeToString(autre.Seed())); err == nil {
		t.Error("une autre clé que celle du verrou a été acceptée")
	}
}
