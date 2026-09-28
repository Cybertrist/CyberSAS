package pont

import (
	"errors"
	"strings"
	"testing"

	"github.com/Cybertrist/CyberSAS/internal/verrou"
)

// La sauvegarde faite depuis le coffre se restaure sur le même réseau, et
// redonne exactement la graine rangée.
func TestSecoursAllerRetour(t *testing.T) {
	dossier, graine, _, pub := banc(t)
	const phrase = "cheval agrafe pile correcte"
	if _, err := SauverVerrou(dossier, graine, "trop court"); !errors.Is(err, verrou.ErrPhraseCourte) {
		t.Fatalf("phrase courte : %v", err)
	}
	s, err := SauverVerrou(dossier, graine, phrase)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(s, verrou.PrefixeSecours) {
		t.Fatalf("sauvegarde : %q", s)
	}
	if e, err := EmpreinteSecours(s); err != nil || e != verrou.Empreinte(pub) {
		t.Fatalf("empreinte : %q, %v", e, err)
	}
	if _, err := RestaurerVerrou(dossier, s, "une autre phrase"); !errors.Is(err, verrou.ErrPhraseFausse) {
		t.Fatalf("phrase fausse : %v", err)
	}
	g, err := RestaurerVerrou(dossier, s, phrase)
	if err != nil || g != graine {
		t.Fatalf("restauration : %v", err)
	}
	// La graine rendue passe la vérification du coffre.
	if _, err := VerifierVerrou(dossier, g); err != nil {
		t.Fatal(err)
	}
}

// Une clé qui n'est pas celle du verrou retenu ne se sauvegarde pas, et la
// sauvegarde d'un autre verrou ne se restaure pas ici.
func TestSecoursAutreVerrou(t *testing.T) {
	dossier, _, _, _ := banc(t)
	_, graineAutre, _, _ := banc(t)
	const phrase = "cheval agrafe pile correcte"
	if _, err := SauverVerrou(dossier, graineAutre, phrase); err == nil {
		t.Fatal("clé d'un autre verrou sauvegardée")
	}
	_, prive, _ := verrou.Generer()
	s, err := verrou.Secours(prive, phrase)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RestaurerVerrou(dossier, s, phrase); err == nil || !strings.Contains(err.Error(), "autre verrou") {
		t.Fatalf("sauvegarde d'un autre verrou : %v", err)
	}
	if _, err := RestaurerVerrou(dossier, "bonjour", phrase); !errors.Is(err, verrou.ErrSecoursIllisible) {
		t.Fatalf("texte quelconque : %v", err)
	}
}
