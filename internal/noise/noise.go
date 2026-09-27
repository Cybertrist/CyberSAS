// Package noise écrit la poignée de main du tunnel CyberSAS :
// Noise_IK_25519_ChaChaPoly_BLAKE2s, d'après la spécification du Noise
// Protocol Framework (révision 34, noiseprotocol.org).
//
// Tout l'assemblage est écrit ici. Seules les primitives viennent
// d'ailleurs, et c'est voulu : X25519 de la bibliothèque standard de Go,
// ChaCha20-Poly1305 et BLAKE2s de golang.org/x/crypto. Personne de sérieux
// n'écrit ses propres primitives. Les tests vérifient que chaque message
// produit ici est lu par une implémentation de référence, et l'inverse.
//
// IK veut dire : l'initiateur connaît d'avance la clé publique statique du
// répondeur (K), et envoie la sienne dans le premier message (I), chiffrée.
// Un seul aller-retour suffit à établir deux clés de session, une par sens.
package noise

import (
	"crypto/ecdh"
	"crypto/hmac"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"hash"

	"golang.org/x/crypto/blake2s"
	"golang.org/x/crypto/chacha20poly1305"
)

// NomProtocole est haché au départ de chaque poignée de main : deux
// implémentations qui n'utilisent pas exactement les mêmes algorithmes ne
// peuvent pas tomber d'accord par accident.
const NomProtocole = "Noise_IK_25519_ChaChaPoly_BLAKE2s"

const (
	TailleCle  = 32
	tailleTag  = chacha20poly1305.Overhead
	TailleMsg1 = TailleCle + (TailleCle + tailleTag) // e, puis s chiffrée
	TailleMsg2 = TailleCle                           // e
)

var ErrMessage = errors.New("noise : message invalide")

// GenererCle tire une paire de clés X25519.
func GenererCle() (*ecdh.PrivateKey, error) {
	return ecdh.X25519().GenerateKey(rand.Reader)
}

// ClePrivee relit une clé privée de 32 octets.
func ClePrivee(b []byte) (*ecdh.PrivateKey, error) {
	return ecdh.X25519().NewPrivateKey(b)
}

// dh refuse un point de petit ordre : crypto/ecdh renvoie une erreur si le
// secret partagé est nul, ce qui arriverait avec une clé publique piégée.
func dh(p *ecdh.PrivateKey, publique []byte) ([]byte, error) {
	k, err := ecdh.X25519().NewPublicKey(publique)
	if err != nil {
		return nil, err
	}
	return p.ECDH(k)
}

func nouveauBlake() hash.Hash {
	h, _ := blake2s.New256(nil)
	return h
}

func hmacBlake(cle []byte, donnees ...[]byte) (sortie [32]byte) {
	m := hmac.New(nouveauBlake, cle)
	for _, d := range donnees {
		m.Write(d)
	}
	copy(sortie[:], m.Sum(nil))
	return
}

// hkdf est la fonction HKDF de la section 4.3 de la spécification, avec
// deux sorties : Noise n'en demande jamais trois dans ce motif.
func hkdf(ck [32]byte, ikm []byte) (a, b [32]byte) {
	t := hmacBlake(ck[:], ikm)
	a = hmacBlake(t[:], []byte{1})
	b = hmacBlake(t[:], a[:], []byte{2})
	return
}

// Nonce ChaChaPoly de Noise : quatre octets nuls, puis le compteur sur huit
// octets en petit-boutiste.
func Nonce(n uint64) []byte {
	var b [chacha20poly1305.NonceSize]byte
	binary.LittleEndian.PutUint64(b[4:], n)
	return b[:]
}

// etatSymetrique : le SymmetricState de la spécification. ck est la clé de
// chaînage, h le hachage de tout ce qui a été échangé, k la clé courante.
type etatSymetrique struct {
	ck, h, k [32]byte
	aCle     bool
	n        uint64
}

func (s *etatSymetrique) initialiser(prologue []byte) {
	nom := []byte(NomProtocole)
	if len(nom) <= 32 {
		copy(s.h[:], nom)
	} else {
		s.h = blake2s.Sum256(nom)
	}
	s.ck = s.h
	s.melangerHachage(prologue)
}

func (s *etatSymetrique) melangerHachage(d []byte) {
	h := nouveauBlake()
	h.Write(s.h[:])
	h.Write(d)
	copy(s.h[:], h.Sum(nil))
}

func (s *etatSymetrique) melangerCle(ikm []byte) {
	s.ck, s.k = hkdf(s.ck, ikm)
	s.aCle, s.n = true, 0
}

// Chaque morceau chiffré est authentifié avec h en données associées : un
// message ne se lit que si tout ce qui le précède a été vu à l'identique.
func (s *etatSymetrique) chiffrerEtHacher(clair []byte) []byte {
	if !s.aCle {
		s.melangerHachage(clair)
		return append([]byte(nil), clair...)
	}
	a, _ := chacha20poly1305.New(s.k[:])
	c := a.Seal(nil, Nonce(s.n), clair, s.h[:])
	s.n++
	s.melangerHachage(c)
	return c
}

func (s *etatSymetrique) dechiffrerEtHacher(c []byte) ([]byte, error) {
	if !s.aCle {
		s.melangerHachage(c)
		return append([]byte(nil), c...), nil
	}
	a, _ := chacha20poly1305.New(s.k[:])
	clair, err := a.Open(nil, Nonce(s.n), c, s.h[:])
	if err != nil {
		return nil, ErrMessage
	}
	s.n++
	s.melangerHachage(c)
	return clair, nil
}

// separer donne les deux clés de transport : la première pour le sens
// initiateur vers répondeur, la seconde pour le retour.
func (s *etatSymetrique) separer() (versRepondeur, versInitiateur [32]byte) {
	return hkdf(s.ck, nil)
}

