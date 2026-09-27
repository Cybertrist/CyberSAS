// sas : le client CyberSAS pour Linux, pensé pour les machines sans écran.
//
//	SAS_CLE=sas-... sas rejoindre --serveur https://vpn.exemple.fr [--nom maison] [--verrou <clé>]
//	sas demon                       tient le tunnel ouvert (à lancer au démarrage)
//	sas appareils                   les pairs de cet appareil, et ceux refusés
//	sas etat                        l'inscription et la clé publique de cet appareil
//	sas quitter                     se désinscrit et oublie tout
//
// Et, sur l'ordinateur de l'admin seulement, jamais sur le serveur :
//
//	sas verrou creer --fichier F                    crée la clé du verrou
//	sas verrou signer --fichier F --cle K[,K...]    signe ces appareils-là, et eux seuls
//	sas verrou politique --fichier F < politique.json
//	sas verrou revoquer --fichier F --cle K < revocations.json
//
// La clé privée de l'appareil est générée ici et reste dans /var/lib/sas,
// lisible par root seul. Le serveur n'en voit que la moitié publique.
package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/netip"
	"os"
	"os/signal"
	"runtime"
	"slices"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/Cybertrist/CyberSAS/internal/appareil"
	"github.com/Cybertrist/CyberSAS/internal/b64"
	"github.com/Cybertrist/CyberSAS/internal/client"
	"github.com/Cybertrist/CyberSAS/internal/politique"
	"github.com/Cybertrist/CyberSAS/internal/protocole"
	"github.com/Cybertrist/CyberSAS/internal/tun"
	"github.com/Cybertrist/CyberSAS/internal/verrou"
)

// stockage : l'état de cet appareil, dans /var/lib/sas, lisible par root
// seul (voir internal/appareil).
var stockage = appareil.Stockage{Dossier: func() string {
	if d := os.Getenv("SAS_ETAT"); d != "" {
		return d
	}
	return "/var/lib/sas"
}()}

func meurt(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "sas : "+format+"\n", args...)
	os.Exit(1)
}

// nouvelleAPI : SAS_CA ajoute une autorité, celle du labo par exemple, aux
// racines du système.
func nouvelleAPI(base string) (*client.API, error) {
	var ca []byte
	if chemin := os.Getenv("SAS_CA"); chemin != "" {
		var err error
		if ca, err = os.ReadFile(chemin); err != nil {
			return nil, fmt.Errorf("autorité %s illisible", chemin)
		}
	}
	return client.NouvelleAPI(base, ca)
}

func api(base string) *client.API {
	a, err := nouvelleAPI(base)
	if err != nil {
		meurt("%v", err)
	}
	return a
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage : sas rejoindre|demon|appareils|etat|quitter|verrou")
		os.Exit(2)
	}
	switch os.Args[1] {
	case "rejoindre":
		rejoindre(os.Args[2:])
	case "demon":
		if err := demon(); err != nil {
			meurt("%v", err)
		}
	case "appareils":
		appareils()
	case "etat":
		e, err := stockage.Lire()
		if err != nil {
			meurt("pas inscrit")
		}
		a := e.Inscription.Appareil
		fmt.Printf("%s  %s  sur %s  (réseau %s, verrou %s)\n", client.Propre(a.Nom), a.Adresse, e.Serveur,
			e.Inscription.Reseau, client.EmpreinteVerrou(e.Retenu.Verrou))
		fmt.Printf("clé publique : %s  (empreinte %s)\n", e.Retenu.MaCle, client.EmpreinteCle(e.Retenu.MaCle))
	case "quitter":
		e, err := stockage.Lire()
		if err != nil {
			meurt("pas inscrit")
		}
		if err := api(e.Serveur).Appel("POST", protocole.CheminDeconnexion, e.Inscription.Jeton, nil, nil); err != nil {
			fmt.Fprintf(os.Stderr, "sas : le serveur n'a pas confirmé (%v), l'état local est effacé quand même\n", err)
		}
		// La clé privée est dans ce fichier : s'il reste, le dire.
		if err := stockage.Effacer(); err != nil {
			meurt("l'état local n'a pas pu être effacé (%v) : supprimer %s à la main", err, stockage.Fichier())
		}
		fmt.Println("désinscrit")
	case "verrou":
		cmdVerrou(os.Args[2:])
	default:
		meurt("commande inconnue : %s", os.Args[1])
	}
}

