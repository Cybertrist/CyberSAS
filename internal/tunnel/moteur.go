package tunnel

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"encoding/binary"
	"log/slog"
	"net"
	"net/netip"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Cybertrist/CyberSAS/internal/noise"
)

// Tun : l'interface réseau virtuelle, qui rend et prend des paquets IP
// bruts. Sous Linux, /dev/net/tun ; sous Android, le descripteur donné par
// VpnService ; dans les tests, une simple file.
type Tun interface {
	Read([]byte) (int, error)
	Write([]byte) (int, error)
	Close() error
}

// Pair : un bout de tunnel avec qui l'on échange.
type Pair struct {
	Publique [32]byte
	// Adresses que ce pair a le droit d'utiliser comme source, et vers
	// lesquelles on lui envoie les paquets.
	Adresses []netip.Prefix
	// Point, s'il est donné, se joint en direct en UDP : c'est le serveur,
	// vu d'un client. On initie les poignées de main vers lui.
	Point netip.AddrPort
	// Numero : numéro d'appareil attribué par le serveur. Sert d'adresse
	// dans les trames de relais.
	Numero uint32
	// ParRelais : ce pair est un autre appareil, joint à travers le
	// serveur. On initie aussi vers lui.
	ParRelais bool
	// Maintien : un paquet vide à cet intervalle garde ouverte la
	// traduction d'adresse de la box. Zéro pour ne rien envoyer.
	Maintien time.Duration
	// Ce que ce pair peut ouvrir chez nous. ToutEntrant supprime le filtre :
	// sur le serveur, c'est le pare-feu du noyau qui filtre.
	ToutEntrant bool
	Entrant     []Regle
}

type Config struct {
	Prive   *ecdh.PrivateKey
	Tun     Tun
	Conn    *net.UDPConn
	Journal *slog.Logger
	// Relais, côté serveur : ce moteur relaie les trames d'un appareil à un
	// autre si cette fonction l'autorise. Nil chez un client.
	Relais func(de, vers uint32) bool
	// SeuilCharge : poignées de main par seconde au-delà desquelles le
	// moteur exige un cookie. Zéro vaut 200.
	SeuilCharge int
	// Adresse : la nôtre dans le VPN. Un paquet reçu qui ne lui est pas
	// destiné est refusé : sans cela, un pair autorisé sur un port pourrait
	// nous faire relayer ses paquets vers notre réseau local.
	Adresse netip.Addr
}

type EtatPair struct {
	Publique        [32]byte
	Numero          uint32
	Point           netip.AddrPort
	DernierePoignee time.Time
	Recus, Envoyes  uint64
}

type pair struct {
	pub      [32]byte
	cles     clesMac // pour signer ce qu'on lui envoie
	adresses atomic.Pointer[[]netip.Prefix]
	entrant  atomic.Pointer[regles]
	numero   atomic.Uint32
	// retire : ce pair a quitté la liste. Protégé par Moteur.mu : un pair
	// lu juste avant son retrait ne doit pas se réinscrire dans les indices.
	retire bool

	mu                             sync.Mutex
	point                          netip.AddrPort
	direct, relais                 atomic.Bool // on initie vers lui, en direct ou par relais
	maintien                       time.Duration
	courante, precedente, suivante *session
	poignee                        *noise.Initiateur
	indicePoignee                  uint32
	derniereTentative              time.Time
	debutTentatives                time.Time
	dernierMac1                    [tailleMac]byte
	cookie                         [tailleMac]byte
	cookieRecu                     time.Time
	dernierHorodatage              []byte
	dernierEnvoi, dernierePoignee  time.Time
	derniereReception              time.Time // dernier paquet authentifié
	recuSansRepondre               time.Time // première donnée reçue depuis notre dernier envoi
	sansReponseDepuis              time.Time
	recus, envoyes                 uint64
	attente                        [][]byte
}

func (p *pair) actif() bool { return p.direct.Load() || p.relais.Load() }

func (p *pair) autorise(a netip.Addr) bool {
	for _, pr := range *p.adresses.Load() {
		if pr.Contains(a) {
			return true
		}
	}
	return false
}

