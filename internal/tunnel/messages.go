// Package tunnel est le moteur du VPN CyberSAS : il lit les paquets IP de
// l'interface virtuelle, les chiffre, les envoie en UDP, et fait l'inverse
// à l'arrivée. Le protocole est décrit en détail dans docs/protocole.md ;
// en bref :
//
//   - Chaque appareil a une session avec le serveur (la couche transport),
//     et une session avec chaque appareil qu'il a le droit de joindre (la
//     couche de bout en bout). Toutes sont des poignées de main Noise IK.
//   - Un paquet d'un appareil à un autre est chiffré pour le destinataire,
//     puis glissé dans une trame de relais, elle-même chiffrée pour le
//     serveur. Le serveur ouvre la trame, voit à qui la remettre, mais ne
//     peut pas lire le paquet : il n'a pas la clé.
//   - Chaque paquet de données porte un compteur, qui sert de nonce. Un
//     compteur déjà vu, ou trop ancien, est rejeté.
//   - Un paquet ne sort du tunnel que si son adresse source appartient à
//     l'appareil qui l'a chiffré, et si le filtre de l'appareil qui le
//     reçoit l'autorise.
//   - Les poignées de main portent deux codes d'authentification bon
//     marché (mac1, mac2). Un inconnu ne peut pas faire calculer au serveur
//     un échange de clés, et sous charge le serveur exige la preuve que
//     l'expéditeur possède bien son adresse IP (le cookie).
package tunnel

import (
	"encoding/binary"
	"time"

	"github.com/Cybertrist/CyberSAS/internal/noise"
)

// Prologue : haché au départ de chaque poignée de main. Une autre version
// du protocole ne pourra pas s'entendre avec celle-ci par erreur.
var Prologue = []byte("CyberSas tunnel v2")

const (
	typeInitiation = 1
	typeReponse    = 2
	typeDonnees    = 3
	typeCookie     = 4

	tailleHorodatage = 12
	tailleTag        = 16
	tailleMac        = 16

	// type (1) + réservé (3) + indice de l'émetteur (4) + Noise + mac1 + mac2
	tailleInitiation = 8 + noise.TailleMsg1 + tailleHorodatage + tailleTag + 2*tailleMac
	// type + réservé + émetteur (4) + destinataire (4) + Noise + mac1 + mac2
	tailleReponse = 12 + noise.TailleMsg2 + tailleTag + 2*tailleMac
	// type + réservé + destinataire (4) + nonce (24) + cookie chiffré (16 + 16)
	tailleCookie = 8 + 24 + tailleMac + tailleTag
	// type + réservé + destinataire (4) + compteur (8)
	enteteDonnees = 16
	// Trame de relais, à l'intérieur d'un paquet de données chiffré :
	// zéro (1) + réservé (1) + longueur du message (2, grand-boutiste) +
	// numéro d'appareil (4), puis un message complet du protocole, chiffré
	// de bout en bout.
	enteteRelais = 8

	// MTU de l'interface. Le pire cas est un paquet relayé : IPv6 (40) +
	// UDP (8) + transport (16 + 16 de tag) + remplissage (jusqu'à 8) +
	// relais (8) + données de bout en bout (16 + 16) doit tenir dans 1500.
	// 1360 laisse cette marge, en multiple de 16.
	MTU = 1360
	// Le plus gros clair jamais chiffré : un paquet relayé, remplissage
	// compris.
	tailleMaxClair = MTU + 48
)

const (
	renouvelerApres  = 120 * time.Second
	rejeterApres     = 180 * time.Second
	relancerApres    = 5 * time.Second
	abandonnerApres  = 90 * time.Second
	renouvelerApresN = uint64(1) << 60
	rejeterApresN    = ^uint64(0) - (1 << 13)
	maxEnAttente     = 32
	// Un pair qui nous a envoyé des données, à qui l'on n'a rien renvoyé
	// depuis ce délai, reçoit un paquet vide : il sait que la liaison vit.
	maintienPassif = 10 * time.Second
	// Durée de vie d'un cookie reçu, et de la clé qui les fabrique.
	dureeCookie = 120 * time.Second
)

// Des données parties sans rien en retour depuis ce délai : la session est
// sans doute morte de l'autre côté (serveur redémarré), on en rouvre une
// sans attendre le renouvellement. Chaque moteur en garde sa copie, que
// les tests abrègent.
const sansReponseDefaut = 15 * time.Second

func entete(t byte) []byte {
	return []byte{t, 0, 0, 0}
}

// horodatage TAI64N : secondes depuis 1970 décalées de 2^62, puis les
// nanosecondes. Il croît avec le temps et se compare octet par octet : une
// initiation qui n'est pas plus récente que la dernière vue est un rejeu.
func horodatage(t time.Time) []byte {
	var b [tailleHorodatage]byte
	binary.BigEndian.PutUint64(b[:8], uint64(t.Unix())+(1<<62))
	// Arrondi à 2^24 ns (environ 17 ms), comme WireGuard : la précision
	// complète renseignerait trop finement sur l'horloge de l'appareil.
	binary.BigEndian.PutUint32(b[8:], uint32(t.Nanosecond())&^0xffffff)
	return b[:]
}

const (
	protoICMP = 1
	protoTCP  = 6
	protoUDP  = 17
)

// ipv4 : ce que le moteur lit d'un paquet, pour le router et le filtrer.
type ipv4 struct {
	longueur int
	proto    uint8
	fragment bool // fragment qui n'est pas le premier : pas d'en-tête de transport
	ports    bool // portSrc et portDst sont valides
	source   [4]byte
	dest     [4]byte
	portSrc  uint16
	portDst  uint16
	typeICMP uint8
}

func lireIPv4(p []byte) (ipv4, bool) {
	if len(p) < 20 || p[0]>>4 != 4 {
		return ipv4{}, false
	}
	var r ipv4
	ihl := int(p[0]&0x0f) * 4
	r.longueur = int(binary.BigEndian.Uint16(p[2:4]))
	if ihl < 20 || r.longueur < ihl || r.longueur > len(p) {
		return ipv4{}, false
	}
	r.proto = p[9]
	r.fragment = binary.BigEndian.Uint16(p[6:8])&0x1fff != 0
	copy(r.source[:], p[12:16])
	copy(r.dest[:], p[16:20])
	if r.fragment {
		return r, true
	}
	l4 := p[ihl:r.longueur]
	switch r.proto {
	case protoTCP, protoUDP:
		if len(l4) >= 4 {
			r.portSrc = binary.BigEndian.Uint16(l4[0:2])
			r.portDst = binary.BigEndian.Uint16(l4[2:4])
			r.ports = true
		}
	case protoICMP:
		// Pour un écho, l'identifiant tient lieu de port des deux côtés :
		// la réponse porte le même que la demande.
		if len(l4) >= 8 {
			r.typeICMP = l4[0]
			id := binary.BigEndian.Uint16(l4[4:6])
			r.portSrc, r.portDst, r.ports = id, id, true
		}
	}
	return r, true
}
