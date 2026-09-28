package base

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

// Une base d'avant la liste des invitations : ses clés restent valables,
// apparaissent dans la liste sans date de création ni créateur, et
// s'annulent comme les autres.
func TestInvitationsBaseAncienne(t *testing.T) {
	chemin := filepath.Join(t.TempDir(), "sas.db")
	db, err := sql.Open("sqlite", "file:"+chemin)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE cles (empreinte BLOB PRIMARY KEY, etiquette TEXT NOT NULL DEFAULT '',
		utilisateur TEXT NOT NULL DEFAULT '', nom TEXT NOT NULL DEFAULT '', expire INTEGER NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO cles (empreinte, utilisateur, expire) VALUES (?, 'alice@x.fr', ?)`,
		Empreinte("sas-ancienne"), time.Now().Add(time.Hour).Unix()); err != nil {
		t.Fatal(err)
	}
	db.Close()

	b, err := Ouvrir(chemin)
	if err != nil {
		t.Fatal(err)
	}
	defer b.db.Close()
	if err := b.CreerInvitation("sas-nouvelle", "", "bob@x.fr", "", "admin@x.fr", time.Now().Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	liste, err := b.Invitations()
	if err != nil || len(liste) != 2 {
		t.Fatalf("%d invitations, %v", len(liste), err)
	}
	if a := liste[0]; a.Utilisateur != "alice@x.fr" || !a.Cree.IsZero() || a.Createur != "" {
		t.Errorf("ancienne clé : %+v", a)
	}
	if n := liste[1]; n.Utilisateur != "bob@x.fr" || n.Cree.IsZero() || n.Createur != "admin@x.fr" {
		t.Errorf("nouvelle clé : %+v", n)
	}
	if _, err := b.AnnulerInvitation(liste[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := b.AnnulerInvitation(liste[0].ID); err != ErrIntrouvable {
		t.Errorf("deuxième annulation : %v", err)
	}
	if liste, _ := b.Invitations(); len(liste) != 1 || liste[0].Utilisateur != "bob@x.fr" {
		t.Errorf("après l'annulation : %+v", liste)
	}
}
