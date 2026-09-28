// Les icônes au trait des maquettes (24 × 24, trait 1,7, bouts arrondis),
// reprises telles quelles de leur SVG.
import 'dart:ui' show ImageFilter;

import 'package:flutter/material.dart';
import 'package:flutter_svg/flutter_svg.dart';

import 'theme.dart';

enum Ico {
  accueil('<path d="M3.5 20.5V10L12 3.5l8.5 6.5v10.5"/><path d="M8.5 20.5v-7L12 11l3.5 2.5v7"/>'),
  appareils('<circle cx="12" cy="5" r="2.2"/><circle cx="5" cy="18.5" r="2.2"/><circle cx="19" cy="18.5" r="2.2"/>'
      '<path d="M11 7l-4.8 9.4M13 7l4.8 9.4M7.3 18.5h9.4"/>'),
  reglages('<path d="M4 7h9M18 7h2M4 17h3M12 17h8"/><circle cx="15.5" cy="7" r="2.4"/><circle cx="9.5" cy="17" r="2.4"/>'),
  cadenas('<rect x="5" y="11" width="14" height="9" rx="2.5"/><path d="M8 11V8a4 4 0 0 1 8 0v3"/>'),
  bouclier('<path d="M12 3l7 3v5c0 4.5-3 8-7 10-4-2-7-5.5-7-10V6z"/><path d="M9 12l2 2 4-4"/>'),
  serveur('<rect x="4" y="4" width="16" height="6.5" rx="2"/><rect x="4" y="13.5" width="16" height="6.5" rx="2"/>'
      '<path d="M7.5 7.25h.01M7.5 16.75h.01M11 7.25h5M11 16.75h5"/>'),
  serveurLigne('<rect x="3.5" y="4" width="17" height="6.5" rx="2"/><rect x="3.5" y="13.5" width="17" height="6.5" rx="2"/>'
      '<path d="M7 7.2h.01M7 16.8h.01"/>'),
  maison('<path d="M4 20V10.5L12 4l8 6.5V20"/><path d="M9.5 20v-5.5h5V20"/>'),
  telephone('<rect x="6.5" y="2.5" width="11" height="19" rx="2.8"/><path d="M10.5 18.5h3"/>'),
  pc('<rect x="3" y="4" width="18" height="12" rx="2"/><path d="M8.5 20h7M12 16v4"/>'),
  portable('<rect x="4.5" y="5" width="15" height="10.5" rx="1.5"/><path d="M2 19h20"/>'),
  tablette('<rect x="4.5" y="3" width="15" height="18" rx="2.5"/><path d="M11 18h2"/>'),
  puce('<rect x="6" y="6" width="12" height="12" rx="2"/><rect x="9.5" y="9.5" width="5" height="5" rx="1"/>'
      '<path d="M9 3v3M15 3v3M9 18v3M15 18v3M3 9h3M3 15h3M18 9h3M18 15h3"/>'),
  empreinte('<path d="M2 12C2 6.5 6.5 2 12 2a10 10 0 0 1 8 4"/><path d="M5 19.5C5.5 18 6 15 6 12c0-.7.12-1.37.34-2"/>'
      '<path d="M17.29 21.02c.12-.6.43-2.3.5-3.02"/><path d="M12 10a2 2 0 0 0-2 2c0 1.02-.1 2.51-.26 4"/>'
      '<path d="M8.65 22c.21-.66.45-1.32.57-2"/><path d="M14 13.12c0 2.38 0 6.38-1 8.88"/><path d="M2 16h.01"/>'
      '<path d="M21.8 16c.2-2 .131-5.354 0-6"/><path d="M9 6.8a6 6 0 0 1 9 5.2c0 .47 0 1.17-.02 2"/>'),
  globe('<circle cx="12" cy="12" r="9"/>'
      '<path d="M3 12h18M12 3c2.5 2.7 3.6 5.6 3.6 9s-1.1 6.3-3.6 9c-2.5-2.7-3.6-5.6-3.6-9S9.5 5.7 12 3z"/>'),
  plus('<path d="M12 5v14M5 12h14"/>'),
  etiquette('<path d="M3.5 12.5V4.5h8l9 9-8 8z"/><path d="M8 8h.01"/>'),
  repere('<path d="M12 21s-6.5-5.6-6.5-11a6.5 6.5 0 0 1 13 0c0 5.4-6.5 11-6.5 11z"/><circle cx="12" cy="10" r="2.3"/>'),
  quitter('<path d="M15 4h3a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2h-3"/><path d="M10 8l-4 4 4 4M6 12h10"/>'),
  activite('<path d="M2 12h4l3-8 6 16 3-8h4"/>'),
  coche('<path d="M5 12.5l4.5 4.5L19 7.5"/>'),
  alimentation('<path d="M12 3v8"/><path d="M6.3 6.8a8 8 0 1 0 11.4 0"/>'),
  cle('<circle cx="8" cy="15" r="4"/><path d="M11 12l8.5-8.5M16.5 6.5l2.5 2.5M14 9l2 2"/>'),
  partage('<circle cx="18" cy="5" r="2.5"/><circle cx="6" cy="12" r="2.5"/><circle cx="18" cy="19" r="2.5"/>'
      '<path d="M8.2 10.8l7.6-4.4M8.2 13.2l7.6 4.4"/>'),
  horloge('<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>'),
  info('<circle cx="12" cy="12" r="9"/><path d="M12 11v6M12 7.5h.01"/>'),
  oeil('<path d="M3 3l18 18"/><path d="M10.6 5.1A9.8 9.8 0 0 1 12 5c5 0 8.5 4.2 9.5 7-.4 1.1-1.2 2.5-2.4 3.8M6.6 6.6C4.5 8 3.1 10.1 2.5 12c1 2.8 4.5 7 9.5 7 1.9 0 3.6-.6 5-1.5"/><path d="M9.9 9.9a3 3 0 0 0 4.2 4.2"/>'),
  crayon('<path d="M4 20h4L19 9a2.8 2.8 0 0 0-4-4L4 16z"/><path d="M13.5 6.5l4 4"/>'),
  equipe('<circle cx="9" cy="8" r="3.2"/><path d="M3 20c0-3.3 2.7-6 6-6s6 2.7 6 6"/>'
      '<circle cx="17" cy="9" r="2.5"/><path d="M16.5 14.2c2.6.3 4.5 2.5 4.5 5.3"/>'),
  chevron('<path d="M9.5 6l6 6-6 6"/>'),
  retour('<path d="M14.5 6l-6 6 6 6"/>');

