// L'accessibilité des écrans principaux, en démo :
// - avec les grandes polices du système (échelle 1,3 et 2, que l'appli
//   plafonne à 1,5), rien ne déborde (une RenderFlex qui déborde fait
//   échouer le test) ;
// - chaque cible tactile fait au moins 48 × 48 dp et porte un libellé.
//   flutter test test/accessibilite_test.dart
import 'dart:io';

import 'package:cybersas/composants.dart';
import 'package:cybersas/donnees.dart';
import 'package:cybersas/ecrans/accueil.dart';
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
  'fold-deplie-portrait': Size(710, 940),
  'fold-deplie-paysage': Size(940, 710),
};

/// Les échelles de police testées : « grande » et la plus grande d'Android
/// (ramenée à 1,5 par l'appli).
const _echelles = [1.3, 2.0];

/// Laisse les SVG se charger, puis avance les animations.
Future<void> _attendre(WidgetTester t, [int ms = 1600]) async {
  await t.runAsync(() => Future<void>.delayed(const Duration(milliseconds: 100)));
  for (var i = 0; i < ms ~/ 100; i++) {
    await t.pump(const Duration(milliseconds: 100));
  }
}

Future<void> _ouvrir(WidgetTester t, Size taille, double echelle, Reseau r) async {
  t.view.physicalSize = taille * 2;
  t.view.devicePixelRatio = 2;
  t.platformDispatcher.textScaleFactorTestValue = echelle;
  addTearDown(t.view.reset);
  addTearDown(t.platformDispatcher.clearTextScaleFactorTestValue);
  await t.pumpWidget(CyberSAS(reseau: r));
}

/// Les deux règles d'Android : 48 × 48 dp au moins, et un libellé.
Future<void> _cibles(WidgetTester t) async {
  await expectLater(t, meetsGuideline(androidTapTargetGuideline));
  await expectLater(t, meetsGuideline(labeledTapTargetGuideline));
}

void main() {
  setUpAll(() async {
    SharedPreferences.setMockInitialValues({});
    await _polices();
  });

  for (final f in _formats.entries) {
    for (final e in _echelles) {
      for (final (onglet, nom) in [(0, 'accueil'), (1, 'appareils'), (2, 'reglages')]) {
        testWidgets('${f.key} x$e $nom', (t) async {
          await _ouvrir(t, f.value, e, Reseau(inscrit: true));
          await _attendre(t, 400);
          if (onglet > 0) await t.tap(find.text(['Accueil', 'Appareils', 'Réglages'][onglet]).last);
          await _attendre(t);
          await _cibles(t);
        });
      }

      testWidgets('${f.key} x$e connexion', (t) async {
        await _ouvrir(t, f.value, e, Reseau());
        await _attendre(t, 600);
        await _cibles(t);
      });

      for (final (nom, ecran) in [
        ('detail', const EcranDetail(adresse: '10.77.0.2') as Widget),
        ('ajout', const EcranAjout()),
        ('demandes', const EcranDemandes()),
        ('equipe', const EcranEquipe()),
      ]) {
        testWidgets('${f.key} x$e $nom', (t) async {
          await _ouvrir(t, f.value, e, Reseau(inscrit: true));
          await _attendre(t, 400);
          t.state<NavigatorState>(find.byType(Navigator).first).push(MaterialPageRoute<void>(builder: (_) => ecran));
          await _attendre(t, 800);
          await _cibles(t);
        });
      }
    }
  }

  testWidgets("la police du téléphone est suivie jusqu'à 1,5 fois", (t) async {
    await _ouvrir(t, _formats['telephone']!, 2.0, Reseau(inscrit: true));
    await _attendre(t, 400);
    expect(MediaQuery.textScalerOf(t.element(find.byType(EcranAccueil))).scale(10), 15);
  });

  // La zone agrandie répond au doigt, hors du dessin du bouton : 4 dp sous
  // le retour (38 dp de haut) et 8 dp à droite du crayon (30 dp de côté).
  testWidgets('les zones tactiles agrandies répondent', (t) async {
    await _ouvrir(t, _formats['telephone']!, 1.0, Reseau(inscrit: true));
    await _attendre(t, 400);
    t.state<NavigatorState>(find.byType(Navigator).first).push(MaterialPageRoute<void>(builder: (_) => const EcranDetail(adresse: '10.77.0.2')));
    await _attendre(t, 800);
    final crayon = t.getRect(find.ancestor(of: find.byType(InkResponse), matching: find.byType(ZoneTactile)));
    expect(crayon.width, lessThan(48));
    await t.tapAt(crayon.centerRight + const Offset(8, 0));
    await _attendre(t, 600);
    expect(find.byType(Dialog), findsOneWidget);
    Navigator.of(t.element(find.byType(Dialog))).pop();
    await _attendre(t, 600);
    final retour = t.getRect(find.byType(BoutonRetour));
    expect(retour.height, lessThan(48));
    await t.tapAt(retour.bottomCenter + const Offset(0, 4));
    await _attendre(t, 800);
    expect(find.byType(EcranDetail), findsNothing);
  });

  // Le verrou et la fenêtre de renommage, au téléphone seulement.
  for (final e in _echelles) {
    testWidgets('telephone x$e verrou', (t) async {
      await _ouvrir(t, _formats['telephone']!, e, Reseau(inscrit: true)..verrouAppli = true);
      await _attendre(t, 1400);
      await _cibles(t);
      await _attendre(t, 800);
    });

    testWidgets('telephone x$e renommer', (t) async {
      await _ouvrir(t, _formats['telephone']!, e, Reseau(inscrit: true));
      await _attendre(t, 400);
      await t.tap(find.text('Réglages').last);
      await _attendre(t, 500);
      await t.tap(find.text('Nom'));
      await _attendre(t, 800);
      await _cibles(t);
    });

    // Les fenêtres ajoutées depuis : les invitations en cours, et la fiche
    // d'un membre de l'équipe.
    testWidgets('telephone x$e invitations', (t) async {
      await _ouvrir(t, _formats['telephone']!, e, Reseau(inscrit: true));
      await _attendre(t, 400);
      t.state<NavigatorState>(find.byType(Navigator).first).push(MaterialPageRoute<void>(builder: (_) => const EcranAjout()));
      await _attendre(t, 800);
      await t.ensureVisible(find.textContaining('invitations en cours'));
      await t.tap(find.textContaining('invitations en cours'));
      await _attendre(t, 800);
      await _cibles(t);
    });

    testWidgets('telephone x$e membre', (t) async {
      await _ouvrir(t, _formats['telephone']!, e, Reseau(inscrit: true));
      await _attendre(t, 400);
      t.state<NavigatorState>(find.byType(Navigator).first).push(MaterialPageRoute<void>(builder: (_) => const EcranEquipe()));
      await _attendre(t, 800);
      await t.tap(find.text('lea.martin@gmail.com'));
      await _attendre(t, 800);
      await _cibles(t);
    });
  }
}
