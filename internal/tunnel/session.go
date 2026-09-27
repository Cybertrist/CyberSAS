package tunnel

import (
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Cybertrist/CyberSAS/internal/noise"
	"golang.org/x/crypto/chacha20poly1305"
)

// session : une paire de clés issue d'une poignée de main.
type session struct {
	locale, distante uint32 // nos indices : le nôtre, celui de l'autre
	envoi, reception cipher.AEAD
	compteur         atomic.Uint64
	rejeu            fenetre
	creee            time.Time
	initiateur       bool
}

func nouvelleSession(locale, distante uint32, cles noise.Cles, initiateur bool) *session {
	e, _ := chacha20poly1305.New(cles.Envoi[:])
	r, _ := chacha20poly1305.New(cles.Reception[:])
	return &session{locale: locale, distante: distante, envoi: e, reception: r, creee: time.Now(), initiateur: initiateur}
}

// utilisable : une session trop vieille ou trop usée ne chiffre plus rien.
func (s *session) utilisable() bool {
	return s != nil && time.Since(s.creee) < rejeterApres && s.compteur.Load() < rejeterApresN
}

// aRenouveler : le client prend les devants avant que la session expire.
func (s *session) aRenouveler() bool {
	return s == nil || (s.initiateur && (time.Since(s.creee) >= renouvelerApres || s.compteur.Load() >= renouvelerApresN))
}

// chiffrer produit un message de données. Le clair est complété par des
// zéros jusqu'à un multiple de 16 octets : la taille exacte des paquets
// en dit moins long sur leur contenu.
func (s *session) chiffrer(clair []byte) []byte {
	n := s.compteur.Add(1) - 1
	remplissage := (16 - len(clair)%16) % 16
	if len(clair)+remplissage > tailleMaxClair {
		remplissage = 0
	}
	msg := make([]byte, enteteDonnees, enteteDonnees+len(clair)+remplissage+tailleTag)
	copy(msg, entete(typeDonnees))
	binary.LittleEndian.PutUint32(msg[4:8], s.distante)
	binary.LittleEndian.PutUint64(msg[8:16], n)
	bourre := append(append([]byte(nil), clair...), make([]byte, remplissage)...)
	return s.envoi.Seal(msg, noise.Nonce(n), bourre, nil)
}

// dechiffrer n'accepte un compteur qu'après avoir vérifié le tag : un
// paquet forgé ne peut pas faire avancer la fenêtre anti-rejeu.
func (s *session) dechiffrer(msg []byte) ([]byte, bool) {
	if len(msg) < enteteDonnees+tailleTag {
		return nil, false
	}
	n := binary.LittleEndian.Uint64(msg[8:16])
	if !s.rejeu.possible(n) {
		return nil, false
	}
	clair, err := s.reception.Open(nil, noise.Nonce(n), msg[enteteDonnees:], nil)
	if err != nil || !s.rejeu.accepter(n) {
		return nil, false
	}
	return clair, true
}

// fenetre anti-rejeu : les 2048 derniers compteurs, un bit chacun.
const tailleFenetre = 2048

type fenetre struct {
	mu   sync.Mutex
	max  uint64
	vu   bool
	bits [tailleFenetre / 64]uint64
}

func (f *fenetre) bit(n uint64) (int, uint64) {
	i := n % tailleFenetre
	return int(i / 64), uint64(1) << (i % 64)
}

func (f *fenetre) possible(n uint64) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if n >= rejeterApresN {
		return false
	}
	if !f.vu || n > f.max {
		return true
	}
	if f.max-n >= tailleFenetre {
		return false
	}
	m, b := f.bit(n)
	return f.bits[m]&b == 0
}

func (f *fenetre) accepter(n uint64) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if n >= rejeterApresN {
		return false
	}
	if !f.vu || n > f.max {
		if !f.vu || n-f.max >= tailleFenetre {
			f.bits = [tailleFenetre / 64]uint64{}
		} else {
			for i := f.max + 1; i < n; i++ {
				m, b := f.bit(i)
				f.bits[m] &^= b
			}
		}
		f.max, f.vu = n, true
		m, b := f.bit(n)
		f.bits[m] |= b
		return true
	}
	if f.max-n >= tailleFenetre {
		return false
	}
	m, b := f.bit(n)
	if f.bits[m]&b != 0 {
		return false
	}
	f.bits[m] |= b
	return true
}

func indiceAleatoire() uint32 {
	var b [4]byte
	rand.Read(b[:])
	return binary.LittleEndian.Uint32(b[:])
}