// envoi : un message prêt à partir. Tout est préparé sous verrou, puis
// expédié une fois les verrous rendus : envoyer par relais demande le
// verrou d'un autre pair, et l'on ne tient jamais deux verrous de pair.
type envoi struct {
	msg    []byte
	point  netip.AddrPort // en direct
	relais uint32         // ou par le serveur, vers ce numéro
}

// origine : d'où vient un message reçu.
type origine struct {
	point  netip.AddrPort // en direct
	relais bool           // ou relayé par le serveur,
	numero uint32         // depuis cet appareil
}

// Moteur : un bout du tunnel. Le serveur en a un avec un pair par appareil ;
// un client en a un avec le serveur, plus un pair par appareil qu'il a le
// droit de joindre ou qui a le droit de le joindre.
type Moteur struct {
	prive   atomic.Pointer[ecdh.PrivateKey]
	nosCles atomic.Pointer[clesMac] // pour vérifier ce qu'on reçoit
	tun     Tun
	conn    *net.UDPConn
	journal *slog.Logger
	relais  func(de, vers uint32) bool

	// Ordre des verrous : pair.mu, puis Moteur.mu. Jamais deux pair.mu.
	mu        sync.RWMutex
	pairs     map[[32]byte]*pair
	indices   map[uint32]*pair
	parNumero map[uint32]*pair
	serveur   *pair // le pair direct, chez un client

	cookies        fabrique
	charge         charge
	suivi          suivi
	sansReponseMax time.Duration
	// sondeRelais, pour les tests : reçoit chaque message relayé, tel que
	// le serveur l'a en main.
	sondeRelais atomic.Pointer[func([]byte)]

	adresse atomic.Pointer[netip.Addr]
	// Poignées de main relayées : un seau par pair (chez un client) ou par
	// couple d'appareils (chez le serveur). Elles ne passent pas par les
	// cookies, qui ne protègent que le chemin direct.
	limiteRelais limiteur[uint64]
}

// DefinirAdresse change notre adresse dans le VPN, après une inscription.
func (m *Moteur) DefinirAdresse(a netip.Addr) { m.adresse.Store(&a) }

// reserverIndice tire un indice libre et non nul, et le lie à ce pair,
// sauf s'il vient d'être retiré. Appelée avec m.mu tenu.
func (m *Moteur) reserverIndice(p *pair) (uint32, bool) {
	if p.retire {
		return 0, false
	}
	for {
		i := indiceAleatoire()
		if _, pris := m.indices[i]; i != 0 && !pris {
			m.indices[i] = p
			return i, true
		}
	}
}

func Nouveau(c Config) *Moteur {
	if c.Journal == nil {
		c.Journal = slog.New(slog.DiscardHandler)
	}
	if c.SeuilCharge == 0 {
		c.SeuilCharge = 200
	}
	m := &Moteur{tun: c.Tun, conn: c.Conn, journal: c.Journal, relais: c.Relais,
		pairs: map[[32]byte]*pair{}, indices: map[uint32]*pair{}, parNumero: map[uint32]*pair{},
		sansReponseMax: sansReponseDefaut}
	m.charge.seuil = c.SeuilCharge
	if c.Adresse.IsValid() {
		m.DefinirAdresse(c.Adresse)
	}
	m.DefinirCle(c.Prive)
	return m
}

// DefinirCle change la clé privée de ce bout du tunnel, après une nouvelle
// inscription. Toutes les sessions établies avec l'ancienne tombent : il
// faut redonner les pairs ensuite.
func (m *Moteur) DefinirCle(prive *ecdh.PrivateKey) {
	cles := deriverClesMac(prive.PublicKey().Bytes())
	m.mu.Lock()
	m.prive.Store(prive)
	m.nosCles.Store(&cles)
	for _, p := range m.pairs {
		p.retire = true
	}
	m.pairs = map[[32]byte]*pair{}
	m.indices = map[uint32]*pair{}
	m.parNumero = map[uint32]*pair{}
	m.serveur = nil
	m.mu.Unlock()
}

