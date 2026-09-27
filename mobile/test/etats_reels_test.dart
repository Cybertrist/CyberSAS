// Les états du vrai réseau que la démo ne montre pas : un faux moteur
// répond sur le canal, et l'on vérifie que l'appli dit la vérité.
//   flutter test test/etats_reels_test.dart
// Avec --dart-define=CAPTURES=<dossier>, chaque écran est aussi capturé.
import 'dart:convert';
import 'dart:io';

import 'package:cybersas/donnees.dart';
import 'package:cybersas/main.dart';
import 'package:cybersas/moteur.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';

const _captures = String.fromEnvironment('CAPTURES');
const _canal = MethodChannel('fr.cybersas/moteur');

Future<void> _polices() async {
  for (final famille in ['Syne', 'SpaceGrotesk', 'JetBrainsMono']) {
    final l = FontLoader(famille)..addFont(Future.value(ByteData.sublistView(File('assets/polices/$famille.ttf').readAsBytesSync())));
    await l.load();
  }
}

Map<String, dynamic> _inscription({String email = 'tristan@exemple.fr'}) => {
      'serveur': 'https://vpn.exemple.fr',
      'reseau': '10.77.0.0/24',
      'proprietaire': email,
      'groupe': 'admins',
      'verrou': 'verrou',
      'nom': 'fold8-tristan',
      'libelle': 'Z Fold8 Tristan',
      'adresse': '10.77.0.66',
      'empreinte': 'OBJJ-x1TE-zv9P',
    };

Map<String, dynamic> _pair(String nom, String adresse,
        {String proprietaire = 'tristan@exemple.fr', String libelle = '', bool moi = false, bool signe = true, String raison = '', bool enLigne = true, DateTime? expire}) =>
    {
      'nom': nom,
      'libelle': libelle,
      'adresse': adresse,
      'proprietaire': proprietaire,
      'etiquette': '',
      'groupe': 'admins',
      'systeme': 'android',
      'en_ligne': enLigne,
      'moi': moi,
      'empreinte': 'AAAA-BBBB-CCCC',
      'cle': 'cle-$nom',
      'signe': signe,
      'raison': raison,
      if (expire != null) 'expire': expire.toUtc().toIso8601String(),
    };

final _serveur = {'nom': 'serveur', 'adresse': '10.77.0.1', 'etiquette': 'serveur', 'serveur': true, 'en_ligne': false, 'signe': true};

/// Ouvre l'appli sur un faux moteur. [etat] répond à « etat », [reseau] à
/// « reseau » (null : serveur injoignable).
Future<Reseau> _ouvrir(
  WidgetTester t, {
  required Map<String, dynamic> etat,
  List<Map<String, dynamic>>? reseau,
  Map<String, dynamic>? inscription,
  Map<String, Object> preferences = const {},
}) async {
  t.view.physicalSize = const Size(360, 780) * 2;
  t.view.devicePixelRatio = 2;
  SharedPreferences.setMockInitialValues({'verrou_appli': false, ...preferences});
  t.binding.defaultBinaryMessenger.setMockMethodCallHandler(_canal, (appel) async {
    switch (appel.method) {
      case 'inscription':
        return jsonEncode(inscription ?? _inscription());
      case 'etat':
        return jsonEncode(etat);
      case 'reseau':
        if (reseau == null) throw PlatformException(code: 'moteur', message: 'dial tcp: lookup vpn.exemple.fr: no such host');
        return jsonEncode({'pairs': reseau});
      case 'appareils':
        return '[]';
      case 'coffrePresent':
        return false;
    }
    return null;
  });
  final r = Reseau.reel();
  await t.runAsync(() async {
    await r.chargerReglages();
    await r.charger();
  });
  await t.pumpWidget(CyberSAS(reseau: r));
  await _laisser(t);
  return r;
}

Future<void> _laisser(WidgetTester t) async {
  for (var i = 0; i < 15; i++) {
    await t.pump(const Duration(milliseconds: 100));
  }
}

Future<void> _capturer(WidgetTester t, String nom) async {
  if (_captures.isEmpty) return;
  await expectLater(find.byType(CyberSAS), matchesGoldenFile('$_captures/$nom.png'));
}

Future<void> _fermer(WidgetTester t, Reseau r) async {
  await t.pumpWidget(const SizedBox());
  r.dispose();
  t.view.reset();
}