func rejoindre(args []string) {
	f := flag.NewFlagSet("rejoindre", flag.ExitOnError)
	serveur := f.String("serveur", "", "adresse de l'API, https://vpn.exemple.fr")
	verrouAttendu := f.String("verrou", "", "clé publique du verrou, si l'admin l'a donnée (recommandé)")
	oublier := f.Bool("oublier", false, "accepter un autre serveur ou un autre verrou que ceux déjà retenus")
	nom, _ := os.Hostname()
	f.StringVar(&nom, "nom", nom, "nom de cet appareil dans le VPN")
	f.Parse(args)
	// La clé d'inscription passe par l'environnement : en argument, elle
	// serait visible de tous dans la liste des processus.
	cle := os.Getenv("SAS_CLE")
	if *serveur == "" || cle == "" {
		meurt("il faut --serveur, et la clé d'inscription dans SAS_CLE")
	}
	e, err := appareil.Rejoindre(stockage, api(*serveur), *serveur, client.Justificatif{CleInscription: cle},
		appareil.Options{Nom: nom, Systeme: runtime.GOOS, VerrouAttendu: *verrouAttendu, Oublier: *oublier})
	if err != nil {
		if strings.Contains(err.Error(), "a changé") {
			meurt("%v. --oublier pour l'accepter", err)
		}
		meurt("%v", err)
	}
	r, ret := e.Inscription, e.Retenu
	fmt.Printf("inscrit : %s, %s\n", client.Propre(r.Appareil.Nom), r.Appareil.Adresse)
	fmt.Printf("clé publique : %s  (empreinte %s)\n", ret.MaCle, client.EmpreinteCle(ret.MaCle))
	if r.Verrou != "" {
		fmt.Printf("verrou retenu : %s. Donnez la clé publique ci-dessus à l'admin, pour qu'il la signe.\n", client.EmpreinteVerrou(r.Verrou))
	}
}

func appareils() {
	e, err := stockage.Lire()
	if err != nil {
		meurt("pas inscrit")
	}
	r, err := appareil.LireReseau(api(e.Serveur), e)
	if err != nil {
		meurt("%v", err)
	}
	ret := e.Retenu
	_, ecartes, err := client.Construire(r, &ret, netip.AddrPort{}, time.Now())
	if err != nil {
		meurt("%v", err)
	}
	refus := map[string]string{}
	var autres []client.Ecarte
	for _, x := range ecartes {
		if x.Adresse != "" && x.Adresse != ret.Moi.String() {
			refus[x.Adresse] = x.Raison
		} else {
			autres = append(autres, x)
		}
	}
	t := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(t, "%s\t%s\t%s\t%s\t(cet appareil)\n", client.Propre(r.Moi.Nom), r.Moi.Adresse, qui(r.Moi), "en ligne")
	for _, a := range r.Pairs {
		etat := "hors ligne"
		if a.EnLigne {
			etat = "en ligne"
		}
		note := ""
		if raison, ok := refus[a.Adresse]; ok {
			note = "REFUSÉ : " + raison
		}
		fmt.Fprintf(t, "%s\t%s\t%s\t%s\t%s\n", client.Propre(a.Nom), client.Propre(a.Adresse), qui(a), etat, note)
	}
	t.Flush()
	for _, x := range autres {
		fmt.Printf("note : %s : %s\n", x.Nom, x.Raison)
	}
}

func qui(a protocole.Appareil) string {
	if a.Etiquette != "" {
		return "étiquette " + client.Propre(a.Etiquette)
	}
	return client.Propre(a.Proprietaire)
}

