//go:build !unix

package serveur

// verrouillerEquipe : sasd ne tourne que sous Linux. Ailleurs (les tests
// sous Windows), muEquipe suffit : il n'y a pas de sas.sh à côté.
func verrouillerEquipe(string) (func(), error) {
	return func() {}, nil
}
