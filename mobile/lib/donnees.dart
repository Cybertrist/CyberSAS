// Les données affichées par l'appli : le vrai réseau, tenu par le moteur Go
// (Reseau.reel), ou, pour la démo et les captures, un réseau d'exemple
// qui reprend le labo (serveur, maison, téléphone, poste).
import 'dart:async';
import 'dart:convert';
import 'dart:math';

import 'package:flutter/foundation.dart';
import 'package:flutter/services.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'icones.dart';
import 'moteur.dart';
import 'theme.dart';

enum TypeAppareil {
  serveur('Serveur', Ico.serveur),
  maison('Serveur maison', Ico.maison),
  telephone('Téléphone', Ico.telephone),
  pc('Ordinateur', Ico.pc),
  portable('Portable', Ico.portable),
  tablette('Tablette', Ico.tablette);

  const TypeAppareil(this.libelle, this.ico);
  final String libelle;
  final Ico ico;
}

/// Où en est le certificat d'un appareil, pour le dire sans mentir.
enum EtatCertificat { signe, attente, revoque, expire }

class Certificat {
  const Certificat({required this.debut, required this.fin, required this.empreinte, this.finConnue = true});
  final DateTime debut;
  final DateTime fin;

  /// Faux quand le moteur n'a donné aucune date : fin ne veut rien dire.
  final bool finConnue;

  /// Les 12 premiers caractères du hash de la clé publique, en base64url,
  /// affichés en 3 blocs de 4.
  final List<String> empreinte;

  /// Arrondi au jour supérieur : un certificat qui expire dans 3 jours
  /// moins une heure dit « 3 j », et « 0 » veut vraiment dire expiré.
  int joursRestants(DateTime maintenant) => (fin.difference(maintenant).inMinutes / (24 * 60)).ceil().clamp(0, 99999);

  double progression(DateTime maintenant) {
    final total = fin.difference(debut).inSeconds;
    if (total <= 0) return 0;
    return (fin.difference(maintenant).inSeconds / total).clamp(0.0, 1.0);
  }
}

class Appareil {
  const Appareil({
    required this.nom,
    required this.adresse,
    required this.type,
    required this.proprietaire,
    required this.certificat,
    this.enLigne = false,
    this.moi = false,
    this.ports = const [],
    this.portsConnus = true,
    this.signe = true,
    this.raison = '',
    this.suffixeReseau = '',
    this.libelle = '',
    this.cle = '',
    this.groupe = '',
  });

  /// Son groupe dans l'équipe, tel que le verrou l'a signé (« admins »).
  final String groupe;

  /// Le nom affiché choisi par la personne (« Z Fold8 Tristan »), s'il y
  /// en a un. [nom] reste l'adresse sur le réseau.
  final String libelle;

  /// Sa clé publique, qui le désigne auprès du serveur.
  final String cle;

  /// Le suffixe que le serveur ajoute au nom d'un appareil personnel
  /// (« -tristanjoncour29 ») : il empêche de se faire passer pour une machine,
  /// mais n'a pas à s'afficher.
  final String suffixeReseau;

  /// Le nom à afficher : sans le suffixe du propriétaire.
  String get nomAffiche => libelle.isNotEmpty
      ? libelle
      : suffixeReseau.isNotEmpty && nom.endsWith(suffixeReseau) && nom.length > suffixeReseau.length
          ? nom.substring(0, nom.length - suffixeReseau.length)
          : nom;

  /// Son certificat est valide pour ce téléphone. Sinon, [raison] dit
  /// pourquoi il est écarté (pas encore signé, expiré, révoqué).
  final bool signe;
  final String raison;

  final String nom;
  final String adresse;
  final TypeAppareil type;

  /// « admin » ou le prénom du propriétaire.
  final String proprietaire;
  final Certificat certificat;
  final bool enLigne;
  final bool moi;

  /// Ce que ce téléphone a le droit d'ouvrir chez lui, écrit comme dans la
  /// politique : « tcp:80 », « udp:53 », « icmp », « * » pour tout.
  final List<String> ports;

  /// Faux quand le moteur n'a pas pu vérifier la politique (pas encore
  /// signé, politique refusée) : [ports] vide ne veut alors rien dire.
  final bool portsConnus;

  /// Tout est ouvert (« * »), comme pour un admin.
  bool get toutOuvert => ports.contains('*');

  /// Un port TCP précis est ouvert, seul ou dans une plage.
  bool ouvert(int port) => toutOuvert || ports.any((p) {
        if (!p.startsWith('tcp:')) return false;
        final plage = p.substring(4).split('-');
        final debut = int.tryParse(plage.first);
        final fin = int.tryParse(plage.last);
        return debut != null && fin != null && debut <= port && port <= fin;
      });

  /// L'adresse à ouvrir dans le navigateur, ou null. Le port 80 d'abord :
  /// dans le tunnel, le trafic est déjà chiffré de bout en bout. « * »
  /// seul ne suffit que pour une machine : un téléphone ou un PC ouvert à
  /// l'admin ne sert pas de page pour autant.
  Uri? get web {
    if (moi || type == TypeAppareil.serveur || !signe) return null;
    final explicite = ports.any((p) => p.startsWith('tcp:'));
    if (toutOuvert && !explicite && type != TypeAppareil.maison) return null;
    if (ouvert(80)) return Uri.parse('http://$nomInterne');
    if (ouvert(443)) return Uri.parse('https://$nomInterne');
    return null;
  }

