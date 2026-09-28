//go:build unix

package serveur

import (
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"golang.org/x/sys/unix"
)

// attenteVerrouEquipe : variable pour que les tests attendent moins.
var attenteVerrouEquipe = 10 * time.Second

// verrouillerEquipe prend le verrou de fichier que « sas.sh » prend aussi
// (flock) avant de toucher à l'équipe : sans lui, le script et l'appli
// pourraient relire le même equipe.txt en même temps, et le second à
// écrire effacerait le changement du premier.
//
// Le fichier est ouvert en lecture seule, sans suivre de lien symbolique
// (O_NOFOLLOW), sans attendre sur un tube (O_NONBLOCK), et doit être un
// fichier ordinaire. Au bout de dix secondes d'attente, on abandonne :
// mieux vaut un refus qu'une demande bloquée.
func verrouillerEquipe(dossier string) (func(), error) {
	chemin := filepath.Join(dossier, fichierVerrouEquipe)
	fd, err := unix.Open(chemin, unix.O_RDONLY|unix.O_CREAT|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0o644)
	if err != nil {
		return nil, fmt.Errorf("verrou de l'équipe : %w", err)
	}
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil || st.Mode&unix.S_IFMT != unix.S_IFREG {
		unix.Close(fd)
		return nil, fmt.Errorf("verrou de l'équipe : %s n'est pas un fichier ordinaire", chemin)
	}
	limite := time.Now().Add(attenteVerrouEquipe)
	for {
		err := unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			return func() { unix.Close(fd) }, nil
		}
		if !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EINTR) {
			unix.Close(fd)
			return nil, fmt.Errorf("verrou de l'équipe : %w", err)
		}
		if time.Now().After(limite) {
			unix.Close(fd)
			return nil, fmt.Errorf("verrou de l'équipe : tenu depuis plus de %s", attenteVerrouEquipe)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
