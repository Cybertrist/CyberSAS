// Package protocole décrit ce que les applis et le serveur s'échangent sur
// l'API HTTPS : s'inscrire, connaître le réseau, se déconnecter.
//
// L'API ne transporte jamais de clé privée. L'appareil génère sa paire de
// clés lui-même et n'envoie que la moitié publique. Et rien de ce qu'elle
// dit n'est cru sans vérification quand le verrou du réseau est en place :
// chaque clé d'appareil doit porter la signature de l'admin.
package protocole

import "time"

const (
	CheminConnexion   = "/api/v1/connexion"
	CheminReseau      = "/api/v1/reseau"
	CheminDeconnexion = "/api/v1/deconnexion"
	CheminSante       = "/api/v1/sante"
	// CheminServeur rend la clé publique du serveur, pour calculer la
	// preuve de possession avant de s'inscrire.
	CheminServeur = "/api/v1/serveur"
	// CheminLibelle : changer le nom affiché de son appareil (un admin :
	// de n'importe lequel).
	CheminLibelle = "/api/v1/libelle"

	// Pour les admins seulement : la liste des appareils, signés ou non,
	// les signatures faites sur le téléphone de l'admin, et le refus d'un
	// appareil. Le serveur ne signe jamais : il vérifie ce qu'on lui donne
	// avec la clé publique du verrou, et un certificat faux est refusé.
	CheminAppareils  = "/api/v1/appareils"
	CheminSignatures = "/api/v1/signatures"
	CheminRetrait    = "/api/v1/retrait"
	// CheminInvitation : un admin crée une clé d'inscription pour un membre
	// de l'équipe. CheminRevocations : il envoie la liste de révocation
	// qu'il vient de signer sur son téléphone.
	CheminInvitation  = "/api/v1/invitation"
	CheminRevocations = "/api/v1/revocations"
	// CheminInvitations : les invitations en cours, sans leur clé.
	// CheminAnnulation : en annuler une avant qu'elle ne serve.
	CheminInvitations = "/api/v1/invitations"
	CheminAnnulation  = "/api/v1/invitations/annulation"
	// CheminEquipe : la liste de l'équipe (GET), et un membre ajouté,
	// changé de groupe ou retiré (POST). Admins seulement.
	CheminEquipe = "/api/v1/equipe"
)

// DemandeLibelle : le nouveau nom affiché. ClePublique désigne un autre
// appareil que le sien (admin seulement) ; vide, c'est le sien.
type DemandeLibelle struct {
	ClePublique string `json:"cle_publique,omitempty"`
	Libelle     string `json:"libelle"`
}

// DemandeRetrait : l'appareil à retirer du réseau (une demande refusée).
type DemandeRetrait struct {
	ClePublique string `json:"cle_publique"`
}

// DemandeConnexion : l'un des deux justificatifs, jamais les deux. Un jeton
// Google pour une personne, une clé d'inscription pour une machine.
type DemandeConnexion struct {
	JetonGoogle    string `json:"jeton_google,omitempty"`
	CleInscription string `json:"cle_inscription,omitempty"`
	ClePublique    string `json:"cle_publique"`
	Nom            string `json:"nom"`
	Systeme        string `json:"systeme"`
	// Horodatage (secondes Unix) et Preuve : la preuve que l'appareil détient
	// la clé privée qui va avec ClePublique. Voir Prouver.
	Horodatage int64  `json:"horodatage"`
	Preuve     string `json:"preuve"`
}

type Serveur struct {
	ClePublique string `json:"cle_publique"`
	Point       string `json:"point"`   // hôte:port UDP du tunnel
	Adresse     string `json:"adresse"` // son adresse dans le VPN, 10.77.0.1
}

type ReponseConnexion struct {
	Appareil Appareil `json:"appareil"`
	Reseau   string   `json:"reseau"`  // 10.77.0.0/24
	DNS      string   `json:"dns"`     // le résolveur du VPN
	Domaine  string   `json:"domaine"` // sas.internal
	Serveur  Serveur  `json:"serveur"`
	// Verrou : clé publique Ed25519 du verrou du réseau, si l'admin en a
	// mis un. L'appareil la retient et ne la laisse plus changer.
	Verrou string `json:"verrou,omitempty"`
	// Jeton : à présenter ensuite à l'API. Il n'ouvre pas le tunnel, qui ne
	// se fie qu'aux clés.
	Jeton string `json:"jeton"`
}

