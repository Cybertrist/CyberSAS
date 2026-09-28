package serveur

import (
	"cmp"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strings"

	"github.com/Cybertrist/CyberSAS/internal/politique"
	"github.com/Cybertrist/CyberSAS/internal/protocole"
)

// L'équipe depuis l'appli : la liste, un membre ajouté, changé de groupe
// ou retiré. C'est ce que font « sas.sh membre » et « sas.sh retirer »,
// sur le même fichier : equipe.txt reste la seule liste des personnes
// autorisées, et ses commentaires sont gardés.
//
// Cela ne donne au serveur aucun pouvoir de plus : il lit déjà ce fichier,
// et un serveur piraté pourrait déjà le changer. Avec un verrou, entrer
// dans l'équipe ne suffit toujours pas : chaque appareil doit encore être
// signé sur le téléphone de l'admin, pour le groupe qu'il voit.

// adresseMembre : la même règle que « sas.sh membre ».
var adresseMembre = regexp.MustCompile(`^[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]+$`)

// adresseValide : une adresse de courriel, en minuscules, pas un roman.
func adresseValide(a string) bool {
	return len(a) <= 254 && adresseMembre.MatchString(a)
}

// membres : l'équipe en liste, les admins d'abord, puis par adresse.
func membres(e politique.Equipe, moi string) []protocole.Membre {
	r := []protocole.Membre{}
	for a, g := range e {
		r = append(r, protocole.Membre{Adresse: a, Groupe: g, Moi: a == moi})
	}
	slices.SortFunc(r, func(x, y protocole.Membre) int {
		if (x.Groupe == "admins") != (y.Groupe == "admins") {
			if x.Groupe == "admins" {
				return -1
			}
			return 1
		}
		return cmp.Compare(x.Adresse, y.Adresse)
	})
	return r
}

// listeEquipe : qui est dans l'équipe, et dans quel groupe.
func (s *Serveur) listeEquipe(w http.ResponseWriter, r *http.Request) {
	moi, ok := s.admin(w, r)
	if !ok {
		return
	}
	repondre(w, http.StatusOK, membres(s.chargerEquipe(), moi.Proprietaire))
}

// reecrireEquipe : le texte de equipe.txt après ce changement. Les
// commentaires et les lignes des autres restent tels quels ; la ligne de
// la personne est remplacée à sa place (ses doublons disparaissent), ou
// ajoutée à la fin. Groupe vide : elle est retirée.
func reecrireEquipe(contenu, adresse, groupe string) string {
	var sortie []string
	fait := false
	if strings.TrimSpace(contenu) != "" {
		for _, l := range strings.Split(strings.TrimRight(contenu, "\r\n"), "\n") {
			ch := strings.Fields(l)
			if len(ch) >= 2 && !strings.HasPrefix(ch[0], "#") && strings.ToLower(ch[0]) == adresse {
				if groupe != "" && !fait {
					sortie = append(sortie, adresse+" "+groupe)
					fait = true
				}
				continue
			}
			sortie = append(sortie, l)
		}
	}
	if groupe != "" && !fait {
		sortie = append(sortie, adresse+" "+groupe)
	}
	if len(sortie) == 0 {
		return ""
	}
	return strings.Join(sortie, "\n") + "\n"
}

// emailsWeb : les adresses seules, une par ligne, comme les écrit
// « sas.sh » pour oauth2-proxy.
func emailsWeb(e politique.Equipe) []byte {
	var b strings.Builder
	for _, m := range membres(e, "") {
		b.WriteString(m.Adresse + "\n")
	}
	return []byte(b.String())
}

// refusChangement : pourquoi moi ne peut pas mettre adresse dans ce
// groupe (vide : la retirer), avec le code HTTP ; 0 si rien ne s'y oppose.
// Tout est vérifié sur l'équipe relue sous muEquipe : deux admins qui se
// retirent l'un l'autre en même temps n'en laissent pas zéro.
func refusChangement(e politique.Equipe, moi, adresse, groupe string) (int, string) {
	if adresse == moi {
		return http.StatusBadRequest, "on ne change pas son propre accès : un autre admin doit le faire"
	}
	if e[adresse] == "admins" && groupe != "admins" {
		admins := 0
		for _, g := range e {
			if g == "admins" {
				admins++
			}
		}
		if admins <= 1 {
			return http.StatusConflict, "c'est le dernier admin : nommer d'abord un autre admin"
		}
	}
	switch {
	case e[moi] != "admins":
		// Retiré ou rétrogradé entre la vérification et ce moment-ci.
		return http.StatusForbidden, "réservé aux admins"
	case groupe == "" && e[adresse] == "":
		return http.StatusNotFound, texteCourt(adresse, 80) + " n'est pas dans l'équipe"
	}
	return 0, ""
}

