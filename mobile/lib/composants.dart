// Les pièces réutilisées par tous les écrans.
import 'dart:ui' as ui;

import 'package:flutter/material.dart';
import 'package:flutter/rendering.dart';

import 'donnees.dart';
import 'icones.dart';
import 'theme.dart';

/// « CyberSAS », « SAS » en cyan.
class Marque extends StatelessWidget {
  const Marque({super.key, this.taille = 24});
  final double taille;

  @override
  Widget build(BuildContext context) => Text.rich(
    TextSpan(
      children: [
        TextSpan(text: 'Cyber', style: syne(taille)),
        TextSpan(
          text: 'SAS',
          style: syne(taille, couleur: Couleurs.cyan),
        ),
      ],
    ),
    maxLines: 1,
    softWrap: false,
  );
}

/// Une cible tactile d'au moins 48 × 48 dp, comme le demande Android, sans
/// rien changer au dessin ni à la mise en page : la zone qui répond au
/// doigt (et que voit le lecteur d'écran) déborde autour de l'enfant, dans
/// les marges, au lieu de l'agrandir. Un toucher dans ce débord est rendu
/// au bord de l'enfant le plus proche. Quand un parent plus petit que la
/// zone (un Align, une ligne) arrête le toucher avant elle,
/// [RattrapageTactile] le lui rend.
class ZoneTactile extends SingleChildRenderObjectWidget {
  const ZoneTactile({super.key, required Widget super.child, this.minimum = 48});
  final double minimum;

  @override
  RenderObject createRenderObject(BuildContext context) => _RenduZoneTactile(minimum);

  @override
  void updateRenderObject(BuildContext context, RenderObject renderObject) => (renderObject as _RenduZoneTactile).minimum = minimum;
}

class _RenduZoneTactile extends RenderProxyBox {
  _RenduZoneTactile(this._minimum);
  double _minimum;
  set minimum(double v) {
    if (v == _minimum) return;
    _minimum = v;
    markNeedsSemanticsUpdate();
  }

  /// Les zones à l'écran, pour [RattrapageTactile].
  static final _posees = <_RenduZoneTactile>{};

  @override
  void attach(PipelineOwner owner) {
    super.attach(owner);
    _posees.add(this);
  }

  @override
  void detach() {
    _posees.remove(this);
    super.detach();
  }

  /// L'enfant, agrandi de ce qui lui manque pour faire [_minimum] de côté.
  Rect get _zone {
    final dx = ((_minimum - size.width) / 2).clamp(0.0, _minimum);
    final dy = ((_minimum - size.height) / 2).clamp(0.0, _minimum);
    return Rect.fromLTRB(-dx, -dy, size.width + dx, size.height + dy);
  }

  @override
  bool hitTest(BoxHitTestResult result, {required Offset position}) {
    if (child == null || !_zone.contains(position)) return false;
    final dedans = Offset(position.dx.clamp(0.0, size.width - 0.01), position.dy.clamp(0.0, size.height - 0.01));
    if (!child!.hitTest(result, position: dedans)) return false;
    result.add(BoxHitTestEntry(this, position));
    return true;
  }

  @override
  Rect get semanticBounds => _zone;

  // Un seul nœud pour le lecteur d'écran, à la taille de la zone : le
  // bouton, son libellé et son action.
  @override
  void describeSemanticsConfiguration(SemanticsConfiguration config) {
    super.describeSemanticsConfiguration(config);
    config
      ..isSemanticBoundary = true
      ..isMergingSemanticsOfDescendants = true;
  }
}

/// Posé une fois autour de l'appli : un toucher qui tombe dans le débord
/// d'une [ZoneTactile] sans rien toucher d'autre (le fond, une marge) va à
/// cette zone. Ce qui se touche déjà (un autre bouton, une ligne, un champ)
/// garde la priorité ; entre deux zones, la plus proche l'emporte.
class RattrapageTactile extends SingleChildRenderObjectWidget {
  const RattrapageTactile({super.key, required Widget super.child});

  @override
  RenderObject createRenderObject(BuildContext context) => _RenduRattrapage();
}

