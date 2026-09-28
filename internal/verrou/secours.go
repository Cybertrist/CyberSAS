package verrou

// La sauvegarde de secours de la clé du verrou.
//
// Sans elle, un téléphone perdu sans autre copie de la clé oblige à créer
// un nouveau verrou et à réinscrire tout le réseau. La sauvegarde est un
// petit texte, à ranger hors du téléphone (gestionnaire de mots de passe,
// clé USB, papier) :
//
//	cybersas-secours-1:<base64 url, sans bourrage>
//
// Une fois décodé, 127 octets :
//
//	version     1 octet   (1)
//	mémoire     4 octets  Argon2id, en Kio
//	passes      1 octet   Argon2id
//	parallèle   1 octet   Argon2id
//	sel        16 octets  aléatoire
//	nonce      24 octets  aléatoire (XChaCha20-Poly1305)
//	publique   32 octets  la clé publique du verrou, en clair
//	chiffré    48 octets  la graine Ed25519 (32) et l'étiquette (16)
//
// La clé de chiffrement vient de la phrase de passe par Argon2id, une
// dérivation lente et gourmande en mémoire : chaque essai d'un voleur du
// fichier lui coûte autant qu'à l'admin. Tout ce qui précède le chiffré est
// en clair, mais authentifié comme données associées : changer un seul
// octet de l'en-tête (les paramètres, la clé publique) fait échouer
// l'ouverture comme une mauvaise phrase.
//
// La clé publique en clair permet de reconnaître la sauvegarde (son
// empreinte est celle du verrou) sans connaître la phrase. Elle n'apprend
// rien à personne : tous les appareils du réseau la connaissent déjà.

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/chacha20poly1305"
)

// PrefixeSecours : le début de toute sauvegarde de secours, suivi du
// numéro de version et de deux-points.
const PrefixeSecours = "cybersas-secours-"

const (
	versionSecours  = 1
	contexteSecours = "CyberSas secours v1\x00"

	tailleSel     = 16
	tailleEntete  = 1 + 4 + 1 + 1 + tailleSel + chacha20poly1305.NonceSizeX + ed25519.PublicKeySize
	tailleSecours = tailleEntete + ed25519.SeedSize + chacha20poly1305.Overhead

	// PhraseMin : la phrase de passe la plus courte acceptée, en
	// caractères. Au-dessous, même Argon2id ne tient pas longtemps face à
	// qui a volé le fichier.
	PhraseMin = 12
	// phraseMax : de quoi coller une vraie phrase, pas un livre.
	phraseMax = 1024
)

// ParametresSecours : le coût d'Argon2id.
type ParametresSecours struct {
	Memoire   uint32 // Kio
	Passes    uint8
	Parallele uint8
}

// Les paramètres des nouvelles sauvegardes : la seconde recommandation de
// la RFC 9106 (64 Mio, 3 passes, 4 voies), pensée pour les appareils dont
// la mémoire est comptée. Sur un téléphone récent, l'ouverture prend de
// l'ordre d'une seconde : rien pour l'admin, qui le fait deux fois par an,
// beaucoup pour qui essaie des milliards de phrases. Une variable, pour que
// les tests aillent plus vite.
var parametresSecours = ParametresSecours{Memoire: 64 * 1024, Passes: 3, Parallele: 4}

// Les bornes acceptées à la lecture. Les paramètres sont lus avant de
// pouvoir vérifier quoi que ce soit : sans bornes, un fichier forgé
// demanderait 4 Tio de mémoire ou des milliers de passes, et ferait tomber
// l'appli qui l'ouvre. 256 Mio laissent de la marge pour durcir plus tard.
const (
	memoireMin, memoireMax     = 16 * 1024, 256 * 1024
	passesMin, passesMax       = 1, 16
	paralleleMin, paralleleMax = 1, 8
)

var (
	ErrPhraseCourte      = errors.New("phrase de passe trop courte : 12 caractères au moins")
	ErrPhraseLongue      = errors.New("phrase de passe trop longue")
	ErrSecoursIllisible  = errors.New("ce n'est pas une sauvegarde de secours CyberSAS, ou elle est incomplète")
	ErrVersionSecours    = errors.New("sauvegarde de secours d'une version inconnue : mettre CyberSAS à jour")
	ErrParametresSecours = errors.New("sauvegarde de secours aux paramètres refusés")
	// Une mauvaise phrase et un octet changé donnent la même erreur :
	// l'étiquette du chiffrement authentifié ne permet pas de les
	// distinguer, et il n'y a rien de plus à dire à qui essaie des phrases.
	ErrPhraseFausse      = errors.New("phrase de passe fausse, ou sauvegarde modifiée")
	ErrSecoursIncoherent = errors.New("sauvegarde de secours incohérente : la clé ne correspond pas à son verrou")
)

func (p ParametresSecours) valides() bool {
	return p.Memoire >= memoireMin && p.Memoire <= memoireMax &&
		p.Passes >= passesMin && p.Passes <= passesMax &&
		p.Parallele >= paralleleMin && p.Parallele <= paralleleMax
}

// VerifierPhrase : assez longue, pas démesurée, et du vrai texte. Compte
// les caractères, pas les octets : « éééé » ne vaut pas huit.
func VerifierPhrase(phrase string) error {
	if len(phrase) > phraseMax || !utf8.ValidString(phrase) {
		return ErrPhraseLongue
	}
	if utf8.RuneCountInString(phrase) < PhraseMin {
		return ErrPhraseCourte
	}
	return nil
}

func deriver(phrase string, sel []byte, p ParametresSecours) []byte {
	return argon2.IDKey([]byte(phrase), sel, uint32(p.Passes), p.Memoire, p.Parallele, chacha20poly1305.KeySize)
}

