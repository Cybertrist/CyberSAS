package protocole

import (
	"crypto/ecdh"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"hash"
	"sync"
	"time"

	"github.com/Cybertrist/CyberSAS/internal/b64"

	"golang.org/x/crypto/blake2s"
)

// Preuve de possession de la clé privée, à l'inscription.
//
// Sans elle, n'importe qui pourrait inscrire la clé publique d'un autre
// appareil (les clés publiques circulent) et s'approprier sa fiche. Une clé
// X25519 ne signe pas ; on fait donc comme Noise : un échange de clés entre
// la clé de l'appareil et celle du serveur donne un secret que seuls ces deux-
// là peuvent calculer, et l'on en tire un code sur la demande entière.
//
// Le code couvre tout ce qui compte dans la demande : les deux clés,
// l'horodatage, le justificatif, le nom et le système. On ne peut donc pas
// réutiliser la preuve d'un appareil avec son propre jeton Google pour
// inscrire la clé de l'autre à son nom.

const contextePreuve = "CyberSas inscription v2"

// FenetrePreuve : écart toléré entre l'horloge de l'appareil et celle du
// serveur. Une preuve plus vieille est refusée ; plus récente, elle n'est
// acceptée qu'une fois (voir Rejeux).
const FenetrePreuve = 5 * time.Minute

func champ(b []byte, s string) []byte {
	b = binary.BigEndian.AppendUint32(b, uint32(len(s)))
	return append(b, s...)
}

func (d DemandeConnexion) aProuver(serveur []byte) []byte {
	justificatif := sha256.Sum256([]byte(d.JetonGoogle + "\x00" + d.CleInscription))
	m := []byte(contextePreuve)
	m = champ(m, d.ClePublique)
	m = append(m, serveur...)
	m = binary.BigEndian.AppendUint64(m, uint64(d.Horodatage))
	m = append(m, justificatif[:]...)
	m = champ(m, d.Nom)
	return champ(m, d.Systeme)
}

func code(secret, message []byte) []byte {
	m := hmac.New(func() hash.Hash { h, _ := blake2s.New256(nil); return h }, secret)
	m.Write(message)
	return m.Sum(nil)
}

// Prouver remplit Preuve, côté appareil. Horodatage et le reste de la
// demande doivent être déjà remplis.
func (d *DemandeConnexion) Prouver(prive *ecdh.PrivateKey, publiqueServeur []byte) error {
	pub, err := ecdh.X25519().NewPublicKey(publiqueServeur)
	if err != nil {
		return err
	}
	secret, err := prive.ECDH(pub)
	if err != nil {
		return err
	}
	d.Preuve = base64.StdEncoding.EncodeToString(code(secret, d.aProuver(publiqueServeur)))
	return nil
}

// VerifierPreuve, côté serveur. Comparaison en temps constant.
func (d DemandeConnexion) VerifierPreuve(serveur *ecdh.PrivateKey, publiqueAppareil []byte, maintenant time.Time) bool {
	ecart := maintenant.Sub(time.Unix(d.Horodatage, 0))
	if ecart > FenetrePreuve || ecart < -FenetrePreuve {
		return false
	}
	recue, err := b64.Decoder(d.Preuve)
	if err != nil {
		return false
	}
	pub, err := ecdh.X25519().NewPublicKey(publiqueAppareil)
	if err != nil {
		return false
	}
	secret, err := serveur.ECDH(pub)
	if err != nil {
		return false
	}
	return hmac.Equal(recue, code(secret, d.aProuver(serveur.PublicKey().Bytes())))
}

// Rejeux retient les preuves des inscriptions réussies, pendant deux
// fenêtres : une demande capturée (dans un journal, par un mandataire) ne
// se rejoue pas. Une demande refusée n'est pas retenue : la refaire telle
// quelle, une fois la cause corrigée, doit rester possible.
type Rejeux struct {
	mu   sync.Mutex
	vues map[string]time.Time
}

// DejaVue dit si cette preuve a déjà servi à une inscription réussie.
func (r *Rejeux) DejaVue(preuve string, maintenant time.Time) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for p, t := range r.vues {
		if maintenant.Sub(t) > 2*FenetrePreuve {
			delete(r.vues, p)
		}
	}
	_, vue := r.vues[preuve]
	return vue
}

// Retenir : cette preuve vient de servir.
func (r *Rejeux) Retenir(preuve string, maintenant time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.vues == nil {
		r.vues = map[string]time.Time{}
	}
	r.vues[preuve] = maintenant
}
