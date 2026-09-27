package tunnel

import (
	"context"
	"encoding/binary"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/Cybertrist/CyberSAS/internal/noise"
)

// Tests de non-régression des constats de l'audit du protocole
// (docs/audit.md, section « Tunnel »). Chacun reproduit l'attaque décrite,
// et vérifie qu'elle échoue désormais.

// T1 : un pair autorisé sur le TCP 80 ne peut pas se servir de nous comme
// routeur vers notre réseau local.
func TestAuditDestinationVerifiee(t *testing.T) {
	r := nouveauReseau(t, true)
	r.b.moteur.DefinirAdresse(netip.MustParseAddr("10.77.0.3"))
	r.a.tun.entree <- paquetTCP("10.77.0.2", "192.168.1.50", 40000, 80, "vers le réseau de B")
	rienNeSort(t, r.b.tun.sortie)
	r.a.tun.entree <- paquetTCP("10.77.0.2", "10.77.0.3", 40000, 80, "vers B lui-même")
	if got := attendre(t, r.b.tun.sortie); contenu(got) != "vers B lui-même" {
		t.Fatalf("B a reçu %q", contenu(got))
	}
}

// T3 : sous charge, un /64 n'a qu'un seau, pas un par adresse.
func TestAuditSeauParReseauIPv6(t *testing.T) {
	var c charge
	passes := 0
	for i := range 1000 {
		var b [16]byte
		b[0], b[1] = 0x20, 0x01
		binary.BigEndian.PutUint64(b[8:], uint64(i)+1) // même /64, adresses différentes
		if c.autoriser(netip.AddrFrom16(b)) {
			passes++
		}
	}
	if passes > jetonsMax+1 {
		t.Fatalf("%d poignées de main autorisées depuis un seul /64, plafond %d", passes, jetonsMax)
	}
}

// T4 : les initiations relayées sont limitées par pair.
func TestAuditInitiationsRelayeesLimitees(t *testing.T) {
	var l limiteur[uint64]
	passes := 0
	for range 500 {
		if l.autoriser(42) {
			passes++
		}
	}
	if passes > jetonsMax+1 {
		t.Fatalf("%d initiations relayées acceptées d'affilée, plafond %d", passes, jetonsMax)
	}
	if !l.autoriser(43) {
		t.Fatal("un autre pair ne doit pas payer pour le premier")
	}
}

// T5 : un pair ne peut pas remplir le suivi au détriment des autres.
func TestAuditSuiviBudgetParPair(t *testing.T) {
	var s suivi
	bavard, autre := &pair{}, &pair{}
	for i := range fluxParPair + 100 {
		s.noter(ipv4{proto: protoTCP, ports: true, source: [4]byte{10, 77, 0, 2}, dest: [4]byte{10, 77, 0, 3},
			portSrc: uint16(i), portDst: 80}, bavard)
	}
	aller := ipv4{proto: protoTCP, ports: true, source: [4]byte{10, 77, 0, 2}, dest: [4]byte{10, 77, 0, 4}, portSrc: 5000, portDst: 443}
	s.noter(aller, autre)
	retour := ipv4{proto: protoTCP, ports: true, source: aller.dest, dest: aller.source, portSrc: 443, portDst: 5000}
	if !s.retour(retour, autre) {
		t.Fatal("la réponse de l'autre pair est refusée : le bavard a rempli la table")
	}
	// Et une réponse ne vaut que venant du pair à qui l'on a écrit.
	if s.retour(retour, bavard) {
		t.Fatal("un autre pair peut se faire passer pour la réponse")
	}
}

// T5 bis : répondre à ce qu'une règle laisse entrer ne consomme rien.
func TestAuditReponseAutoriseeNonRetenue(t *testing.T) {
	r := &regles{liste: []Regle{{Proto: protoTCP, Debut: 80, Fin: 80}}}
	// Nous (10.77.0.3) répondons depuis le port 80 à un pair qui l'a ouvert.
	reponse := ipv4{proto: protoTCP, ports: true, source: [4]byte{10, 77, 0, 3}, dest: [4]byte{10, 77, 0, 2}, portSrc: 80, portDst: 40000}
	if !reponseAutorisee(reponse, r) {
		t.Fatal("une réponse sur un port ouvert par règle devrait être reconnue")
	}
}