void main() {
  setUpAll(_polices);

  const coupe = {'en_marche': false, 'connecte': false, 'pairs': <Object>[]};

  testWidgets('révoqué : il le dit, au lieu de « en attente de signature »', (t) async {
    final r = await _ouvrir(t, etat: coupe, reseau: [
      _serveur,
      _pair('fold8-tristan', '10.77.0.66', moi: true, signe: false, raison: 'révoqué par le verrou', expire: DateTime.now().add(const Duration(days: 60))),
    ]);
    expect(find.text('appareil révoqué'), findsOneWidget);
    expect(find.text('Appareil révoqué'), findsOneWidget);
    expect(find.text('en attente de signature'), findsNothing);
    await _capturer(t, 'revoque-accueil');
    await t.tap(find.text('Réglages').last);
    await _laisser(t);
    expect(find.text('révoqué'), findsOneWidget);
    await _capturer(t, 'revoque-reglages');
    await _fermer(t, r);
  });

  testWidgets('serveur injoignable : « Hors ligne », pas « non signé »', (t) async {
    final r = await _ouvrir(t, etat: coupe);
    expect(find.text('Hors ligne'), findsOneWidget);
    expect(find.text('Serveur injoignable'), findsOneWidget);
    await _capturer(t, 'injoignable-accueil');
    await t.tap(find.text('Réglages').last);
    await _laisser(t);
    expect(find.text('serveur injoignable'), findsOneWidget);
    expect(find.textContaining('encore 0 j'), findsNothing);
    await _capturer(t, 'injoignable-reglages');
    await _fermer(t, r);
  });

  testWidgets('tunnel ouvert sans serveur : « serveur injoignable » après le délai', (t) async {
    final r = await _ouvrir(
      t,
      etat: {'en_marche': true, 'connecte': false, 'pairs': [_serveur, _pair('fold8-tristan', '10.77.0.66', moi: true)]},
      preferences: {'debut_tunnel': DateTime.now().subtract(const Duration(minutes: 1)).millisecondsSinceEpoch},
    );
    expect(find.text('Connexion…'), findsOneWidget);
    expect(find.text('serveur injoignable'), findsOneWidget);
    await _capturer(t, 'panne-accueil');
    await _fermer(t, r);
  });

  testWidgets('connecté : la durée vient du vrai début, pas de la démo', (t) async {
    final r = await _ouvrir(
      t,
      etat: {'en_marche': true, 'connecte': true, 'pairs': [_serveur, _pair('fold8-tristan', '10.77.0.66', moi: true, expire: DateTime.now().add(const Duration(days: 3)))]},
      preferences: {'debut_tunnel': DateTime.now().subtract(const Duration(minutes: 5)).millisecondsSinceEpoch},
    );
    expect(find.text('Connecté'), findsOneWidget);
    expect(find.textContaining('2 h 14'), findsNothing);
    // Trois jours avant l'expiration : l'accueil prévient.
    expect(find.text('Expire dans 3 j'), findsOneWidget);
    await _capturer(t, 'connecte-accueil');
    await _fermer(t, r);
  });

  testWidgets('adresse sans prénom et appareil sans propriétaire : pas de plantage', (t) async {
    final r = await _ouvrir(t, etat: coupe, inscription: _inscription(email: '29@gmail.com'), reseau: [
      _serveur,
      _pair('fold8-29', '10.77.0.66', proprietaire: '29@gmail.com', moi: true),
      _pair('inconnu', '10.77.0.9', proprietaire: ''),
      _pair('pc-essai', '10.77.0.10', proprietaire: 'essai@labo.local', signe: false),
    ]);
    await t.tap(find.text('Appareils').last);
    await _laisser(t);
    expect(t.takeException(), isNull);
    await _capturer(t, 'sans-prenom-appareils');
    final r2 = RegExp(r'^(autres appareils|appareils d.essai|mes appareils)$', caseSensitive: false);
    expect(find.textContaining(r2), findsNWidgets(3));
    await t.tap(find.text('Réglages').last);
    await _laisser(t);
    expect(t.takeException(), isNull);
    await _capturer(t, 'sans-prenom-reglages');
    await _fermer(t, r);
  });

  testWidgets('détail d\'un appareil non signé : pas de « verrou vérifié »', (t) async {
    final r = await _ouvrir(t, etat: coupe, reseau: [
      _serveur,
      _pair('fold8-tristan', '10.77.0.66', moi: true),
      _pair('pc-essai', '10.77.0.10', proprietaire: 'essai@labo.local', libelle: 'PC essai', signe: false),
    ]);
    await t.tap(find.text('Appareils').last);
    await _laisser(t);
    await t.tap(find.text('PC essai').last);
    await _laisser(t);
    expect(find.text('Pas encore signé'), findsOneWidget);
    expect(find.text('Certificat signé'), findsNothing);
    await _capturer(t, 'non-signe-detail');
    await _fermer(t, r);
  });

  test('lien du réseau : sans clé, Google fait entrer, mais le verrou est obligatoire', () {
    final reseau = Invitation.lire('cybersas://rejoindre?serveur=https%3A%2F%2Fvpn.exemple.fr&verrou=DkF6TQ%3D');
    expect(reseau, isNotNull);
    expect(reseau!.parGoogle, isTrue);
    expect(Invitation.lire('cybersas://rejoindre?serveur=https%3A%2F%2Fvpn.exemple.fr'), isNull);
    final invitation = Invitation.lire('cybersas://rejoindre?serveur=https%3A%2F%2Fvpn.exemple.fr&cle=sas-abc&verrou=DkF6TQ%3D');
    expect(invitation!.parGoogle, isFalse);
  });
}
