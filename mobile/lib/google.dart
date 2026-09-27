// La connexion Google : un jeton d'identité, que le serveur vérifie
// (signature de Google, destinataire, adresse de l'équipe). L'appli ne
// garde rien : ni mot de passe, ni jeton après l'inscription.
import 'package:google_sign_in/google_sign_in.dart';

/// Le client Web du projet Google Cloud CyberSAS : le jeton est émis pour
/// lui, et le serveur n'accepte que lui (sas.sh google). Il n'est pas
/// secret.
const clientWebGoogle = '113721225113-go82cqnttopvabihfle9dgjk4h22ph0b.apps.googleusercontent.com';

bool _pret = false;

/// Demande à Google un jeton d'identité. Rend (jeton, null), (null, null)
/// si la personne a fermé la fenêtre, ou (null, erreur à afficher).
Future<(String?, String?)> jetonGoogle() async {
  final g = GoogleSignIn.instance;
  try {
    if (!_pret) {
      await g.initialize(serverClientId: clientWebGoogle);
      _pret = true;
    }
    final compte = await g.authenticate(scopeHint: const ['email']);
    final jeton = compte.authentication.idToken;
    if (jeton == null || jeton.isEmpty) return (null, "Google n'a pas donné de jeton d'identité.");
    return (jeton, null);
  } on GoogleSignInException catch (e) {
    return switch (e.code) {
      GoogleSignInExceptionCode.canceled => (null, null),
      GoogleSignInExceptionCode.interrupted => (null, 'Connexion Google interrompue, réessaie.'),
      _ => (null, 'Connexion Google impossible (${e.code.name}).'),
    };
  } on Exception {
    return (null, 'Connexion Google impossible.');
  }
}

/// Oublie le compte choisi : au prochain appareil, Google redemande lequel.
Future<void> oublierGoogle() async {
  if (!_pret) return;
  try {
    await GoogleSignIn.instance.signOut();
  } on Exception {
    // Rien à oublier.
  }
}