// membre : un admin ajoute quelqu'un à l'équipe, change son groupe, ou
// l'en retire. Ses appareils suivent à la synchronisation : retiré, il
// est coupé de partout.
//
// Deux garde-fous : on ne change pas son propre accès (un admin ne se
// retire ni ne se rétrograde lui-même), et l'équipe garde toujours au
// moins un admin.
func (s *Serveur) membre(w http.ResponseWriter, r *http.Request) {
	moi, ok := s.admin(w, r)
	if !ok {
		return
	}
	var d protocole.DemandeMembre
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&d); err != nil {
		refuser(w, http.StatusBadRequest, "demande illisible")
		return
	}
	adresse := strings.ToLower(strings.TrimSpace(d.Adresse))
	if !adresseValide(adresse) {
		refuser(w, http.StatusBadRequest, "adresse invalide")
		return
	}
	switch d.Groupe {
	case "", "admins", "equipe":
	default:
		refuser(w, http.StatusBadRequest, "groupe inconnu (admins ou equipe)")
		return
	}
	s.muEquipe.Lock()
	defer s.muEquipe.Unlock()
	// On relit le fichier lui-même, pas la dernière version gardée : une
	// équipe illisible ne doit pas être réécrite à partir d'un souvenir.
	brut, err := os.ReadFile(s.cfg.Equipe)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		s.journal.Error("équipe illisible", "evenement", "equipe", "erreur", err)
		refuser(w, http.StatusInternalServerError, "équipe illisible")
		return
	}
	actuelle, err := politique.LireEquipe(strings.NewReader(string(brut)))
	if err != nil {
		refuser(w, http.StatusInternalServerError, "équipe illisible")
		return
	}
	avant := actuelle[adresse]
	if code, message := refusChangement(actuelle, moi.Proprietaire, adresse, d.Groupe); code != 0 {
		refuser(w, code, message)
		return
	}
	if avant == d.Groupe {
		repondre(w, http.StatusOK, membres(actuelle, moi.Proprietaire))
		return
	}

	texte := reecrireEquipe(string(brut), adresse, d.Groupe)
	nouvelle, err := politique.LireEquipe(strings.NewReader(texte))
	if err != nil || nouvelle[adresse] != d.Groupe {
		refuser(w, http.StatusInternalServerError, "équipe non enregistrée")
		return
	}
	// Lisibles par l'utilisateur de sas.sh et par oauth2-proxy, qui ne
	// sont pas root : 0644. Le dossier etat/ reste fermé aux autres.
	// La liste des services web d'abord : si elle échoue, rien n'a changé.
	if s.cfg.EquipeWeb != "" {
		if err := ecrireAtomiqueMode(s.cfg.EquipeWeb, emailsWeb(nouvelle), 0o644); err != nil {
			s.journal.Error("liste web non enregistrée", "evenement", "equipe", "erreur", err)
			refuser(w, http.StatusInternalServerError, "équipe non enregistrée")
			return
		}
	}
	if err := ecrireAtomiqueMode(s.cfg.Equipe, []byte(texte), 0o644); err != nil {
		s.journal.Error("équipe non enregistrée", "evenement", "equipe", "erreur", err)
		refuser(w, http.StatusInternalServerError, "équipe non enregistrée")
		return
	}
	s.journal.Warn("équipe modifiée", "evenement", "equipe", "membre", adresse, "avant", avant, "apres", d.Groupe, "par", moi.Nom)
	if err := s.Synchroniser(); err != nil {
		s.journal.Error("synchronisation après changement d'équipe", "evenement", "synchronisation", "erreur", err)
	}
	repondre(w, http.StatusOK, membres(nouvelle, moi.Proprietaire))
}
