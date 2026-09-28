// sasd : le serveur CyberSAS.
//
//	sasd                               fait tourner le serveur
//	sasd cle --etiquette maison [--nom maison]  clé d'inscription pour une machine
//	sasd cle --utilisateur a@b.fr      clé pour l'appareil d'une personne, sans Google
//	sasd appareils [--json]            les appareils inscrits
//	sasd signatures < certificats.json importe les certificats signés par le verrou
//	sasd retirer <nom>                 coupe un appareil
//	sasd invitations                   les clés d'inscription en cours
//	sasd annuler <id>                  en annule une avant qu'elle ne serve
//
// La configuration vient de l'environnement, voir config().
package main

import (
	"cmp"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/Cybertrist/CyberSAS/internal/base"
	"github.com/Cybertrist/CyberSAS/internal/client"
	"github.com/Cybertrist/CyberSAS/internal/dns"
	"github.com/Cybertrist/CyberSAS/internal/noise"
	"github.com/Cybertrist/CyberSAS/internal/politique"
	"github.com/Cybertrist/CyberSAS/internal/protocole"
	"github.com/Cybertrist/CyberSAS/internal/serveur"
	"github.com/Cybertrist/CyberSAS/internal/tun"
	"github.com/Cybertrist/CyberSAS/internal/tunnel"
	"github.com/Cybertrist/CyberSAS/internal/verrou"
)

func env(nom, defaut string) string {
	if v := os.Getenv(nom); v != "" {
		return v
	}
	return defaut
}

type reglages struct {
	serveur.Config
	etat, ecoute string
	port         int
}

func config() reglages {
	reseau := netip.MustParsePrefix(env("SAS_RESEAU", "10.77.0.0/24"))
	domaine := env("SAS_DOMAINE", "sas.local")
	// Un port mal écrit ne doit pas donner en silence le port par défaut :
	// les appareils chercheraient le serveur ailleurs.
	port, err := strconv.ParseUint(env("SAS_PORT", "51820"), 10, 16)
	if err != nil || port == 0 {
		meurt("SAS_PORT invalide : %q", os.Getenv("SAS_PORT"))
	}
	etat := env("SAS_ETAT", "/var/lib/sasd")
	return reglages{
		Config: serveur.Config{
			Domaine:            domaine,
			Point:              env("SAS_POINT", fmt.Sprintf("vpn.%s:%d", domaine, port)),
			Reseau:             reseau,
			Serveur:            reseau.Masked().Addr().Next(),
			Interface:          env("SAS_INTERFACE", "sas0"),
			Equipe:             env("SAS_EQUIPE", "/config/equipe.txt"),
			Politique:          env("SAS_POLITIQUE", "/politique/politique.json"),
			SignaturePolitique: env("SAS_SIGNATURE_POLITIQUE", "/politique/politique.sig"),
			Revocations:        env("SAS_REVOCATIONS", "/config/revocations.json"),
			RevocationsAppli:   filepath.Join(etat, "revocations.json"),
			ClientsGoogle:      env("SAS_CLIENTS_GOOGLE", "/config/clients_google"),
			Verrou:             env("SAS_VERROU", "/config/verrou.pub"),
			DureeAppareil:      30 * 24 * time.Hour,
		},
		etat:   etat,
		ecoute: env("SAS_ECOUTE", "127.0.0.1:8080"),
		port:   int(port),
	}
}

func meurt(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "sasd : "+format+"\n", args...)
	os.Exit(1)
}

func main() {
	cfg := config()
	if len(os.Args) > 1 {
		b, err := base.Ouvrir(filepath.Join(cfg.etat, "sas.db"))
		if err != nil {
			meurt("%v", err)
		}
		switch os.Args[1] {
		case "cle":
			cmdCle(b, os.Args[2:])
		case "appareils":
			cmdAppareils(b, cfg, os.Args[2:])
		case "signatures":
			cmdSignatures(b, cfg)
		case "revocations":
			// La liste en vigueur, pour que « sas.sh revoquer » parte de la
			// dernière, même quand elle vient de l'appli.
			if l := serveur.RevocationsEnVigueur(cfg.Revocations, cfg.RevocationsAppli); l != nil {
				json.NewEncoder(os.Stdout).Encode(l)
			}
		case "invitations":
			cmdInvitations(b)
		case "annuler":
			if len(os.Args) != 3 {
				meurt("usage : sasd annuler <id> (voir sasd invitations)")
			}
			i, err := b.AnnulerInvitation(os.Args[2])
			if err != nil {
				meurt("%s : invitation inconnue, déjà utilisée ou expirée", os.Args[2])
			}
			fmt.Printf("invitation pour %s annulée\n", pourQui(i))
		case "retirer":
			if len(os.Args) != 3 {
				meurt("usage : sasd retirer <nom>")
			}
			if err := b.SupprimerParNom(os.Args[2]); err != nil {
				meurt("%s : %v", os.Args[2], err)
			}
			fmt.Printf("%s retiré, coupé à la prochaine synchronisation (5 s au plus)\n", os.Args[2])
		default:
			meurt("commande inconnue : %s", os.Args[1])
		}
		return
	}
	if err := servir(cfg); err != nil {
		meurt("%v", err)
	}
}

