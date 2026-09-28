// Package base garde les appareils inscrits, les clés d'inscription des
// machines et les comptes Google, dans un fichier SQLite.
//
// Rien de secret n'y est stocké en clair : les jetons de l'API et les clés
// d'inscription n'y figurent que par leur empreinte SHA-256, et les clés
// privées des appareils n'arrivent jamais jusqu'au serveur.
package base

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"net/netip"
	"regexp"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

var (
	ErrIntrouvable       = errors.New("introuvable")
	ErrAutreProprietaire = errors.New("cette clé est déjà inscrite pour quelqu'un d'autre")
	ErrNomPris           = errors.New("ce nom de machine est déjà pris")
	ErrAutreCompte       = errors.New("cette adresse est liée à un autre compte Google")
	ErrReseauPlein       = errors.New("plus aucune adresse libre dans le réseau")
	ErrNumerosEpuises    = errors.New("plus aucun numéro d'appareil libre")
)

// NomsReserves : jamais donnés à un appareil. « serveur » désigne le
// serveur dans le DNS du VPN ; le laisser prendre, c'est détourner vers soi
// ce que les autres lui envoient.
var NomsReserves = map[string]bool{"serveur": true, "vpn": true, "auth": true}

type Base struct {
	db *sql.DB
}

type Appareil struct {
	ID  int64
	Nom string
	// Libelle : le nom affiché, tel que la personne l'a écrit (« Z Fold8
	// Tristan »). Nom, lui, sert d'adresse sur le réseau. Le libellé ne
	// fait pas partie du certificat : le changer ne demande pas de signature.
	Libelle      string
	ClePublique  string
	Adresse      netip.Addr
	Proprietaire string // adresse Google, vide pour une machine
	Etiquette    string // vide pour un appareil personnel
	Systeme      string
	Cree, Vu     time.Time
	Expire       time.Time // zéro : n'expire pas
	// Certificat du verrou : signature, et les champs signés qui ne sont
	// pas déjà ci-dessus.
	Signature       []byte
	SignatureGroupe string
	SignatureExpire time.Time
}

// AUTOINCREMENT : un numéro d'appareil n'est jamais redonné. Un pair qui
// n'a pas encore rafraîchi sa liste ne confond donc pas un nouvel appareil
// avec l'ancien.
const schema = `
CREATE TABLE IF NOT EXISTS appareils (
	id               INTEGER PRIMARY KEY AUTOINCREMENT,
	nom              TEXT NOT NULL UNIQUE,
	cle_publique     TEXT NOT NULL UNIQUE,
	adresse          TEXT NOT NULL UNIQUE,
	proprietaire     TEXT NOT NULL DEFAULT '',
	etiquette        TEXT NOT NULL DEFAULT '',
	systeme          TEXT NOT NULL DEFAULT '',
	cree             INTEGER NOT NULL,
	vu               INTEGER NOT NULL,
	expire           INTEGER NOT NULL DEFAULT 0,
	jeton            BLOB,
	signature        BLOB,
	signature_groupe TEXT NOT NULL DEFAULT '',
	signature_expire INTEGER NOT NULL DEFAULT 0,
	libelle          TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS cles (
	empreinte   BLOB PRIMARY KEY,
	etiquette   TEXT NOT NULL DEFAULT '',
	utilisateur TEXT NOT NULL DEFAULT '',
	nom         TEXT NOT NULL DEFAULT '',
	expire      INTEGER NOT NULL,
	cree        INTEGER NOT NULL DEFAULT 0,
	createur    TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS comptes (
	email TEXT PRIMARY KEY,
	sub   TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS reglages (
	cle    TEXT PRIMARY KEY,
	valeur TEXT NOT NULL
);`

// _txlock=immediate : une transaction prend le verrou d'écriture dès son
// début. Deux inscriptions simultanées ne choisissent donc jamais la même
// adresse.
func Ouvrir(chemin string) (*Base, error) {
	db, err := sql.Open("sqlite", "file:"+chemin+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_txlock=immediate")
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("schéma : %w", err)
	}
	// Une base créée avant le libellé : on ajoute la colonne.
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('appareils') WHERE name = 'libelle'`).Scan(&n); err != nil {
		return nil, fmt.Errorf("schéma : %w", err)
	}
	if n == 0 {
		if _, err := db.Exec(`ALTER TABLE appareils ADD COLUMN libelle TEXT NOT NULL DEFAULT ''`); err != nil {
			return nil, fmt.Errorf("schéma : %w", err)
		}
	}
	// Une base créée avant la liste des invitations : la date de création
	// et l'admin qui l'a faite. Les clés déjà là restent sans l'une ni
	// l'autre.
	for _, c := range []string{"cree INTEGER NOT NULL DEFAULT 0", "createur TEXT NOT NULL DEFAULT ''"} {
		nom, _, _ := strings.Cut(c, " ")
		if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('cles') WHERE name = ?`, nom).Scan(&n); err != nil {
			return nil, fmt.Errorf("schéma : %w", err)
		}
		if n == 0 {
			if _, err := db.Exec(`ALTER TABLE cles ADD COLUMN ` + c); err != nil {
				return nil, fmt.Errorf("schéma : %w", err)
			}
		}
	}
	return &Base{db: db}, nil
}