class _RenduRattrapage extends RenderProxyBox {
  @override
  bool hitTestChildren(BoxHitTestResult result, {required Offset position}) {
    final enfant = child;
    if (enfant == null) return false;
    final essai = BoxHitTestResult();
    enfant.hitTest(essai, position: position);
    final zone = _touchable(essai) ? null : _zoneSous(enfant, position);
    if (zone != null) {
      result.addWithPaintTransform(
        transform: zone.getTransformTo(this),
        position: position,
        hitTest: (r, p) => zone.hitTest(r, position: p),
      );
    }
    // Le reste du chemin suit : un défilement qui part du débord défile.
    return enfant.hitTest(result, position: position) || zone != null;
  }

  static bool _touchable(HitTestResult r) => r.path.any((e) {
    final c = e.target;
    return c is _RenduZoneTactile || c is RenderEditable || (c is RenderSemanticsGestureHandler && (c.onTap != null || c.onLongPress != null));
  });

  /// La zone dont le débord contient [position], si elle est bien à l'écran
  /// et touchable (pas sous une fenêtre, pas dans un onglet caché).
  _RenduZoneTactile? _zoneSous(RenderBox enfant, Offset position) {
    _RenduZoneTactile? choisie;
    var ecart = double.infinity;
    for (final z in _RenduZoneTactile._posees) {
      if (!z.attached || !z.hasSize || z.child == null || z.owner != owner) continue;
      final versIci = z.getTransformTo(this);
      final inverse = Matrix4.tryInvert(versIci);
      if (inverse == null) continue;
      final local = MatrixUtils.transformPoint(inverse, position);
      if (!z._zone.contains(local)) continue;
      final bord = Offset(local.dx.clamp(0.0, z.size.width), local.dy.clamp(0.0, z.size.height));
      final d = (local - bord).distance;
      if (d >= ecart) continue;
      final verif = BoxHitTestResult();
      enfant.hitTest(verif, position: MatrixUtils.transformPoint(versIci, z.size.center(Offset.zero)));
      if (!verif.path.any((e) => e.target == z)) continue;
      choisie = z;
      ecart = d;
    }
    return choisie;
  }
}

/// Une boîte au fond sombre et à la bordure d'1 dp en dégradé
/// (padding-box + border-box des maquettes).
class Bordee extends StatelessWidget {
  const Bordee({
    super.key,
    required this.child,
    required this.bordure,
    this.fond = Couleurs.vitre,
    this.rayon = 20,
    this.padding = EdgeInsets.zero,
    this.halo,
    this.onTap,
    this.largeur = 1,
  });
  final Widget child;
  final Gradient bordure;
  final Color fond;
  final double rayon;
  final EdgeInsetsGeometry padding;
  final List<BoxShadow>? halo;
  final VoidCallback? onTap;
  final double largeur;

  @override
  Widget build(BuildContext context) {
    final r = BorderRadius.circular(rayon);
    final boite = Container(
      decoration: BoxDecoration(borderRadius: r, gradient: bordure, boxShadow: halo),
      padding: EdgeInsets.all(largeur),
      child: Material(
        color: fond,
        borderRadius: BorderRadius.circular(rayon - largeur),
        clipBehavior: Clip.antiAlias,
        child: InkWell(
          onTap: onTap,
          splashColor: Couleurs.cyan.withValues(alpha: 0.08),
          highlightColor: Couleurs.cyan.withValues(alpha: 0.04),
          child: Padding(padding: padding, child: child),
        ),
      ),
    );
    return onTap == null ? boite : ZoneTactile(child: boite);
  }
}

/// Les bordures en dégradé des maquettes.
abstract final class Bords {
  static const cyan = LinearGradient(colors: [Couleurs.cyan, Couleurs.bleu]);
  static const reflet = LinearGradient(
    begin: Alignment.topLeft,
    end: Alignment.bottomRight,
    colors: [Color(0x8C31E7FD), Couleurs.bordure, Color(0x4D31E7FD)],
    stops: [0, 0.45, 1],
  );
  static LinearGradient accent(Color c) => LinearGradient(
    begin: Alignment.topLeft,
    end: Alignment.bottomRight,
    colors: [c.withValues(alpha: 0.6), Couleurs.bordure, c.withValues(alpha: 0.25)],
    stops: const [0, 0.5, 1],
  );
  static const plat = LinearGradient(colors: [Couleurs.bordure, Couleurs.bordure]);
}

/// Halo des cartes actives : 0 0 30px -14px rgba(49,231,253,.55).
List<BoxShadow> haloCarte([Color c = Couleurs.cyan, double a = 0.55]) => [
  BoxShadow(color: c.withValues(alpha: a), blurRadius: 30, spreadRadius: -14),
];