  String get nomInterne => '$nom.sas.internal';

  /// Un appareil personnel porte toujours le nom de son propriétaire en
  /// suffixe (« fold8-tristan ») : il ne peut pas se faire passer pour
  /// « serveur ». Les machines de l'admin n'en ont pas. Même règle que
  /// NomPersonnel côté serveur.
  String get suffixe => proprietaire == 'admin' ? '' : '-${nomPropre(proprietaire.split('@').first)}';

  /// Le propriétaire tel qu'on l'affiche : « Admin », ou son prénom.
  /// proprietaire, lui, est l'adresse : deux Tristan restent deux.
  /// Signé, en attente, révoqué ou expiré, d'après ce que le moteur a
  /// vérifié (signe, raison) et la date du certificat.
  EtatCertificat get etatCertificat {
    if (signe) return EtatCertificat.signe;
    if (raison.contains('révoqué')) return EtatCertificat.revoque;
    if (certificat.finConnue && certificat.fin.isBefore(DateTime.now())) return EtatCertificat.expire;
    return EtatCertificat.attente;
  }

  String get nomProprietaire => proprietaire == 'admin' ? 'Admin' : prenom(proprietaire);

  /// La partie du nom qu'on peut changer.
  String get prefixe => suffixe.isNotEmpty && nom.endsWith(suffixe) ? nom.substring(0, nom.length - suffixe.length) : nom;

  Appareil avecLibelle(String l) => Appareil(
        nom: nom,
        adresse: adresse,
        type: type,
        proprietaire: proprietaire,
        certificat: certificat,
        enLigne: enLigne,
        moi: moi,
        ports: ports,
        portsConnus: portsConnus,
        signe: signe,
        raison: raison,
        suffixeReseau: suffixeReseau,
        libelle: l,
        cle: cle,
        groupe: groupe,
      );

  Appareil renomme(String nouveau) => Appareil(
        nom: nouveau,
        adresse: adresse,
        type: type,
        proprietaire: proprietaire,
        certificat: certificat,
        enLigne: enLigne,
        moi: moi,
        ports: ports,
        portsConnus: portsConnus,
        signe: signe,
        raison: raison,
        suffixeReseau: suffixeReseau,
        libelle: libelle,
        cle: cle,
        groupe: groupe,
      );

  /// Un appareil tel que le moteur le décrit (pont.Pair).
  factory Appareil.duMoteur(Map<String, dynamic> j, {bool portsConnus = false}) {
    final etiquette = j['etiquette'] as String? ?? '';
    final systeme = j['systeme'] as String? ?? '';
    final serveur = j['serveur'] == true;
    final type = switch ((serveur, etiquette, systeme)) {
      (true, _, _) => TypeAppareil.serveur,
      (_, final e, _) when e.isNotEmpty => TypeAppareil.maison,
      (_, _, 'android' || 'ios') => TypeAppareil.telephone,
      _ => TypeAppareil.pc,
    };
    final email = j['proprietaire'] as String? ?? '';
    final expire = DateTime.tryParse(j['expire'] as String? ?? '');
    return Appareil(
      nom: j['nom'] as String? ?? '?',
      adresse: j['adresse'] as String? ?? '',
      type: type,
      // Les machines (serveur, maison) sont celles de l'admin.
      proprietaire: serveur || etiquette.isNotEmpty ? 'admin' : email.toLowerCase(),
      enLigne: j['en_ligne'] == true,
      moi: j['moi'] == true,
      ports: (j['ports'] as List? ?? []).whereType<String>().toList(),
      portsConnus: portsConnus,
      signe: j['signe'] != false,
      raison: j['raison'] as String? ?? '',
      libelle: j['libelle'] as String? ?? '',
      cle: j['cle'] as String? ?? '',
      groupe: j['groupe'] as String? ?? '',
      // Même règle que NomPersonnel côté serveur.
      suffixeReseau: email.isEmpty || serveur || etiquette.isNotEmpty ? '' : '-${nomPropre(email.split('@').first)}',
      certificat: Certificat(
        // Le verrou signe pour 90 jours par défaut.
        debut: (expire ?? DateTime.now()).subtract(const Duration(days: 90)),
        fin: expire ?? DateTime.now(),
        finConnue: expire != null,
        empreinte: (j['empreinte'] as String? ?? '').split('-'),
      ),
    );
  }
  String get fin => '.${adresse.split('.').last}';

  /// L'empreinte de sa clé, en un seul texte : « v8Qe-Lm2T-x0Rw ».
  String get empreinte => certificat.empreinte.join('-');

  /// Ce qui le désigne sans ambiguïté, pour une confirmation ou l'invite
  /// d'empreinte : le nom, c'est le serveur qui le choisit ; l'empreinte et
  /// l'adresse, non.
  String get resume => [
        nomAffiche == nom ? nom : '$nomAffiche ($nom)',
        'Empreinte $empreinte',
        'Adresse ${adresse.isEmpty ? '?' : adresse}',
      ].join('\n');

  /// La couleur de l'appareil : cyan en ligne, gris hors ligne. Une seule
  /// couleur pour tous, c'est le type qui les distingue.
  Color get couleur => enLigne ? Couleurs.cyan : Couleurs.tertiaire;
}

class Demande {
  const Demande({
    required this.nom,
    required this.compte,
    required this.type,
    required this.empreinte,
    this.cle = '',
    this.adresse = '',
    this.groupe = '',
    this.etiquette = '',
    this.fiche = const {},
  });

