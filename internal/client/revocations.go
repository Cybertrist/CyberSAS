package client

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"math"
	"slices"

	"github.com/Cybertrist/CyberSAS/internal/b64"
	"github.com/Cybertrist/CyberSAS/internal/protocole"
	"github.com/Cybertrist/CyberSAS/internal/verrou"
)

// RevocationsSignees : les clés de la liste, si sa signature par le verrou
// est juste. Chaque clé est décodée en écriture canonique, comme celles
// que les appareils comparent.
func RevocationsSignees(l *protocole.ListeRevocations, cleVerrou ed25519.PublicKey) ([][32]byte, bool) {
	if l == nil {
		return nil, false
	}
	var cles [][32]byte
	for _, c := range l.Cles {
		k, err := cle32(c)
		if err != nil {
			return nil, false
		}
		cles = append(cles, k)
	}
	sig, err := b64.Decoder(l.Signature)
	if err != nil || !verrou.VerifierRevocations(cleVerrou, l.Version, cles, sig) {
		return nil, false
	}
	return cles, true
}

// AllongerRevocations : la liste qui suit, signée par le verrou.
//
// Elle part de ce que l'admin sait déjà sûr : la liste qu'il a retenue
// (version et clés, vérifiées quand il les a adoptées), et celle que sert
// le serveur, seulement si sa signature est juste. Une liste servie mal
// signée est ignorée : sinon un serveur piraté ferait signer à l'admin le
// bannissement d'appareils qu'il n'a jamais choisis, ou pousserait la
// version au plafond pour rendre toute révocation future impossible.
//
// Rend la liste signée et le nombre de clés vraiment nouvelles.
func AllongerRevocations(prive ed25519.PrivateKey, version uint64, retenues []string,
	servie *protocole.ListeRevocations, nouvelles []string) (protocole.ListeRevocations, int, error) {
	liste := slices.Clone(retenues)
	if servie != nil && servie.Version > version {
		if _, ok := RevocationsSignees(servie, prive.Public().(ed25519.PublicKey)); ok {
			version = servie.Version
			for _, c := range servie.Cles {
				if !slices.Contains(liste, c) {
					liste = append(liste, c)
				}
			}
		}
	}
	ajoutees := 0
	for _, c := range nouvelles {
		if c != "" && !slices.Contains(liste, c) {
			liste = append(liste, c)
			ajoutees++
		}
	}
	if ajoutees == 0 {
		return protocole.ListeRevocations{}, 0, errors.New("déjà révoqué")
	}
	if version == math.MaxUint64 {
		return protocole.ListeRevocations{}, 0, errors.New("la liste de révocation est à sa dernière version possible")
	}
	var brutes [][32]byte
	for _, c := range liste {
		k, err := cle32(c)
		if err != nil {
			return protocole.ListeRevocations{}, 0, errors.New("clé illisible dans la liste de révocation")
		}
		brutes = append(brutes, k)
	}
	version++
	return protocole.ListeRevocations{Version: version, Cles: liste,
		Signature: base64.StdEncoding.EncodeToString(verrou.SignerRevocations(prive, version, brutes))}, ajoutees, nil
}
