// Captures d'écran de l'appli aux formats du dossier de design, pour
// vérifier la mise en page sans téléphone :
//   flutter test --update-goldens test/captures_test.dart
// Les images atterrissent dans test/captures/.
import 'dart:io';

import 'package:cybersas/composants.dart';
import 'package:cybersas/donnees.dart';
import 'package:cybersas/ecrans/ajout.dart';
import 'package:cybersas/ecrans/demandes.dart';
import 'package:cybersas/ecrans/detail.dart';
import 'package:cybersas/ecrans/equipe.dart';
import 'package:cybersas/main.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';

Future<void> _polices() async {
  for (final famille in ['Syne', 'SpaceGrotesk', 'JetBrainsMono']) {
    final l = FontLoader(famille)..addFont(Future.value(ByteData.sublistView(File('assets/polices/$famille.ttf').readAsBytesSync())));
    await l.load();
  }
}

const _formats = {
  'telephone': Size(360, 780),
  'fold-exterieur': Size(412, 660),
  'fold-deplie-paysage': Size(940, 710),
  'fold-deplie-portrait': Size(710, 940),
};

/// Laisse les SVG et les images se charger, puis avance les animations.
Future<void> _attendre(WidgetTester t, [int ms = 3200]) async {
  await t.runAsync(() => Future<void>.delayed(const Duration(milliseconds: 200)));
  for (var i = 0; i < ms ~/ 100; i++) {
    await t.pump(const Duration(milliseconds: 100));
  }
  await t.runAsync(() => Future<void>.delayed(const Duration(milliseconds: 100)));
  await t.pump();
}

Future<void> _ouvrir(WidgetTester t, Size taille, Reseau r) async {
  t.view.physicalSize = taille * 2;
  t.view.devicePixelRatio = 2;
  await t.pumpWidget(CyberSAS(reseau: r));
  await t.runAsync(() => precacheImage(const AssetImage('assets/icon/icon.png'), t.element(find.byType(Scaffold).first)));
}