  /// Sa clé publique.
  final String cle;

  final String nom;
  final String compte;
  final TypeAppareil type;
  final List<String> empreinte;

  /// Ce que le verrou inscrira dans le certificat, tel que le serveur le
  /// propose : l'adresse sur le réseau, le groupe (« admins » donne les
  /// droits d'admin) et, pour une machine, son étiquette.
  final String adresse;
  final String groupe;
  final String etiquette;

  /// La fiche complète, telle que le moteur l'a rendue (pont.Fiche). Elle
  /// repart telle quelle au moteur pour la signature : il signe ce que
  /// l'admin a vu, et refuse si le serveur l'a changée entre-temps.
  final Map<String, dynamic> fiche;

  /// Le verrou signe pour 90 jours.
  static const dureeSignature = 90;

  /// « lea.martin@gmail.com » → « lea » : le prénom sert de suffixe au nom.
  String get proprietaire => nomPropre(compte.split('@').first.split('.').first);

  /// À qui il appartient : une personne, ou une machine de l'admin.
  String get titulaire => etiquette.isNotEmpty ? 'machine « $etiquette »' : (compte.isEmpty ? '?' : compte);

  /// Le texte de l'invite d'empreinte : ce qui sera signé.
  String get resume => [
        nom,
        'Empreinte ${empreinte.join('-')}',
        'Adresse ${adresse.isEmpty ? '?' : adresse}, groupe ${groupe.isEmpty ? '?' : groupe}',
        '$titulaire, $dureeSignature jours',
      ].join('\n');
}

/// Rendu par une opération dont l'utilisateur a fermé l'invite d'empreinte :
/// rien à afficher, on revient simplement.
const operationAnnulee = '';

class CodeInvitation {
  CodeInvitation(this.serveur) : code = _code(), expire = DateTime.now().add(const Duration(minutes: 10));

  final String serveur;
  final String code;
  final DateTime expire;

  String get charge => 'cybersas://invitation?serveur=$serveur&code=$code';

  // Sans 0/O ni 1/I : le code se recopie à la main sans hésiter.
  static String _code() {
    const alphabet = 'ABCDEFGHJKLMNPQRSTUVWXYZ23456789';
    final r = Random.secure();
    String bloc() => List.generate(4, (_) => alphabet[r.nextInt(alphabet.length)]).join();
    return 'SAS-${bloc()}-${bloc()}';
  }
}

/// Une personne de l'équipe : un compte Google et son groupe, « admins »
/// ou « equipe », comme une ligne de equipe.txt sur le serveur.
class Membre {
  const Membre({required this.adresse, required this.groupe, this.moi = false});
  final String adresse;
  final String groupe;

  /// C'est le compte de ce téléphone : l'admin ne change pas son propre
  /// accès, un autre admin doit le faire.
  final bool moi;

  bool get admin => groupe == 'admins';

  /// Même règle que « sas.sh membre » et le serveur.
  static bool adresseValide(String a) => a.length <= 254 && RegExp(r'^[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]+$').hasMatch(a);
}

/// L'état de l'appli, partagé par tous les écrans.
class Reseau extends ChangeNotifier {
  /// Le réseau d'exemple (démo, captures d'écran).
  Reseau({this.inscrit = false}) : reel = false;

  /// Le vrai réseau, tenu par le moteur. Vide jusqu'à [charger].
  Reseau.reel()
      : reel = true,
        inscrit = false,
        connecte = false {
    appareils.clear();
    demandes.clear();
  }

  /// Vrai : les données viennent du moteur (pont/ en Go), pas de l'exemple.
  final bool reel;

  /// Faux tant que l'appareil n'a pas rejoint de réseau : l'appli s'ouvre
  /// alors sur l'écran de connexion.
  bool inscrit;

  /// Le tunnel est ouvert (l'interrupteur).
  bool connecte = true;

  /// Une session est établie avec le serveur. Le tunnel peut être ouvert
  /// sans elle, le temps de la poignée de main ou si le serveur est
  /// injoignable.
  bool serveurJoint = true;

  /// La dernière erreur du moteur, à afficher.
  String erreur = '';

  /// Une erreur de l'appli elle-même (autorisation VPN refusée, moteur
  /// qui refuse de démarrer) : le relevé suivant ne l'efface pas, seul le
  /// prochain appui sur l'interrupteur le fait.
  String erreurLocale = '';

  /// Ce que la carte d'état affiche comme erreur.
  String get erreurAffichee => erreurLocale.isNotEmpty ? erreurLocale : erreur;

  /// Le tunnel est ouvert ET le serveur répond : c'est seulement là que
  /// les autres appareils sont joignables.
  bool get enService => connecte && serveurJoint;

  /// Au-delà de ce délai sans session, « Connexion… » devient « serveur
  /// injoignable ».
  static const delaiConnexion = Duration(seconds: 20);

  /// Tunnel ouvert, mais toujours pas de serveur après le délai.
  bool get tunnelEnPanne =>
      connecte && !serveurJoint && !enTransition && DateTime.now().difference(debutConnexion) > delaiConnexion;

  /// Tunnel coupé, l'API n'a pas répondu à la dernière question : ce que
  /// l'appli sait du réseau (et de son propre certificat) peut être périmé.
  bool serveurInjoignable = false;

  DateTime debutConnexion = DateTime.now().subtract(const Duration(hours: 2, minutes: 14));
  String serveur = 'vpn.exemple.fr';
  String cleVerrou = '';
  String plage = '10.77.0.0/24';
  final protocole = 'Noise IK';
  String compte = 'Tristan';