// donneesAssociees : le contexte, puis l'en-tête tel qu'il est écrit.
func donneesAssociees(entete []byte) []byte {
	return append([]byte(contexteSecours), entete...)
}

// Secours : la sauvegarde de secours de prive, chiffrée par phrase.
func Secours(prive ed25519.PrivateKey, phrase string) (string, error) {
	if len(prive) != ed25519.PrivateKeySize {
		return "", errors.New("clé du verrou invalide")
	}
	if err := VerifierPhrase(phrase); err != nil {
		return "", err
	}
	graine := prive.Seed()
	defer clear(graine)
	return sceller(parametresSecours, versionSecours, prive.Public().(ed25519.PublicKey), graine, phrase)
}

// sceller écrit la sauvegarde telle quelle, sans rien vérifier : les tests
// s'en servent aussi pour forger des sauvegardes fausses mais bien
// chiffrées.
func sceller(p ParametresSecours, version byte, publique, graine []byte, phrase string) (string, error) {
	entete := make([]byte, 0, tailleEntete+len(graine)+chacha20poly1305.Overhead)
	entete = append(entete, version)
	entete = binary.BigEndian.AppendUint32(entete, p.Memoire)
	entete = append(entete, p.Passes, p.Parallele)
	aleas := make([]byte, tailleSel+chacha20poly1305.NonceSizeX)
	if _, err := rand.Read(aleas); err != nil {
		return "", err
	}
	entete = append(entete, aleas...)
	entete = append(entete, publique...)
	sel, nonce := aleas[:tailleSel], aleas[tailleSel:]

	k := deriver(phrase, sel, p)
	defer clear(k)
	aead, err := chacha20poly1305.NewX(k)
	if err != nil {
		return "", err
	}
	tout := aead.Seal(entete, nonce, graine, donneesAssociees(entete))
	return PrefixeSecours + "1:" + base64.RawURLEncoding.EncodeToString(tout), nil
}

// EstSecours : texte ressemble à une sauvegarde de secours (sans rien
// vérifier d'autre). Pour choisir, là où l'on colle une clé, entre la
// graine en base64 et une sauvegarde qui demande sa phrase.
func EstSecours(texte string) bool {
	return strings.HasPrefix(strings.TrimSpace(texte), PrefixeSecours)
}

// lireSecours : les octets de la sauvegarde, en-tête contrôlé (version et
// bornes des paramètres), sans la phrase. Un gestionnaire de mots de passe
// ou un courriel coupent parfois les longues lignes : les blancs sont
// ignorés.
func lireSecours(texte string) ([]byte, ParametresSecours, error) {
	var p ParametresSecours
	if len(texte) > 4096 {
		return nil, p, ErrSecoursIllisible
	}
	s := strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, texte)
	reste, ok := strings.CutPrefix(s, PrefixeSecours)
	if !ok {
		return nil, p, ErrSecoursIllisible
	}
	version, corps, ok := strings.Cut(reste, ":")
	if !ok {
		return nil, p, ErrSecoursIllisible
	}
	if version != "1" {
		return nil, p, ErrVersionSecours
	}
	b, err := base64.RawURLEncoding.Strict().DecodeString(corps)
	if err != nil || len(b) != tailleSecours || base64.RawURLEncoding.EncodeToString(b) != corps {
		return nil, p, ErrSecoursIllisible
	}
	if b[0] != versionSecours {
		return nil, p, ErrVersionSecours
	}
	p = ParametresSecours{Memoire: binary.BigEndian.Uint32(b[1:5]), Passes: b[5], Parallele: b[6]}
	if !p.valides() {
		return nil, p, ErrParametresSecours
	}
	return b, p, nil
}

// PubliqueSecours : la clé publique du verrou qu'une sauvegarde dit
// contenir, lue sans la phrase. Elle n'est garantie qu'une fois la
// sauvegarde ouverte (OuvrirSecours) : sert à reconnaître, pas à croire.
func PubliqueSecours(texte string) (ed25519.PublicKey, error) {
	b, _, err := lireSecours(texte)
	if err != nil {
		return nil, err
	}
	return ed25519.PublicKey(b[tailleEntete-ed25519.PublicKeySize : tailleEntete]), nil
}

// OuvrirSecours : la clé du verrou rangée dans la sauvegarde. Refuse toute
// sauvegarde modifiée, et vérifie que la clé ouverte est bien celle dont
// l'en-tête donne la moitié publique.
func OuvrirSecours(texte, phrase string) (ed25519.PrivateKey, error) {
	b, p, err := lireSecours(texte)
	if err != nil {
		return nil, err
	}
	if len(phrase) > phraseMax {
		return nil, ErrPhraseFausse
	}
	entete := b[:tailleEntete]
	sel := entete[7 : 7+tailleSel]
	nonce := entete[7+tailleSel : 7+tailleSel+chacha20poly1305.NonceSizeX]
	publique := ed25519.PublicKey(entete[tailleEntete-ed25519.PublicKeySize:])

	k := deriver(phrase, sel, p)
	defer clear(k)
	aead, err := chacha20poly1305.NewX(k)
	if err != nil {
		return nil, err
	}
	graine, err := aead.Open(nil, nonce, b[tailleEntete:], donneesAssociees(entete))
	if err != nil {
		return nil, ErrPhraseFausse
	}
	defer clear(graine)
	if len(graine) != ed25519.SeedSize {
		return nil, ErrSecoursIncoherent
	}
	prive := ed25519.NewKeyFromSeed(graine)
	if !prive.Public().(ed25519.PublicKey).Equal(publique) {
		clear(prive)
		return nil, ErrSecoursIncoherent
	}
	return prive, nil
}