/// Une carte simple : fond translucide, bordure #2A333D.
class Carte extends StatelessWidget {
  const Carte({
    super.key,
    required this.child,
    this.padding = EdgeInsets.zero,
    this.rayon = 18,
    this.onTap,
    this.fond = Couleurs.carte,
    this.bord = Couleurs.bordure,
  });
  final Widget child;
  final EdgeInsetsGeometry padding;
  final double rayon;
  final VoidCallback? onTap;
  final Color fond;
  final Color bord;

  @override
  Widget build(BuildContext context) {
    final carte = Material(
      color: fond,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(rayon),
        side: BorderSide(color: bord),
      ),
      clipBehavior: Clip.antiAlias,
      child: InkWell(
        onTap: onTap,
        splashColor: Couleurs.cyan.withValues(alpha: 0.08),
        highlightColor: Couleurs.cyan.withValues(alpha: 0.04),
        child: Padding(padding: padding, child: child),
      ),
    );
    return onTap == null ? carte : ZoneTactile(child: carte);
  }
}

/// Une étiquette mono en capitales.
class Etiquette extends StatelessWidget {
  const Etiquette(this.texte, {super.key, this.couleur = Couleurs.etiquette, this.taille = 11});
  final String texte;
  final Color couleur;
  final double taille;

  @override
  Widget build(BuildContext context) => Text(
    texte.toUpperCase(),
    style: etiquette(couleur: couleur, taille: taille),
    maxLines: 1,
    softWrap: false,
    overflow: TextOverflow.fade,
  );
}

/// Une puce : « ADMIN », « :443 », « EN ATTENTE »…
class Puce extends StatelessWidget {
  const Puce(this.libelle, {super.key, this.couleur = Couleurs.cyan, this.fond = true, this.monoPolice = true, this.bord});
  final String libelle;
  final Color couleur;
  final bool fond;
  final bool monoPolice;
  final Color? bord;

  @override
  Widget build(BuildContext context) => Container(
    padding: EdgeInsets.symmetric(horizontal: monoPolice ? 7 : 8, vertical: monoPolice ? 2.5 : 3),
    decoration: BoxDecoration(
      color: fond ? couleur.withValues(alpha: 0.06) : null,
      borderRadius: BorderRadius.circular(monoPolice ? 7 : 8),
      border: Border.all(color: bord ?? couleur.withValues(alpha: 0.38)),
    ),
    child: Text(
      libelle,
      style: monoPolice
          ? mono(11, graisse: 400, couleur: couleur, espacement: 0.66)
          : texte(11.5, graisse: 600, couleur: couleur),
      maxLines: 1,
      softWrap: false,
    ),
  );
}

/// « :80 » : un port ouvert, écrit comme dans la politique (« tcp:80 »,
/// « udp:53 », « icmp »). Le TCP, le plus courant, perd son préfixe.
class PucePort extends StatelessWidget {
  const PucePort(this.port, {super.key});
  final String port;

  @override
  Widget build(BuildContext context) => Container(
    padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
    decoration: BoxDecoration(
      borderRadius: BorderRadius.circular(6),
      border: Border.all(color: Couleurs.cyan.withValues(alpha: 0.32)),
    ),
    child: Text(port.startsWith('tcp:') ? port.substring(3) : port, style: mono(11.5, graisse: 400, couleur: Couleurs.cyan)),
  );
}

/// Un avatar rond : « T ».
class Avatar extends StatelessWidget {
  const Avatar({super.key, required this.lettre, this.taille = 36, this.plein = false, this.onTap});
  final String lettre;
  final double taille;
  final bool plein;
  final VoidCallback? onTap;