// DefinirPairs remplace la liste des pairs. Ceux qui disparaissent perdent
// leurs sessions sur-le-champ : un appareil retiré est coupé.
func (m *Moteur) DefinirPairs(liste []Pair) {
	type maj struct {
		p *pair
		c Pair
	}
	var majs []maj
	m.mu.Lock()
	vus := map[[32]byte]bool{}
	m.parNumero = map[uint32]*pair{}
	m.serveur = nil
	for _, c := range liste {
		p := m.pairs[c.Publique]
		if p == nil {
			p = &pair{pub: c.Publique, cles: deriverClesMac(c.Publique[:])}
			m.pairs[c.Publique] = p
		}
		a := slices.Clone(c.Adresses)
		p.adresses.Store(&a)
		p.entrant.Store(&regles{toutes: c.ToutEntrant, liste: slices.Clone(c.Entrant)})
		p.numero.Store(c.Numero)
		if c.Numero != 0 {
			m.parNumero[c.Numero] = p
		}
		if c.Point.IsValid() {
			m.serveur = p
		}
		vus[c.Publique] = true
		majs = append(majs, maj{p, c})
	}
	for k, p := range m.pairs {
		if !vus[k] {
			p.retire = true
			delete(m.pairs, k)
			for i, q := range m.indices {
				if q == p {
					delete(m.indices, i)
				}
			}
		}
	}
	m.mu.Unlock()
	for _, x := range majs {
		x.p.mu.Lock()
		if x.c.Point.IsValid() {
			x.p.point = x.c.Point
		}
		x.p.direct.Store(x.c.Point.IsValid())
		x.p.relais.Store(x.c.ParRelais)
		x.p.maintien = x.c.Maintien
		x.p.mu.Unlock()
	}
}

func (m *Moteur) Etat() []EtatPair {
	var r []EtatPair
	for _, p := range m.listePairs() {
		p.mu.Lock()
		r = append(r, EtatPair{p.pub, p.numero.Load(), p.point, p.dernierePoignee, p.recus, p.envoyes})
		p.mu.Unlock()
	}
	return r
}

func (m *Moteur) listePairs() []*pair {
	m.mu.RLock()
	defer m.mu.RUnlock()
	l := make([]*pair, 0, len(m.pairs))
	for _, p := range m.pairs {
		l = append(l, p)
	}
	return l
}

// Lancer fait tourner le moteur jusqu'à l'annulation du contexte.
func (m *Moteur) Lancer(ctx context.Context) error {
	erreurs := make(chan error, 2)
	go func() { erreurs <- m.boucleTun() }()
	go func() { erreurs <- m.boucleUDP() }()
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			m.conn.Close()
			m.tun.Close()
			return nil
		case err := <-erreurs:
			return err
		case <-t.C:
			m.minuterie()
		}
	}
}

// --- expédition ----------------------------------------------------------------

func (m *Moteur) expedier(envois []envoi) {
	for _, e := range envois {
		switch {
		case e.relais != 0:
			m.relayer(e.relais, e.msg)
		case e.point.IsValid():
			m.conn.WriteToUDPAddrPort(e.msg, e.point)
		}
	}
}

// trameRelais : zéro, réservé, longueur du message (2 octets, grand-
// boutiste), numéro d'appareil (4 octets), puis le message. La longueur
// permet d'écarter le remplissage ajouté par la couche transport.
//
// Un message trop long pour tenir dans un clair est refusé ici, plutôt que
// de laisser sa longueur déborder des deux octets : ok vaut alors false.
func trameRelais(numero uint32, msg []byte) (trame []byte, ok bool) {
	if len(msg) > tailleMaxClair-enteteRelais {
		return nil, false
	}
	t := make([]byte, enteteRelais, enteteRelais+len(msg))
	binary.BigEndian.PutUint16(t[2:4], uint16(len(msg))) // #nosec G115 -- borné juste au-dessus
	binary.LittleEndian.PutUint32(t[4:8], numero)
	return append(t, msg...), true
}

func lireTrame(t []byte) (numero uint32, msg []byte, ok bool) {
	if len(t) < enteteRelais || t[0] != 0 {
		return 0, nil, false
	}
	n := int(binary.BigEndian.Uint16(t[2:4]))
	if n > len(t)-enteteRelais {
		return 0, nil, false
	}
	return binary.LittleEndian.Uint32(t[4:8]), t[enteteRelais : enteteRelais+n], true
}

