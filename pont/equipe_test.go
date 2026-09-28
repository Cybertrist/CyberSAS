package pont

import (
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Cybertrist/CyberSAS/internal/appareil"
	"github.com/Cybertrist/CyberSAS/internal/protocole"
)

// L'écran Équipe : la liste lue est nettoyée, la demande part telle
// quelle, et un refus du serveur revient avec sa phrase.
func TestEquipe(t *testing.T) {
	var recue *protocole.DemandeMembre
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != protocole.CheminEquipe {
			http.NotFound(w, r)
			return
		}
		if r.Method == "POST" {
			recue = &protocole.DemandeMembre{}
			json.NewDecoder(r.Body).Decode(recue)
			if recue.Groupe == "" && recue.Adresse == "admin@x.fr" {
				w.WriteHeader(http.StatusConflict)
				json.NewEncoder(w).Encode(protocole.Erreur{Erreur: "c'est le dernier admin : nommer d'abord un autre admin"})
				return
			}
		}
		json.NewEncoder(w).Encode([]protocole.Membre{
			{Adresse: "admin@x.fr", Groupe: "admins", Moi: true},
			{Adresse: "lea@x.fr\x1b[31m", Groupe: "equipe"},
		})
	}))
	t.Cleanup(srv.Close)
	dossier := t.TempDir()
	autorite := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	if err := (appareil.Stockage{Dossier: dossier}).Ecrire(appareil.Etat{Serveur: srv.URL, Autorite: string(autorite)}); err != nil {
		t.Fatal(err)
	}

	brut, err := Equipe(dossier)
	if err != nil {
		t.Fatal(err)
	}
	var liste []Membre
	if err := json.Unmarshal([]byte(brut), &liste); err != nil || len(liste) != 2 {
		t.Fatalf("liste : %q, %v", brut, err)
	}
	if !liste[0].Moi || liste[1].Moi || strings.ContainsRune(liste[1].Adresse, 0x1b) {
		t.Errorf("liste mal lue ou pas nettoyée : %+v", liste)
	}

	if _, err := ChangerMembre(dossier, "dave@x.fr", "equipe"); err != nil || recue == nil || *recue != (protocole.DemandeMembre{Adresse: "dave@x.fr", Groupe: "equipe"}) {
		t.Errorf("ajout : %v, demande %+v", err, recue)
	}
	if _, err := ChangerMembre(dossier, "admin@x.fr", ""); err == nil || !strings.Contains(err.Error(), "dernier admin") {
		t.Errorf("refus du serveur : %v", err)
	}
}