  @override
  Widget build(BuildContext context) {
    final t = Text(
      lettre,
      style: texte(taille * (plein ? 0.4 : 0.39), graisse: plein ? 700 : 600, couleur: plein ? Couleurs.fond : Couleurs.texte),
    );
    final Widget rond = plein
        ? Container(
            width: taille,
            height: taille,
            alignment: Alignment.center,
            decoration: const BoxDecoration(
              shape: BoxShape.circle,
              gradient: LinearGradient(
                begin: Alignment.topLeft,
                end: Alignment.bottomRight,
                colors: [Couleurs.cyan, Couleurs.bleu],
              ),
            ),
            child: t,
          )
        : Container(
            width: taille,
            height: taille,
            padding: const EdgeInsets.all(1),
            decoration: const BoxDecoration(
              shape: BoxShape.circle,
              gradient: LinearGradient(
                begin: Alignment.topLeft,
                end: Alignment.bottomRight,
                colors: [Couleurs.cyan, Couleurs.bordure],
                stops: [0, 0.6],
              ),
            ),
            child: Container(
              alignment: Alignment.center,
              decoration: const BoxDecoration(shape: BoxShape.circle, color: Color(0xFF0A1219)),
              child: t,
            ),
          );
    if (onTap == null) return rond;
    return ZoneTactile(child: GestureDetector(onTap: onTap, behavior: HitTestBehavior.opaque, child: rond));
  }
}

/// La case d'icône d'un appareil : fond teinté à 8 % et bordure à 32 % en
/// ligne, pointillés gris hors ligne, pointillés colorés en attente.
enum EtatIcone { enLigne, horsLigne, attente }

class CaseIcone extends StatelessWidget {
  const CaseIcone(
    this.ico, {
    super.key,
    required this.couleur,
    this.etat = EtatIcone.enLigne,
    this.taille = 34,
    this.halo = false,
  });
  final Ico ico;
  final Color couleur;
  final EtatIcone etat;
  final double taille;
  final bool halo;

  factory CaseIcone.de(Appareil a, {double taille = 34}) =>
      CaseIcone(a.type.ico, couleur: a.couleur, etat: a.enLigne ? EtatIcone.enLigne : EtatIcone.horsLigne, taille: taille);

  @override
  Widget build(BuildContext context) {
    final r = taille * 0.3;
    final icone = Icone(
      ico,
      couleur: etat == EtatIcone.horsLigne ? Couleurs.tertiaire : couleur,
      taille: taille * 0.54,
      lueur: etat != EtatIcone.horsLigne,
    );
    if (etat == EtatIcone.enLigne) {
      return Container(
        width: taille,
        height: taille,
        alignment: Alignment.center,
        decoration: BoxDecoration(
          color: couleur.withValues(alpha: 0.08),
          borderRadius: BorderRadius.circular(r),
          border: Border.all(color: couleur.withValues(alpha: 0.32)),
          boxShadow: halo ? [BoxShadow(color: couleur.withValues(alpha: 0.6), blurRadius: 14, spreadRadius: -4)] : null,
        ),
        child: icone,
      );
    }
    final bord = etat == EtatIcone.attente ? couleur.withValues(alpha: 0.6) : Couleurs.pointilles;
    return CustomPaint(
      painter: Pointilles(bord, r),
      child: Container(
        width: taille,
        height: taille,
        alignment: Alignment.center,
        decoration: BoxDecoration(
          color: etat == EtatIcone.attente ? couleur.withValues(alpha: 0.08) : Couleurs.surface,
          borderRadius: BorderRadius.circular(r),
        ),
        child: icone,
      ),
    );
  }
}

/// Une bordure en pointillés, arrondie.
class Pointilles extends CustomPainter {
  Pointilles(this.couleur, this.rayon, {this.tiret = 3, this.espace = 3, this.largeur = 1});
  final Color couleur;
  final double rayon;
  final double tiret;
  final double espace;
  final double largeur;

  @override
  void paint(Canvas canvas, Size s) {
    final p = Paint()
      ..color = couleur
      ..style = PaintingStyle.stroke
      ..strokeWidth = largeur;
    final chemin = Path()..addRRect(RRect.fromRectAndRadius((Offset.zero & s).deflate(largeur / 2), Radius.circular(rayon)));
    for (final m in chemin.computeMetrics()) {
      for (var d = 0.0; d < m.length; d += tiret + espace) {
        canvas.drawPath(m.extractPath(d, d + tiret), p);
      }
    }
  }

  @override
  bool shouldRepaint(covariant Pointilles old) => old.couleur != couleur || old.rayon != rayon;
}

/// L'empreinte d'une clé en trois blocs mono, faite pour être comparée à
/// l'œil.
class Empreinte extends StatelessWidget {
  const Empreinte(this.blocs, {super.key, this.couleur = Couleurs.cyan, this.hauteur = 36, this.bord = Couleurs.bordure});
  final List<String> blocs;
  final Color couleur;
  final Color bord;
  final double hauteur;

