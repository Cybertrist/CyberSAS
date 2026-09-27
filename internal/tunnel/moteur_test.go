package tunnel

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"net"
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/Cybertrist/CyberSAS/internal/noise"
)

// --- outils ------------------------------------------------------------------

// tunFactice : une interface virtuelle en mémoire. Ce que le moteur écrit
// arrive dans sortie ; ce qu'on pousse dans entree, le moteur le lit.
type tunFactice struct {
	entree, sortie chan []byte
	ferme          chan struct{}
	once           sync.Once
}

func nouveauTun() *tunFactice {
	return &tunFactice{entree: make(chan []byte, 64), sortie: make(chan []byte, 64), ferme: make(chan struct{})}
}

func (t *tunFactice) Read(b []byte) (int, error) {
	select {
	case p := <-t.entree:
		return copy(b, p), nil
	case <-t.ferme:
		return 0, errors.New("fermé")
	}
}

func (t *tunFactice) Write(b []byte) (int, error) {
	t.sortie <- append([]byte(nil), b...)
	return len(b), nil
}

func (t *tunFactice) Close() error { t.once.Do(func() { close(t.ferme) }); return nil }

// paquet construit un paquet IPv4 minimal. Avec un port, c'est du TCP, et
// la charge suit l'en-tête TCP de vingt octets.
func paquetTCP(source, dest string, portSrc, portDst uint16, charge string) []byte {
	p := make([]byte, 40+len(charge))
	p[0] = 0x45
	binary.BigEndian.PutUint16(p[2:4], uint16(len(p)))
	p[9] = protoTCP
	s, d := netip.MustParseAddr(source).As4(), netip.MustParseAddr(dest).As4()
	copy(p[12:16], s[:])
	copy(p[16:20], d[:])
	binary.BigEndian.PutUint16(p[20:22], portSrc)
	binary.BigEndian.PutUint16(p[22:24], portDst)
	p[32] = 0x50
	copy(p[40:], charge)
	return p
}

func paquet(source, dest string, charge string) []byte {
	return paquetTCP(source, dest, 40000, 80, charge)
}

func contenu(p []byte) string { return string(p[40:]) }

func attendre(t *testing.T, c chan []byte) []byte {
	t.Helper()
	select {
	case p := <-c:
		return p
	case <-time.After(5 * time.Second):
		t.Fatal("rien n'est sorti du tunnel")
		return nil
	}
}

func rienNeSort(t *testing.T, c chan []byte) {
	t.Helper()
	select {
	case p := <-c:
		t.Fatalf("un paquet est sorti alors qu'il aurait dû être rejeté : %q", contenu(p))
	case <-time.After(500 * time.Millisecond):
	}
}

func publique(k *ecdh.PrivateKey) (r [32]byte) {
	copy(r[:], k.PublicKey().Bytes())
	return
}

func prefixe(s string) []netip.Prefix { return []netip.Prefix{netip.MustParsePrefix(s)} }

