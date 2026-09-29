// Le moteur du tunnel, écrit en Go (pont/ à la racine du dépôt) et tenu
// par TunnelService côté Android. Ici, seulement les appels et la lecture
// des liens d'invitation.
import 'dart:async';
import 'dart:convert';

import 'package:flutter/services.dart';

const _canal = MethodChannel('fr.cybersas/moteur');

/// Une erreur du moteur, prête à afficher.
class ErreurMoteur implements Exception {
  ErreurMoteur(String message, {this.code = 'moteur'}) : message = lisible(message);
  final String message;

  /// Les erreurs réseau de Go (« dial tcp: lookup vpn…: no such host »)
  /// deviennent une phrase ; les autres gardent leur texte, avec une
  /// majuscule et sans le code HTTP entre parenthèses.
  static String lisible(String brut) {
    final m = brut.toLowerCase();
    const reseau = ['no such host', 'connection refused', 'network is unreachable', 'timeout', 'dial tcp', 'connection reset'];
    if (reseau.any(m.contains)) return 'Serveur injoignable : vérifie ta connexion, puis réessaie.';
    final t = brut.replaceFirst(RegExp(r'\s*\(\d{3}\)\s*$'), '').trim();
    if (t.isEmpty) return 'Erreur inconnue';
    return t[0].toUpperCase() + t.substring(1);
  }

  /// « annulee » : l'invite d'empreinte a été fermée. « coffre » : le
  /// coffre ne s'ouvre plus (empreinte ajoutée au téléphone), il a été
  /// effacé. « impossible » : pas d'empreinte sur ce téléphone.
  final String code;

  /// L'utilisateur a fermé l'invite d'empreinte : rien à afficher.
  bool get annulee => code == 'annulee';

  /// Le coffre de la clé du verrou a été effacé par Android.
  bool get coffrePerdu => code == 'coffre';

  @override
  String toString() => message;
}

abstract final class Moteur {
  /// L'inscription de cet appareil, ou null s'il n'a rejoint aucun réseau.
  static Future<Map<String, dynamic>?> inscription() async {
    final s = await _canal.invokeMethod<String>('inscription') ?? '';
    return s.isEmpty ? null : jsonDecode(s) as Map<String, dynamic>;
  }

  /// Rejoint le réseau de l'[invitation]. Rend l'inscription.
  static Future<Map<String, dynamic>> rejoindre(Invitation i, {String nom = '', String jeton = ''}) async {
    try {
      final s = await _canal.invokeMethod<String>('rejoindre', {
        'serveur': i.serveur,
        'verrou': i.verrou,
        'cle': i.cle,
        'jeton': jeton,
        'nom': nom,
        'autorite': i.autorite,
      });
      return jsonDecode(s!) as Map<String, dynamic>;
    } on PlatformException catch (e) {
      throw ErreurMoteur(e.message ?? e.code, code: e.code);
    }
  }

  /// Ouvre le tunnel. La première fois, Android demande l'autorisation :
  /// false si elle est refusée.
  static Future<bool> demarrer() async => await _canal.invokeMethod<bool>('demarrer') ?? false;

  static Future<void> arreter() => _canal.invokeMethod<void>('arreter');

  /// L'état du tunnel et du réseau (voir pont.Vue).
  static Future<Map<String, dynamic>> etat() async {
    final s = await _canal.invokeMethod<String>('etat') ?? '{}';
    return jsonDecode(s) as Map<String, dynamic>;
  }

  static Future<void> quitter() async {
    try {
      await _canal.invokeMethod<void>('quitter');
    } on PlatformException catch (e) {
      throw ErreurMoteur(e.message ?? e.code, code: e.code);
    }
  }

  static Future<T?> _appel<T>(String methode, [Map<String, dynamic>? args]) async {
    try {
      return await _canal.invokeMethod<T>(methode, args);
    } on PlatformException catch (e) {
      throw ErreurMoteur(e.message ?? e.code, code: e.code);
    }
  }

  /// Le nom affiché d'un appareil : le sien si [cle] est vide.
  static Future<void> libeller(String cle, String libelle) => _appel<void>('libeller', {'cle': cle, 'libelle': libelle});

  /// L'état du réseau lu sur l'API, sans tunnel (même forme que [etat]).
  static Future<Map<String, dynamic>> reseau() async =>
      jsonDecode(await _appel<String>('reseau') ?? '{}') as Map<String, dynamic>;