func Empreinte(s string) []byte {
	h := sha256.Sum256([]byte(s))
	return h[:]
}

const colonnes = `id, nom, cle_publique, adresse, proprietaire, etiquette, systeme, cree, vu, expire,
	signature, signature_groupe, signature_expire, libelle`

func lire(r interface{ Scan(...any) error }) (Appareil, error) {
	var a Appareil
	var adresse string
	var cree, vu, expire, sigExpire int64
	if err := r.Scan(&a.ID, &a.Nom, &a.ClePublique, &adresse, &a.Proprietaire, &a.Etiquette, &a.Systeme,
		&cree, &vu, &expire, &a.Signature, &a.SignatureGroupe, &sigExpire, &a.Libelle); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return a, ErrIntrouvable
		}
		return a, err
	}
	a.Adresse, _ = netip.ParseAddr(adresse)
	a.Cree, a.Vu = time.Unix(cree, 0), time.Unix(vu, 0)
	if expire != 0 {
		a.Expire = time.Unix(expire, 0)
	}
	if sigExpire != 0 {
		a.SignatureExpire = time.Unix(sigExpire, 0)
	}
	return a, nil
}

func (b *Base) Appareils() ([]Appareil, error) {
	rows, err := b.db.Query(`SELECT ` + colonnes + ` FROM appareils ORDER BY nom`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var r []Appareil
	for rows.Next() {
		a, err := lire(rows)
		if err != nil {
			return nil, err
		}
		r = append(r, a)
	}
	return r, rows.Err()
}

func (b *Base) ParJeton(jeton string) (Appareil, error) {
	return lire(b.db.QueryRow(`SELECT `+colonnes+` FROM appareils WHERE jeton = ?`, Empreinte(jeton)))
}

func (b *Base) Supprimer(id int64) error {
	_, err := b.db.Exec(`DELETE FROM appareils WHERE id = ?`, id)
	return err
}

// DefinirLibelle change le nom affiché d'un appareil.
func (b *Base) DefinirLibelle(id int64, libelle string) error {
	r, err := b.db.Exec(`UPDATE appareils SET libelle = ? WHERE id = ?`, libelle, id)
	if err != nil {
		return err
	}
	if n, _ := r.RowsAffected(); n == 0 {
		return ErrIntrouvable
	}
	return nil
}

// ParCle : l'appareil qui a cette clé publique.
func (b *Base) ParCle(cle string) (Appareil, error) {
	return lire(b.db.QueryRow(`SELECT `+colonnes+` FROM appareils WHERE cle_publique = ?`, cle))
}

func (b *Base) SupprimerParNom(nom string) error {
	r, err := b.db.Exec(`DELETE FROM appareils WHERE nom = ?`, nom)
	if err != nil {
		return err
	}
	if n, _ := r.RowsAffected(); n == 0 {
		return ErrIntrouvable
	}
	return nil
}

var horsNom = regexp.MustCompile(`[^a-z0-9-]+`)

// NomPropre : minuscules, chiffres et tirets, pour servir de nom DNS.
func NomPropre(s string) string {
	s = horsNom.ReplaceAllString(strings.ToLower(strings.TrimSpace(s)), "-")
	s = strings.Trim(s, "-")
	if len(s) > 30 {
		s = strings.Trim(s[:30], "-")
	}
	if s == "" {
		s = "appareil"
	}
	return s
}

// NomPersonnel : le nom d'un appareil personnel porte toujours celui de
// son propriétaire. « portable-alice » ne peut pas se faire passer pour
// « maison » ni pour « serveur » : les noms des machines, eux, n'ont pas
// de suffixe, et ce sont les admins qui les choisissent.
func NomPersonnel(nom, email string) string {
	local, _, _ := strings.Cut(email, "@")
	return NomPropre(nom) + "-" + NomPropre(local)
}

// Inscription : ce que le serveur a vérifié avant d'inscrire.
type Inscription struct {
	Appareil Appareil
	Jeton    string
	// CleInscription, si non vide, est consommée dans la même transaction :
	// si l'inscription échoue, la clé reste valable.
	CleInscription string
	// Accepter est appelé avec l'étiquette et l'utilisateur de la clé, avant
	// toute écriture : c'est là que le serveur vérifie l'équipe.
	Accepter func(etiquette, utilisateur string) error
	// DureePersonnel : un appareil personnel, qu'il vienne d'un jeton Google
	// ou d'une clé, expire au bout de ce délai et doit se réinscrire.
	DureePersonnel time.Duration
}

// Enregistrer inscrit un appareil, ou met à jour celui qui a déjà cette clé
// publique : il garde son adresse. Un nouvel appareil reçoit une adresse
// libre du réseau, et un nom unique.
func (b *Base) Enregistrer(ins Inscription, reseau netip.Prefix, serveur netip.Addr) (Appareil, error) {
	a := ins.Appareil
	tx, err := b.db.Begin()
	if err != nil {
		return a, err
	}
	defer tx.Rollback()
	now := time.Now()

	nomMachine := ""
	if ins.CleInscription != "" {
		var expire int64
		err := tx.QueryRow(`DELETE FROM cles WHERE empreinte = ? RETURNING etiquette, utilisateur, nom, expire`,
			Empreinte(ins.CleInscription)).Scan(&a.Etiquette, &a.Proprietaire, &nomMachine, &expire)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && now.Unix() > expire) {
			return a, ErrIntrouvable
		}
		if err != nil {
			return a, err
		}
	}
	if ins.Accepter != nil {
		if err := ins.Accepter(a.Etiquette, a.Proprietaire); err != nil {
			return a, err
		}
	}
	if a.Proprietaire != "" && ins.DureePersonnel > 0 {
		a.Expire = now.Add(ins.DureePersonnel)
	}
	var expire int64
	if !a.Expire.IsZero() {
		expire = a.Expire.Unix()
	}

	existant, err := lire(tx.QueryRow(`SELECT `+colonnes+` FROM appareils WHERE cle_publique = ?`, a.ClePublique))
	switch {
	case err == nil:
		// Une clé déjà inscrite ne change jamais de main : même avec la
		// clé privée, on ne fait pas passer l'appareil d'Alice à Bob, ni
		// d'une étiquette à une autre. Il faut le retirer d'abord.
		if existant.Proprietaire != a.Proprietaire || existant.Etiquette != a.Etiquette {
			return a, ErrAutreProprietaire
		}
		a.ID, a.Adresse, a.Cree, a.Nom, a.Libelle = existant.ID, existant.Adresse, existant.Cree, existant.Nom, existant.Libelle
		a.Signature, a.SignatureGroupe, a.SignatureExpire = existant.Signature, existant.SignatureGroupe, existant.SignatureExpire
		if _, err := tx.Exec(`UPDATE appareils SET systeme=?, vu=?, expire=?, jeton=? WHERE id=?`,
			a.Systeme, now.Unix(), expire, Empreinte(ins.Jeton), a.ID); err != nil {
			return a, err
		}
	case errors.Is(err, ErrIntrouvable):
		if a.Adresse, err = adresseLibre(tx, reseau, serveur); err != nil {
			return a, err
		}
		if a.Nom, err = choisirNom(tx, a, nomMachine); err != nil {
			return a, err
		}
		r, err := tx.Exec(`INSERT INTO appareils (nom, cle_publique, adresse, proprietaire, etiquette, systeme, cree, vu, expire, jeton, libelle)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			a.Nom, a.ClePublique, a.Adresse.String(), a.Proprietaire, a.Etiquette, a.Systeme, now.Unix(), now.Unix(), expire, Empreinte(ins.Jeton), a.Libelle)
		if err != nil {
			return a, err
		}
		if a.ID, err = r.LastInsertId(); err != nil {
			return a, err
		}
		// L'identifiant sert de numéro d'appareil dans le tunnel, sur quatre
		// octets : au-delà, deux appareils partageraient le même numéro et
		// le serveur relaierait vers le mauvais.
		if a.ID <= 0 || a.ID > math.MaxUint32 {
			return a, ErrNumerosEpuises
		}
		a.Cree = now
	default:
		return a, err
	}
	a.Vu = now
	return a, tx.Commit()
}

// choisirNom : une machine prend le nom que l'admin a mis dans sa clé
// d'inscription, ou à défaut son étiquette, et le refuse s'il est pris.
// Un appareil personnel prend le nom qu'il propose, suffixé du nom de son
// propriétaire, puis numéroté s'il le faut.
func choisirNom(tx *sql.Tx, a Appareil, nomMachine string) (string, error) {
	pris := func(n string) (bool, error) {
		var c int
		err := tx.QueryRow(`SELECT COUNT(*) FROM appareils WHERE nom = ?`, n).Scan(&c)
		return c > 0 || NomsReserves[n], err
	}
	if a.Etiquette != "" {
		nom := NomPropre(nomMachine)
		if nomMachine == "" {
			nom = NomPropre(a.Etiquette)
		}
		p, err := pris(nom)
		if err != nil {
			return "", err
		}
		if p {
			return "", ErrNomPris
		}
		return nom, nil
	}
	base := NomPersonnel(a.Nom, a.Proprietaire)
	candidat := base
	for i := 2; ; i++ {
		p, err := pris(candidat)
		if err != nil {
			return "", err
		}
		if !p {
			return candidat, nil
		}
		candidat = fmt.Sprintf("%s-%d", base, i)
	}
}

// adresseLibre : en tourniquet, à partir de la dernière donnée. Une adresse
// libérée n'est redonnée qu'après avoir fait le tour du réseau : un pair
// qui n'a pas encore rafraîchi ses règles ne les applique pas au nouvel
// appareil qui l'aurait reprise tout de suite.
func adresseLibre(tx *sql.Tx, reseau netip.Prefix, serveur netip.Addr) (netip.Addr, error) {
	prises := map[string]bool{serveur.String(): true}
	rows, err := tx.Query(`SELECT adresse FROM appareils`)
	if err != nil {
		return netip.Addr{}, err
	}
	// Une adresse mal lue serait redonnée à un second appareil : toute
	// erreur de lecture arrête l'inscription.
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			_ = rows.Close() // l'erreur de Scan est celle qui compte
			return netip.Addr{}, err
		}
		prises[s] = true
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return netip.Addr{}, err
	}
	var derniere string
	err = tx.QueryRow(`SELECT valeur FROM reglages WHERE cle = 'derniere_adresse'`).Scan(&derniere)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return netip.Addr{}, err
	}
	depart, err := netip.ParseAddr(derniere)
	if err != nil || !reseau.Contains(depart) {
		depart = reseau.Masked().Addr()
	}
	premiere := reseau.Masked().Addr().Next()
	a := depart
	for range 1 << (32 - reseau.Bits()) {
		a = a.Next()
		// On saute l'adresse du réseau lui-même, et celle de diffusion.
		if !reseau.Contains(a) || !reseau.Contains(a.Next()) {
			a = premiere
		}
		if !prises[a.String()] {
			_, err := tx.Exec(`INSERT INTO reglages (cle, valeur) VALUES ('derniere_adresse', ?)
				ON CONFLICT(cle) DO UPDATE SET valeur = excluded.valeur`, a.String())
			return a, err
		}
	}
	return netip.Addr{}, ErrReseauPlein
}

// NouvelleCle : une clé d'inscription tirée au hasard (24 octets).
func NouvelleCle() string {
	brut := make([]byte, 24)
	rand.Read(brut)
	return "sas-" + base64.RawURLEncoding.EncodeToString(brut)
}

// CreerCle enregistre une clé d'inscription à usage unique. Pour une
// machine, nom est le nom qu'elle portera dans le VPN.
func (b *Base) CreerCle(cle, etiquette, utilisateur, nom string, expire time.Time) error {
	return b.CreerInvitation(cle, etiquette, utilisateur, nom, "", expire)
}

// CreerInvitation : CreerCle, en retenant qui l'a créée (l'adresse de
// l'admin, vide depuis « sasd cle »).
func (b *Base) CreerInvitation(cle, etiquette, utilisateur, nom, createur string, expire time.Time) error {
	// Une clé expirée ne sert plus à rien : on fait le ménage à chaque
	// création, sinon seules les clés consommées quitteraient la table.
	if _, err := b.db.Exec(`DELETE FROM cles WHERE expire < ?`, time.Now().Unix()); err != nil {
		return err
	}
	_, err := b.db.Exec(`INSERT INTO cles (empreinte, etiquette, utilisateur, nom, expire, cree, createur) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		Empreinte(cle), etiquette, strings.ToLower(utilisateur), nom, expire.Unix(), time.Now().Unix(), createur)
	return err
}

// Invitation : une clé d'inscription en cours, sans la clé. ID est le
// début de son empreinte (8 octets en hexadécimal) : de quoi la désigner
// pour l'annuler, rien pour la retrouver ni s'en servir.
type Invitation struct {
	ID          string
	Etiquette   string
	Utilisateur string
	Nom         string
	Createur    string
	Cree        time.Time // zéro : clé créée avant qu'on retienne la date
	Expire      time.Time
}

// longueurID : les octets d'empreinte qui désignent une invitation.
const longueurID = 8

func lireInvitation(r interface{ Scan(...any) error }) (Invitation, error) {
	var i Invitation
	var id []byte
	var cree, expire int64
	if err := r.Scan(&id, &i.Etiquette, &i.Utilisateur, &i.Nom, &i.Createur, &cree, &expire); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return i, ErrIntrouvable
		}
		return i, err
	}
	i.ID, i.Expire = hex.EncodeToString(id), time.Unix(expire, 0)
	if cree != 0 {
		i.Cree = time.Unix(cree, 0)
	}
	return i, nil
}