// demon ouvre l'interface, attend une inscription, et tient le tunnel
// (voir appareil.Tenir).
func demon() error {
	journal := slog.New(slog.NewTextHandler(os.Stderr, nil))
	iface, err := tun.Ouvrir("sas0")
	if err != nil {
		return err
	}
	conn, err := net.ListenUDP("udp", &net.UDPAddr{})
	if err != nil {
		return err
	}
	ctx, arret := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer arret()
	return appareil.Tenir(ctx, appareil.Tenue{Stockage: stockage, Interface: iface, Conn: conn, Journal: journal, API: nouvelleAPI})
}

// --- le verrou, côté admin ------------------------------------------------------

func lireVerrou(fichier string) ed25519.PrivateKey {
	texte, err := os.ReadFile(fichier)
	if err != nil {
		meurt("%v", err)
	}
	graine, err := b64.Decoder(strings.TrimSpace(string(texte)))
	if err != nil || len(graine) != ed25519.SeedSize {
		meurt("clé de verrou illisible")
	}
	return ed25519.NewKeyFromSeed(graine)
}

func cmdVerrou(args []string) {
	if len(args) == 0 {
		meurt("usage : sas verrou creer|signer|politique|revoquer --fichier F")
	}
	f := flag.NewFlagSet("verrou", flag.ExitOnError)
	fichier := f.String("fichier", "", "clé privée du verrou")
	cles := f.String("cle", "", "clés publiques à signer ou à révoquer, séparées par des virgules")
	duree := f.Duration("duree", 90*24*time.Hour, "durée de validité des certificats")
	f.Parse(args[1:])
	if *fichier == "" {
		meurt("il faut --fichier")
	}
	switch args[0] {
	case "creer":
		// O_EXCL : on ne remplace jamais une clé de verrou existante, et l'on
		// ne suit pas un lien symbolique posé à sa place.
		sortie, err := os.OpenFile(*fichier, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			meurt("%v (on ne remplace pas une clé de verrou)", err)
		}
		pub, prive, err := verrou.Generer()
		if err != nil {
			meurt("%v", err)
		}
		fmt.Fprintln(sortie, base64.StdEncoding.EncodeToString(prive.Seed()))
		if err := sortie.Close(); err != nil {
			meurt("%v", err)
		}
		fmt.Println(base64.StdEncoding.EncodeToString(pub))
		fmt.Fprintf(os.Stderr, "verrou créé, empreinte %s. La clé privée reste dans %s : jamais sur le serveur.\n",
			verrou.Empreinte(pub), *fichier)

	case "signer":
		signer(lireVerrou(*fichier), *cles, *duree)

	case "politique":
		prive := lireVerrou(*fichier)
		brut, err := io.ReadAll(io.LimitReader(os.Stdin, 1<<20))
		if err != nil {
			meurt("%v", err)
		}
		var p politique.Politique
		if err := json.Unmarshal(brut, &p); err != nil {
			meurt("politique illisible : %v", err)
		}
		if _, err := politique.Verifier(p); err != nil {
			meurt("politique invalide : %v", err)
		}
		if p.Version == 0 {
			meurt("la politique doit porter un numéro de version, qui croît à chaque modification")
		}
		json.NewEncoder(os.Stdout).Encode(map[string]any{"version": p.Version,
			"signature": base64.StdEncoding.EncodeToString(verrou.SignerPolitique(prive, p.Version, brut))})
		fmt.Fprintf(os.Stderr, "politique version %d signée (%d règles)\n", p.Version, len(p.Regles))

	case "revoquer":
		prive := lireVerrou(*fichier)
		var l protocole.ListeRevocations
		if brut, _ := io.ReadAll(io.LimitReader(os.Stdin, 1<<20)); len(bytes.TrimSpace(brut)) > 0 {
			if err := json.Unmarshal(brut, &l); err != nil {
				meurt("liste actuelle illisible : %v", err)
			}
		}
		// La liste actuelle vient du serveur : on ne la reprend que signée
		// par ce verrou. Sinon, un serveur piraté ferait signer à l'admin
		// des bannissements qu'il n'a pas choisis, ou une version au plafond.
		if len(l.Cles) > 0 || l.Version > 0 {
			if _, ok := client.RevocationsSignees(&l, prive.Public().(ed25519.PublicKey)); !ok {
				meurt("la liste actuelle n'est pas signée par ce verrou : refusée")
			}
		}
		var nouvelles []string
		for _, k := range strings.Split(*cles, ",") {
			nouvelles = append(nouvelles, strings.TrimSpace(k))
		}
		suite, _, err := client.AllongerRevocations(prive, 0, nil, &l, nouvelles)
		if err != nil {
			meurt("%v", err)
		}
		// Toute la liste signée, pas seulement ce qui s'ajoute : l'admin voit
		// ce qu'il signe.
		for _, k := range suite.Cles {
			etat := "déjà révoquée"
			if !slices.Contains(l.Cles, k) {
				etat = "révoquée"
			}
			fmt.Fprintf(os.Stderr, "%-14s %s\n", etat, client.EmpreinteCle(k))
		}
		fmt.Fprintf(os.Stderr, "liste version %d, %d clés\n", suite.Version, len(suite.Cles))
		json.NewEncoder(os.Stdout).Encode(suite)

	default:
		meurt("usage : sas verrou creer|signer|politique|revoquer --fichier F")
	}
}