func cmdCle(b *base.Base, args []string) {
	f := flag.NewFlagSet("cle", flag.ExitOnError)
	etiquette := f.String("etiquette", "", "étiquette de la machine (maison, nas…)")
	nom := f.String("nom", "", "nom de la machine dans le VPN (par défaut, son étiquette)")
	utilisateur := f.String("utilisateur", "", "adresse de la personne à qui appartient l'appareil")
	duree := f.Duration("duree", 10*time.Minute, "durée de validité")
	f.Parse(args)
	if (*etiquette == "") == (*utilisateur == "") {
		meurt("il faut --etiquette ou --utilisateur, pas les deux")
	}
	if *nom != "" && *etiquette == "" {
		meurt("--nom ne vaut que pour une machine : un appareil personnel porte le nom de son propriétaire")
	}
	cle := base.NouvelleCle()
	if err := b.CreerCle(cle, *etiquette, *utilisateur, *nom, time.Now().Add(*duree)); err != nil {
		meurt("%v", err)
	}
	fmt.Println(cle)
}

func cmdAppareils(b *base.Base, cfg reglages, args []string) {
	f := flag.NewFlagSet("appareils", flag.ExitOnError)
	enJSON := f.Bool("json", false, "sortie JSON, pour sas verrou signer")
	f.Parse(args)
	liste, err := b.Appareils()
	if err != nil {
		meurt("%v", err)
	}
	equipe, _ := politique.ChargerEquipe(cfg.Equipe)
	if *enJSON {
		// Le groupe est celui de l'équipe aujourd'hui : c'est lui que l'admin
		// signera, et qu'il voit avant de signer.
		var r []protocole.Appareil
		for _, a := range liste {
			// #nosec G115 -- a.ID est borné par base.Enregistrer.
			p := protocole.Appareil{Numero: uint32(a.ID), Nom: a.Nom, Adresse: a.Adresse.String(), ClePublique: a.ClePublique,
				Proprietaire: a.Proprietaire, Etiquette: a.Etiquette, Systeme: a.Systeme, Groupe: equipe[a.Proprietaire],
				SignatureExpire: a.SignatureExpire}
			if len(a.Signature) > 0 {
				p.Signature = base64.StdEncoding.EncodeToString(a.Signature)
			}
			r = append(r, p)
		}
		json.NewEncoder(os.Stdout).Encode(r)
		return
	}
	t := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(t, "NOM\tADRESSE\tPROPRIÉTAIRE\tÉTIQUETTE\tEMPREINTE\tCERTIFICAT\tVU LE")
	for _, a := range liste {
		cert := "aucun"
		if len(a.Signature) > 0 {
			cert = "jusqu'au " + a.SignatureExpire.Format("02/01/2006")
		}
		fmt.Fprintf(t, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", a.Nom, a.Adresse, tiret(a.Proprietaire), tiret(a.Etiquette),
			client.EmpreinteCle(a.ClePublique), cert, a.Vu.Format("02/01 15:04"))
	}
	t.Flush()
}

// cmdInvitations : les clés d'inscription ni utilisées ni expirées. On
// n'en montre que l'identifiant : la base n'a pas la clé.
func cmdInvitations(b *base.Base) {
	liste, err := b.Invitations()
	if err != nil {
		meurt("%v", err)
	}
	t := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(t, "ID\tPOUR\tCRÉÉE PAR\tEXPIRE LE")
	for _, i := range liste {
		fmt.Fprintf(t, "%s\t%s\t%s\t%s\n", i.ID, pourQui(i), tiret(i.Createur), i.Expire.Format("02/01 15:04"))
	}
	t.Flush()
}

