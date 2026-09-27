// Package verrou : le verrou du réseau, pour ne pas avoir à croire le
// serveur sur parole.
//
// Le chiffrement de bout en bout empêche le serveur de lire ce qui passe
// entre deux appareils. Mais c'est lui qui distribue les clés publiques et
// les règles d'accès : un serveur piraté pourrait annoncer, sous le nom de
// « maison », une clé qui est la sienne, ou s'ouvrir les ports de tous les
// appareils. Le verrou ferme ces portes.
//
// L'admin garde une clé de signature Ed25519 hors du serveur, sur son
// propre ordinateur. Il signe trois sortes de documents :
//
//   - un certificat par appareil : sa clé publique, son adresse, son
//     étiquette ou son propriétaire et son groupe, et une date d'expiration ;
//   - la politique, telle quelle, avec un numéro de version ;
//   - la liste des clés révoquées, elle aussi numérotée.
//
// Les appareils connaissent la clé publique du verrou. Ils refusent tout
// pair sans certificat valide, calculent eux-mêmes leurs règles d'entrée à
// partir de la politique signée et des certificats, et n'acceptent jamais
// une politique ou une liste de révocation plus ancienne que la dernière
// vue. Pour tricher, il faudrait la clé du verrou, qui n'a jamais touché le
// serveur.
//
// C'est le principe du « Tailnet Lock » de Tailscale, en plus simple : une
// seule clé de signature, pas de chaîne de délégation.
package verrou

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"time"

	"github.com/Cybertrist/CyberSAS/internal/b64"
)

// Chaque type de document a son contexte : une signature faite pour l'un
// ne peut pas servir pour un autre.
const (
	contexteCertificat = "CyberSas certificat v2\x00"
	contextePolitique  = "CyberSas politique v1\x00"
	contexteRevocation = "CyberSas revocations v1\x00"
)

var ErrAdresse = errors.New("verrou : seules les adresses IPv4 sont signées")

// Certificat : ce que l'admin affirme d'un appareil.
type Certificat struct {
	Cle          [32]byte
	Adresse      netip.Addr
	Etiquette    string // une machine
	Proprietaire string // ou une personne,
	Groupe       string // et son groupe
	Expire       time.Time
}

// ErrChamp : un texte du certificat dépasse MaxChamp octets.
var ErrChamp = errors.New("verrou : étiquette, propriétaire ou groupe trop long")

// MaxChamp : la plus grande étiquette, adresse ou groupe signable. Une
// adresse e-mail tient en 254 octets.
const MaxChamp = 255

// champ : longueur sur deux octets, puis la valeur. Sans longueur, deux
// certificats différents pourraient donner le même message. Message a
// vérifié avant que la longueur tient dans ces deux octets.
func champ(b []byte, s string) []byte {
	b = binary.BigEndian.AppendUint16(b, uint16(len(s))) // #nosec G115 -- borné par MaxChamp
	return append(b, s...)
}

// Message : ce qui est signé pour un certificat.
func (c Certificat) Message() ([]byte, error) {
	if !c.Adresse.Is4() {
		return nil, ErrAdresse
	}
	if len(c.Etiquette) > MaxChamp || len(c.Proprietaire) > MaxChamp || len(c.Groupe) > MaxChamp {
		return nil, ErrChamp
	}
	a := c.Adresse.As4()
	m := append([]byte(contexteCertificat), c.Cle[:]...)
	m = append(m, a[:]...)
	m = binary.BigEndian.AppendUint64(m, uint64(c.Expire.Unix()))
	m = champ(m, c.Etiquette)
	m = champ(m, c.Proprietaire)
	return champ(m, c.Groupe), nil
}

func Generer() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	return ed25519.GenerateKey(rand.Reader)
}

func (c Certificat) Signer(prive ed25519.PrivateKey) ([]byte, error) {
	m, err := c.Message()
	if err != nil {
		return nil, err
	}
	return ed25519.Sign(prive, m), nil
}

// Verifier : signature juste, et certificat pas encore expiré.
func (c Certificat) Verifier(publique ed25519.PublicKey, signature []byte, maintenant time.Time) bool {
	m, err := c.Message()
	if err != nil || !valide(publique, signature) || !maintenant.Before(c.Expire) {
		return false
	}
	return ed25519.Verify(publique, m, signature)
}

func valide(publique ed25519.PublicKey, signature []byte) bool {
	return len(publique) == ed25519.PublicKeySize && len(signature) == ed25519.SignatureSize
}

func messageVersionne(contexte string, version uint64, contenu []byte) []byte {
	m := binary.BigEndian.AppendUint64([]byte(contexte), version)
	return append(m, contenu...)
}

// SignerPolitique signe le fichier de politique tel quel, octet pour octet.
func SignerPolitique(prive ed25519.PrivateKey, version uint64, politique []byte) []byte {
	return ed25519.Sign(prive, messageVersionne(contextePolitique, version, politique))
}

func VerifierPolitique(publique ed25519.PublicKey, version uint64, politique, signature []byte) bool {
	return valide(publique, signature) && ed25519.Verify(publique, messageVersionne(contextePolitique, version, politique), signature)
}

// contenuRevocations : les clés triées, pour que l'ordre ne change pas la
// signature.
func contenuRevocations(cles [][32]byte) []byte {
	triees := slices.Clone(cles)
	slices.SortFunc(triees, func(a, b [32]byte) int { return bytes.Compare(a[:], b[:]) })
	var b []byte
	for _, k := range triees {
		b = append(b, k[:]...)
	}
	return b
}

func SignerRevocations(prive ed25519.PrivateKey, version uint64, cles [][32]byte) []byte {
	return ed25519.Sign(prive, messageVersionne(contexteRevocation, version, contenuRevocations(cles)))
}

func VerifierRevocations(publique ed25519.PublicKey, version uint64, cles [][32]byte, signature []byte) bool {
	return valide(publique, signature) &&
		ed25519.Verify(publique, messageVersionne(contexteRevocation, version, contenuRevocations(cles)), signature)
}

// Empreinte : de quoi comparer une clé à voix haute, ou d'un coup d'œil
// entre deux écrans. Sert pour la clé du verrou comme pour celle d'un
// appareil.
func Empreinte(cle []byte) string {
	h := sha256.Sum256(cle)
	s := base64.RawStdEncoding.EncodeToString(h[:9])
	return fmt.Sprintf("%s-%s-%s", s[0:4], s[4:8], s[8:12])
}

func LirePublique(texte string) (ed25519.PublicKey, error) {
	b, err := b64.Decoder(texte)
	if err != nil || len(b) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("clé de verrou invalide")
	}
	return ed25519.PublicKey(b), nil
}
