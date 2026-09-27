package tunnel

import (
	"net/netip"
	"testing"
	"time"

	"github.com/Cybertrist/CyberSAS/internal/noise"
)

// Fuzzing : Go génère des millions d'entrées, en partant des exemples
// donnés et en les mutant. Le but n'est pas de vérifier une réponse, mais
// qu'aucune entrée, aussi tordue soit-elle, ne fasse planter le moteur ni
// ne passe les contrôles par accident.
//
//	go test -fuzz=FuzzRecevoir -fuzztime=60s ./internal/tunnel

func FuzzLireIPv4(f *testing.F) {
	f.Add(paquet("10.77.0.2", "10.77.0.3", "x"))
	f.Add([]byte{0x45, 0, 0, 20})
	f.Add([]byte{0x4f, 0, 0xff, 0xff, 0, 0, 0x20, 0})
	f.Fuzz(func(t *testing.T, b []byte) {
		ip, ok := lireIPv4(b)
		if ok && (ip.longueur > len(b) || ip.longueur < 20) {
			t.Fatalf("longueur %d acceptée pour un paquet de %d octets", ip.longueur, len(b))
		}
	})
}

func FuzzLireTrame(f *testing.F) {
	graine, _ := trameRelais(3, []byte{typeDonnees, 0, 0, 0, 1})
	f.Add(graine)
	f.Add([]byte{0, 0, 0xff, 0xff, 1, 0, 0, 0})
	f.Fuzz(func(t *testing.T, b []byte) {
		_, msg, ok := lireTrame(b)
		if ok && len(msg) > len(b)-enteteRelais {
			t.Fatal("message plus long que la trame")
		}
	})
}

func FuzzFenetre(f *testing.F) {
	f.Add(uint64(0), uint64(1), uint64(2))
	f.Fuzz(func(t *testing.T, a, b, c uint64) {
		var fe fenetre
		for _, n := range []uint64{a, b, c} {
			premier := fe.accepter(n)
			if premier && fe.accepter(n) {
				t.Fatalf("compteur %d accepté deux fois", n)
			}
		}
	})
}

// FuzzRecevoir envoie n'importe quoi au moteur d'un serveur qui a un pair
// inscrit. Rien ne doit sortir sur l'interface : sans les clés, aucun
// message ne peut produire un paquet.
func FuzzRecevoir(f *testing.F) {
	cleServeur, _ := noise.GenererCle()
	cleClient, _ := noise.GenererCle()
	tun := nouveauTun()
	m := Nouveau(Config{Prive: cleServeur, Tun: tun, Conn: nil})
	m.DefinirPairs([]Pair{{Publique: publique(cleClient), Adresses: prefixe("10.77.0.2/32"), Numero: 2, ToutEntrant: true}})

	// Des messages bien formés en graine, pour que le fuzzeur parte de
	// structures plausibles.
	ini := noise.NouvelInitiateur(cleClient, cleServeur.PublicKey().Bytes(), Prologue)
	corps, _ := ini.Message1(horodatage(time.Now()))
	init := append(append([]byte{typeInitiation, 0, 0, 0, 1, 0, 0, 0}, corps...), make([]byte, 2*tailleMac)...)
	signerMacs(init, deriverClesMac(cleServeur.PublicKey().Bytes()), nil)
	f.Add(init)
	f.Add(make([]byte, tailleReponse))
	f.Add(make([]byte, tailleCookie))
	f.Add(append([]byte{typeDonnees, 0, 0, 0}, make([]byte, 60)...))

	src := netip.MustParseAddrPort("192.0.2.1:4000")
	f.Fuzz(func(t *testing.T, msg []byte) {
		// On n'expédie rien (pas de connexion) : seul compte ce qui arrive
		// à l'interface.
		m.traiter(msg, origine{point: src})
		select {
		case p := <-tun.sortie:
			t.Fatalf("un message forgé a produit un paquet : %x", p)
		default:
		}
	})
}