  /// L'adresse du compte (tristan@exemple.fr), pour s'inviter soi-même.
  String courriel = '';
  bool admin = true;

  /// Cet appareil, d'après son inscription, avant que le moteur ait décrit
  /// le réseau.
  Appareil? _moiInscrit;

  /// L'appareil choisi dans la liste, quand la liste et le détail sont
  /// côte à côte (Fold déplié).
  String selection = 'maison';

  static final _certificat = Certificat(
    debut: DateTime(2026, 8, 25),
    fin: DateTime(2026, 12, 23),
    empreinte: const ['zNYM', 'GYdG', 'paJ5'],
  );

  final appareils = <Appareil>[
    Appareil(
      nom: 'serveur',
      adresse: '10.77.0.1',
      type: TypeAppareil.serveur,
      proprietaire: 'admin',
      enLigne: true,
      certificat: Certificat(debut: DateTime(2026, 8, 25), fin: DateTime(2026, 12, 23), empreinte: const ['v8Qe', 'Lm2T', 'x0Rw']),
    ),
    Appareil(
      nom: 'maison',
      adresse: '10.77.0.2',
      type: TypeAppareil.maison,
      proprietaire: 'admin',
      enLigne: true,
      ports: const ['tcp:80', 'tcp:443'],
      certificat: _certificat,
    ),
    Appareil(
      nom: 'fold8-tristan',
      adresse: '10.77.0.18',
      type: TypeAppareil.telephone,
      proprietaire: 'tristan',
      enLigne: true,
      moi: true,
      certificat: Certificat(debut: DateTime(2026, 8, 25), fin: DateTime(2026, 12, 23), empreinte: const ['uO8I', 'L02v', '_TRr']),
    ),
    Appareil(
      nom: 'pc-tristan',
      adresse: '10.77.0.19',
      type: TypeAppareil.pc,
      proprietaire: 'tristan',
      certificat: Certificat(debut: DateTime(2026, 8, 25), fin: DateTime(2026, 12, 23), empreinte: const ['k3Pw', 'Q9sA', 'x1Mf']),
    ),
  ];

  final demandes = <Demande>[
    const Demande(
      nom: 'laptop-lea',
      compte: 'lea.martin@gmail.com',
      type: TypeAppareil.portable,
      empreinte: ['Qm7X', 'tR2k', '9vLp'],
      adresse: '10.77.0.20',
      groupe: 'equipe',
    ),
    const Demande(
      nom: 'tab-tristan',
      compte: 'tristan@gmail.com',
      type: TypeAppareil.tablette,
      empreinte: ['Hc4W', 'pZ8n', 'Ke3s'],
      adresse: '10.77.0.21',
      groupe: 'admins',
    ),
  ];

  Appareil get moi => appareils.firstWhere((a) => a.moi, orElse: () => _moiInscrit ?? appareils.first);
  int get enLigne => appareils.where((a) => a.enLigne).length;
  int get horsLigne => appareils.length - enLigne;

  /// Les appareils rangés par propriétaire : les machines, les miens, les
  /// autres par ordre alphabétique, ceux sans propriétaire à la fin.
  Map<String, List<Appareil>> get parProprietaire {
    final m = <String, List<Appareil>>{};
    for (final a in appareils) {
      (m[a.proprietaire] ??= []).add(a);
    }
    int rang(String p) => p == 'admin'
        ? 0
        : p == courriel.toLowerCase() || p == compte.toLowerCase()
            ? 1
            : p.isEmpty
                ? 3
                : 2;
    final cles = m.keys.toList()..sort((a, b) => rang(a) != rang(b) ? rang(a) - rang(b) : a.compareTo(b));
    return {for (final k in cles) k: m[k]!};
  }

  Appareil appareil(String nom) => appareils.firstWhere((a) => a.nom == nom, orElse: () => appareils.isEmpty ? moi : appareils.first);
  /// null si l'appareil n'est plus sur le réseau (retiré, révoqué) : on le
  /// dit, plutôt que de montrer un autre appareil à sa place.
  Appareil? parAdresse(String adresse) {
    for (final a in appareils) {
      if (a.adresse == adresse) return a;
    }
    final m = _moiInscrit;
    return m != null && m.adresse == adresse ? m : null;
  }

  /// Vrai pendant qu'on allume ou coupe le tunnel : l'interrupteur ne
  /// répond plus tant que l'animation (et demain le moteur) n'a pas fini.
  bool enTransition = false;
  Timer? _transition;

  /// Le temps que met le tunnel de l'accueil à s'allumer ou s'éteindre.
  static const dureeTransition = Duration(milliseconds: 3200);

  /// Sur le vrai réseau, cet appareil n'est pas (encore) signé par le verrou.
  bool get nonSigne => reel && !moi.signe;

  void basculer(bool v) {
    if (v == connecte || enTransition || (v && nonSigne)) return;
    if (reel) {
      _basculerReel(v);
      return;
    }
    connecte = v;
    if (v) debutConnexion = DateTime.now();
    _attendreAnimation();
    notifyListeners();
  }

  void _attendreAnimation() {
    enTransition = true;
    _transition?.cancel();
    _transition = Timer(dureeTransition, () {
      enTransition = false;
      notifyListeners();
    });
  }

  /// Vrai pendant qu'Android demande l'autorisation VPN : un second toucher
  /// n'ouvre pas une seconde demande.
  bool _demarrage = false;

