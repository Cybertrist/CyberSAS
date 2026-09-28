//go:build unix

package serveur

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/Cybertrist/CyberSAS/internal/protocole"
)

// Le verrou de fichier commun avec sas.sh : tant que le script le tient,
// l'appli ne réécrit pas l'équipe ; un verrou remplacé par un lien
// symbolique n'est pas suivi.
func TestVerrouEquipe(t *testing.T) {
	attenteVerrouEquipe = 200 * time.Millisecond
	t.Cleanup(func() { attenteVerrouEquipe = 10 * time.Second })
	b := nouveauBanc(t)
	os.WriteFile(b.srv.cfg.Equipe, []byte("admin@x.fr admins\n"), 0o600)
	admin := b.inscrire("", "admin@x.fr", "fold")
	demande := protocole.DemandeMembre{Adresse: "dave@x.fr", Groupe: "equipe"}

	// sas.sh tient le verrou (flock sur le même fichier).
	chemin := filepath.Join(b.dossier, fichierVerrouEquipe)
	f, err := os.OpenFile(chemin, os.O_RDONLY|os.O_CREATE, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	if code := b.appel("POST", protocole.CheminEquipe, admin.Jeton, demande, nil); code != http.StatusServiceUnavailable {
		t.Errorf("équipe changée pendant que sas.sh tient le verrou : %d, 503 attendu", code)
	}
	if brut, _ := os.ReadFile(b.srv.cfg.Equipe); string(brut) != "admin@x.fr admins\n" {
		t.Errorf("equipe.txt réécrit malgré le verrou : %q", brut)
	}
	f.Close()
	if code := b.appel("POST", protocole.CheminEquipe, admin.Jeton, demande, nil); code != http.StatusOK {
		t.Errorf("verrou rendu : %d", code)
	}

	// Un lien symbolique à la place du verrou : refusé, et la cible
	// n'est ni créée ni ouverte.
	os.Remove(chemin)
	cible := filepath.Join(t.TempDir(), "cible")
	if err := os.Symlink(cible, chemin); err != nil {
		t.Fatal(err)
	}
	if _, err := verrouillerEquipe(b.dossier); err == nil {
		t.Error("verrou pris à travers un lien symbolique")
	}
	if _, err := os.Lstat(cible); err == nil {
		t.Error("la cible du lien a été créée")
	}
	// Un dossier à la place du verrou : refusé aussi.
	os.Remove(chemin)
	os.Mkdir(chemin, 0o700)
	if _, err := verrouillerEquipe(b.dossier); err == nil {
		t.Error("verrou pris sur un dossier")
	}
}