// relayer, côté client : remet au serveur un message destiné à un autre
// appareil. Le message est déjà chiffré de bout en bout ; la trame, elle,
// est chiffrée pour le serveur.
func (m *Moteur) relayer(numero uint32, msg []byte) {
	m.mu.RLock()
	s := m.serveur
	m.mu.RUnlock()
	t, ok := trameRelais(numero, msg)
	if s == nil || !ok {
		return
	}
	m.expedier(m.envoyerClair(s, t))
}

// envoyerClair chiffre un clair pour ce pair, avec la session courante.
// Sans session, le clair attend et une poignée de main part.
func (m *Moteur) envoyerClair(p *pair, clair []byte) []envoi {
	p.mu.Lock()
	defer p.mu.Unlock()
	var envois []envoi
	s := p.courante
	if !s.utilisable() {
		// Le serveur n'initie jamais : sans session, le paquet est perdu,
		// et l'appareil en rouvrira une de lui-même.
		if p.actif() && len(p.attente) < maxEnAttente {
			p.attente = append(p.attente, clair)
		}
		return m.lancerPoignee(p)
	}
	if s.aRenouveler() {
		envois = m.lancerPoignee(p)
	}
	p.envoyes++
	p.dernierEnvoi = time.Now()
	p.recuSansRepondre = time.Time{}
	if p.actif() && len(clair) > 0 && p.sansReponseDepuis.IsZero() {
		p.sansReponseDepuis = p.dernierEnvoi
	}
	return append(envois, p.versLui(s.chiffrer(clair)))
}

// versLui : un envoi vers ce pair, en direct ou par relais. Appelée avec
// p.mu tenu.
func (p *pair) versLui(msg []byte) envoi {
	if p.relais.Load() {
		return envoi{msg: msg, relais: p.numero.Load()}
	}
	return envoi{msg: msg, point: p.point}
}

// lancerPoignee prépare une initiation, au plus toutes les cinq secondes.
// Appelée avec p.mu tenu.
func (m *Moteur) lancerPoignee(p *pair) []envoi {
	if !p.actif() || (p.direct.Load() && !p.point.IsValid()) {
		return nil
	}
	now := time.Now()
	if p.poignee != nil && now.Sub(p.derniereTentative) < relancerApres {
		return nil
	}
	if p.debutTentatives.IsZero() {
		p.debutTentatives = now
	}
	ini := noise.NouvelInitiateur(m.prive.Load(), p.pub[:], Prologue)
	corps, err := ini.Message1(horodatage(now))
	if err != nil {
		m.journal.Warn("initiation impossible", "erreur", err)
		return nil
	}
	m.mu.Lock()
	if p.indicePoignee != 0 {
		delete(m.indices, p.indicePoignee)
	}
	idx, ok := m.reserverIndice(p)
	m.mu.Unlock()
	if !ok {
		return nil
	}
	p.poignee, p.indicePoignee, p.derniereTentative = ini, idx, now

	msg := binary.LittleEndian.AppendUint32(entete(typeInitiation), idx)
	msg = append(append(msg, corps...), make([]byte, 2*tailleMac)...)
	var cookie *[tailleMac]byte
	if p.direct.Load() && !p.cookieRecu.IsZero() && now.Sub(p.cookieRecu) < dureeCookie {
		cookie = &p.cookie
	}
	p.dernierMac1 = signerMacs(msg, p.cles, cookie)
	return []envoi{p.versLui(msg)}
}

// --- sortie : de l'interface vers le réseau ----------------------------------

func (m *Moteur) boucleTun() error {
	buf := make([]byte, MTU+128)
	for {
		n, err := m.tun.Read(buf)
		if err != nil {
			return err
		}
		ip, ok := lireIPv4(buf[:n])
		if !ok {
			continue
		}
		p := m.router(netip.AddrFrom4(ip.dest))
		if p == nil {
			continue
		}
		if !reponseAutorisee(ip, p.entrant.Load()) {
			m.suivi.noter(ip, p)
		}
		m.expedier(m.envoyerClair(p, append([]byte(nil), buf[:ip.longueur]...)))
	}
}

