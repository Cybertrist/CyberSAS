package verrou

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"strings"
	"testing"
)

const phrase = "cheval agrafe pile correcte"

// Les tests dérivent au plus bas des bornes acceptées : 64 Mio et trois
// passes, sous le détecteur de courses, les rendraient très lents.
func init() {
	parametresSecours = ParametresSecours{Memoire: memoireMin, Passes: 1, Parallele: 1}
}

// octets et texte : aller et retour entre la sauvegarde et ses octets.
func octets(t *testing.T, s string) []byte {
	t.Helper()
	b, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(s, PrefixeSecours+"1:"))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func texte(b []byte) string { return PrefixeSecours + "1:" + base64.RawURLEncoding.EncodeToString(b) }

func TestSecoursAllerRetour(t *testing.T) {
	pub, prive, _ := Generer()
	s, err := Secours(prive, phrase)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(s, "cybersas-secours-1:") || len(s) > 200 || !EstSecours(s) {
		t.Fatalf("sauvegarde mal écrite : %q", s)
	}
	// Même clé, même phrase : sel et nonce neufs, jamais deux fois le même texte.
	if s2, _ := Secours(prive, phrase); s2 == s {
		t.Fatal("deux sauvegardes identiques : sel ou nonce pas aléatoire")
	}
	// Reconnaissable sans la phrase.
	p, err := PubliqueSecours(s)
	if err != nil || !p.Equal(pub) || Empreinte(p) != Empreinte(pub) {
		t.Fatalf("clé publique de l'en-tête : %v", err)
	}
	ouverte, err := OuvrirSecours(s, phrase)
	if err != nil || !ouverte.Equal(prive) {
		t.Fatalf("aller-retour raté : %v", err)
	}
	// Coupée en lignes et entourée de blancs, comme dans un gestionnaire
	// de mots de passe : toujours lisible.
	coupee := "  " + s[:40] + "\n" + s[40:100] + "\r\n" + s[100:] + "\n"
	if ouverte, err := OuvrirSecours(coupee, phrase); err != nil || !ouverte.Equal(prive) {
		t.Fatalf("sauvegarde coupée illisible : %v", err)
	}
	// Les paramètres par défaut sont bien écrits dans l'en-tête.
	b := octets(t, s)
	if b[0] != 1 || binary.BigEndian.Uint32(b[1:5]) != parametresSecours.Memoire || b[5] != parametresSecours.Passes {
		t.Fatalf("en-tête : % x", b[:7])
	}
}

func TestSecoursPhrase(t *testing.T) {
	_, prive, _ := Generer()
	for _, p := range []string{"", "court", "onze carac.", "éééééééééé"} {
		if _, err := Secours(prive, p); !errors.Is(err, ErrPhraseCourte) {
			t.Errorf("phrase %q acceptée : %v", p, err)
		}
	}
	// Douze caractères, même accentués (vingt-quatre octets).
	if _, err := Secours(prive, "éééééééééééé"); err != nil {
		t.Fatalf("phrase de douze caractères refusée : %v", err)
	}
	if _, err := Secours(prive, strings.Repeat("a", 2000)); !errors.Is(err, ErrPhraseLongue) {
		t.Fatalf("phrase de 2000 octets : %v", err)
	}
	if _, err := Secours(prive, "douze octets\xff invalide"); err == nil {
		t.Fatal("phrase qui n'est pas de l'UTF-8 acceptée")
	}

	s, _ := Secours(prive, phrase)
	for _, fausse := range []string{"", "cheval agrafe pile correctE", phrase + " ", strings.Repeat("x", 5000)} {
		if _, err := OuvrirSecours(s, fausse); !errors.Is(err, ErrPhraseFausse) {
			t.Errorf("phrase fausse %.30q : %v", fausse, err)
		}
	}
}