// T10 : le suivi ICMP n'accepte en retour que la réponse d'écho.
func TestAuditSuiviICMP(t *testing.T) {
	var s suivi
	p := &pair{}
	ping := ipv4{proto: protoICMP, ports: true, typeICMP: 8, source: [4]byte{10, 77, 0, 2}, dest: [4]byte{10, 77, 0, 3}, portSrc: 7, portDst: 7}
	s.noter(ping, p)
	for _, c := range []struct {
		typ uint8
		ok  bool
	}{{0, true}, {8, false}, {5, false}, {3, false}} {
		ret := ipv4{proto: protoICMP, ports: true, typeICMP: c.typ, source: ping.dest, dest: ping.source, portSrc: 7, portDst: 7}
		if s.retour(ret, p) != c.ok {
			t.Errorf("ICMP de type %d en retour : accepté=%v, attendu %v", c.typ, !c.ok, c.ok)
		}
	}
}

// T6 : un flux continu à sens unique ne provoque pas de poignées de main
// à répétition : le maintien passif de l'autre bout part à temps.
func TestAuditFluxSensUnique(t *testing.T) {
	ctx, annuler := context.WithCancel(context.Background())
	defer annuler()
	cleS, _ := noise.GenererCle()
	cleC, _ := noise.GenererCle()
	cs, cc := ecouter(t), ecouter(t)
	relais := ecouter(t)
	defer relais.Close()
	initiations := make(chan struct{}, 100)
	go func() {
		buf := make([]byte, 2048)
		var client netip.AddrPort
		serveur := cs.LocalAddr().(*net.UDPAddr).AddrPort()
		for {
			n, src, err := relais.ReadFromUDPAddrPort(buf)
			if err != nil {
				return
			}
			if src == serveur {
				relais.WriteToUDPAddrPort(buf[:n], client)
				continue
			}
			client = src
			if buf[0] == typeInitiation {
				initiations <- struct{}{}
			}
			relais.WriteToUDPAddrPort(buf[:n], serveur)
		}
	}()
	s := Nouveau(Config{Prive: cleS, Tun: nouveauTun(), Conn: cs})
	s.DefinirPairs([]Pair{{Publique: publique(cleC), Adresses: prefixe("10.77.0.2/32"), ToutEntrant: true}})
	go s.Lancer(ctx)
	tunC := nouveauTun()
	c := Nouveau(Config{Prive: cleC, Tun: tunC, Conn: cc})
	c.sansReponseMax = 12 * time.Second // au-delà du maintien passif (10 s)
	c.DefinirPairs([]Pair{{Publique: publique(cleS), Adresses: prefixe("10.77.0.1/32"),
		Point: relais.LocalAddr().(*net.UDPAddr).AddrPort(), ToutEntrant: true}})
	go c.Lancer(ctx)

	// Seize secondes d'envoi continu, sans rien en retour que les maintiens.
	fin := time.Now().Add(16 * time.Second)
	for time.Now().Before(fin) {
		tunC.entree <- paquet("10.77.0.2", "10.77.0.1", "journal")
		time.Sleep(100 * time.Millisecond)
	}
	if n := len(initiations); n > 1 {
		t.Fatalf("%d poignées de main pendant un flux à sens unique, une seule attendue", n)
	}
}

// T7 : après un abandon, les tentatives repartent de zéro.
func TestAuditDebutTentativesRemis(t *testing.T) {
	cle, _ := noise.GenererCle()
	autre, _ := noise.GenererCle()
	m := Nouveau(Config{Prive: cle, Tun: nouveauTun(), Conn: ecouter(t)})
	m.DefinirPairs([]Pair{{Publique: publique(autre), Adresses: prefixe("10.77.0.3/32"), Numero: 3, ParRelais: true}})
	p := m.listePairs()[0]
	p.mu.Lock()
	p.attente = [][]byte{{1}}
	p.debutTentatives = time.Now().Add(-2 * abandonnerApres)
	p.mu.Unlock()
	m.minuterie()
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.debutTentatives.IsZero() || p.attente != nil {
		t.Fatal("après un abandon, la série de tentatives doit repartir de zéro")
	}
}

// T8 et T9 : un pair retiré ne se réinscrit pas dans les indices, et un
// indice n'est jamais nul ni déjà pris.
func TestAuditIndices(t *testing.T) {
	cle, _ := noise.GenererCle()
	autre, _ := noise.GenererCle()
	m := Nouveau(Config{Prive: cle, Tun: nouveauTun()})
	m.DefinirPairs([]Pair{{Publique: publique(autre), Adresses: prefixe("10.77.0.3/32")}})
	p := m.listePairs()[0]
	vus := map[uint32]bool{}
	m.mu.Lock()
	for range 1000 {
		i, ok := m.reserverIndice(p)
		if !ok || i == 0 || vus[i] {
			t.Fatalf("indice %d invalide ou déjà pris", i)
		}
		vus[i] = true
	}
	m.mu.Unlock()

	m.DefinirPairs(nil) // retrait
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.reserverIndice(p); ok {
		t.Fatal("un pair retiré a pu reprendre un indice")
	}
}