type Appareil struct {
	Numero      uint32 `json:"numero"`
	Nom         string `json:"nom"`
	Libelle     string `json:"libelle,omitempty"`
	Adresse     string `json:"adresse"`
	ClePublique string `json:"cle_publique,omitempty"`
	Signature   string `json:"signature,omitempty"` // certificat du verrou
	// Groupe et SignatureExpire font partie du certificat signé : le client
	// en a besoin pour le vérifier.
	Groupe          string    `json:"groupe,omitempty"`
	SignatureExpire time.Time `json:"signature_expire,omitzero"`
	Proprietaire    string    `json:"proprietaire,omitempty"`
	Etiquette       string    `json:"etiquette,omitempty"`
	Systeme         string    `json:"systeme,omitempty"`
	EnLigne         bool      `json:"en_ligne"`
	Moi             bool      `json:"moi,omitempty"`
	Expire          time.Time `json:"expire,omitzero"`
}

// RegleEntrante : ce que ces sources ont le droit d'ouvrir chez l'appareil
// qui la reçoit. Ports : "*", "icmp", "tcp:443", "udp:53", "tcp:8000-8100".
type RegleEntrante struct {
	Sources []string `json:"sources"`
	Ports   []string `json:"ports"`
}

// EtatReseau : tout ce qu'un appareil doit savoir pour configurer son
// tunnel. Il le redemande régulièrement.
type EtatReseau struct {
	Moi     Appareil `json:"moi"`
	Serveur Serveur  `json:"serveur"`
	Verrou  string   `json:"verrou,omitempty"`
	// Pairs : les appareils avec qui une session de bout en bout est
	// permise, dans un sens ou dans l'autre.
	Pairs []Appareil `json:"pairs"`
	// Entrant : ce que chacun peut ouvrir chez moi. Le reste est refusé,
	// sauf les réponses à ce que j'ai ouvert. Sans verrou seulement : avec
	// un verrou, l'appareil ignore ce champ et calcule ses règles lui-même
	// à partir de la politique signée.
	Entrant []RegleEntrante `json:"entrant"`
	// Politique : le fichier de politique tel quel (base64), sa version et
	// la signature du verrou.
	Politique          string `json:"politique,omitempty"`
	PolitiqueVersion   uint64 `json:"politique_version,omitempty"`
	PolitiqueSignature string `json:"politique_signature,omitempty"`
	// Revocations : la liste signée des clés bannies.
	Revocations *ListeRevocations `json:"revocations,omitempty"`
}

// ListeRevocations : les clés que l'admin a bannies, signées ensemble. Un
// appareil garde la plus récente qu'il ait vue, et ne revient jamais en
// arrière.
type ListeRevocations struct {
	Version   uint64   `json:"version"`
	Cles      []string `json:"cles"`
	Signature string   `json:"signature"`
}

// DemandeInvitation : pour qui, et combien de minutes la clé vaut.
type DemandeInvitation struct {
	Utilisateur string `json:"utilisateur"`
	Minutes     int    `json:"minutes"`
}

// ReponseInvitation : la clé d'inscription, à usage unique.
type ReponseInvitation struct {
	Cle    string    `json:"cle"`
	Expire time.Time `json:"expire"`
}

// Invitation : une clé d'inscription en cours, jamais la clé elle-même.
// ID la désigne pour l'annuler, et ne permet pas de s'inscrire. Pour une
// machine (« sasd cle --etiquette »), Etiquette et Nom au lieu
// d'Utilisateur ; Createur est vide pour une clé créée sur le serveur.
type Invitation struct {
	ID          string    `json:"id"`
	Utilisateur string    `json:"utilisateur,omitempty"`
	Etiquette   string    `json:"etiquette,omitempty"`
	Nom         string    `json:"nom,omitempty"`
	Createur    string    `json:"createur,omitempty"`
	Cree        time.Time `json:"cree,omitzero"`
	Expire      time.Time `json:"expire"`
}

// DemandeAnnulation : l'invitation à annuler.
type DemandeAnnulation struct {
	ID string `json:"id"`
}

// Membre : une ligne de equipe.txt. Moi : c'est le compte qui demande.
type Membre struct {
	Adresse string `json:"adresse"`
	Groupe  string `json:"groupe"`
	Moi     bool   `json:"moi,omitempty"`
}

// DemandeMembre : la personne entre dans ce groupe (admins ou equipe), ou
// change de groupe. Groupe vide : elle sort de l'équipe, et ses appareils
// avec elle.
type DemandeMembre struct {
	Adresse string `json:"adresse"`
	Groupe  string `json:"groupe"`
}

type Erreur struct {
	Erreur string `json:"erreur"`
}