  @override
  Widget build(BuildContext context) => Row(
    children: [
      for (var i = 0; i < blocs.length; i++) ...[
        if (i > 0) const SizedBox(width: 8),
        Expanded(
          child: Container(
            height: hauteur,
            alignment: Alignment.center,
            decoration: BoxDecoration(
              color: Couleurs.bloc,
              borderRadius: BorderRadius.circular(10),
              border: Border.all(color: bord),
            ),
            child: Text(blocs[i], style: mono(hauteur * 0.44, couleur: couleur)),
          ),
        ),
      ],
    ],
  );
}

/// Bouton principal : fond sombre, bordure en dégradé cyan → bleu, halo.
class BoutonContour extends StatelessWidget {
  const BoutonContour({
    super.key,
    required this.libelle,
    this.ico,
    this.onTap,
    this.hauteur = 50,
    this.rayon = 18,
    this.taillePolice = 15.5,
  });
  final String libelle;
  final Ico? ico;
  final VoidCallback? onTap;
  final double hauteur;
  final double rayon;
  final double taillePolice;

  @override
  Widget build(BuildContext context) => Bordee(
    bordure: Bords.cyan,
    fond: const Color(0xF20A121A),
    rayon: rayon,
    onTap: onTap,
    halo: [BoxShadow(color: Couleurs.cyan.withValues(alpha: 0.7), blurRadius: 26, spreadRadius: -10)],
    child: Container(
      height: hauteur - 2,
      decoration: BoxDecoration(
        gradient: RadialGradient(radius: 2.4, colors: [Colors.transparent, Couleurs.cyan.withValues(alpha: 0.06)]),
      ),
      child: Row(
        mainAxisAlignment: MainAxisAlignment.center,
        children: [
          if (ico != null) ...[Icone(ico!, taille: 18, lueur: true), const SizedBox(width: 10)],
          Flexible(
            child: Text(libelle, style: texte(taillePolice, graisse: 600), maxLines: 1, overflow: TextOverflow.ellipsis),
          ),
        ],
      ),
    ),
  );
}

/// Bouton fantôme : bordure fine colorée.
class BoutonFantome extends StatelessWidget {
  const BoutonFantome({
    super.key,
    required this.libelle,
    this.couleur = Couleurs.secondaire,
    this.bord = Couleurs.bordure,
    this.onTap,
    this.hauteur = 44,
  });
  final String libelle;
  final Color couleur;
  final Color bord;
  final VoidCallback? onTap;
  final double hauteur;

  @override
  Widget build(BuildContext context) => Carte(
    rayon: 14,
    bord: bord,
    fond: const Color(0x990C1117),
    onTap: onTap,
    child: SizedBox(
      height: hauteur - 2,
      child: Center(
        child: Text(libelle, style: texte(15, graisse: 600, couleur: couleur)),
      ),
    ),
  );
}

/// « ‹ Appareils » : le retour, en pilule.
class BoutonRetour extends StatelessWidget {
  const BoutonRetour(this.libelle, {super.key, this.onTap});
  final String libelle;
  final VoidCallback? onTap;

  @override
  Widget build(BuildContext context) => ZoneTactile(
    child: Material(
      color: Couleurs.carte,
      shape: const StadiumBorder(side: BorderSide(color: Couleurs.bordure)),
      clipBehavior: Clip.antiAlias,
      child: InkWell(
        onTap: onTap ?? () => Navigator.of(context).maybePop(),
        child: SizedBox(
          height: 38,
          child: Padding(
            padding: const EdgeInsets.fromLTRB(9, 0, 15, 0),
            child: Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                const Icone(Ico.retour, taille: 17, trait: 2),
                const SizedBox(width: 6),
                Text(libelle, style: texte(14)),
              ],
            ),
          ),
        ),
      ),
    ),
  );
}

/// L'interrupteur principal, 54 × 32 : dégradé cyan et halo quand il est
/// activé ; piste #1A222B et bouton #7C8894 sinon.
class Interrupteur extends StatelessWidget {
  const Interrupteur({
    super.key,
    required this.valeur,
    required this.onChanged,
    this.largeur = 54,
    this.hauteur = 32,
    this.libelle = 'Tunnel',
  });
  final String libelle;
  final bool valeur;