void main() {
  setUpAll(() async {
    SharedPreferences.setMockInitialValues({});
    await _polices();
  });

  for (final f in _formats.entries) {
    for (final (onglet, nom) in [(0, 'accueil'), (1, 'appareils'), (2, 'reglages')]) {
      testWidgets('${f.key} $nom', (t) async {
        addTearDown(t.view.reset);
        await _ouvrir(t, f.value, Reseau(inscrit: true));
        if (onglet > 0) await t.tap(find.text(['Accueil', 'Appareils', 'Réglages'][onglet]).last);
        await _attendre(t);
        await expectLater(find.byType(CyberSAS), matchesGoldenFile('captures/${f.key}-$nom.png'));
      });
    }
  }

  testWidgets('telephone accueil-eteint', (t) async {
    addTearDown(t.view.reset);
    await _ouvrir(t, _formats['telephone']!, Reseau(inscrit: true));
    await _attendre(t, 500);
    await t.tap(find.byType(Interrupteur));
    await _attendre(t, 3600);
    await expectLater(find.byType(CyberSAS), matchesGoldenFile('captures/telephone-accueil-eteint.png'));
  });

  for (final f in ['telephone', 'fold-deplie-portrait']) {
    testWidgets('$f connexion', (t) async {
      addTearDown(t.view.reset);
      await _ouvrir(t, _formats[f]!, Reseau());
      await _attendre(t, 600);
      await expectLater(find.byType(CyberSAS), matchesGoldenFile('captures/$f-connexion.png'));
    });
  }

  for (final (nom, ecran) in [
    ('detail', const EcranDetail(adresse: '10.77.0.2') as Widget),
    ('ajout', const EcranAjout()),
    ('demandes', const EcranDemandes()),
    ('equipe', const EcranEquipe()),
  ]) {
    for (final f in ['telephone', 'fold-exterieur']) {
      testWidgets('$f $nom', (t) async {
        addTearDown(t.view.reset);
        await _ouvrir(t, _formats[f]!, Reseau(inscrit: true));
        final nav = t.state<NavigatorState>(find.byType(Navigator).first);
        nav.push(MaterialPageRoute<void>(builder: (_) => ecran));
        await _attendre(t, 800);
        await expectLater(find.byType(CyberSAS), matchesGoldenFile('captures/$f-$nom.png'));
      });
    }
  }

  // L'équipe : sur le Fold déplié, et ses deux fenêtres (ajouter, la
  // fiche d'un membre) sur le téléphone.
  testWidgets('fold-deplie-paysage equipe', (t) async {
    addTearDown(t.view.reset);
    await _ouvrir(t, _formats['fold-deplie-paysage']!, Reseau(inscrit: true));
    t.state<NavigatorState>(find.byType(Navigator).first).push(MaterialPageRoute<void>(builder: (_) => const EcranEquipe()));
    await _attendre(t, 800);
    await expectLater(find.byType(CyberSAS), matchesGoldenFile('captures/fold-deplie-paysage-equipe.png'));
  });
  for (final (nom, bouton) in [('equipe-ajout', 'Ajouter un membre'), ('equipe-membre', 'lea.martin@gmail.com')]) {
    testWidgets('telephone $nom', (t) async {
      addTearDown(t.view.reset);
      await _ouvrir(t, _formats['telephone']!, Reseau(inscrit: true));
      t.state<NavigatorState>(find.byType(Navigator).first).push(MaterialPageRoute<void>(builder: (_) => const EcranEquipe()));
      await _attendre(t, 800);
      await t.tap(find.text(bouton));
      await _attendre(t, 800);
      await expectLater(find.byType(CyberSAS), matchesGoldenFile('captures/telephone-$nom.png'));
    });
  }

  // L'interrupteur bloqué pendant que le tunnel s'éteint.
  testWidgets('telephone accueil-transition', (t) async {
    addTearDown(t.view.reset);
    await _ouvrir(t, _formats['telephone']!, Reseau(inscrit: true));
    await _attendre(t, 500);
    await t.tap(find.byType(Interrupteur));
    await _attendre(t, 1200);
    await expectLater(find.byType(CyberSAS), matchesGoldenFile('captures/telephone-accueil-transition.png'));
    await _attendre(t, 2500);
  });

  testWidgets('telephone verrou', (t) async {
    addTearDown(t.view.reset);
    await _ouvrir(t, _formats['telephone']!, Reseau(inscrit: true)..verrouAppli = true);
    // L'entrée du logo, juste avant la demande d'empreinte (1,5 s) : en
    // test, pas d'empreinte, et l'appli s'ouvrirait d'elle-même.
    await _attendre(t, 1400);
    await expectLater(find.byType(CyberSAS), matchesGoldenFile('captures/telephone-verrou.png'));
    // La demande d'empreinte part ensuite : on la laisse finir.
    await _attendre(t, 800);
  });

  testWidgets('telephone renommer', (t) async {
    addTearDown(t.view.reset);
    await _ouvrir(t, _formats['telephone']!, Reseau(inscrit: true));
    await t.tap(find.text('Réglages').last);
    await _attendre(t, 500);
    await t.tap(find.text('Nom'));
    await _attendre(t, 800);
    await expectLater(find.byType(CyberSAS), matchesGoldenFile('captures/telephone-renommer.png'));
  });

  // Les deux demandes signées : laptop-lea et tab-tristan dans la liste.
  testWidgets('telephone appareils-signes', (t) async {
    addTearDown(t.view.reset);
    final r = Reseau(inscrit: true);
    for (final d in [...r.demandes]) {
      r.signer(d);
    }
    await _ouvrir(t, _formats['telephone']!, r);
    await t.tap(find.text('Appareils').last);
    await _attendre(t);
    await expectLater(find.byType(CyberSAS), matchesGoldenFile('captures/telephone-appareils-signes.png'));
  });

  testWidgets('telephone appareils-coupe', (t) async {
    addTearDown(t.view.reset);
    await _ouvrir(t, _formats['telephone']!, Reseau(inscrit: true)..connecte = false);
    await t.tap(find.text('Appareils').last);
    await _attendre(t);
    await expectLater(find.byType(CyberSAS), matchesGoldenFile('captures/telephone-appareils-coupe.png'));
  });

  // La carte à mi-coupure : réseau déjà gris, fil du Fold en train de se vider.
  testWidgets('telephone appareils-coupure', (t) async {
    addTearDown(t.view.reset);
    final r = Reseau(inscrit: true);
    await _ouvrir(t, _formats['telephone']!, r);
    await t.tap(find.text('Appareils').last);
    await _attendre(t, 800);
    r.basculer(false);
    await _attendre(t, 1900);
    await expectLater(find.byType(CyberSAS), matchesGoldenFile('captures/telephone-appareils-coupure.png'));
    await _attendre(t, 2000);
  });

  // Paysage : un appareil touché, la liste glisse à gauche, le détail à droite.
  testWidgets('fold-deplie-paysage appareils-detail', (t) async {
    addTearDown(t.view.reset);
    await _ouvrir(t, _formats['fold-deplie-paysage']!, Reseau(inscrit: true));
    await t.tap(find.text('Appareils').last);
    await _attendre(t, 800);
    await t.tap(find.text('maison').last);
    await _attendre(t, 1200);
    await expectLater(find.byType(CyberSAS), matchesGoldenFile('captures/fold-deplie-paysage-appareils-detail.png'));
  });

  // Au milieu du décalage : la carte part à gauche, la liste arrive.
  testWidgets('fold-deplie-paysage appareils-glisse', (t) async {
    addTearDown(t.view.reset);
    await _ouvrir(t, _formats['fold-deplie-paysage']!, Reseau(inscrit: true));
    await t.tap(find.text('Appareils').last);
    await _attendre(t, 800);
    await t.tap(find.text('maison').last);
    await t.pump();
    await t.pump(const Duration(milliseconds: 120));
    await expectLater(find.byType(CyberSAS), matchesGoldenFile('captures/fold-deplie-paysage-appareils-glisse.png'));
    await _attendre(t, 800);
  });
}