  Future<void> _basculerReel(bool v) async {
    if (_demarrage) return;
    erreur = '';
    erreurLocale = '';
    if (v) {
      // La première fois, Android demande d'autoriser le VPN.
      _demarrage = true;
      bool autorise;
      try {
        autorise = await Moteur.demarrer();
      } on Exception catch (e) {
        erreurLocale = _messageDe(e, 'Le tunnel ne démarre pas');
        notifyListeners();
        return;
      } finally {
        _demarrage = false;
      }
      if (!autorise) {
        erreurLocale = 'Autorisation VPN refusée';
        notifyListeners();
        return;
      }
      _noterDebut(DateTime.now());
      serveurJoint = false;
      serveurInjoignable = false;
    } else {
      try {
        await Moteur.arreter();
      } on Exception catch (e) {
        erreurLocale = _messageDe(e, 'Le tunnel ne se coupe pas');
        notifyListeners();
        return;
      }
    }
    connecte = v;
    _attendreAnimation();
    notifyListeners();
  }

  // ─── Le vrai réseau ───

  static const _cleDebut = 'debut_tunnel';
  static const _cleAdmin = 'admin';

  /// Le début de la connexion, gardé pour que « Connecté depuis » reste
  /// juste quand l'appli se rouvre sur un tunnel déjà ouvert.
  void _noterDebut(DateTime t) {
    debutConnexion = t;
    SharedPreferences.getInstance().then((p) => p.setInt(_cleDebut, t.millisecondsSinceEpoch));
  }

  String _messageDe(Exception e, String defaut) => e is ErreurMoteur
      ? e.message
      : e is PlatformException
          ? ErreurMoteur.lisible(e.message ?? defaut)
          : defaut;

  Timer? _suivi;

  /// Lit l'inscription : sans elle, l'appli s'ouvre sur la connexion.
  /// Puis suit l'état du moteur toutes les deux secondes.
  Future<void> charger() async {
    final i = await Moteur.inscription();
    if (i == null) {
      inscrit = false;
      notifyListeners();
      return;
    }
    _adopterInscription(i);
    final p = await SharedPreferences.getInstance();
    final debut = p.getInt(_cleDebut);
    debutConnexion = debut == null ? DateTime.now() : DateTime.fromMillisecondsSinceEpoch(debut);
    // Hors ligne au démarrage, l'appli se souvient qu'elle est admin.
    admin = p.getBool(_cleAdmin) ?? admin;
    cleVerrouPresente = await Moteur.verrouPresent();
    await _lireEtat();
    _suivi?.cancel();
    _suivi = Timer.periodic(const Duration(seconds: 2), (_) => _lireEtat());
    notifyListeners();
  }

  void _adopterInscription(Map<String, dynamic> i) {
    inscrit = true;
    serveur = Uri.tryParse(i['serveur'] as String? ?? '')?.host ?? '';
    plage = i['reseau'] as String? ?? plage;
    compte = prenom(i['proprietaire'] as String? ?? '');
    courriel = i['proprietaire'] as String? ?? '';
    admin = i['groupe'] == 'admins';
    cleVerrou = i['verrou'] as String? ?? '';
    _moiInscrit = Appareil(
      nom: i['nom'] as String? ?? '',
      libelle: i['libelle'] as String? ?? '',
      adresse: i['adresse'] as String? ?? '',
      type: TypeAppareil.telephone,
      proprietaire: courriel.toLowerCase(),
      suffixeReseau: '-${nomPropre((i['proprietaire'] as String? ?? '').split('@').first)}',
      moi: true,
      enLigne: false,
      signe: false,
      certificat: Certificat(debut: DateTime.now(), fin: DateTime.now(), finConnue: false, empreinte: (i['empreinte'] as String? ?? '').split('-')),
    );
  }

  /// Vrai pendant un relevé : sans serveur, un appel peut attendre plus
  /// longtemps que les deux secondes entre deux relevés.
  bool _enLecture = false;

  /// L'état du tunnel au relevé précédent (null : aucun relevé encore).
  bool? _enMarcheVu;

  Future<void> _lireEtat() async {
    if (_enLecture) return;
    _enLecture = true;
    try {
      await _relever();
    } finally {
      _enLecture = false;
    }
  }