  /// Nul : l'interrupteur est bloqué (tunnel en train de changer d'état)
  /// et s'estompe.
  final ValueChanged<bool>? onChanged;
  final double largeur;
  final double hauteur;

  @override
  Widget build(BuildContext context) {
    final d = hauteur - 6;
    final bloque = onChanged == null;
    return ZoneTactile(
      child: Semantics(
        toggled: valeur,
        enabled: !bloque,
        label: libelle,
        child: GestureDetector(
          onTap: bloque ? null : () => onChanged!(!valeur),
          behavior: HitTestBehavior.opaque,
          child: AnimatedOpacity(
            duration: const Duration(milliseconds: 200),
            opacity: bloque ? 0.45 : 1,
            child: Padding(
              padding: const EdgeInsets.all(6),
              child: AnimatedContainer(
                duration: const Duration(milliseconds: 260),
                curve: Curves.easeOutCubic,
                width: largeur,
                height: hauteur,
                decoration: BoxDecoration(
                  borderRadius: BorderRadius.circular(hauteur / 2),
                  gradient: valeur ? Couleurs.degrade : const LinearGradient(colors: [Couleurs.separateur, Couleurs.separateur]),
                  border: Border.all(color: valeur ? Colors.transparent : Couleurs.bordure),
                  boxShadow: valeur
                      ? [BoxShadow(color: Couleurs.cyan.withValues(alpha: 0.65), blurRadius: 18, spreadRadius: -2)]
                      : const [],
                ),
                child: AnimatedAlign(
                  duration: const Duration(milliseconds: 260),
                  curve: Curves.easeOutCubic,
                  alignment: valeur ? Alignment.centerRight : Alignment.centerLeft,
                  child: AnimatedContainer(
                    duration: const Duration(milliseconds: 260),
                    margin: const EdgeInsets.symmetric(horizontal: 2),
                    width: d,
                    height: d,
                    decoration: BoxDecoration(
                      shape: BoxShape.circle,
                      color: valeur ? Couleurs.texte : Couleurs.tertiaire,
                      boxShadow: valeur ? [const BoxShadow(color: Color(0x80001E32), blurRadius: 5, offset: Offset(0, 1))] : null,
                    ),
                  ),
                ),
              ),
            ),
          ),
        ),
      ),
    );
  }
}

/// Une ligne de réglage : icône, libellé, valeur à droite.
class LigneReglage extends StatelessWidget {
  const LigneReglage({
    super.key,
    required this.ico,
    required this.libelle,
    this.couleurIco = Couleurs.cyan,
    this.couleurTexte = Couleurs.texte,
    this.valeur,
    this.valeurMono = false,
    this.fin,
    this.onTap,
    this.separateur = true,
    this.dense = false,
    this.sousTitre,
  });
  final Ico ico;
  final String libelle;

  /// Une ligne d'explication sous le libellé.
  final String? sousTitre;
  final Color couleurIco;
  final Color couleurTexte;
  final String? valeur;
  final bool valeurMono;
  final Widget? fin;
  final VoidCallback? onTap;
  final bool separateur;
  final bool dense;

  @override
  Widget build(BuildContext context) => InkWell(
    onTap: onTap,
    child: Container(
      padding: EdgeInsets.symmetric(horizontal: 16, vertical: dense ? 10 : 12),
      decoration: BoxDecoration(
        border: separateur ? const Border(bottom: BorderSide(color: Couleurs.separateur)) : null,
      ),
      child: Row(
        children: [
          Icone(ico, couleur: couleurIco, taille: 19, lueur: true),
          const SizedBox(width: 13),
          Expanded(
            child: sousTitre == null
                ? Text(
                    libelle,
                    style: texte(15, couleur: couleurTexte),
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                  )
                : Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        libelle,
                        style: texte(15, couleur: couleurTexte),
                        maxLines: 1,
                        overflow: TextOverflow.ellipsis,
                      ),
                      const SizedBox(height: 2),
                      Text(sousTitre!, style: texte(12.5, couleur: Couleurs.secondaire, hauteur: 1.3)),
                    ],
                  ),
          ),
          if (valeur != null) ...[
            const SizedBox(width: 10),
            // Une valeur longue (nom, serveur) se coupe au lieu de pousser
            // le libellé hors de la ligne.
            ConstrainedBox(
              constraints: BoxConstraints(maxWidth: MediaQuery.sizeOf(context).width * 0.45),
              child: Text(
                valeur!,
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                textAlign: TextAlign.right,
                style: valeurMono
                    ? mono(13.5, graisse: 400, couleur: Couleurs.etiquette)
                    : texte(13.5, couleur: Couleurs.etiquette),
              ),
            ),
          ],
          if (fin != null) ...[const SizedBox(width: 8), fin!],
        ],
      ),
    ),
  );
}

