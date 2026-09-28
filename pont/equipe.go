package pont

import (
	"encoding/json"

	"github.com/Cybertrist/CyberSAS/internal/client"
	"github.com/Cybertrist/CyberSAS/internal/protocole"
)

// L'équipe, pour l'admin : ce que font « sas.sh membre » et « sas.sh
// retirer », depuis l'appli. Le serveur tient equipe.txt ; il refuse qu'un
// admin change son propre accès ou retire le dernier admin.

// Membre : une personne de l'équipe, pour l'écran Équipe.
type Membre struct {
	Adresse string `json:"adresse"`
	Groupe  string `json:"groupe"`
	Moi     bool   `json:"moi"`
}

// membresJSON : la liste, nettoyée comme le reste de ce que dit le serveur.
func membresJSON(liste []protocole.Membre) string {
	r := []Membre{}
	for _, m := range liste {
		r = append(r, Membre{Adresse: client.Propre(m.Adresse), Groupe: client.Propre(m.Groupe), Moi: m.Moi})
	}
	b, _ := json.Marshal(r)
	return string(b)
}

// Equipe : l'équipe, en JSON ([]Membre). Réservé aux admins.
func Equipe(dossier string) (string, error) {
	var liste []protocole.Membre
	if err := appel(dossier, "GET", protocole.CheminEquipe, nil, &liste); err != nil {
		return "", err
	}
	return membresJSON(liste), nil
}

// ChangerMembre met adresse dans ce groupe (admins ou equipe) : elle entre
// dans l'équipe, ou change de groupe. Groupe vide : elle en sort, et ses
// appareils sont coupés. Rend la nouvelle équipe, comme Equipe.
func ChangerMembre(dossier, adresse, groupe string) (string, error) {
	var liste []protocole.Membre
	if err := appel(dossier, "POST", protocole.CheminEquipe, protocole.DemandeMembre{Adresse: adresse, Groupe: groupe}, &liste); err != nil {
		return "", err
	}
	return membresJSON(liste), nil
}
