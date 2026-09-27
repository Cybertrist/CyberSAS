package client

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"math"
	"net/netip"
	"testing"
	"time"

	"github.com/Cybertrist/CyberSAS/internal/protocole"
	"github.com/Cybertrist/CyberSAS/internal/tunnel"
	"github.com/Cybertrist/CyberSAS/internal/verrou"
)

func aleatoire64(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return base64.StdEncoding.EncodeToString(b)
}

// reseauSigne : un serveur, cet appareil (poste d'alice, groupe equipe),
// une maison et un nas, avec une politique qui ouvre à l'équipe le TCP 80
// de la maison, et au serveur le TCP 80 de tout le monde.
type reseauSigne struct {
	prive ed25519.PrivateKey
	ret   Retenu
	etat  protocole.EtatReseau
}

const politiqueJSON = `{"version": 3, "regles": [
	{"de": ["groupe:equipe"], "vers": ["etiquette:maison"], "ports": ["tcp:80"]},
	{"de": ["serveur"], "vers": ["*"], "ports": ["tcp:80"]}
]}`

func (r *reseauSigne) appareil(numero uint32, nom, adresse, etiquette, proprietaire, groupe string) protocole.Appareil {
	cle := aleatoire64(32)
	k, _ := cle32(cle)
	exp := time.Now().Add(time.Hour).Truncate(time.Second)
	c := verrou.Certificat{Cle: k, Adresse: netip.MustParseAddr(adresse), Etiquette: etiquette,
		Proprietaire: proprietaire, Groupe: groupe, Expire: exp}
	sig, _ := c.Signer(r.prive)
	return protocole.Appareil{Numero: numero, Nom: nom, Adresse: adresse, ClePublique: cle, Etiquette: etiquette,
		Proprietaire: proprietaire, Groupe: groupe, SignatureExpire: exp, Signature: base64.StdEncoding.EncodeToString(sig)}
}

func nouveauReseauSigne(t *testing.T) *reseauSigne {
	pub, prive, _ := verrou.Generer()
	r := &reseauSigne{prive: prive}
	texteV := base64.StdEncoding.EncodeToString(pub)
	cleServeur := aleatoire64(32)
	moi := r.appareil(4, "poste-alice", "10.77.0.4", "", "alice@x.fr", "equipe")
	r.ret = Retenu{CleServeur: cleServeur, Verrou: texteV, MaCle: moi.ClePublique,
		Moi: netip.MustParseAddr("10.77.0.4"), Reseau: netip.MustParsePrefix("10.77.0.0/24")}
	r.etat = protocole.EtatReseau{
		Serveur: protocole.Serveur{ClePublique: cleServeur, Adresse: "10.77.0.1"},
		Verrou:  texteV,
		Moi:     moi,
		Pairs: []protocole.Appareil{
			r.appareil(2, "maison", "10.77.0.2", "maison", "", ""),
			r.appareil(3, "nas", "10.77.0.3", "nas", "", ""),
		},
		Politique:          base64.StdEncoding.EncodeToString([]byte(politiqueJSON)),
		PolitiqueVersion:   3,
		PolitiqueSignature: base64.StdEncoding.EncodeToString(verrou.SignerPolitique(prive, 3, []byte(politiqueJSON))),
	}
	return r
}

