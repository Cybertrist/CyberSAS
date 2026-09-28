// Package politique décide qui peut joindre quoi dans le VPN, et traduit
// cette décision en règles de pare-feu pour le serveur.
//
// Tout le trafic entre appareils passe par le serveur : c'est là que les
// règles s'appliquent, dans le noyau, par nftables. Un appareil ne peut
// pas les contourner, puisqu'il n'a de session qu'avec le serveur.
package politique

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/netip"
	"os"
	"slices"
	"strconv"
	"strings"
)

// Une règle ouvre des ports, de certains appareils vers d'autres.
//
// Sélecteurs de De et Vers :
//
//	"*"                 tous les appareils
//	"groupe:admins"     les appareils des personnes de ce groupe
//	"etiquette:maison"  les machines inscrites sous cette étiquette
//	"utilisateur:a@b"   les appareils d'une personne
//	"serveur"           le serveur lui-même (Nginx, qui publie les services)
//	"soi"               dans Vers seulement : les autres appareils de la
//	                    même personne
//
// Ports : "*" (tout), "icmp", "tcp:443", "udp:53", "tcp:8000-8100".
type Regle struct {
	De    []string `json:"de"`
	Vers  []string `json:"vers"`
	Ports []string `json:"ports"`
}

type Politique struct {
	// Version : croît à chaque modification. Signée avec le fichier, elle
	// empêche un serveur de resservir une ancienne politique plus permissive.
	Version uint64  `json:"version"`
	Regles  []Regle `json:"regles"`
}

// Equipe : adresse Google, en minuscules, vers le groupe.
type Equipe map[string]string

type Appareil struct {
	Adresse      netip.Addr
	Proprietaire string
	Etiquette    string
}

type Port struct {
	Proto      string // "*", "tcp", "udp", "icmp"
	Debut, Fin uint16
}

// Flux : ce qu'une règle autorise, une fois les sélecteurs résolus.
type Flux struct {
	Sources, Dest []netip.Addr
	Ports         []Port
}

func Charger(chemin string) (Politique, error) {
	var p Politique
	b, err := os.ReadFile(chemin)
	if err != nil {
		return p, err
	}
	if err := json.Unmarshal(b, &p); err != nil {
		return p, fmt.Errorf("%s : %w", chemin, err)
	}
	if _, err := Verifier(p); err != nil {
		return p, fmt.Errorf("%s, %w", chemin, err)
	}
	return p, nil
}

// Verifier : chaque règle doit avoir des ports lisibles.
func Verifier(p Politique) (Politique, error) {
	for i, r := range p.Regles {
		if _, err := LirePorts(r.Ports); err != nil {
			return p, fmt.Errorf("règle %d : %w", i+1, err)
		}
	}
	return p, nil
}

// ChargerEquipe lit « adresse groupe » par ligne ; # commente.
func ChargerEquipe(chemin string) (Equipe, error) {
	f, err := os.Open(chemin)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return LireEquipe(f)
}

// LireEquipe : la même lecture, depuis un texte déjà ouvert (le serveur
// relit ainsi le fichier qu'il s'apprête à réécrire).
func LireEquipe(r io.Reader) (Equipe, error) {
	e := Equipe{}
	s := bufio.NewScanner(r)
	for s.Scan() {
		ch := strings.Fields(s.Text())
		if len(ch) >= 2 && !strings.HasPrefix(ch[0], "#") {
			e[strings.ToLower(ch[0])] = ch[1]
		}
	}
	return e, s.Err()
}

func LirePorts(liste []string) ([]Port, error) {
	var r []Port
	for _, s := range liste {
		switch {
		case s == "*":
			r = append(r, Port{Proto: "*"})
		case s == "icmp":
			r = append(r, Port{Proto: "icmp"})
		case strings.HasPrefix(s, "tcp:"), strings.HasPrefix(s, "udp:"):
			proto, plage, _ := strings.Cut(s, ":")
			d, f, estPlage := strings.Cut(plage, "-")
			debut, err := strconv.ParseUint(d, 10, 16)
			if err != nil {
				return nil, fmt.Errorf("port illisible : %q", s)
			}
			fin := debut
			if estPlage {
				if fin, err = strconv.ParseUint(f, 10, 16); err != nil || fin < debut {
					return nil, fmt.Errorf("plage illisible : %q", s)
				}
			}
			r = append(r, Port{Proto: proto, Debut: uint16(debut), Fin: uint16(fin)})
		default:
			return nil, fmt.Errorf("port inconnu : %q", s)
		}
	}
	return r, nil
}

func correspond(sel string, a Appareil, e Equipe) bool {
	switch {
	case sel == "*":
		return true
	case strings.HasPrefix(sel, "groupe:"):
		return a.Proprietaire != "" && e[a.Proprietaire] == strings.TrimPrefix(sel, "groupe:")
	case strings.HasPrefix(sel, "etiquette:"):
		return a.Etiquette != "" && a.Etiquette == strings.TrimPrefix(sel, "etiquette:")
	case strings.HasPrefix(sel, "utilisateur:"):
		return a.Proprietaire != "" && a.Proprietaire == strings.ToLower(strings.TrimPrefix(sel, "utilisateur:"))
	}
	return false
}