/// Un bloc de lignes, sous son étiquette.
class Groupe extends StatelessWidget {
  const Groupe({super.key, required this.titre, required this.enfants});
  final String titre;
  final List<Widget> enfants;

  @override
  Widget build(BuildContext context) => Column(
    crossAxisAlignment: CrossAxisAlignment.stretch,
    children: [
      Padding(padding: const EdgeInsets.fromLTRB(6, 0, 6, 8), child: Etiquette(titre)),
      Carte(child: Column(children: enfants)),
    ],
  );
}

/// Le chevron « › » des lignes cliquables.
class Chevron extends StatelessWidget {
  const Chevron({super.key});
  @override
  Widget build(BuildContext context) => const Icone(Ico.chevron, couleur: Couleurs.tertiaire, taille: 16, trait: 2);
}

/// Une pastille ronde avec un nombre : « 2 ».
class Compteur extends StatelessWidget {
  const Compteur(this.n, {super.key, this.couleur = Couleurs.rouge});
  final int n;
  final Color couleur;

  @override
  Widget build(BuildContext context) => Container(
    constraints: const BoxConstraints(minWidth: 24),
    height: 24,
    padding: const EdgeInsets.symmetric(horizontal: 7),
    alignment: Alignment.center,
    decoration: BoxDecoration(
      color: couleur,
      borderRadius: BorderRadius.circular(12),
      boxShadow: [BoxShadow(color: couleur.withValues(alpha: 0.5), blurRadius: 10, spreadRadius: -2)],
    ),
    child: Text('$n', style: texte(12.5, graisse: 700, couleur: Colors.white)),
  );
}

/// Les coins de visée d'un cadre.
class Coins extends CustomPainter {
  Coins({this.couleur = Couleurs.cyan, this.longueur = 18, this.largeur = 2, this.rayon = 18, this.haut = true, this.bas = true});
  final Color couleur;
  final double longueur;
  final double largeur;
  final double rayon;
  final bool haut;
  final bool bas;

  @override
  void paint(Canvas canvas, Size s) {
    final p = Paint()
      ..color = couleur
      ..style = PaintingStyle.stroke
      ..strokeWidth = largeur;
    final r = rayon.clamp(0.0, longueur);
    void coin(Offset o, double dx, double dy) {
      final chemin = Path()
        ..moveTo(o.dx, o.dy + dy * longueur)
        ..lineTo(o.dx, o.dy + dy * r)
        ..arcToPoint(Offset(o.dx + dx * r, o.dy), radius: Radius.circular(r), clockwise: dx * dy > 0)
        ..lineTo(o.dx + dx * longueur, o.dy);
      canvas.drawPath(chemin, p);
    }

    final m = largeur / 2;
    if (haut) {
      coin(Offset(m, m), 1, 1);
      coin(Offset(s.width - m, m), -1, 1);
    }
    if (bas) {
      coin(Offset(m, s.height - m), 1, -1);
      coin(Offset(s.width - m, s.height - m), -1, -1);
    }
  }

  @override
  bool shouldRepaint(covariant Coins old) => false;
}

/// Tout doit tenir dans la hauteur visible : si le contenu déborde (petit
/// écran), il est réduit au lieu de défiler. Avec une police agrandie dans
/// les réglages du téléphone, il défile : le réduire annulerait le choix
/// de la personne, et rapetisserait les boutons sous 48 dp.
class SansDefilement extends StatefulWidget {
  const SansDefilement({super.key, required this.child, this.alignement = Alignment.topCenter});
  final Widget child;
  final Alignment alignement;

  @override
  State<SansDefilement> createState() => _SansDefilementState();
}

class _SansDefilementState extends State<SansDefilement> {
  // Le contenu change de parent quand le clavier s'ouvre : sans cette clé
  // il serait recréé, et le champ touché perdrait le clavier aussitôt.
  final _contenu = GlobalKey();