// router choisit le pair dont une adresse contient la destination, le
// préfixe le plus précis l'emportant.
func (m *Moteur) router(dest netip.Addr) *pair {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var meilleur *pair
	bits := -1
	for _, p := range m.pairs {
		for _, pr := range *p.adresses.Load() {
			if pr.Contains(dest) && pr.Bits() > bits {
				meilleur, bits = p, pr.Bits()
			}
		}
	}
	return meilleur
}

// --- entrée : du réseau vers l'interface ------------------------------------

func (m *Moteur) boucleUDP() error {
	buf := make([]byte, 1<<16)
	for {
		n, src, err := m.conn.ReadFromUDPAddrPort(buf)
		if err != nil {
			return err
		}
		src = netip.AddrPortFrom(src.Addr().Unmap(), src.Port())
		m.recevoir(buf[:n], origine{point: src})
	}
}

func (m *Moteur) recevoir(msg []byte, o origine) {
	m.expedier(m.traiter(msg, o))
}

// traiter : ce qu'un message reçu change, et ce qu'il faut envoyer en
// retour. Séparé de l'expédition pour que le fuzzing puisse l'appeler seul.
func (m *Moteur) traiter(msg []byte, o origine) []envoi {
	if len(msg) < 4 {
		return nil
	}
	switch msg[0] {
	case typeInitiation:
		return m.recevoirInitiation(msg, o)
	case typeReponse:
		return m.recevoirReponse(msg, o)
	case typeCookie:
		m.recevoirCookie(msg)
	case typeDonnees:
		return m.recevoirDonnees(msg, o)
	}
	return nil
}

// verifierVenue : un message relayé doit venir d'un appareil joint par
// relais, sous le numéro que le serveur annonce ; un message direct, d'un
// pair qui n'est pas joint par relais. Un serveur ne peut donc pas faire
// passer un appareil pour un autre, ni un appareil contourner le serveur.
func verifierVenue(p *pair, o origine) bool {
	if o.relais {
		return p.relais.Load() && p.numero.Load() == o.numero
	}
	return !p.relais.Load()
}