func choisir(sels []string, apps []Appareil, e Equipe, serveur netip.Addr) []netip.Addr {
	var r []netip.Addr
	if slices.Contains(sels, "serveur") {
		r = append(r, serveur)
	}
	for _, a := range apps {
		// Une personne sortie de l'équipe n'apparaît plus dans aucune
		// règle, même si ses appareils n'ont pas encore été purgés.
		if a.Proprietaire != "" && e[a.Proprietaire] == "" {
			continue
		}
		for _, s := range sels {
			if correspond(s, a, e) {
				r = append(r, a.Adresse)
				break
			}
		}
	}
	return r
}

// Compiler résout les sélecteurs sur les appareils du moment.
func (p Politique) Compiler(e Equipe, apps []Appareil, serveur netip.Addr) []Flux {
	var flux []Flux
	for _, r := range p.Regles {
		ports, _ := LirePorts(r.Ports)
		sources := choisir(r.De, apps, e, serveur)
		if len(sources) == 0 {
			continue
		}
		if d := choisir(r.Vers, apps, e, serveur); len(d) > 0 {
			flux = append(flux, Flux{sources, d, ports})
		}
		if !slices.Contains(r.Vers, "soi") {
			continue
		}
		// « soi » : pour chaque personne, de ses appareils vers ses appareils.
		parPersonne := map[string][]netip.Addr{}
		for _, a := range apps {
			if a.Proprietaire != "" && e[a.Proprietaire] != "" {
				parPersonne[a.Proprietaire] = append(parPersonne[a.Proprietaire], a.Adresse)
			}
		}
		for _, siens := range parPersonne {
			var s []netip.Addr
			for _, x := range siens {
				if slices.Contains(sources, x) {
					s = append(s, x)
				}
			}
			if len(s) > 0 && len(siens) > 1 {
				flux = append(flux, Flux{s, siens, ports})
			}
		}
	}
	return flux
}

// Joignables : les adresses qu'un appareil a le droit d'atteindre. Sert à
// l'appli, qui n'affiche que ce qu'on peut ouvrir.
func Joignables(flux []Flux, depuis netip.Addr) map[netip.Addr]bool {
	r := map[netip.Addr]bool{}
	for _, f := range flux {
		if slices.Contains(f.Sources, depuis) {
			for _, d := range f.Dest {
				if d != depuis {
					r[d] = true
				}
			}
		}
	}
	return r
}

// Sortant : pour un appareil, ce qu'il a le droit d'ouvrir chez chacun des
// autres, ports rassemblés sans doublon (« * » seul s'il couvre tout).
// L'appli s'en sert pour les ports affichés et le bouton du navigateur ;
// c'est le pendant d'Entrant, que le pair appliquera de son côté.
func Sortant(flux []Flux, moi netip.Addr) map[netip.Addr][]Port {
	r := map[netip.Addr][]Port{}
	for _, f := range flux {
		if !slices.Contains(f.Sources, moi) {
			continue
		}
		for _, d := range f.Dest {
			if d == moi {
				continue
			}
			for _, p := range f.Ports {
				if !slices.Contains(r[d], p) {
					r[d] = append(r[d], p)
				}
			}
		}
	}
	for d, ports := range r {
		if slices.Contains(ports, Port{Proto: "*"}) {
			r[d] = []Port{{Proto: "*"}}
		}
	}
	return r
}

// String rend un port sous la forme qu'accepte LirePorts.
func (p Port) String() string {
	switch {
	case p.Proto == "*" || p.Proto == "icmp":
		return p.Proto
	case p.Debut == p.Fin:
		return fmt.Sprintf("%s:%d", p.Proto, p.Debut)
	}
	return fmt.Sprintf("%s:%d-%d", p.Proto, p.Debut, p.Fin)
}

// Relations : les paires d'adresses qui ont le droit d'échanger, dans un
// sens au moins. Le serveur ne relaie qu'entre elles : deux appareils sans
// relation ne peuvent même pas se faire une poignée de main.
func Relations(flux []Flux, serveur netip.Addr) map[[2]netip.Addr]bool {
	r := map[[2]netip.Addr]bool{}
	for _, f := range flux {
		for _, s := range f.Sources {
			for _, d := range f.Dest {
				if s != d && s != serveur && d != serveur {
					r[[2]netip.Addr{s, d}] = true
					r[[2]netip.Addr{d, s}] = true
				}
			}
		}
	}
	return r
}

// Entree : ce qu'une source peut ouvrir chez un appareil.
type Entree struct {
	Source netip.Addr
	Ports  []Port
}

// Entrant : pour un appareil, ce que chaque source a le droit d'ouvrir
// chez lui. C'est ce que son moteur appliquera, puisque le serveur ne voit
// plus ce qui passe entre appareils.
func Entrant(flux []Flux, moi netip.Addr) []Entree {
	parSource := map[netip.Addr][]Port{}
	var ordre []netip.Addr
	for _, f := range flux {
		if !slices.Contains(f.Dest, moi) {
			continue
		}
		for _, s := range f.Sources {
			if s == moi {
				continue
			}
			if _, vu := parSource[s]; !vu {
				ordre = append(ordre, s)
			}
			parSource[s] = append(parSource[s], f.Ports...)
		}
	}
	r := make([]Entree, 0, len(ordre))
	for _, s := range ordre {
		r = append(r, Entree{s, parSource[s]})
	}
	return r
}