const colonnesInvitation = `substr(empreinte, 1, 8), etiquette, utilisateur, nom, createur, cree, expire`

// Invitations : les clés d'inscription ni consommées ni expirées, la plus
// proche de l'expiration d'abord.
func (b *Base) Invitations() ([]Invitation, error) {
	lignes, err := b.db.Query(`SELECT `+colonnesInvitation+` FROM cles WHERE expire >= ? ORDER BY expire`, time.Now().Unix())
	if err != nil {
		return nil, err
	}
	defer lignes.Close()
	var r []Invitation
	for lignes.Next() {
		i, err := lireInvitation(lignes)
		if err != nil {
			return nil, err
		}
		r = append(r, i)
	}
	return r, lignes.Err()
}

// AnnulerInvitation efface la clé d'inscription que désigne id, si elle
// vit encore. Enregistrer la consomme par un DELETE lui aussi : une fois
// annulée, elle n'inscrit plus personne, même une demande déjà en route.
func (b *Base) AnnulerInvitation(id string) (Invitation, error) {
	debut, err := hex.DecodeString(id)
	if err != nil || len(debut) != longueurID {
		return Invitation{}, ErrIntrouvable
	}
	return lireInvitation(b.db.QueryRow(`DELETE FROM cles WHERE substr(empreinte, 1, 8) = ? AND expire >= ?
		RETURNING `+colonnesInvitation, debut, time.Now().Unix()))
}