// Cles de transport d'un côté de la poignée de main.
type Cles struct {
	Envoi, Reception [32]byte
}

// Initiateur : le client, qui connaît la clé publique du serveur.
type Initiateur struct {
	es     etatSymetrique
	s, e   *ecdh.PrivateKey
	rs     []byte
	envoye bool
}

func NouvelInitiateur(statique *ecdh.PrivateKey, publiqueRepondeur, prologue []byte) *Initiateur {
	i := &Initiateur{s: statique, rs: append([]byte(nil), publiqueRepondeur...)}
	i.es.initialiser(prologue)
	i.es.melangerHachage(i.rs) // pré-message : <- s
	return i
}

// Message1 : -> e, es, s, ss, puis la charge chiffrée.
func (i *Initiateur) Message1(charge []byte) ([]byte, error) {
	if i.envoye {
		return nil, errors.New("noise : message 1 déjà envoyé")
	}
	var err error
	if i.e == nil {
		i.e, err = GenererCle()
	}
	if err != nil {
		return nil, err
	}
	e := i.e.PublicKey().Bytes()
	i.es.melangerHachage(e)
	secret, err := dh(i.e, i.rs)
	if err != nil {
		return nil, err
	}
	i.es.melangerCle(secret)
	s := i.es.chiffrerEtHacher(i.s.PublicKey().Bytes())
	if secret, err = dh(i.s, i.rs); err != nil {
		return nil, err
	}
	i.es.melangerCle(secret)
	msg := append(append(e, s...), i.es.chiffrerEtHacher(charge)...)
	i.envoye = true
	return msg, nil
}

// LireMessage2 : <- e, ee, se, puis la charge. Rend les clés de session.
func (i *Initiateur) LireMessage2(m []byte) ([]byte, Cles, error) {
	if !i.envoye || len(m) < TailleMsg2+tailleTag {
		return nil, Cles{}, ErrMessage
	}
	// On travaille sur une copie de l'état, qui ne remplace l'original
	// qu'en cas de succès. Sinon, une seule réponse forgée (il suffit de
	// connaître notre clé publique et l'indice, qui passe en clair)
	// corromprait l'état et ferait échouer la vraie réponse qui suit.
	es := i.es
	re := m[:TailleCle]
	es.melangerHachage(re)
	for _, p := range []*ecdh.PrivateKey{i.e, i.s} { // ee, puis se
		secret, err := dh(p, re)
		if err != nil {
			return nil, Cles{}, ErrMessage
		}
		es.melangerCle(secret)
	}
	charge, err := es.dechiffrerEtHacher(m[TailleCle:])
	if err != nil {
		return nil, Cles{}, err
	}
	i.es = es
	aller, retour := i.es.separer()
	return charge, Cles{Envoi: aller, Reception: retour}, nil
}

// Repondeur : le serveur. Il découvre qui lui parle en déchiffrant le
// premier message.
type Repondeur struct {
	es     etatSymetrique
	s, e   *ecdh.PrivateKey
	re, rs []byte
	lu     bool
}

func NouveauRepondeur(statique *ecdh.PrivateKey, prologue []byte) *Repondeur {
	r := &Repondeur{s: statique}
	r.es.initialiser(prologue)
	r.es.melangerHachage(statique.PublicKey().Bytes())
	return r
}

// LireMessage1 rend la clé publique statique de l'initiateur et la charge.
// C'est à l'appelant de décider si cette clé a le droit d'entrer.
func (r *Repondeur) LireMessage1(m []byte) (publique, charge []byte, err error) {
	if r.lu || len(m) < TailleMsg1+tailleTag {
		return nil, nil, ErrMessage
	}
	r.re = append([]byte(nil), m[:TailleCle]...)
	r.es.melangerHachage(r.re)
	secret, err := dh(r.s, r.re) // es
	if err != nil {
		return nil, nil, ErrMessage
	}
	r.es.melangerCle(secret)
	rs, err := r.es.dechiffrerEtHacher(m[TailleCle:TailleMsg1])
	if err != nil {
		return nil, nil, err
	}
	if secret, err = dh(r.s, rs); err != nil { // ss
		return nil, nil, ErrMessage
	}
	r.es.melangerCle(secret)
	if charge, err = r.es.dechiffrerEtHacher(m[TailleMsg1:]); err != nil {
		return nil, nil, err
	}
	r.rs, r.lu = rs, true
	return append([]byte(nil), rs...), charge, nil
}

// Message2 : <- e, ee, se, puis la charge. Rend les clés de session.
func (r *Repondeur) Message2(charge []byte) ([]byte, Cles, error) {
	if !r.lu {
		return nil, Cles{}, errors.New("noise : message 1 pas encore lu")
	}
	var err error
	if r.e == nil {
		r.e, err = GenererCle()
	}
	if err != nil {
		return nil, Cles{}, err
	}
	e := r.e.PublicKey().Bytes()
	r.es.melangerHachage(e)
	for _, publique := range [][]byte{r.re, r.rs} { // ee, puis se
		secret, err := dh(r.e, publique)
		if err != nil {
			return nil, Cles{}, ErrMessage
		}
		r.es.melangerCle(secret)
	}
	msg := append(e, r.es.chiffrerEtHacher(charge)...)
	aller, retour := r.es.separer()
	return msg, Cles{Envoi: retour, Reception: aller}, nil
}

// HachagePoignee rend h à la fin de la poignée de main : les deux côtés
// doivent obtenir la même valeur. Sert aux vecteurs de test officiels, et
// pourra lier une session à la poignée de main qui l'a créée.
func (i *Initiateur) HachagePoignee() [32]byte { return i.es.h }
func (r *Repondeur) HachagePoignee() [32]byte  { return r.es.h }