// signer : l'admin désigne les clés à signer, qu'il a lues sur les
// appareils eux-mêmes (sas etat, ou l'écran de l'appli). La liste venue
// du serveur ne sert qu'à connaître les autres champs : on ne signe
// jamais un appareil simplement parce que le serveur le présente, sinon un
// serveur piraté ferait signer sa propre clé.
func signer(prive ed25519.PrivateKey, cles string, duree time.Duration) {
	voulues := map[string]bool{}
	for _, k := range strings.Split(cles, ",") {
		if k = strings.TrimSpace(k); k != "" {
			voulues[k] = true
		}
	}
	if len(voulues) == 0 {
		meurt("il faut --cle : les clés publiques des appareils à signer, lues sur les appareils eux-mêmes")
	}
	var liste []protocole.Appareil
	if err := json.NewDecoder(io.LimitReader(os.Stdin, 1<<20)).Decode(&liste); err != nil {
		meurt("entrée illisible : %v", err)
	}
	expire := time.Now().Add(duree).Truncate(time.Second)
	var sortie []protocole.Appareil
	for _, a := range liste {
		if !voulues[a.ClePublique] {
			continue
		}
		delete(voulues, a.ClePublique)
		k, err := b64.Decoder(a.ClePublique)
		adresse, err2 := netip.ParseAddr(a.Adresse)
		if err != nil || err2 != nil || len(k) != 32 || !adresse.Is4() {
			fmt.Fprintf(os.Stderr, "ignoré, illisible : %s\n", client.Propre(a.Nom))
			continue
		}
		c := verrou.Certificat{Cle: [32]byte(k), Adresse: adresse, Etiquette: a.Etiquette,
			Proprietaire: a.Proprietaire, Groupe: a.Groupe, Expire: expire}
		sig, err := c.Signer(prive)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ignoré : %s : %v\n", client.Propre(a.Nom), err)
			continue
		}
		a.Signature, a.SignatureExpire = base64.StdEncoding.EncodeToString(sig), expire
		fmt.Fprintf(os.Stderr, "signé : %s, %s, %s, groupe %q, empreinte %s, jusqu'au %s\n",
			client.Propre(a.Nom), a.Adresse, qui(a), client.Propre(a.Groupe), client.EmpreinteCle(a.ClePublique), expire.Format("02/01/2006"))
		sortie = append(sortie, a)
	}
	for k := range voulues {
		fmt.Fprintf(os.Stderr, "introuvable sur le serveur : %s (rien signé pour elle)\n", client.EmpreinteCle(k))
	}
	json.NewEncoder(os.Stdout).Encode(sortie)
}