  /// Admin : tous les appareils du serveur, signés ou non (pont.Fiche).
  static Future<List<Map<String, dynamic>>> appareils() async =>
      (jsonDecode(await _appel<String>('appareils') ?? '[]') as List).cast<Map<String, dynamic>>();

  /// La clé du verrou est dans le coffre de ce téléphone.
  static Future<bool> verrouPresent() async {
    try {
      return await _canal.invokeMethod<bool>('coffrePresent') ?? false;
    } on MissingPluginException {
      return false;
    }
  }

  /// Range la clé du verrou dans le coffre. Android vérifie d'abord que
  /// c'est bien celle du verrou de ce réseau, puis ouvre l'invite
  /// d'empreinte ([titre]) : c'est elle qui autorise le rangement. Rend
  /// l'empreinte du verrou. [graine] peut aussi être une sauvegarde de
  /// secours : le moteur l'ouvre avec [phrase], et la clé qui en sort va
  /// droit au coffre.
  static Future<String> rangerVerrou(String graine, {required String titre, String phrase = ''}) async =>
      await _appel<String>('coffreRanger', {'graine': graine, 'titre': titre, 'phrase': phrase}) ?? '';

  /// La sauvegarde de secours de la clé du verrou, chiffrée par [phrase]
  /// (pont.SauverVerrou). L'invite d'empreinte ([titre], [detail]) ouvre le
  /// coffre pour cette seule opération ; seul le texte chiffré revient.
  static Future<String> sauverVerrou(String phrase, {required String titre, required String detail}) async =>
      await _appel<String>('secours', {'phrase': phrase, 'titre': titre, 'detail': detail}) ?? '';

  static Future<void> effacerVerrou() => _appel<void>('coffreEffacer');

  /// Signe ces [fiches] (JSON, telles que [appareils] les a rendues) avec
  /// la clé du coffre. L'invite d'empreinte d'Android ([titre], [detail])
  /// ouvre le coffre pour cette seule signature ; le moteur refuse si le
  /// serveur a changé une fiche entre-temps.
  static Future<int> signer(String fiches, {required String titre, required String detail}) async =>
      await _appel<int>('signer', {'fiches': fiches, 'titre': titre, 'detail': detail}) ?? 0;

  /// Admin : révoque ces appareils (clés séparées par des virgules) avec la
  /// clé du verrou, sortie du coffre par l'invite d'empreinte. Rend la
  /// version de la nouvelle liste.
  static Future<int> revoquer(String cles, {required String titre, required String detail}) async =>
      await _appel<int>('revoquer', {'cles': cles, 'titre': titre, 'detail': detail}) ?? 0;

  /// Admin : un lien d'invitation pour un membre de l'équipe.
  /// Le lien permanent du réseau (serveur et verrou, sans clé) : avec lui
  /// et un compte Google de l'équipe, un appareil demande à entrer.
  static Future<String> lienReseau() async => await _appel<String>('lienReseau') ?? '';

  static Future<String> inviter(String utilisateur, int minutes) async =>
      await _appel<String>('inviter', {'utilisateur': utilisateur, 'minutes': minutes}) ?? '';

  /// Admin : les invitations pas encore utilisées (pont.EnCours), sans
  /// leur clé.
  static Future<List<Map<String, dynamic>>> invitations() async =>
      (jsonDecode(await _appel<String>('invitations') ?? '[]') as List).cast<Map<String, dynamic>>();

  /// Admin : annule l'invitation [id] ; elle ne fait plus entrer personne.
  static Future<void> annulerInvitation(String id) => _appel<void>('annulerInvitation', {'id': id});

  /// Admin : retire un appareil du serveur.
  static Future<void> retirer(String cle) => _appel<void>('retirer', {'cle': cle});

  /// Admin : l'équipe du serveur (pont.Membre : adresse, groupe, moi).
  static Future<List<Map<String, dynamic>>> equipe() async =>
      (jsonDecode(await _appel<String>('equipe') ?? '[]') as List).cast<Map<String, dynamic>>();