func ecouter(t *testing.T) *net.UDPConn {
	c, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

type noeud struct {
	cle    *ecdh.PrivateKey
	tun    *tunFactice
	conn   *net.UDPConn
	moteur *Moteur
}

func (n *noeud) port() netip.AddrPort { return n.conn.LocalAddr().(*net.UDPAddr).AddrPort() }

func lancer(t *testing.T, ctx context.Context, cfg Config) *noeud {
	n := &noeud{tun: nouveauTun(), conn: ecouter(t)}
	n.cle, _ = noise.GenererCle()
	cfg.Prive, cfg.Tun, cfg.Conn = n.cle, n.tun, n.conn
	n.moteur = Nouveau(cfg)
	go n.moteur.Lancer(ctx)
	return n
}

// --- un client et le serveur ---------------------------------------------------

type banc struct {
	serveur, client *noeud
	relais          *net.UDPConn
	// capture : tout ce que le client envoie passe par ce relais UDP, pour
	// pouvoir rejouer ou abîmer des paquets.
	capture chan []byte
}

func nouveauBanc(t *testing.T) *banc {
	ctx, annuler := context.WithCancel(context.Background())
	t.Cleanup(annuler)
	b := &banc{capture: make(chan []byte, 256)}
	b.serveur = lancer(t, ctx, Config{})
	b.relais = ecouter(t)
	t.Cleanup(func() { b.relais.Close() })
	b.client = &noeud{tun: nouveauTun(), conn: ecouter(t)}
	b.client.cle, _ = noise.GenererCle()

	go func() {
		buf := make([]byte, 2048)
		var client netip.AddrPort
		for {
			n, src, err := b.relais.ReadFromUDPAddrPort(buf)
			if err != nil {
				return
			}
			if src == b.serveur.port() {
				b.relais.WriteToUDPAddrPort(buf[:n], client)
				continue
			}
			client = src
			select {
			case b.capture <- append([]byte(nil), buf[:n]...):
			default:
			}
			b.relais.WriteToUDPAddrPort(buf[:n], b.serveur.port())
		}
	}()

	b.serveur.moteur.DefinirPairs([]Pair{{Publique: publique(b.client.cle), Adresses: prefixe("10.77.0.2/32"), Numero: 2, ToutEntrant: true}})
	b.client.moteur = Nouveau(Config{Prive: b.client.cle, Tun: b.client.tun, Conn: b.client.conn})
	b.client.moteur.DefinirPairs([]Pair{{Publique: publique(b.serveur.cle), Adresses: prefixe("10.77.0.1/32"),
		Point: b.relais.LocalAddr().(*net.UDPAddr).AddrPort(), ToutEntrant: true}})
	go b.client.moteur.Lancer(ctx)
	return b
}

func (b *banc) dernierPaquetDonnees(t *testing.T) []byte {
	var dernier []byte
	for {
		select {
		case p := <-b.capture:
			if p[0] == typeDonnees && len(p) > enteteDonnees+tailleTag {
				dernier = p
			}
		default:
			if dernier == nil {
				t.Fatal("aucun paquet de données capturé")
			}
			return dernier
		}
	}
}

func TestAllerRetour(t *testing.T) {
	b := nouveauBanc(t)
	b.client.tun.entree <- paquet("10.77.0.2", "10.77.0.1", "bonjour")
	if got := attendre(t, b.serveur.tun.sortie); contenu(got) != "bonjour" {
		t.Fatalf("le serveur a reçu %q", contenu(got))
	}
	b.serveur.tun.entree <- paquetTCP("10.77.0.1", "10.77.0.2", 80, 40000, "salut")
	if got := attendre(t, b.client.tun.sortie); contenu(got) != "salut" {
		t.Fatalf("le client a reçu %q", contenu(got))
	}
}

func TestRejeuRefuse(t *testing.T) {
	b := nouveauBanc(t)
	b.client.tun.entree <- paquet("10.77.0.2", "10.77.0.1", "virement")
	attendre(t, b.serveur.tun.sortie)
	capture := b.dernierPaquetDonnees(t)
	// Un attaquant renvoie le paquet capturé, depuis ailleurs.
	b.client.conn.WriteToUDPAddrPort(capture, b.serveur.port())
	rienNeSort(t, b.serveur.tun.sortie)
}

func TestPaquetModifieRefuse(t *testing.T) {
	b := nouveauBanc(t)
	b.client.tun.entree <- paquet("10.77.0.2", "10.77.0.1", "premier")
	attendre(t, b.serveur.tun.sortie)
	capture := b.dernierPaquetDonnees(t)
	// Compteur neuf, contenu abîmé : le tag ne correspond plus.
	binary.LittleEndian.PutUint64(capture[8:16], 1000)
	capture[len(capture)-20] ^= 0xff
	b.client.conn.WriteToUDPAddrPort(capture, b.serveur.port())
	rienNeSort(t, b.serveur.tun.sortie)
}

func TestSourceUsurpeeRefusee(t *testing.T) {
	b := nouveauBanc(t)
	// Le client est 10.77.0.2 : il ne peut pas parler au nom de 10.77.0.9.
	b.client.tun.entree <- paquet("10.77.0.9", "10.77.0.1", "c'est moi le 9")
	rienNeSort(t, b.serveur.tun.sortie)
	b.client.tun.entree <- paquet("10.77.0.2", "10.77.0.1", "vraiment moi")
	if got := attendre(t, b.serveur.tun.sortie); contenu(got) != "vraiment moi" {
		t.Fatalf("reçu %q", contenu(got))
	}
}

func TestInitiationRejoueeIgnoree(t *testing.T) {
	b := nouveauBanc(t)
	b.client.tun.entree <- paquet("10.77.0.2", "10.77.0.1", "a")
	attendre(t, b.serveur.tun.sortie)
	var initiation []byte
	for len(b.capture) > 0 {
		if p := <-b.capture; p[0] == typeInitiation {
			initiation = p
		}
	}
	if initiation == nil {
		t.Fatal("pas d'initiation capturée")
	}
	espion := ecouter(t)
	defer espion.Close()
	espion.WriteToUDPAddrPort(initiation, b.serveur.port())
	espion.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	if _, _, err := espion.ReadFromUDPAddrPort(make([]byte, 256)); err == nil {
		t.Fatal("le serveur a répondu à une initiation rejouée")
	}
}

func TestCleInconnueRefusee(t *testing.T) {
	b := nouveauBanc(t)
	ctx, annuler := context.WithCancel(context.Background())
	defer annuler()
	// Un intrus qui connaît la clé publique du serveur, mais n'est pas
	// inscrit.
	intrus := lancer(t, ctx, Config{})
	intrus.moteur.DefinirPairs([]Pair{{Publique: publique(b.serveur.cle), Adresses: prefixe("10.77.0.1/32"), Point: b.serveur.port()}})
	intrus.tun.entree <- paquet("10.77.0.2", "10.77.0.1", "laissez-moi entrer")
	rienNeSort(t, b.serveur.tun.sortie)
	for _, e := range intrus.moteur.Etat() {
		if !e.DernierePoignee.IsZero() {
			t.Fatal("l'intrus a obtenu une session")
		}
	}
}

func TestMac1SansLaCleDuServeur(t *testing.T) {
	b := nouveauBanc(t)
	// Sans la clé publique du serveur, impossible de produire un mac1 :
	// le serveur ne déchiffre rien, et ne répond rien.
	faux := make([]byte, tailleInitiation)
	rand.Read(faux)
	faux[0] = typeInitiation
	espion := ecouter(t)
	defer espion.Close()
	espion.WriteToUDPAddrPort(faux, b.serveur.port())
	espion.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	if _, _, err := espion.ReadFromUDPAddrPort(make([]byte, 256)); err == nil {
		t.Fatal("le serveur a répondu à une initiation sans mac1 valide")
	}
}

func TestCookieSousCharge(t *testing.T) {
	ctx, annuler := context.WithCancel(context.Background())
	defer annuler()
	// Un serveur toujours « sous charge » : chaque initiation doit porter
	// un cookie valide.
	serveur := lancer(t, ctx, Config{SeuilCharge: -1})
	client := lancer(t, ctx, Config{})
	serveur.moteur.DefinirPairs([]Pair{{Publique: publique(client.cle), Adresses: prefixe("10.77.0.2/32"), ToutEntrant: true}})
	client.moteur.DefinirPairs([]Pair{{Publique: publique(serveur.cle), Adresses: prefixe("10.77.0.1/32"), Point: serveur.port(), ToutEntrant: true}})

	limite := time.Now().Add(10 * time.Second)
	for time.Now().Before(limite) {
		client.tun.entree <- paquet("10.77.0.2", "10.77.0.1", "avec cookie")
		select {
		case p := <-serveur.tun.sortie:
			if contenu(p) == "avec cookie" {
				return
			}
		case <-time.After(time.Second):
		}
	}
	t.Fatal("le client n'a pas obtenu de session avec un cookie")
}

func TestServeurRedemarre(t *testing.T) {
	cleServeur, _ := noise.GenererCle()
	cleClient, _ := noise.GenererCle()
	cs, cc := ecouter(t), ecouter(t)
	port := cs.LocalAddr().(*net.UDPAddr).AddrPort()
	pairClient := []Pair{{Publique: publique(cleClient), Adresses: prefixe("10.77.0.2/32"), ToutEntrant: true}}

	ctxS, arretS := context.WithCancel(context.Background())
	tunS := nouveauTun()
	s := Nouveau(Config{Prive: cleServeur, Tun: tunS, Conn: cs})
	s.DefinirPairs(pairClient)
	go s.Lancer(ctxS)

	ctx, annuler := context.WithCancel(context.Background())
	defer annuler()
	tunC := nouveauTun()
	c := Nouveau(Config{Prive: cleClient, Tun: tunC, Conn: cc})
	c.sansReponseMax = time.Second
	c.DefinirPairs([]Pair{{Publique: publique(cleServeur), Adresses: prefixe("10.77.0.1/32"), Point: port, ToutEntrant: true}})
	go c.Lancer(ctx)

	tunC.entree <- paquet("10.77.0.2", "10.77.0.1", "avant")
	attendre(t, tunS.sortie)

	// Le serveur redémarre : même clé, même port, aucune session en mémoire.
	arretS()
	time.Sleep(200 * time.Millisecond)
	cs2, err := net.ListenUDP("udp4", net.UDPAddrFromAddrPort(port))
	if err != nil {
		t.Fatal(err)
	}
	tunS2 := nouveauTun()
	s2 := Nouveau(Config{Prive: cleServeur, Tun: tunS2, Conn: cs2})
	s2.DefinirPairs(pairClient)
	go s2.Lancer(ctx)

	// Le premier paquet se perd dans une session que le serveur a oubliée.
	// Le client s'en rend compte seul et rouvre une session.
	tunC.entree <- paquet("10.77.0.2", "10.77.0.1", "perdu")
	limite := time.Now().Add(10 * time.Second)
	for time.Now().Before(limite) {
		tunC.entree <- paquet("10.77.0.2", "10.77.0.1", "après")
		select {
		case p := <-tunS2.sortie:
			if contenu(p) == "après" {
				return
			}
		case <-time.After(time.Second):
		}
	}
	t.Fatal("le client ne s'est pas reconnecté au serveur redémarré")
}

// --- deux appareils, de bout en bout ------------------------------------------

// reseau : un serveur et deux appareils, A (10.77.0.2, numéro 2) et
// B (10.77.0.3, numéro 3). B accepte le TCP 80 venant de A.
type reseau struct {
	serveur, a, b *noeud
	relie         bool // le serveur relaie-t-il entre A et B ?
	mu            sync.Mutex
}

func nouveauReseau(t *testing.T, relie bool) *reseau {
	ctx, annuler := context.WithCancel(context.Background())
	t.Cleanup(annuler)
	r := &reseau{relie: relie}
	r.serveur = lancer(t, ctx, Config{Relais: func(de, vers uint32) bool {
		r.mu.Lock()
		defer r.mu.Unlock()
		return r.relie && ((de == 2 && vers == 3) || (de == 3 && vers == 2))
	}})
	r.a = lancer(t, ctx, Config{})
	r.b = lancer(t, ctx, Config{})
	r.serveur.moteur.DefinirPairs([]Pair{
		{Publique: publique(r.a.cle), Adresses: prefixe("10.77.0.2/32"), Numero: 2, ToutEntrant: true},
		{Publique: publique(r.b.cle), Adresses: prefixe("10.77.0.3/32"), Numero: 3, ToutEntrant: true},
	})
	versServeur := Pair{Publique: publique(r.serveur.cle), Adresses: prefixe("10.77.0.1/32"), Point: r.serveur.port(), ToutEntrant: true}
	r.a.moteur.DefinirPairs([]Pair{versServeur,
		// A n'accepte rien de B : les réponses passent par le suivi.
		{Publique: publique(r.b.cle), Adresses: prefixe("10.77.0.3/32"), Numero: 3, ParRelais: true}})
	r.b.moteur.DefinirPairs([]Pair{versServeur,
		{Publique: publique(r.a.cle), Adresses: prefixe("10.77.0.2/32"), Numero: 2, ParRelais: true,
			Entrant: []Regle{{Proto: protoTCP, Debut: 80, Fin: 80}}}})
	// Comme en vrai, chaque appareil a d'abord sa session avec le serveur :
	// sans elle, personne ne peut le joindre.
	for _, n := range []*noeud{r.a, r.b} {
		connecteAuServeur(t, n, publique(r.serveur.cle))
	}
	return r
}

func connecteAuServeur(t *testing.T, n *noeud, serveur [32]byte) {
	t.Helper()
	limite := time.Now().Add(5 * time.Second)
	for time.Now().Before(limite) {
		for _, e := range n.moteur.Etat() {
			if e.Publique == serveur && !e.DernierePoignee.IsZero() {
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("un appareil ne s'est pas connecté au serveur")
}

func TestBoutEnBout(t *testing.T) {
	r := nouveauReseau(t, true)
	r.a.tun.entree <- paquetTCP("10.77.0.2", "10.77.0.3", 40000, 80, "secret de A pour B")
	if got := attendre(t, r.b.tun.sortie); contenu(got) != "secret de A pour B" {
		t.Fatalf("B a reçu %q", contenu(got))
	}
	// Le serveur a relayé sans rien voir : rien n'est sorti chez lui.
	rienNeSort(t, r.serveur.tun.sortie)

	// La réponse de B entre chez A, qui n'a pourtant aucune règle pour B :
	// elle répond à un flux que A a ouvert.
	r.b.tun.entree <- paquetTCP("10.77.0.3", "10.77.0.2", 80, 40000, "réponse de B")
	if got := attendre(t, r.a.tun.sortie); contenu(got) != "réponse de B" {
		t.Fatalf("A a reçu %q", contenu(got))
	}
}

func TestFiltreDuDestinataire(t *testing.T) {
	r := nouveauReseau(t, true)
	// B n'accepte de A que le port 80 : le 22 ne passe pas.
	r.a.tun.entree <- paquetTCP("10.77.0.2", "10.77.0.3", 40001, 22, "ssh")
	rienNeSort(t, r.b.tun.sortie)
	// Et B ne peut pas ouvrir de connexion vers A, qui n'accepte rien de lui.
	r.b.tun.entree <- paquetTCP("10.77.0.3", "10.77.0.2", 40002, 80, "coucou A")
	rienNeSort(t, r.a.tun.sortie)
	// Le port 80, lui, passe.
	r.a.tun.entree <- paquetTCP("10.77.0.2", "10.77.0.3", 40003, 80, "web")
	if got := attendre(t, r.b.tun.sortie); contenu(got) != "web" {
		t.Fatalf("B a reçu %q", contenu(got))
	}
}

func TestRelaisRefuseSansRelation(t *testing.T) {
	r := nouveauReseau(t, false)
	// La politique ne relie pas A et B : le serveur ne relaie même pas la
	// poignée de main.
	r.a.tun.entree <- paquetTCP("10.77.0.2", "10.77.0.3", 40000, 80, "psst")
	rienNeSort(t, r.b.tun.sortie)
	for _, e := range r.a.moteur.Etat() {
		if e.Numero == 3 && !e.DernierePoignee.IsZero() {
			t.Fatal("A a obtenu une session avec B sans que la politique les relie")
		}
	}
}

func TestLeServeurNeLitPasCeQuIlRelaie(t *testing.T) {
	r := nouveauReseau(t, true)
	// La sonde voit exactement ce que le serveur a en clair après avoir
	// ouvert la couche transport : le message qu'il relaie.
	var mu sync.Mutex
	var vus [][]byte
	sonde := func(msg []byte) {
		mu.Lock()
		vus = append(vus, append([]byte(nil), msg...))
		mu.Unlock()
	}
	r.serveur.moteur.sondeRelais.Store(&sonde)
	r.a.tun.entree <- paquetTCP("10.77.0.2", "10.77.0.3", 40000, 80, "MARQUEUR-SECRET-123")
	if got := attendre(t, r.b.tun.sortie); contenu(got) != "MARQUEUR-SECRET-123" {
		t.Fatalf("B a reçu %q", contenu(got))
	}
	mu.Lock()
	defer mu.Unlock()
	if len(vus) == 0 {
		t.Fatal("la sonde n'a rien vu passer : le test ne prouve rien")
	}
	for _, m := range vus {
		if bytes.Contains(m, []byte("MARQUEUR")) {
			t.Fatal("le serveur a vu le contenu du paquet en clair")
		}
	}
}

// --- pièces détachées ----------------------------------------------------------

func TestFenetreAntiRejeu(t *testing.T) {
	var f fenetre
	for _, n := range []uint64{0, 1, 2, 5, 4} {
		if !f.accepter(n) {
			t.Fatalf("compteur %d refusé", n)
		}
	}
	for _, n := range []uint64{0, 2, 5} {
		if f.accepter(n) {
			t.Fatalf("compteur %d accepté deux fois", n)
		}
	}
	if !f.accepter(3) {
		t.Fatal("un compteur en retard mais jamais vu doit passer")
	}
	if !f.accepter(5000) || f.accepter(5000-tailleFenetre) {
		t.Fatal("un compteur sorti de la fenêtre doit être refusé")
	}
}

func TestTrameRelais(t *testing.T) {
	msg := []byte{typeDonnees, 0, 0, 0, 1, 2, 3}
	tr, ok := trameRelais(7, msg)
	if !ok {
		t.Fatal("trame refusée")
	}
	tr = append(tr, 0, 0, 0, 0) // remplissage de la couche transport
	n, m, ok := lireTrame(tr)
	if !ok || n != 7 || !bytes.Equal(m, msg) {
		t.Fatalf("trame mal relue : %d %v %v", n, m, ok)
	}
	tr[3] = 200 // longueur plus grande que la trame
	if _, _, ok := lireTrame(tr); ok {
		t.Fatal("une trame à la longueur mensongère a été acceptée")
	}
	// Plus long qu'un clair : refusé avant que sa longueur ne déborde des
	// deux octets de l'en-tête.
	if _, ok := trameRelais(7, make([]byte, 1<<16+10)); ok {
		t.Fatal("un message de 64 Kio a été mis en trame")
	}
}
