// La charte de CyberSAS, reprise du logo : un cyan qui glisse vers le bleu
// sur un bleu nuit presque noir. Toutes les couleurs et tous les styles de
// texte de l'appli viennent d'ici (valeurs du dossier de design v3).
import 'package:flutter/material.dart';

abstract final class Couleurs {
  // Fonds.
  static const fond = Color(0xFF04060A);
  static const fondHalo = Color(0xFF07131C);
  static const bloc = Color(0xFF060F16);
  static const surface = Color(0xFF0C1117);
  static const carte = Color(0xBF0C1117); // rgba(12,17,23,.75)
  static const vitre = Color(0xF0090E14); // rgba(9,14,20,.94)
  static const barre = Color(0xD9080D13); // rgba(8,13,19,.85)

  // Traits.
  static const bordure = Color(0xFF2A333D);
  static const separateur = Color(0xFF1A222B);
  static const pointilles = Color(0xFF3A4450);

  // Textes.
  static const texte = Color(0xFFF1F5F9);
  static const secondaire = Color(0xFF94A3B0);
  static const tertiaire = Color(0xFF7C8894);
  static const etiquette = Color(0xFFA3B1BD);

  // La marque.
  static const cyan = Color(0xFF31E7FD);
  static const bleu = Color(0xFF01B9FD);

  // Deux accents seulement, en plus du cyan et du blanc : le vert dit
  // « c'est bon » (en ligne, vérifié, Signer), le rouge dit « attention »
  // (Refuser, Quitter, une demande qui attend).
  static const vert = Color(0xFF3DDC97);
  static const rouge = Color(0xFFFF5C63);
  static const rougeClair = Color(0xFFFF8A8F);

  static const degrade = LinearGradient(colors: [cyan, bleu]);
}

// Polices variables : la graisse se règle par l'axe « wght ».
TextStyle _police(String famille, double taille, double graisse, Color couleur,
        {double espacement = 0, double? hauteur}) =>
    TextStyle(
      fontFamily: famille,
      fontSize: taille,
      fontWeight: FontWeight.values[((graisse / 100).round() - 1).clamp(0, 8)],
      fontVariations: [FontVariation('wght', graisse)],
      color: couleur,
      letterSpacing: espacement,
      height: hauteur,
    );

/// Syne 800 : uniquement pour le mot « CyberSAS ».
TextStyle syne(double taille, {Color couleur = Couleurs.texte}) =>
    _police('Syne', taille, 800, couleur, espacement: -0.01 * taille, hauteur: 1);

/// Space Grotesk : tout le texte courant.
TextStyle texte(double taille,
        {double graisse = 400, Color couleur = Couleurs.texte, double? hauteur, double espacement = 0}) =>
    _police('SpaceGrotesk', taille, graisse, couleur, hauteur: hauteur, espacement: espacement);

/// Un titre de page : 26 à 34, gras, resserré.
TextStyle titre(double taille) =>
    _police('SpaceGrotesk', taille, 700, Couleurs.texte, espacement: -0.035 * taille, hauteur: 1);

/// JetBrains Mono : adresses IP, empreintes.
TextStyle mono(double taille,
        {double graisse = 500, Color couleur = Couleurs.texte, double espacement = 0}) =>
    _police('JetBrainsMono', taille, graisse, couleur, espacement: espacement);

/// Une étiquette en capitales : « RÉSEAU SAS · 10.77.0.0/24 ». 11 au minimum.
TextStyle etiquette({Color couleur = Couleurs.etiquette, double taille = 11, double espacement = 0.14}) =>
    mono(taille, graisse: 400, couleur: couleur, espacement: espacement * taille);

ThemeData themeCyberSAS() => ThemeData(
      brightness: Brightness.dark,
      useMaterial3: true,
      fontFamily: 'SpaceGrotesk',
      scaffoldBackgroundColor: Couleurs.fond,
      splashFactory: InkSparkle.splashFactory,
      colorScheme: const ColorScheme.dark(
        primary: Couleurs.cyan,
        secondary: Couleurs.bleu,
        surface: Couleurs.surface,
        error: Couleurs.rouge,
      ),
      textSelectionTheme: const TextSelectionThemeData(
        cursorColor: Couleurs.cyan,
        selectionColor: Color(0x5531E7FD),
        selectionHandleColor: Couleurs.cyan,
      ),
      snackBarTheme: SnackBarThemeData(
        backgroundColor: const Color(0xFF111923),
        contentTextStyle: texte(14),
        behavior: SnackBarBehavior.floating,
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(14),
          side: const BorderSide(color: Couleurs.bordure),
        ),
      ),
    );
