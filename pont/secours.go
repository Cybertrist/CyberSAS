package pont

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"

	"github.com/Cybertrist/CyberSAS/internal/appareil"
	"github.com/Cybertrist/CyberSAS/internal/verrou"
)

// La sauvegarde de secours de la clé du verrou (voir internal/verrou).
//
// Sur le téléphone, la clé ne sort du coffre que déchiffrée pour une seule
// opération, après l'empreinte : la sauvegarde est cette opération-là.
// Kotlin ouvre le coffre et passe la graine à SauverVerrou, comme à
// Signer ; seul le texte chiffré remonte vers Flutter, qui le partage.
// Dans l'autre sens, RestaurerVerrou rend la graine à Kotlin, qui la
// range aussitôt dans le coffre : elle ne passe pas par Flutter non plus.

// SauverVerrou : la sauvegarde de secours de la clé du verrou (sa graine
// en base64, sortie du coffre), chiffrée par phrase. La clé doit être
// celle du verrou retenu, comme pour signer.
func SauverVerrou(dossier, graine, phrase string) (string, error) {
	if err := verrou.VerifierPhrase(phrase); err != nil {
		return "", err
	}
	prive, err := lireVerrou(dossier, graine)
	if err != nil {
		return "", err
	}
	defer clear(prive)
	return verrou.Secours(prive, phrase)
}

// RestaurerVerrou ouvre une sauvegarde de secours avec sa phrase, et rend
// la graine de la clé du verrou en base64 (comme etat/verrou/cle), à
// ranger dans le coffre. Elle doit être celle du verrou que cet appareil a
// retenu : une sauvegarde d'un autre réseau est refusée avant même de
// dériver la phrase.
func RestaurerVerrou(dossier, texte, phrase string) (string, error) {
	annoncee, err := verrou.PubliqueSecours(texte)
	if err != nil {
		return "", err
	}
	e, err := appareil.Stockage{Dossier: dossier}.Lire()
	if err != nil {
		return "", errors.New("pas inscrit")
	}
	attendu, err := verrou.LirePublique(e.Retenu.Verrou)
	if err != nil {
		return "", errors.New("ce réseau n'a pas de verrou")
	}
	if !annoncee.Equal(attendu) {
		return "", errors.New("cette sauvegarde est celle d'un autre verrou que celui de ce réseau")
	}
	prive, err := verrou.OuvrirSecours(texte, phrase)
	if err != nil {
		return "", err
	}
	defer clear(prive)
	return base64.StdEncoding.EncodeToString(prive.Seed()), nil
}

// EmpreinteSecours : l'empreinte du verrou qu'une sauvegarde annonce, lue
// sans la phrase, pour que l'admin la reconnaisse avant de la taper.
func EmpreinteSecours(texte string) (string, error) {
	p, err := verrou.PubliqueSecours(texte)
	if err != nil {
		return "", err
	}
	return verrou.Empreinte(ed25519.PublicKey(p)), nil
}