  /// Admin : met [adresse] dans [groupe] (admins ou equipe) ; groupe vide,
  /// elle sort de l'équipe. Rend la nouvelle équipe, comme [equipe].
  static Future<List<Map<String, dynamic>>> changerMembre(String adresse, String groupe) async =>
      (jsonDecode(await _appel<String>('changerMembre', {'adresse': adresse, 'groupe': groupe}) ?? '[]') as List)
          .cast<Map<String, dynamic>>();

  /// Le nom du téléphone dans ses réglages (« Galaxy Z Fold8 »), sinon
  /// son modèle.
  static Future<String> nomAppareil() async {
    try {
      return await _canal.invokeMethod<String>('nomAppareil') ?? '';
    } on MissingPluginException {
      return '';
    }
  }

  /// Le lien d'invitation qui a ouvert l'appli, s'il y en a un.
  static Future<Invitation?> lienInitial() async {
    try {
      final s = await _canal.invokeMethod<String>('lien');
      return s == null ? null : Invitation.lire(s);
    } on MissingPluginException {
      return null;
    }
  }

  /// Les liens d'invitation ouverts pendant que l'appli tourne.
  static final liens = StreamController<Invitation>.broadcast();

  static void ecouter() {
    _canal.setMethodCallHandler((appel) async {
      if (appel.method == 'lien' && appel.arguments is String) {
        final i = Invitation.lire(appel.arguments as String);
        if (i != null) liens.add(i);
      }
    });
  }
}

/// Une invitation : ce que « sas.sh invitation » donne, sous forme de lien
///   https://vpn.exemple.fr/rejoindre#serveur=…&cle=…&verrou=…[&autorite=…]
/// (ou, plus ancien, cybersas://rejoindre?serveur=…).
class Invitation {
  const Invitation({required this.serveur, required this.cle, this.verrou = '', this.autorite = ''});

  /// L'adresse de l'API, https://vpn.exemple.fr.
  final String serveur;

  /// La clé d'inscription, à usage unique. Vide : c'est le lien du
  /// réseau, et la personne se connecte avec son compte Google.
  final String cle;

  /// Le lien du réseau : pas de clé, Google fait entrer.
  bool get parGoogle => cle.isEmpty;

  /// La clé publique du verrou : l'appareil la retient dès le départ.
  final String verrou;

  /// L'autorité du labo, en PEM ; vide avec un vrai certificat.
  final String autorite;

  /// Trois formes :
  ///   https://vpn.exemple.fr/rejoindre#serveur=…&cle=…&verrou=…
  ///   cybersas://rejoindre?serveur=…&cle=…&verrou=…
  /// et l'une ou l'autre collée à la main. La forme https ne se lit que
  /// dans le fragment, que le navigateur n'envoie jamais au serveur : des
  /// paramètres dans la requête (?serveur=…) sont refusés, comme un lien
  /// dont l'hôte n'est pas celui du serveur qu'il annonce.
  static Invitation? lire(String texte) {
    final u = Uri.tryParse(texte.trim());
    if (u == null) return null;
    final Map<String, String> p;
    if (u.scheme == 'cybersas' && u.host == 'rejoindre') {
      p = u.queryParameters;
    } else if (u.scheme == 'https' && u.path == '/rejoindre' && !u.hasQuery && u.userInfo.isEmpty && u.hasFragment) {
      try {
        p = Uri.splitQueryString(u.fragment);
      } on ArgumentError {
        return null;
      } on FormatException {
        return null;
      }
      final s = Uri.tryParse(p['serveur'] ?? '');
      if (s == null || s.scheme != 'https' || s.host.isEmpty || s.userInfo.isNotEmpty || s.host != u.host || s.port != u.port) return null;
    } else {
      return null;
    }
    final serveur = p['serveur'] ?? '';
    final cle = p['cle'] ?? '';
    // Sans clé, le lien du réseau doit au moins donner le verrou : c'est
    // lui qui garantit qu'on parle au bon réseau.
    if (!serveur.startsWith('https://') || (cle.isEmpty && (p['verrou'] ?? '').isEmpty)) return null;
    var autorite = '';
    if ((p['autorite'] ?? '').isNotEmpty) {
      try {
        autorite = utf8.decode(base64.decode(p['autorite']!));
      } on FormatException {
        return null;
      }
    }
    return Invitation(serveur: serveur, cle: cle, verrou: p['verrou'] ?? '', autorite: autorite);
  }

  /// « vpn.exemple.fr ».
  String get hote => Uri.parse(serveur).host;
}