// ClesVivantes : le nombre de clés d'inscription pas encore expirées.
func (b *Base) ClesVivantes() (int, error) {
	var n int
	err := b.db.QueryRow(`SELECT COUNT(*) FROM cles WHERE expire >= ?`, time.Now().Unix()).Scan(&n)
	return n, err
}

// DefinirCertificat enregistre la signature du verrou pour un appareil,
// avec les champs signés qui ne sont pas déjà dans sa fiche. L'appelant l'a
// vérifiée avant.
func (b *Base) DefinirCertificat(clePublique string, adresse netip.Addr, signature []byte, groupe string, expire time.Time) error {
	r, err := b.db.Exec(`UPDATE appareils SET signature = ?, signature_groupe = ?, signature_expire = ?
		WHERE cle_publique = ? AND adresse = ?`,
		signature, groupe, expire.Unix(), clePublique, adresse.String())
	if err != nil {
		return err
	}
	if n, _ := r.RowsAffected(); n == 0 {
		return ErrIntrouvable
	}
	return nil
}

// LierCompte fixe, à la première inscription d'une adresse Google,
// l'identifiant permanent du compte (sub). Ensuite, seul ce compte-là peut
// se servir de cette adresse : une adresse recyclée par Google, ou
// réattribuée dans une organisation, n'ouvre pas l'accès de l'ancien
// titulaire.
func (b *Base) LierCompte(email, sub string) error {
	_, err := b.db.Exec(`INSERT INTO comptes (email, sub) VALUES (?, ?) ON CONFLICT(email) DO NOTHING`, email, sub)
	if err != nil {
		return err
	}
	var connu string
	if err := b.db.QueryRow(`SELECT sub FROM comptes WHERE email = ?`, email).Scan(&connu); err != nil {
		return err
	}
	if connu != sub {
		return ErrAutreCompte
	}
	return nil
}

// OublierCompte : l'admin retire quelqu'un de l'équipe pour de bon.
func (b *Base) OublierCompte(email string) error {
	_, err := b.db.Exec(`DELETE FROM comptes WHERE email = ?`, email)
	return err
}