  Future<void> _relever() async {
    final Map<String, dynamic> e;
    try {
      e = await Moteur.etat();
    } on Exception {
      erreur = 'Le moteur ne répond pas';
      notifyListeners();
      return;
    }
    final enMarche = e['en_marche'] == true;
    // Un tunnel qu'on a vu coupé, puis ouvert sans nous (service relancé
    // par Android) : on ne sait pas depuis quand, on part de maintenant.
    // Au premier relevé, on garde le début enregistré par charger().
    if (enMarche && _enMarcheVu == false && !enTransition && !_demarrage) _noterDebut(DateTime.now());
    _enMarcheVu = enMarche;
    // Pendant qu'on change d'état, l'interrupteur a la main.
    if (!enTransition) connecte = enMarche;
    serveurJoint = e['connecte'] == true;
    erreur = e['erreur'] as String? ?? '';
    List<Map<String, dynamic>> pairs = [];
    var portsConnus = false;
    if (enMarche) {
      // Tunnel ouvert : le moteur sait tout, et dit s'il joint le serveur.
      pairs = (e['pairs'] as List? ?? []).cast<Map<String, dynamic>>();
      portsConnus = e['ports_connus'] == true;
      if (serveurJoint) serveurInjoignable = false;
    } else if (_tours % 3 == 0 || appareils.isEmpty) {
      // Tunnel coupé : ce que le moteur garde est périmé, on demande à
      // l'API (une fois sur trois, toutes les six secondes).
      try {
        final r = await Moteur.reseau();
        pairs = (r['pairs'] as List? ?? []).cast<Map<String, dynamic>>();
        portsConnus = r['ports_connus'] == true;
        serveurInjoignable = false;
      } on ErreurMoteur {
        // Serveur injoignable : on garde ce qu'on sait, et on le dit.
        serveurInjoignable = true;
      }
    }
    if (pairs.isNotEmpty) {
      appareils
        ..clear()
        ..addAll(pairs.map((p) => Appareil.duMoteur(p, portsConnus: portsConnus)));
      if (!appareils.any((a) => a.nom == selection)) selection = appareils.first.nom;
      // Admin : d'après le groupe que le verrou a signé pour cet appareil.
      final m = appareils.where((a) => a.moi);
      if (m.isNotEmpty && m.first.groupe.isNotEmpty) {
        admin = m.first.groupe == 'admins';
        SharedPreferences.getInstance().then((p) => p.setBool(_cleAdmin, admin));
      }
    }
    // Les demandes : une fois sur trois (toutes les six secondes).
    if (_tours++ % 3 == 0) await _lireDemandes();
    notifyListeners();
  }

  int _tours = 0;

  @override
  void dispose() {
    _transition?.cancel();
    _suivi?.cancel();
    super.dispose();
  }

  void choisir(String nom) {
    selection = nom;
    notifyListeners();
  }

  /// Rejoint le réseau de l'invitation. Rend l'erreur à afficher, ou null.
  Future<String?> rejoindre(Invitation i, {String nom = '', String jeton = ''}) async {
    if (!reel) {
      serveur = i.hote;
      cleVerrou = i.verrou;
      inscrit = true;
      notifyListeners();
      return null;
    }
    try {
      await Moteur.rejoindre(i, nom: nom, jeton: jeton);
    } on ErreurMoteur catch (e) {
      return e.message;
    }
    await charger();
    return null;
  }

  /// Quitte le réseau. Rend l'erreur à afficher, ou null.
  Future<String?> quitter() async {
    if (reel) {
      try {
        await Moteur.quitter();
      } on ErreurMoteur catch (e) {
        return e.message;
      }
      _suivi?.cancel();
      appareils.clear();
      demandes.clear();
      connecte = false;
      admin = false;
      cleVerrouPresente = false;
      serveurInjoignable = false;
      erreur = '';
      erreurLocale = '';
      final p = await SharedPreferences.getInstance();
      await p.remove(_cleDebut);
      await p.remove(_cleAdmin);
    }
    inscrit = false;
    notifyListeners();
    return null;
  }

  /// Refusée : la demande disparaît, rien n'entre dans le réseau. Sur le
  /// vrai réseau, l'appareil est retiré du serveur. Rend l'erreur, ou null.
  Future<String?> traiter(Demande d) async {
    if (reel) {
      try {
        await Moteur.retirer(d.cle);
      } on ErreurMoteur catch (e) {
        return e.message;
      }
    }
    demandes.remove(d);
    notifyListeners();
    return null;
  }

  /// Une erreur du coffre, prête à rendre : [operationAnnulee] si
  /// l'invite d'empreinte a été fermée ; si Android a effacé le coffre
  /// (empreinte ajoutée au téléphone), ce téléphone n'a plus la clé.
  String _erreurCoffre(ErreurMoteur e) {
    if (e.annulee) return operationAnnulee;
    if (e.coffrePerdu) {
      cleVerrouPresente = false;
      notifyListeners();
    }
    return e.message;
  }

  /// Signée : sur le vrai réseau, l'invite d'empreinte d'Android ouvre le
  /// coffre pour cette seule signature, et le moteur signe la fiche que
  /// l'admin a vue. Dans la démo, l'appareil reçoit l'adresse de sa demande
  /// et un certificat de 90 jours. Rend l'erreur à afficher,
  /// [operationAnnulee], ou null.
  Future<String?> signer(Demande d) async {
    if (reel) {
      try {
        await Moteur.signer(jsonEncode([d.fiche]), titre: 'Signer ${d.nom}', detail: d.resume);
      } on ErreurMoteur catch (e) {
        return _erreurCoffre(e);
      }
      demandes.remove(d);
      notifyListeners();
      await _lireDemandes();
      return null;
    }
    demandes.remove(d);
    final prises = appareils.map((a) => a.adresse).toSet();
    var adresse = d.adresse;
    if (adresse.isEmpty || prises.contains(adresse)) {
      var n = 2;
      while (prises.contains('10.77.0.$n')) {
        n++;
      }
      adresse = '10.77.0.$n';
    }
    final maintenant = DateTime.now();
    appareils.add(Appareil(
      nom: d.nom,
      adresse: adresse,
      type: d.type,
      proprietaire: d.proprietaire,
      groupe: d.groupe,
      certificat: Certificat(
        debut: maintenant,
        fin: maintenant.add(const Duration(days: Demande.dureeSignature)),
        empreinte: d.empreinte,
      ),
    ));
    notifyListeners();
    return null;
  }