func (r *reseauSigne) construire(t *testing.T) ([]tunnel.Pair, []Ecarte) {
	t.Helper()
	pairs, ecartes, err := Construire(r.etat, &r.ret, netip.MustParseAddrPort("192.0.2.1:51820"), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return pairs, ecartes
}

func entrantDe(pairs []tunnel.Pair, adresse string) []tunnel.Regle {
	for _, p := range pairs {
		if p.Adresses[0].Addr() == netip.MustParseAddr(adresse) {
			return p.Entrant
		}
	}
	return nil
}

func TestVerrouReglesCalculeesLocalement(t *testing.T) {
	r := nouveauReseauSigne(t)
	// Le serveur, piraté, s'ouvre tous les ports de l'appareil.
	r.etat.Entrant = []protocole.RegleEntrante{{Sources: []string{"10.77.0.1", "10.77.0.2"}, Ports: []string{"*"}}}
	pairs, _ := r.construire(t)
	// Rien de tout ça ne passe : ce qui compte est la politique signée, qui
	// n'ouvre chez alice que le TCP 80 au serveur, et rien à la maison.
	if e := entrantDe(pairs, "10.77.0.1"); len(e) != 1 || e[0].Proto != 6 || e[0].Debut != 80 {
		t.Fatalf("règles du serveur chez alice : %+v, attendu TCP 80 seulement", e)
	}
	if e := entrantDe(pairs, "10.77.0.2"); len(e) != 0 {
		t.Fatalf("la maison ne doit rien pouvoir ouvrir chez alice : %+v", e)
	}
}

func TestVerrouPairsNonSignes(t *testing.T) {
	r := nouveauReseauSigne(t)
	maison := r.etat.Pairs[0]
	intrus := maison
	intrus.Numero, intrus.Nom, intrus.Adresse, intrus.ClePublique = 9, "maison-bis", "10.77.0.9", aleatoire64(32) // clé du serveur piraté
	vol := maison
	vol.Numero, vol.Nom, vol.Adresse = 8, "vol", "10.77.0.8" // bonne signature, autre adresse
	promu := r.etat.Pairs[1]
	promu.Etiquette = "maison" // le nas se fait passer pour une maison
	r.etat.Pairs = []protocole.Appareil{maison, intrus, vol, promu}
	pairs, ecartes := r.construire(t)
	if len(pairs) != 2 || pairs[1].Numero != 2 {
		t.Fatalf("seuls le serveur et la vraie maison devaient rester, obtenu %d pairs", len(pairs))
	}
	if len(ecartes) != 3 {
		t.Fatalf("trois pairs à écarter, obtenu : %+v", ecartes)
	}
}

func TestVerrouAdressesInvalides(t *testing.T) {
	r := nouveauReseauSigne(t)
	for _, a := range []string{"2001:db8::1", "::ffff:10.77.0.2", "192.168.1.1", "10.77.0.4", "10.77.0.1"} {
		p := r.etat.Pairs[0]
		p.Adresse = a
		r.etat.Pairs = []protocole.Appareil{p}
		pairs, _ := r.construire(t) // ne doit jamais paniquer
		if len(pairs) != 1 {
			t.Errorf("adresse %s acceptée pour un pair", a)
		}
	}
}

func TestVerrouRevocationsEtRetourEnArriere(t *testing.T) {
	r := nouveauReseauSigne(t)
	maison := r.etat.Pairs[0]
	k, _ := cle32(maison.ClePublique)
	r.etat.Revocations = &protocole.ListeRevocations{Version: 2, Cles: []string{maison.ClePublique},
		Signature: base64.StdEncoding.EncodeToString(verrou.SignerRevocations(r.prive, 2, [][32]byte{k}))}
	pairs, _ := r.construire(t)
	if len(pairs) != 2 { // serveur et nas
		t.Fatalf("la maison révoquée est encore là : %d pairs", len(pairs))
	}
	// Le serveur resservirait une liste plus ancienne, vide : ignorée.
	r.etat.Revocations = &protocole.ListeRevocations{Version: 1, Cles: nil,
		Signature: base64.StdEncoding.EncodeToString(verrou.SignerRevocations(r.prive, 1, nil))}
	if pairs, _ := r.construire(t); len(pairs) != 2 {
		t.Fatal("une liste de révocation plus ancienne a fait revenir la maison")
	}
	// Ni la politique : on a vu la version 3, la 2 est refusée.
	vieille := `{"version": 2, "regles": [{"de": ["*"], "vers": ["*"], "ports": ["*"]}]}`
	r.etat.Politique = base64.StdEncoding.EncodeToString([]byte(vieille))
	r.etat.PolitiqueVersion = 2
	r.etat.PolitiqueSignature = base64.StdEncoding.EncodeToString(verrou.SignerPolitique(r.prive, 2, []byte(vieille)))
	pairs, _ = r.construire(t)
	if e := entrantDe(pairs, "10.77.0.3"); len(e) != 0 {
		t.Fatalf("une politique plus ancienne et plus permissive a été appliquée : %+v", e)
	}
}

func TestVerrouCetAppareilNonSigne(t *testing.T) {
	r := nouveauReseauSigne(t)
	r.etat.Moi.Signature = ""
	pairs, ecartes := r.construire(t)
	for _, p := range pairs {
		if len(p.Entrant) != 0 {
			t.Fatal("sans certificat pour soi, rien ne doit pouvoir entrer")
		}
	}
	if len(ecartes) == 0 {
		t.Fatal("le client doit signaler qu'il n'est pas signé")
	}
}

func TestConstruireRefuseAutreServeurOuVerrou(t *testing.T) {
	r := nouveauReseauSigne(t)
	autre := r.etat
	autre.Serveur.ClePublique = aleatoire64(32)
	if _, _, err := Construire(autre, &r.ret, netip.AddrPort{}, time.Now()); err == nil {
		t.Fatal("une clé de serveur différente de celle retenue a été acceptée")
	}
	autre = r.etat
	autre.Verrou = ""
	if _, _, err := Construire(autre, &r.ret, netip.AddrPort{}, time.Now()); err == nil {
		t.Fatal("la disparition du verrou a été acceptée")
	}
}

func TestAPIRefuseHTTP(t *testing.T) {
	for _, u := range []string{"http://vpn.exemple.fr", "vpn.exemple.fr", "ftp://x"} {
		if _, err := NouvelleAPI(u, nil); err == nil {
			t.Errorf("%s accepté", u)
		}
	}
	if _, err := NouvelleAPI("https://vpn.exemple.fr", nil); err != nil {
		t.Fatal(err)
	}
}

func TestPropre(t *testing.T) {
	if Propre("maison\x1b[2J\x1b[Hsigné") != "maison?[2J?[Hsigné" {
		t.Fatal("les séquences de terminal doivent être neutralisées")
	}
}

// Revue de sécurité : un serveur piraté réécrit la clé d'un appareil
// révoqué avec d'autres bits de bourrage. Mêmes octets, autre texte : elle
// ne doit ni échapper à la révocation, ni même être lue.
func TestRevocationEcritureNonCanonique(t *testing.T) {
	r := nouveauReseauSigne(t)
	maison := r.etat.Pairs[0]
	k, _ := cle32(maison.ClePublique)
	r.etat.Revocations = &protocole.ListeRevocations{Version: 2, Cles: []string{maison.ClePublique},
		Signature: base64.StdEncoding.EncodeToString(verrou.SignerRevocations(r.prive, 2, [][32]byte{k}))}
	autre := []byte(maison.ClePublique)
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	i := len(autre) - 2
	for j := range alphabet {
		if alphabet[j] == autre[i] {
			autre[i] = alphabet[j^1] // change un bit de bourrage
			break
		}
	}
	if d, err := base64.StdEncoding.DecodeString(string(autre)); err != nil || [32]byte(d) != k {
		t.Fatal("hypothèse du test fausse : la variante devait donner la même clé au décodeur standard")
	}
	maison.ClePublique = string(autre)
	r.etat.Pairs[0] = maison
	pairs, _ := r.construire(t)
	for _, p := range pairs {
		if p.Publique == k {
			t.Fatal("la maison révoquée est revenue sous une autre écriture de sa clé")
		}
	}
}

// Une liste retenue déjà au plafond ne repasse pas à zéro : la révocation
// est refusée, pas signée avec la version 0.
func TestAllongerRevocationsPlafond(t *testing.T) {
	_, prive, _ := verrou.Generer()
	var k [32]byte
	k[0] = 1
	_, _, err := AllongerRevocations(prive, math.MaxUint64, nil, nil, []string{base64.StdEncoding.EncodeToString(k[:])})
	if err == nil {
		t.Fatal("version au plafond acceptée")
	}
}