// Un octet changé, où qu'il soit, et rien ne s'ouvre : ni dans l'en-tête
// (version et paramètres mis à part, contrôlés avant), ni dans le chiffré.
func TestSecoursModifiee(t *testing.T) {
	_, prive, _ := Generer()
	s, _ := Secours(prive, phrase)
	b := octets(t, s)
	for _, i := range []int{
		7,                // sel
		7 + tailleSel,    // nonce
		tailleEntete - 1, // clé publique
		tailleEntete,     // chiffré
		len(b) - 1,       // étiquette
	} {
		c := append([]byte(nil), b...)
		c[i] ^= 1
		if _, err := OuvrirSecours(texte(c), phrase); !errors.Is(err, ErrPhraseFausse) {
			t.Errorf("octet %d changé : %v", i, err)
		}
	}
	// Les paramètres changés mais dans les bornes : l'en-tête est
	// authentifié, la sauvegarde ne s'ouvre pas.
	c := append([]byte(nil), b...)
	binary.BigEndian.PutUint32(c[1:5], memoireMin+1024)
	if _, err := OuvrirSecours(texte(c), phrase); !errors.Is(err, ErrPhraseFausse) {
		t.Errorf("mémoire changée : %v", err)
	}
	// Tronquée, allongée, préfixe absent, base64 non canonique.
	for nom, x := range map[string]string{
		"tronquée":     s[:len(s)-4],
		"allongée":     s + "AAAA",
		"sans préfixe": strings.TrimPrefix(s, PrefixeSecours),
		"autre chose":  "bonjour",
		"bourrage":     s + "=",
		"base64 std":   PrefixeSecours + "1:" + strings.NewReplacer("-", "+", "_", "/").Replace(strings.TrimPrefix(s, PrefixeSecours+"1:")),
		"démesurée":    s + strings.Repeat(" ", 5000),
	} {
		if x == s {
			continue
		}
		if _, err := OuvrirSecours(x, phrase); !errors.Is(err, ErrSecoursIllisible) {
			t.Errorf("%s : %v", nom, err)
		}
	}
}

func TestSecoursVersion(t *testing.T) {
	_, prive, _ := Generer()
	s, _ := Secours(prive, phrase)
	// Le numéro du préfixe.
	if _, err := OuvrirSecours(strings.Replace(s, "-1:", "-2:", 1), phrase); !errors.Is(err, ErrVersionSecours) {
		t.Errorf("préfixe version 2 : %v", err)
	}
	// L'octet de version.
	b := octets(t, s)
	b[0] = 2
	if _, err := OuvrirSecours(texte(b), phrase); !errors.Is(err, ErrVersionSecours) {
		t.Errorf("octet de version 2 : %v", err)
	}
	if _, err := PubliqueSecours(texte(b)); !errors.Is(err, ErrVersionSecours) {
		t.Errorf("clé publique d'une version inconnue : %v", err)
	}
}

// Un fichier forgé ne doit pas pouvoir faire dériver l'appli avec 4 Gio
// de mémoire ou des centaines de passes : refusé avant toute dérivation
// (sinon ce test ne finirait pas).
func TestSecoursParametresAbsurdes(t *testing.T) {
	_, prive, _ := Generer()
	b := octets(t, func() string { s, _ := Secours(prive, phrase); return s }())
	for nom, modif := range map[string]func([]byte){
		"4 Gio":         func(c []byte) { binary.BigEndian.PutUint32(c[1:5], 1<<22) },
		"64 Gio":        func(c []byte) { binary.BigEndian.PutUint32(c[1:5], 1<<26) },
		"mémoire nulle": func(c []byte) { binary.BigEndian.PutUint32(c[1:5], 0) },
		"8 Kio":         func(c []byte) { binary.BigEndian.PutUint32(c[1:5], 8) },
		"255 passes":    func(c []byte) { c[5] = 255 },
		"0 passe":       func(c []byte) { c[5] = 0 },
		"0 voie":        func(c []byte) { c[6] = 0 },
		"255 voies":     func(c []byte) { c[6] = 255 },
	} {
		c := append([]byte(nil), b...)
		modif(c)
		if _, err := OuvrirSecours(texte(c), phrase); !errors.Is(err, ErrParametresSecours) {
			t.Errorf("%s : %v", nom, err)
		}
	}
	// Les paramètres de production sont dans les bornes.
	if !(ParametresSecours{Memoire: 64 * 1024, Passes: 3, Parallele: 4}).valides() {
		t.Fatal("les paramètres par défaut sont hors bornes")
	}
}

// Quelqu'un qui connaît la phrase forge une sauvegarde bien chiffrée dont
// l'en-tête annonce un autre verrou : refusée.
func TestSecoursIncoherent(t *testing.T) {
	autre, _, _ := Generer()
	_, prive, _ := Generer()
	s, err := sceller(parametresSecours, versionSecours, autre, prive.Seed(), phrase)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := OuvrirSecours(s, phrase); !errors.Is(err, ErrSecoursIncoherent) {
		t.Fatalf("sauvegarde d'un autre verrou : %v", err)
	}
	if _, err := Secours(ed25519.PrivateKey(make([]byte, 10)), phrase); err == nil {
		t.Fatal("clé invalide sauvegardée")
	}
}