  /// Chacun renomme son appareil ; l'admin, n'importe lequel. Le nom
  /// affiché n'est pas dans le certificat : pas besoin de resigner.
  /// L'admin peut retirer un appareil du réseau, sauf le sien et le serveur.
  bool peutRetirer(Appareil a) => admin && !a.moi && a.type != TypeAppareil.serveur && (!reel || a.cle.isNotEmpty);

  /// Retire [a] du réseau : le serveur l'oublie et ne relaie plus rien pour
  /// lui. Rend l'erreur à afficher, ou null si c'est fait.
  Future<String?> retirerAppareil(Appareil a) async {
    if (reel) {
      try {
        await Moteur.retirer(a.cle);
      } on ErreurMoteur catch (e) {
        return e.message;
      }
    }
    appareils.removeWhere((x) => x.adresse == a.adresse);
    notifyListeners();
    return null;
  }

  /// L'admin peut révoquer un appareil signé : il faut la clé du verrou.
  bool peutRevoquer(Appareil a) => peutRetirer(a) && a.signe && (!reel || cleVerrouPresente);

  /// Révoque [a] : la liste signée par le verrou le bannit pour tous les
  /// appareils, et le serveur l'oublie. L'invite d'empreinte montre ce qui
  /// est révoqué (nom, empreinte, adresse). Rend l'erreur,
  /// [operationAnnulee], ou null.
  Future<String?> revoquerAppareil(Appareil a) async {
    if (reel) {
      try {
        await Moteur.revoquer(a.cle, titre: 'Révoquer ${a.nomAffiche}', detail: a.resume);
      } on ErreurMoteur catch (e) {
        return _erreurCoffre(e);
      }
    }
    appareils.removeWhere((x) => x.adresse == a.adresse);
    notifyListeners();
    return null;
  }

  // ─── L'équipe, pour l'admin ───

  /// L'équipe telle que le serveur la tient (equipe.txt). Dans la démo,
  /// une équipe inventée.
  final membres = <Membre>[
    const Membre(adresse: 'tristan@gmail.com', groupe: 'admins', moi: true),
    const Membre(adresse: 'ana.roux@gmail.com', groupe: 'admins'),
    const Membre(adresse: 'lea.martin@gmail.com', groupe: 'equipe'),
    const Membre(adresse: 'hugo.petit@gmail.com', groupe: 'equipe'),
  ];

  /// Relit l'équipe sur le serveur. Rend l'erreur à afficher, ou null.
  Future<String?> lireEquipe() async {
    if (!reel) return null;
    try {
      _adopterEquipe(await Moteur.equipe());
    } on ErreurMoteur catch (e) {
      return e.message;
    }
    return null;
  }

  void _adopterEquipe(List<Map<String, dynamic>> liste) {
    membres
      ..clear()
      ..addAll([
        for (final m in liste)
          Membre(adresse: m['adresse'] as String? ?? '?', groupe: m['groupe'] as String? ?? '', moi: m['moi'] == true),
      ]);
    notifyListeners();
  }

  /// Met [adresse] dans [groupe] (« admins » ou « equipe ») : elle entre
  /// dans l'équipe, ou change de groupe. [groupe] vide : elle en sort, et
  /// ses appareils sont coupés. Le serveur refuse qu'on change son propre
  /// accès ou qu'on retire le dernier admin ; la démo fait de même. Rend
  /// l'erreur à afficher, ou null.
  Future<String?> changerMembre(String adresse, String groupe) async {
    final a = adresse.trim().toLowerCase();
    if (!Membre.adresseValide(a)) return 'Adresse invalide';
    if (reel) {
      try {
        _adopterEquipe(await Moteur.changerMembre(a, groupe));
      } on ErreurMoteur catch (e) {
        return e.message;
      }
      return null;
    }
    final i = membres.indexWhere((m) => m.adresse == a);
    if (i >= 0 && membres[i].moi) return "On ne change pas son propre accès : un autre admin doit le faire";
    if (i >= 0 && membres[i].admin && groupe != 'admins' && membres.where((m) => m.admin).length <= 1) {
      return "C'est le dernier admin : nommer d'abord un autre admin";
    }
    if (groupe.isEmpty) {
      if (i < 0) return "$a n'est pas dans l'équipe";
      membres.removeAt(i);
    } else if (i >= 0) {
      membres[i] = Membre(adresse: a, groupe: groupe);
    } else {
      membres.add(Membre(adresse: a, groupe: groupe));
    }
    notifyListeners();
    return null;
  }

  /// Un lien d'invitation pour [qui], valable [minutes]. Rend (lien, erreur).
  Future<(String, String?)> inviter(String qui, int minutes) async {
    if (!reel) return (CodeInvitation(serveur).charge, null);
    try {
      return (await Moteur.inviter(qui, minutes), null);
    } on ErreurMoteur catch (e) {
      return ('', e.message);
    }
  }

  bool peutRenommer(Appareil a) => reel ? (a.moi || admin) : (admin || a.proprietaire == compte.toLowerCase());

  /// Renomme [a]. Sur le vrai réseau, c'est le nom affiché qui change, tel
  /// quel (majuscules, espaces) ; dans la démo, le nom devient
  /// « [texte][suffixe] ». Rend l'erreur à afficher, ou null si c'est fait.
  Future<String?> renommer(Appareil a, String texte) async {
    if (reel) {
      final libelle = texte.trim();
      if (libelle.isEmpty) return 'Le nom est vide';
      try {
        await Moteur.libeller(a.moi ? '' : a.cle, libelle);
      } on ErreurMoteur catch (e) {
        return e.message;
      }
      final i = appareils.indexWhere((b) => b.adresse == a.adresse);
      if (i >= 0) appareils[i] = appareils[i].avecLibelle(libelle);
      notifyListeners();
      return null;
    }
    final nouveau = nomPropre(texte) + a.suffixe;
    if (nouveau == a.nom) return null;
    if (appareils.any((b) => b.nom == nouveau)) return '« $nouveau » est déjà pris';
    final i = appareils.indexOf(a);
    if (i < 0) return 'Appareil introuvable';
    appareils[i] = a.renomme(nouveau);
    if (selection == a.nom) selection = nouveau;
    notifyListeners();
    return null;
  }