  const Ico(this.corps);
  final String corps;

  String svg(Color c, double trait) {
    final hex = (c.toARGB32() & 0xFFFFFF).toRadixString(16).padLeft(6, '0');
    final o = c.a.toStringAsFixed(3);
    return '<svg viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg" fill="none" stroke="#$hex" '
        'stroke-opacity="$o" stroke-width="$trait" stroke-linecap="round" stroke-linejoin="round">$corps</svg>';
  }
}

/// Une icône au trait. [lueur] : un halo doux de la même couleur
/// (drop-shadow 0 0 4px à 55 %).
class Icone extends StatelessWidget {
  const Icone(this.ico, {super.key, this.couleur = Couleurs.cyan, this.taille = 22, this.lueur = false, this.trait = 1.7});
  final Ico ico;
  final Color couleur;
  final double taille;
  final bool lueur;
  final double trait;

  @override
  Widget build(BuildContext context) {
    final dessin = SvgPicture.string(ico.svg(couleur, trait), width: taille, height: taille);
    if (!lueur) return dessin;
    return SizedBox(
      width: taille,
      height: taille,
      child: Stack(clipBehavior: Clip.none, children: [
        Positioned.fill(
          child: ImageFiltered(
            imageFilter: _flou,
            child: SvgPicture.string(ico.svg(couleur.withValues(alpha: 0.55), trait * 1.4), width: taille, height: taille),
          ),
        ),
        dessin,
      ]),
    );
  }
}

final _flou = ImageFilter.blur(sigmaX: 2.2, sigmaY: 2.2);

/// Le « G » de Google, en couleurs.
const logoGoogle = '<svg viewBox="0 0 48 48" xmlns="http://www.w3.org/2000/svg">'
    '<path fill="#EA4335" d="M24 9.5c3.5 0 6.6 1.2 9 3.6l6.7-6.7C35.6 2.4 30.2 0 24 0 14.6 0 6.6 5.4 2.7 13.2l7.8 6.1C12.4 13.7 17.7 9.5 24 9.5z"/>'
    '<path fill="#4285F4" d="M46.1 24.5c0-1.6-.1-3.1-.4-4.5H24v9h12.4c-.5 2.9-2.2 5.3-4.6 7l7.2 5.6c4.2-3.9 7.1-9.7 7.1-17.1z"/>'
    '<path fill="#FBBC05" d="M10.5 28.7A14.5 14.5 0 0 1 9.5 24c0-1.6.3-3.2.8-4.7l-7.8-6.1A24 24 0 0 0 0 24c0 3.9.9 7.5 2.6 10.8l7.9-6.1z"/>'
    '<path fill="#34A853" d="M24 48c6.5 0 11.9-2.1 15.9-5.8l-7.2-5.6c-2 1.4-4.7 2.3-8.7 2.3-6.3 0-11.6-4.2-13.5-9.9l-7.9 6.1C6.6 42.6 14.6 48 24 48z"/>'
    '</svg>';