func (m *Moteur) recevoirInitiation(msg []byte, o origine) []envoi {
	if len(msg) != tailleInitiation || !mac1Valide(msg, *m.nosCles.Load()) {
		return nil
	}
	envoyeur := binary.LittleEndian.Uint32(msg[4:8])
	// Relayée, une initiation ne passe pas par les cookies : on la limite
	// par pair. Sans cela, un appareil relié pourrait en envoyer en boucle
	// et épuiser le processeur et la batterie de l'autre.
	if o.relais && !m.limiteRelais.autoriser(uint64(o.numero)) {
		return nil
	}
	// En direct et sous charge, seul un expéditeur qui a prouvé posséder son
	// adresse passe, et à un rythme limité.
	if !o.relais && m.charge.sousCharge() {
		n := len(msg)
		var mac1 [tailleMac]byte
		copy(mac1[:], msg[n-2*tailleMac:n-tailleMac])
		if !mac2Valide(msg, m.cookies.cookie(o.point)) {
			return []envoi{{msg: m.cookies.reponseCookie(envoyeur, mac1, o.point, *m.nosCles.Load()), point: o.point}}
		}
		if !m.charge.autoriser(o.point.Addr()) {
			return nil
		}
	}

	r := noise.NouveauRepondeur(m.prive.Load(), Prologue)
	publique, charge, err := r.LireMessage1(msg[8 : len(msg)-2*tailleMac])
	if err != nil || len(charge) != tailleHorodatage {
		return nil
	}
	var cle [32]byte
	copy(cle[:], publique)
	m.mu.RLock()
	p := m.pairs[cle]
	m.mu.RUnlock()
	if p == nil {
		m.journal.Info("initiation d'une clé inconnue", "source", o.point, "relais", o.numero)
		return nil
	}
	if !verifierVenue(p, o) {
		m.journal.Warn("initiation par un chemin inattendu", "relais", o.relais, "numero", o.numero)
		return nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	// Une initiation qui n'est pas plus récente que la dernière est un
	// rejeu : on l'ignore sans répondre.
	if p.dernierHorodatage != nil && bytes.Compare(charge, p.dernierHorodatage) <= 0 {
		m.journal.Warn("initiation rejouée", "source", o.point, "relais", o.numero)
		return nil
	}
	corps, cles, err := r.Message2(nil)
	if err != nil {
		return nil
	}
	m.mu.Lock()
	if p.suivante != nil {
		delete(m.indices, p.suivante.locale)
	}
	locale, ok := m.reserverIndice(p)
	m.mu.Unlock()
	if !ok {
		return nil
	}
	p.dernierHorodatage = charge
	s := nouvelleSession(locale, envoyeur, cles, false)
	// La session n'est promue qu'au premier paquet de données reçu avec
	// elle : c'est la preuve que l'initiateur détient bien les mêmes clés.
	p.suivante = s
	if !o.relais && !p.direct.Load() {
		p.point = o.point
	}
	rep := binary.LittleEndian.AppendUint32(entete(typeReponse), locale)
	rep = binary.LittleEndian.AppendUint32(rep, envoyeur)
	rep = append(append(rep, corps...), make([]byte, 2*tailleMac)...)
	signerMacs(rep, p.cles, nil)
	if o.relais {
		return []envoi{{msg: rep, relais: o.numero}}
	}
	return []envoi{{msg: rep, point: o.point}}
}

func (m *Moteur) recevoirReponse(msg []byte, o origine) []envoi {
	if len(msg) != tailleReponse || !mac1Valide(msg, *m.nosCles.Load()) {
		return nil
	}
	envoyeur := binary.LittleEndian.Uint32(msg[4:8])
	dest := binary.LittleEndian.Uint32(msg[8:12])
	m.mu.RLock()
	p := m.indices[dest]
	m.mu.RUnlock()
	if p == nil || !verifierVenue(p, o) {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.poignee == nil || p.indicePoignee != dest {
		return nil
	}
	// En direct, la réponse doit venir d'où l'on a envoyé l'initiation.
	if !o.relais && o.point != p.point {
		return nil
	}
	_, cles, err := p.poignee.LireMessage2(msg[12 : len(msg)-2*tailleMac])
	if err != nil {
		return nil
	}
	s := nouvelleSession(dest, envoyeur, cles, true)
	m.mu.Lock()
	if p.precedente != nil {
		delete(m.indices, p.precedente.locale)
	}
	m.mu.Unlock()
	p.precedente, p.courante = p.courante, s
	p.poignee, p.indicePoignee = nil, 0
	now := time.Now()
	p.debutTentatives = time.Time{}
	p.dernierePoignee, p.derniereReception, p.sansReponseDepuis = now, now, time.Time{}

	// Les paquets en attente partent. S'il n'y en a pas, un paquet vide
	// confirme la session à l'autre bout, qui ne s'en sert qu'à partir de là.
	attente := p.attente
	p.attente = nil
	if len(attente) == 0 {
		attente = [][]byte{nil}
	}
	var envois []envoi
	for _, clair := range attente {
		envois = append(envois, p.versLui(s.chiffrer(clair)))
		p.envoyes++
	}
	p.dernierEnvoi = now
	p.recuSansRepondre = time.Time{}
	return envois
}

func (m *Moteur) recevoirCookie(msg []byte) {
	if len(msg) != tailleCookie {
		return
	}
	dest := binary.LittleEndian.Uint32(msg[4:8])
	m.mu.RLock()
	p := m.indices[dest]
	m.mu.RUnlock()
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.direct.Load() || p.poignee == nil || p.indicePoignee != dest {
		return
	}
	c, err := lireCookie(msg, p.cles, p.dernierMac1)
	if err != nil {
		return
	}
	p.cookie, p.cookieRecu = c, time.Now()
	// La prochaine tentative portera le cookie, sans attendre cinq secondes.
	p.derniereTentative = time.Time{}
}

func (m *Moteur) recevoirDonnees(msg []byte, o origine) []envoi {
	if len(msg) < enteteDonnees+tailleTag {
		return nil
	}
	dest := binary.LittleEndian.Uint32(msg[4:8])
	m.mu.RLock()
	p := m.indices[dest]
	m.mu.RUnlock()
	if p == nil || !verifierVenue(p, o) {
		return nil
	}
	p.mu.Lock()
	var s *session
	for _, c := range []*session{p.courante, p.suivante, p.precedente} {
		if c != nil && c.locale == dest {
			s = c
			break
		}
	}
	p.mu.Unlock()
	if s == nil || time.Since(s.creee) >= rejeterApres {
		return nil
	}
	clair, ok := s.dechiffrer(msg)
	if !ok {
		return nil
	}

	var envois []envoi
	p.mu.Lock()
	if s == p.suivante {
		m.mu.Lock()
		if p.precedente != nil {
			delete(m.indices, p.precedente.locale)
		}
		m.mu.Unlock()
		p.precedente, p.courante, p.suivante = p.courante, s, nil
		p.dernierePoignee = time.Now()
	}
	// Itinérance : on suit un appareil qui change de réseau. Seul un paquet
	// authentifié, et jamais vu, peut déplacer son adresse.
	if !o.relais && !p.direct.Load() {
		p.point = o.point
	}
	p.recus++
	now := time.Now()
	p.derniereReception, p.sansReponseDepuis = now, time.Time{}
	// Le minuteur du maintien passif part à la première donnée reçue
	// depuis notre dernier envoi, et ne se réarme pas à chaque paquet :
	// sinon, pendant un flux continu à sens unique, il ne partirait jamais
	// et l'autre croirait la session morte.
	if len(clair) > 0 && p.recuSansRepondre.IsZero() {
		p.recuSansRepondre = now
	}
	// Un pair qui ne fait que répondre renvoie un maintien à celui qui en
	// envoie, s'il est resté muet : l'autre sait ainsi que la liaison vit.
	// Celui qui initie ne répond jamais aux maintiens, sinon les deux se
	// renverraient la balle.
	if len(clair) == 0 && !p.actif() && now.Sub(p.dernierEnvoi) >= maintienPassif && p.courante.utilisable() {
		envois = append(envois, p.versLui(p.courante.chiffrer(nil)))
		p.envoyes++
		p.dernierEnvoi = now
		p.recuSansRepondre = time.Time{}
	}
	p.mu.Unlock()

	if len(clair) == 0 {
		return envois // paquet de maintien
	}
	if clair[0] == 0 {
		return append(envois, m.recevoirTrame(p, clair)...)
	}
	ip, ok := lireIPv4(clair)
	if !ok {
		return envois
	}
	if !p.autorise(netip.AddrFrom4(ip.source)) {
		m.journal.Warn("source usurpée", "source", netip.AddrFrom4(ip.source), "numero", p.numero.Load())
		return envois
	}
	// Le paquet doit nous être destiné. Sinon, écrit sur l'interface, il
	// pourrait être routé vers notre réseau local : un pair autorisé sur le
	// port 80 atteindrait le port 80 de toutes les machines derrière nous.
	if moi := m.adresse.Load(); moi != nil && netip.AddrFrom4(ip.dest) != *moi {
		m.journal.Warn("paquet qui ne nous est pas destiné", "dest", netip.AddrFrom4(ip.dest), "numero", p.numero.Load())
		return envois
	}
	if !p.entrant.Load().autorise(ip) && !m.suivi.retour(ip, p) {
		return envois
	}
	m.tun.Write(clair[:ip.longueur])
	return envois
}

// recevoirTrame traite une trame de relais sortie d'une session.
//
// Sur le serveur, elle vient d'un appareil et doit partir vers un autre :
// on vérifie que la politique autorise ces deux-là à se parler, puis on la
// remet au destinataire en indiquant qui l'envoie. Le message qu'elle
// contient reste illisible : il est chiffré pour le destinataire.
//
// Chez un client, elle vient du serveur et contient un message d'un autre
// appareil, traité comme s'il arrivait directement de lui.
func (m *Moteur) recevoirTrame(de *pair, trame []byte) []envoi {
	numero, msg, ok := lireTrame(trame)
	if !ok || len(msg) < 4 || msg[0] < typeInitiation || msg[0] > typeDonnees {
		return nil
	}
	if m.relais != nil {
		source := de.numero.Load()
		if source == 0 || !m.relais(source, numero) {
			return nil
		}
		m.mu.RLock()
		vers := m.parNumero[numero]
		m.mu.RUnlock()
		if vers == nil || vers == de {
			return nil
		}
		// Les initiations relayées sont limitées par couple d'appareils :
		// le serveur n'aide pas un appareil à en inonder un autre.
		if msg[0] == typeInitiation && !m.limiteRelais.autoriser(uint64(source)<<32|uint64(numero)) {
			return nil
		}
		if sonde := m.sondeRelais.Load(); sonde != nil {
			(*sonde)(msg)
		}
		t, ok := trameRelais(source, msg)
		if !ok {
			return nil
		}
		return m.envoyerClair(vers, t)
	}
	m.mu.RLock()
	serveur := m.serveur
	m.mu.RUnlock()
	if de != serveur {
		return nil // seul le serveur relaie
	}
	return m.recevoirRelaye(msg, origine{relais: true, numero: numero})
}

// recevoirRelaye : un message d'un autre appareil, sorti d'une trame. Il
// suit le même chemin qu'un message direct, à ceci près qu'on y répond par
// relais.
func (m *Moteur) recevoirRelaye(msg []byte, o origine) []envoi {
	switch msg[0] {
	case typeInitiation:
		return m.recevoirInitiation(msg, o)
	case typeReponse:
		return m.recevoirReponse(msg, o)
	case typeDonnees:
		return m.recevoirDonnees(msg, o)
	}
	return nil
}

// --- minuterie ---------------------------------------------------------------

func (m *Moteur) minuterie() {
	now := time.Now()
	for _, p := range m.listePairs() {
		var envois []envoi
		p.mu.Lock()
		for _, s := range []**session{&p.precedente, &p.courante, &p.suivante} {
			if *s != nil && now.Sub((*s).creee) >= rejeterApres {
				m.mu.Lock()
				delete(m.indices, (*s).locale)
				m.mu.Unlock()
				*s = nil
			}
		}
		if p.actif() {
			// Session à renouveler, ou morte de l'autre côté : des données
			// sont parties sans retour, ou même les maintiens ne reviennent
			// plus.
			morte := (!p.sansReponseDepuis.IsZero() && now.Sub(p.sansReponseDepuis) > m.sansReponseMax) ||
				(p.maintien > 0 && p.courante != nil && now.Sub(p.derniereReception) > p.maintien+m.sansReponseMax)
			// La session avec le serveur reste toujours ouverte : c'est par
			// elle que les autres appareils nous joignent. Celles avec les
			// autres appareils ne s'ouvrent qu'à la demande, quand un paquet
			// attend.
			aOuvrir := p.courante.aRenouveler() && (p.direct.Load() || len(p.attente) > 0)
			if aOuvrir || morte {
				envois = append(envois, m.lancerPoignee(p)...)
			}
			if p.maintien > 0 && p.courante.utilisable() && now.Sub(p.dernierEnvoi) >= p.maintien {
				envois = append(envois, p.versLui(p.courante.chiffrer(nil)))
				p.envoyes++
				p.dernierEnvoi = now
				p.recuSansRepondre = time.Time{}
			}
			// Injoignable depuis trop longtemps : on ne garde pas
			// indéfiniment de vieux paquets.
			// On repart à zéro : un prochain paquet relancera une série
			// de tentatives complète, au lieu d'être aussitôt jeté.
			if !p.debutTentatives.IsZero() && now.Sub(p.debutTentatives) > abandonnerApres {
				p.attente = nil
				p.debutTentatives = time.Time{}
			}
		}
		// Maintien passif : des données reçues, rien renvoyé depuis dix
		// secondes. Un paquet vide dit à l'autre que la session vit.
		if p.courante.utilisable() && !p.recuSansRepondre.IsZero() && now.Sub(p.recuSansRepondre) >= maintienPassif {
			envois = append(envois, p.versLui(p.courante.chiffrer(nil)))
			p.envoyes++
			p.dernierEnvoi = now
			p.recuSansRepondre = time.Time{}
		}
		p.mu.Unlock()
		m.expedier(envois)
	}
	m.suivi.nettoyer()
	m.charge.parIP.nettoyer()
	m.limiteRelais.nettoyer()
}