  @override
  Widget build(BuildContext context) => LayoutBuilder(
    builder: (context, c) {
      final contenu = SizedBox(width: c.maxWidth, child: KeyedSubtree(key: _contenu, child: widget.child));
      final grandePolice = MediaQuery.textScalerOf(context).scale(10) > 10;
      return View.of(context).viewInsets.bottom > 0 || grandePolice
          // Clavier ouvert, la hauteur fond : réduire rendrait tout
          // minuscule. On défile, à taille normale.
          ? SingleChildScrollView(child: contenu)
          : FittedBox(fit: BoxFit.scaleDown, alignment: widget.alignement, child: contenu);
    },
  );
}

/// Le fond des écrans : dégradé radial #07131C → #04060A et grille de
/// 24 dp à 5 % sous un masque radial. [centre] et [etendue] placent la
/// lueur, comme dans chaque maquette.
class Fond extends StatelessWidget {
  const Fond({super.key, required this.child, this.centre = const Alignment(0, -0.4), this.etendue = const Size(0.8, 0.34)});
  final Widget child;
  final Alignment centre;
  final Size etendue;

  @override
  Widget build(BuildContext context) => CustomPaint(painter: _PeintreFond(centre, etendue), child: child);
}

class _PeintreFond extends CustomPainter {
  _PeintreFond(this.centre, this.etendue);
  final Alignment centre;
  final Size etendue;

  @override
  void paint(Canvas canvas, Size size) {
    final r = Offset.zero & size;
    final c = centre.alongSize(size);
    canvas.drawRect(r, Paint()..color = Couleurs.fond);

    // La lueur, elliptique : on dessine un cercle dans un repère étiré.
    void ellipse(double rx, double ry, List<Color> couleurs, List<double> arrets, [BlendMode? mode]) {
      canvas.save();
      canvas.translate(c.dx, c.dy);
      canvas.scale(1, ry / rx);
      canvas.drawCircle(
        Offset.zero,
        rx,
        Paint()
          ..shader = ui.Gradient.radial(Offset.zero, rx, couleurs, arrets)
          ..blendMode = mode ?? BlendMode.srcOver,
      );
      canvas.restore();
    }

    ellipse(size.width * 1.2, size.height * 0.5, const [Couleurs.fondHalo, Couleurs.fond], const [0, 0.7]);

    // La grille, dans un calque masqué par un dégradé radial.
    final trait = Paint()
      ..color = Couleurs.cyan.withValues(alpha: 0.05)
      ..strokeWidth = 1;
    canvas.saveLayer(r, Paint());
    for (var x = 0.0; x < size.width; x += 24) {
      canvas.drawLine(Offset(x + 0.5, 0), Offset(x + 0.5, size.height), trait);
    }
    for (var y = 0.0; y < size.height; y += 24) {
      canvas.drawLine(Offset(0, y + 0.5), Offset(size.width, y + 0.5), trait);
    }
    ellipse(
      size.width * etendue.width,
      size.height * etendue.height,
      const [Colors.black, Colors.transparent],
      const [0, 1],
      BlendMode.dstIn,
    );
    // Hors de l'ellipse, le masque doit aussi effacer la grille.
    canvas.save();
    final trou = Path()
      ..fillType = PathFillType.evenOdd
      ..addRect(r)
      ..addOval(Rect.fromCenter(center: c, width: size.width * etendue.width * 2, height: size.height * etendue.height * 2));
    canvas.drawPath(trou, Paint()..blendMode = BlendMode.clear);
    canvas.restore();
    canvas.restore();
  }

  @override
  bool shouldRepaint(covariant _PeintreFond old) => old.centre != centre || old.etendue != etendue;
}

/// Les trois mises en page :
/// - compact : téléphone, écran extérieur du Fold ;
/// - portrait : Fold déplié tenu en 3/4 ;
/// - paysage : Fold déplié tenu en 4/3.
/// Le Fold 8 déplié fait environ 830 × 750 dp : on bascule dès 600 dp de
/// large, sans attendre la classe « expanded » (840 dp).
enum Format { compact, portrait, paysage }

Format formatDe(BuildContext context) {
  final s = MediaQuery.sizeOf(context);
  if (s.width >= 700 && s.width > s.height) return Format.paysage;
  if (s.width >= 600) return Format.portrait;
  return Format.compact;
}