// pourQui : l'adresse invitée, ou la machine.
func pourQui(i base.Invitation) string {
	if i.Utilisateur != "" {
		return i.Utilisateur
	}
	return "machine " + cmp.Or(i.Nom, i.Etiquette)
}

// cmdSignatures lit sur l'entrée les certificats produits par
// « sas verrou signer », les vérifie avec la clé publique du verrou et
// avec la fiche de chaque appareil, et enregistre ceux qui tiennent.
func cmdSignatures(b *base.Base, cfg reglages) {
	texte, err := os.ReadFile(cfg.Verrou)
	if err != nil {
		meurt("pas de verrou configuré (%s)", cfg.Verrou)
	}
	cle, err := verrou.LirePublique(strings.TrimSpace(string(texte)))
	if err != nil {
		meurt("%v", err)
	}
	var liste []protocole.Appareil
	if err := json.NewDecoder(io.LimitReader(os.Stdin, 1<<20)).Decode(&liste); err != nil {
		meurt("entrée illisible : %v", err)
	}
	ok, refus := serveur.ImporterCertificats(b, cle, liste)
	fmt.Printf("%d certificat(s) enregistré(s)\n", ok)
	for _, n := range refus {
		fmt.Printf("refusé : %s (signature fausse ou périmée, ou fiche qui ne correspond plus)\n", client.Propre(n))
	}
	if len(refus) > 0 {
		os.Exit(1)
	}
}
func tiret(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// cleServeur relit la clé privée du serveur, ou la crée au premier
// démarrage. Elle ne quitte jamais ce fichier.
func cleServeur(dossier string) ([]byte, error) {
	chemin := filepath.Join(dossier, "cle_serveur")
	if b, err := os.ReadFile(chemin); err == nil {
		return b, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	k, err := noise.GenererCle()
	if err != nil {
		return nil, err
	}
	return k.Bytes(), os.WriteFile(chemin, k.Bytes(), 0o600)
}

func servir(cfg reglages) error {
	journal := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := os.MkdirAll(cfg.etat, 0o700); err != nil {
		return err
	}
	brut, err := cleServeur(cfg.etat)
	if err != nil {
		return fmt.Errorf("clé du serveur : %w", err)
	}
	prive, err := noise.ClePrivee(brut)
	if err != nil {
		return err
	}
	b, err := base.Ouvrir(filepath.Join(cfg.etat, "sas.db"))
	if err != nil {
		return err
	}

	iface, err := tun.Ouvrir(cfg.Interface)
	if err != nil {
		return err
	}
	if err := iface.Configurer(netip.PrefixFrom(cfg.Serveur, cfg.Reseau.Bits()), tunnel.MTU); err != nil {
		return err
	}
	conn, err := net.ListenUDP("udp", &net.UDPAddr{Port: cfg.port})
	if err != nil {
		return err
	}

	ctx, arret := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer arret()

	// Le moteur demande au serveur, pour chaque trame, si les deux
	// appareils ont le droit de se parler. Le serveur est créé juste après,
	// avant que le moteur ne tourne.
	var srv *serveur.Serveur
	moteur := tunnel.Nouveau(tunnel.Config{Prive: prive, Tun: iface, Conn: conn, Adresse: cfg.Serveur,
		Journal: journal.With("composant", "tunnel"),
		Relais:  func(de, vers uint32) bool { return srv.Relie(de, vers) }})

	resolveur := dns.Nouveau("sas.internal", env("SAS_DNS_AMONT", "1.1.1.1:53"))
	srv = serveur.Nouveau(cfg.Config, b, moteur, resolveur, prive, journal)
	if err := srv.Synchroniser(); err != nil {
		return err
	}

	go func() {
		if err := moteur.Lancer(ctx); err != nil {
			journal.Error("tunnel arrêté", "erreur", err)
			arret()
		}
	}()
	go func() {
		if err := resolveur.Lancer(net.JoinHostPort(cfg.Serveur.String(), "53")); err != nil {
			journal.Error("DNS arrêté", "erreur", err)
		}
	}()
	go srv.Boucle(ctx)

	api := &http.Server{Addr: cfg.ecoute, Handler: srv.Routes(), ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 2 * time.Minute}
	go func() {
		<-ctx.Done()
		api.Close()
	}()
	journal.Info("serveur prêt", "cle_publique", base64.StdEncoding.EncodeToString(prive.PublicKey().Bytes()),
		"point", cfg.Point, "reseau", cfg.Reseau, "api", cfg.ecoute)
	if err := api.ListenAndServe(); err != nil && ctx.Err() == nil {
		return err
	}
	return nil
}
