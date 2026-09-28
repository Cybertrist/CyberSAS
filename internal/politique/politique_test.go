package politique

import (
	"net/netip"
	"strings"
	"testing"
)

var (
	serveur = netip.MustParseAddr("10.77.0.1")
	equipe  = Equipe{"admin@x.fr": "admins", "alice@x.fr": "equipe"}
	apps    = []Appareil{
		{Adresse: netip.MustParseAddr("10.77.0.2"), Etiquette: "maison"},
		{Adresse: netip.MustParseAddr("10.77.0.3"), Proprietaire: "alice@x.fr"},
		{Adresse: netip.MustParseAddr("10.77.0.4"), Proprietaire: "alice@x.fr"},
		{Adresse: netip.MustParseAddr("10.77.0.5"), Proprietaire: "admin@x.fr"},
		{Adresse: netip.MustParseAddr("10.77.0.6"), Proprietaire: "bob@x.fr"}, // plus dans l'équipe
	}
	pol = Politique{Regles: []Regle{
		{De: []string{"groupe:admins"}, Vers: []string{"*"}, Ports: []string{"*"}},
		{De: []string{"*"}, Vers: []string{"soi"}, Ports: []string{"*"}},
		{De: []string{"groupe:equipe"}, Vers: []string{"etiquette:maison"}, Ports: []string{"tcp:80", "tcp:443"}},
		{De: []string{"serveur"}, Vers: []string{"etiquette:maison"}, Ports: []string{"tcp:80"}},
	}}
)

func a(s string) netip.Addr { return netip.MustParseAddr(s) }

func TestJoignables(t *testing.T) {
	flux := pol.Compiler(equipe, apps, serveur)
	cas := []struct {
		depuis string
		vers   []string
		pas    []string
	}{
		{"10.77.0.3", []string{"10.77.0.2", "10.77.0.4"}, []string{"10.77.0.5", "10.77.0.6"}}, // alice
		{"10.77.0.5", []string{"10.77.0.2", "10.77.0.3"}, []string{"10.77.0.6"}},              // admin, mais bob est sorti
		{"10.77.0.2", nil, []string{"10.77.0.3", "10.77.0.5"}},                                // maison
		{"10.77.0.6", nil, []string{"10.77.0.2", "10.77.0.3"}},                                // bob, sorti
	}
	for _, c := range cas {
		j := Joignables(flux, a(c.depuis))
		for _, v := range c.vers {
			if !j[a(v)] {
				t.Errorf("%s devrait joindre %s", c.depuis, v)
			}
		}
		for _, v := range c.pas {
			if j[a(v)] {
				t.Errorf("%s ne devrait pas joindre %s", c.depuis, v)
			}
		}
	}
}

func TestNft(t *testing.T) {
	r := Nft(pol.Compiler(equipe, apps, serveur), "sas0", serveur, netip.MustParsePrefix("10.77.0.0/24"))
	for _, attendu := range []string{
		`ip saddr { 10.77.0.1 } ip daddr { 10.77.0.2 } tcp dport 80 accept`,
		`iifname "sas0" counter drop comment "aucun paquet ne traverse le serveur en clair"`,
		"delete table inet cybersas",
	} {
		if !strings.Contains(r, attendu) {
			t.Errorf("règle manquante : %s\n%s", attendu, r)
		}
	}
	// Entre appareils, le noyau du serveur ne laisse plus rien passer : la
	// règle d'alice vers la maison est appliquée par la maison elle-même.
	if strings.Contains(r, "10.77.0.3, 10.77.0.4") {
		t.Error("une règle de transit entre appareils est restée dans le noyau du serveur")
	}
	if strings.Contains(r, "10.77.0.6") {
		t.Error("bob n'est plus dans l'équipe, il ne doit apparaître dans aucune règle")
	}
}

func TestRelationsEtEntrant(t *testing.T) {
	flux := pol.Compiler(equipe, apps, serveur)
	rel := Relations(flux, serveur)
	if !rel[[2]netip.Addr{a("10.77.0.3"), a("10.77.0.2")}] || !rel[[2]netip.Addr{a("10.77.0.2"), a("10.77.0.3")}] {
		t.Error("alice et la maison doivent être reliées, dans les deux sens")
	}
	if rel[[2]netip.Addr{a("10.77.0.2"), a("10.77.0.6")}] {
		t.Error("bob est sorti : aucune relation")
	}
	// Chez la maison : alice (ses deux appareils) en TCP 80 et 443, l'admin
	// en tout, le serveur en TCP 80.
	entrees := map[netip.Addr]string{}
	for _, e := range Entrant(flux, a("10.77.0.2")) {
		var s []string
		for _, p := range e.Ports {
			s = append(s, p.String())
		}
		entrees[e.Source] = strings.Join(s, ",")
	}
	for src, attendu := range map[string]string{"10.77.0.3": "tcp:80,tcp:443", "10.77.0.5": "*", "10.77.0.1": "tcp:80"} {
		if entrees[a(src)] != attendu {
			t.Errorf("entrant de %s chez la maison : %q, attendu %q", src, entrees[a(src)], attendu)
		}
	}
}

func TestSortant(t *testing.T) {
	flux := pol.Compiler(equipe, apps, serveur)
	chaine := func(ports []Port) string {
		var s []string
		for _, p := range ports {
			s = append(s, p.String())
		}
		return strings.Join(s, ",")
	}
	// Alice : la maison en web, son autre appareil en tout, rien d'autre.
	s := Sortant(flux, a("10.77.0.3"))
	for dest, attendu := range map[string]string{"10.77.0.2": "tcp:80,tcp:443", "10.77.0.4": "*", "10.77.0.5": "", "10.77.0.3": ""} {
		if got := chaine(s[a(dest)]); got != attendu {
			t.Errorf("alice vers %s : %q, attendu %q", dest, got, attendu)
		}
	}
	// L'admin : « * » absorbe tout le reste, bob n'y est plus.
	s = Sortant(flux, a("10.77.0.5"))
	if got := chaine(s[a("10.77.0.2")]); got != "*" {
		t.Errorf("admin vers la maison : %q, attendu *", got)
	}
	if _, ok := s[a("10.77.0.6")]; ok {
		t.Error("bob est sorti : l'admin ne doit rien pouvoir lui ouvrir")
	}
}

func TestPortsIllisibles(t *testing.T) {
	for _, p := range []string{"tcp", "tcp:abc", "tcp:90-80", "ssh"} {
		if _, err := LirePorts([]string{p}); err == nil {
			t.Errorf("%q aurait dû être refusé", p)
		}
	}
}