  // ─── La clé du verrou, pour l'admin ───

  /// La clé du verrou est dans le coffre de ce téléphone : il peut signer.
  bool cleVerrouPresente = false;

  /// Range la clé du verrou collée par l'admin : Android la vérifie, puis
  /// demande l'empreinte qui autorise le rangement. Rend l'erreur à
  /// afficher, [operationAnnulee], ou null.
  Future<String?> importerVerrou(String graine) async {
    try {
      await Moteur.rangerVerrou(graine, titre: 'Ranger la clé du verrou');
    } on ErreurMoteur catch (e) {
      return _erreurCoffre(e);
    }
    cleVerrouPresente = true;
    notifyListeners();
    return null;
  }

  Future<void> oublierVerrou() async {
    await Moteur.effacerVerrou();
    cleVerrouPresente = false;
    notifyListeners();
  }

  /// Les appareils qui attendent une signature, pour l'admin : ceux que le
  /// serveur connaît sans certificat.
  Future<void> _lireDemandes() async {
    if (!reel || !admin) return;
    final List<Map<String, dynamic>> liste;
    try {
      liste = await Moteur.appareils();
    } on ErreurMoteur {
      return;
    }
    final moiCle = moi.cle;
    demandes
      ..clear()
      ..addAll([
        for (final f in liste)
          if (f['signe'] != true && f['cle'] != moiCle)
            Demande(
              nom: (f['libelle'] as String? ?? '').isNotEmpty ? f['libelle'] as String : f['nom'] as String? ?? '?',
              compte: f['proprietaire'] as String? ?? '',
              type: (f['etiquette'] as String? ?? '').isNotEmpty
                  ? TypeAppareil.maison
                  : (f['systeme'] == 'android' ? TypeAppareil.telephone : TypeAppareil.pc),
              empreinte: (f['empreinte'] as String? ?? '').split('-'),
              cle: f['cle'] as String? ?? '',
              adresse: f['adresse'] as String? ?? '',
              groupe: f['groupe'] as String? ?? '',
              etiquette: f['etiquette'] as String? ?? '',
              // Gardée entière : c'est elle qui repart au moteur pour la
              // signature, champ pour champ.
              fiche: f,
            ),
      ]);
    notifyListeners();
  }

  // ─── Réglages de l'appli, gardés sur le téléphone ───

  /// Demander l'empreinte à l'ouverture de l'appli.
  bool verrouAppli = false;

  /// Bloquer les captures et l'aperçu dans les applis récentes.
  bool ecranMasque = false;

  static const _cleVerrou = 'verrou_appli';
  static const _cleEcran = 'ecran_masque';

  Future<void> chargerReglages() async {
    final p = await SharedPreferences.getInstance();
    // Sur le vrai réseau, l'appli s'ouvre à l'empreinte par défaut.
    verrouAppli = p.getBool(_cleVerrou) ?? reel;
    ecranMasque = p.getBool(_cleEcran) ?? false;
    notifyListeners();
  }

  Future<void> reglerVerrou(bool v) async {
    verrouAppli = v;
    notifyListeners();
    await (await SharedPreferences.getInstance()).setBool(_cleVerrou, v);
  }

  Future<void> reglerEcran(bool v) async {
    ecranMasque = v;
    notifyListeners();
    await (await SharedPreferences.getInstance()).setBool(_cleEcran, v);
  }
}

/// Minuscules, chiffres et tirets, 30 caractères au plus : le nom sert de
/// nom DNS. Même règle que NomPropre côté serveur.
String nomPropre(String s) {
  var n = s.trim().toLowerCase().replaceAll(RegExp(r'[^a-z0-9-]+'), '-');
  n = n.replaceAll(RegExp(r'^-+|-+$'), '');
  if (n.length > 30) n = n.substring(0, 30).replaceAll(RegExp(r'-+$'), '');
  return n.isEmpty ? 'appareil' : n;
}

/// « 2 h 14 », « 12 min ».
String duree(Duration d) {
  if (d.inHours > 0) return '${d.inHours} h ${(d.inMinutes % 60).toString().padLeft(2, '0')}';
  return '${d.inMinutes} min';
}

/// La version affichée dans « À propos » (même valeur que pubspec.yaml).
const versionAppli = '0.8.1';

/// « tristan.joncour@gmail.com » → « Tristan » : de quoi nommer quelqu'un
/// sans son nom complet.
String prenom(String email) {
  // Les chiffres de fin ne font pas partie du prénom : « tristan29 ».
  final local = email.split('@').first;
  var p = local.split(RegExp(r'[._+-]')).first.replaceAll(RegExp(r'[0-9]+$'), '');
  // « 29@gmail.com » : pas de prénom, on garde ce qu'il y a.
  if (p.isEmpty) p = local;
  if (p.isEmpty) return '?';
  return p[0].toUpperCase() + p.substring(1).toLowerCase();
}
