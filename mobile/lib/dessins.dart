// Les dessins animés de l'appli, tracés en code d'après les SVG des
// maquettes : le tunnel du logo (accueil) et la carte du réseau.
import 'dart:math' as math;
import 'dart:ui' as ui;

import 'package:flutter/material.dart';
import 'package:flutter/scheduler.dart';
import 'package:path_parsing/path_parsing.dart';

import 'donnees.dart';
import 'theme.dart';

Path _svg(String d) {
  final p = Path();
  writeSvgPathDataToPath(d, _VersPath(p));
  return p;
}

class _VersPath extends PathProxy {
  _VersPath(this.p);
  final Path p;
  @override
  void moveTo(double x, double y) => p.moveTo(x, y);
  @override
  void lineTo(double x, double y) => p.lineTo(x, y);
  @override
  void cubicTo(double x1, double y1, double x2, double y2, double x3, double y3) => p.cubicTo(x1, y1, x2, y2, x3, y3);
  @override
  void close() => p.close();
}

Color _c(int rgb, [double a = 1]) => Color(0xFF000000 | rgb).withValues(alpha: a);
Paint _flou(Paint p, double sigma) => p..maskFilter = MaskFilter.blur(BlurStyle.normal, sigma);

// ─── Le tunnel ──────────────────────────────────────────────────────────

/// Les tracés du logo, dans le repère 320 × 338 de la maquette.
abstract final class _T {
  static final maison = _svg('M69 205 V98 Q69 86 80 80 L137 50 Q144 46 151 50 L208 80 Q219 86 219 98 V205');
  static final arche1 = _svg('M97 200 V137 A47 47 0 0 1 191 137 V200');
  static final arche2 = _svg('M124 190 V140 A20 20 0 0 1 164 140 V190');
  static final porte = _svg('M129 160 V141 A15 15 0 0 1 159 141 V160 Z');
  static final sol = _svg('M121 159.5 L167 159.5 L330 250 L330 330 L-42 330 L-42 250 Z');
  static final bords = _svg('M121 159.5 L-42 250 M167 159.5 L330 250');
  static final dessus = _svg('M-60 -60 H350 V260 L167 159.5 H121 L-60 260 Z');
  static final cables = [
    _svg('M34 252 C62 236 84 222 98 200 L134 160'),
    _svg('M144 290 L144 160'),
    _svg('M254 252 C226 236 204 222 190 200 L154 160'),
  ];
  static final mesures = [for (final c in cables) c.computeMetrics().first];
}

/// Une couche du tunnel et son minutage (délai, durée, courbe).
enum _Couche { bg, a2, a1, h, cable, points }

typedef _Minutage = (double delai, double duree, Curve courbe);

const _ease = Cubic(0.25, 0.1, 0.25, 1);

const Map<_Couche, _Minutage> _extinction = {
  _Couche.points: (0, 0.25, _ease),
  _Couche.cable: (0, 1.1, Cubic(0.5, 0, 0.8, 1)),
  _Couche.h: (1.15, 0.45, _ease),
  _Couche.a1: (1.55, 0.45, _ease),
  _Couche.a2: (1.95, 0.45, _ease),
  _Couche.bg: (2.35, 0.8, _ease),
};

const Map<_Couche, _Minutage> _allumage = {
  _Couche.bg: (0, 0.7, _ease),
  _Couche.a2: (0.45, 0.4, _ease),
  _Couche.a1: (0.8, 0.4, _ease),
  _Couche.h: (1.15, 0.4, _ease),
  _Couche.cable: (1.5, 1.1, Cubic(0.2, 0, 0.5, 1)),
  _Couche.points: (2.5, 0.4, _ease),
};

/// Le tunnel du logo, animé. Coupé, il s'éteint couche par couche (les
/// points, les câbles qui se vident, la maison, les arches, le halo) et
/// il ne reste qu'un fantôme bleu nuit à 40 %. Rallumé, tout revient dans
/// l'ordre inverse.
class Tunnel extends StatefulWidget {
  const Tunnel({super.key, required this.allume});
  final bool allume;

  @override
  State<Tunnel> createState() => _TunnelState();
}

class _TunnelState extends State<Tunnel> with SingleTickerProviderStateMixin {
  late final Ticker _ticker = createTicker(_tic);
  final _temps = ValueNotifier<double>(0);

  // Où en est chaque couche (0 éteinte, 1 allumée), et d'où elle part.
  final _valeurs = <_Couche, double>{};
  final _depart = <_Couche, double>{};
  double _debutTransition = -1;
  bool _vers = true;

  @override
  void initState() {
    super.initState();
    for (final c in _Couche.values) {
      _valeurs[c] = widget.allume ? 1 : 0;
    }
    _vers = widget.allume;
    _ticker.start();
  }

  @override
  void didUpdateWidget(Tunnel old) {
    super.didUpdateWidget(old);
    if (old.allume != widget.allume) {
      _vers = widget.allume;
      _depart.addAll(_valeurs);
      // Le câble se remplit toujours depuis le départ.
      if (_vers) _depart[_Couche.cable] = 0;
      // Un ticker relancé repart de zéro : la transition aussi, sinon elle
      // attendrait l'ancienne heure avant de commencer.
      if (_ticker.isActive) {
        _debutTransition = _temps.value;
      } else {
        _debutTransition = 0;
        _ticker.start();
      }
    }
  }

  void _tic(Duration d) {
    final t = d.inMicroseconds / 1e6;
    if (_debutTransition >= 0) {
      final ecoule = t - _debutTransition;
      final minutage = _vers ? _allumage : _extinction;
      var fini = true;
      for (final c in _Couche.values) {
        final (delai, duree, courbe) = minutage[c]!;
        final p = ((ecoule - delai) / duree).clamp(0.0, 1.0);
        if (p < 1) fini = false;
        final cible = _vers ? 1.0 : 0.0;
        _valeurs[c] = _depart[c]! + (cible - _depart[c]!) * courbe.transform(p);
      }
      if (fini) _debutTransition = -1;
    }
    _temps.value = t;
    // Éteint et immobile : plus besoin de redessiner.
    if (!_vers && _debutTransition < 0) _ticker.stop();
  }

  @override
  void dispose() {
    _ticker.dispose();
    _temps.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Stack(fit: StackFit.expand, children: [
      // Le fantôme, toujours là sous le dessin allumé.
      const Opacity(opacity: 0.4, child: RepaintBoundary(child: CustomPaint(painter: _PeintreTunnel.fantome()))),
      RepaintBoundary(
        child: CustomPaint(painter: _PeintreTunnel(_temps, _valeurs, () => _vers)),
      ),
    ]);
  }
}

class _PeintreTunnel extends CustomPainter {
  _PeintreTunnel(this.temps, this.valeurs, this.versAllume)
      : allume = true,
        super(repaint: temps);
  const _PeintreTunnel.fantome()
      : temps = null,
        valeurs = null,
        versAllume = null,
        allume = false;

  final ValueNotifier<double>? temps;
  final Map<_Couche, double>? valeurs;
  final bool Function()? versAllume;
  final bool allume;

  double _v(_Couche c) => valeurs?[c] ?? 1;

  // Le filtre « hrgy » des maquettes : bleu nuit, jamais gris.
  static const _nuit = ColorFilter.matrix([
    .016, .032, .006, 0, .012 * 255, //
    .038, .075, .014, 0, .03 * 255,
    .052, .102, .019, 0, .045 * 255,
    0, 0, 0, 1, 0,
  ]);

  @override
  void paint(Canvas canvas, Size size) {
    final t = temps?.value ?? 0;
    final on = allume;
    // viewBox 320 × 338, « meet », centré, puis translate(1.6 -8) scale(1.1).
    final e = math.min(size.width / 320, size.height / 338);
    canvas.save();
    canvas.translate((size.width - 320 * e) / 2, (size.height - 338 * e) / 2);
    canvas.scale(e);
    canvas.translate(1.6, -8);
    canvas.scale(1.1);
    final zone = const Rect.fromLTRB(-80, -80, 360, 360);

    void couche(double opacite, void Function() dessin, {bool nuit = false}) {
      if (opacite <= 0.001) return;
      if (opacite >= 0.999 && !nuit) {
        dessin();
        return;
      }
      final p = Paint()..color = Colors.black.withValues(alpha: opacite);
      if (nuit) p.colorFilter = _nuit;
      canvas.saveLayer(zone, p);
      dessin();
      canvas.restore();
    }

    double pulse(double a, double b, double periode, [double decalage = 0]) {
      final x = ((t + decalage) % periode) / periode;
      return a + (b - a) * (1 - (2 * x - 1).abs());
    }

    final bg = _v(_Couche.bg);

    // Le ciel et l'anneau, visible au-dessus du sol seulement.
    couche(bg, () {
      canvas.drawOval(
        Rect.fromCenter(center: const Offset(144, 120), width: 320, height: 240),
        Paint()
          ..shader = ui.Gradient.radial(
            const Offset(144, 110),
            150,
            [on ? _c(0x0A3246, .9) : _c(0x0E151C, .9), on ? _c(0x061A26, .5) : _c(0x0A1016, .5), _c(0x04060A, 0)],
            [0, .6, 1],
          ),
      );
      canvas.save();
      canvas.clipPath(_T.dessus);
      final anneau = on ? 0x31E7FD : 0x3A4450;
      canvas.drawCircle(
        const Offset(144, 152),
        128,
        Paint()
          ..style = PaintingStyle.stroke
          ..strokeWidth = 1.3
          ..shader = ui.Gradient.linear(const Offset(0, 20), const Offset(0, 200),
              [_c(anneau, .32), _c(anneau, .12), _c(0x31E7FD, 0)], [0, .7, 1]),
      );
      canvas.restore();
    });

    final nuit = !on;
    Paint trait(double largeur, Shader s) => Paint()
      ..style = PaintingStyle.stroke
      ..strokeWidth = largeur
      ..strokeJoin = StrokeJoin.round
      ..shader = s;

    // La maison, avec son halo qui respire.
    couche(_v(_Couche.h), () {
      canvas.drawPath(
        _T.maison,
        _flou(
            Paint()
              ..style = PaintingStyle.stroke
              ..strokeWidth = 22
              ..strokeJoin = StrokeJoin.round
              ..color = _c(0x31E7FD, on ? pulse(.2, .38, 3) : .28),
            8),
      );
      canvas.drawPath(
        _T.maison,
        trait(16, ui.Gradient.linear(const Offset(0, 44), const Offset(0, 200),
            [_c(0x2DEBFF), _c(0x18DDFF), _c(0x0AA8FF), _c(0x1FD2FF)], [0, .25, .6, 1])),
      );
    }, nuit: nuit);

    couche(_v(_Couche.a1), () {
      canvas.drawPath(
        _T.arche1,
        trait(11, ui.Gradient.linear(const Offset(0, 88), const Offset(0, 200), [_c(0x1CB9DB), _c(0x0E8DB8), _c(0x0A6C92)], [0, .5, 1])),
      );
    }, nuit: nuit);

    couche(_v(_Couche.a2), () {
      canvas.drawPath(_T.arche2, trait(10, ui.Gradient.linear(const Offset(0, 118), const Offset(0, 190), [_c(0x127C9E), _c(0x0A506E)])));
    }, nuit: nuit);

    // La porte, qui éclaire tout (elle suit le halo de fond).
    couche(bg, () {
      canvas.drawPath(_T.porte, _flou(Paint()..color = _c(0x5FF0FF, on ? pulse(.5, .9, 3) : .7), 8));
      canvas.drawPath(
        _T.porte,
        Paint()..shader = ui.Gradient.linear(const Offset(0, 126), const Offset(0, 160), [_c(0xC4FCFF), _c(0x7DF0FF)]),
      );
    }, nuit: nuit);

    // Le sol.
    couche(bg, () {
      canvas.drawPath(
        _T.sol,
        Paint()
          ..shader = ui.Gradient.linear(const Offset(0, 160), const Offset(0, 300),
              [on ? _c(0x0E3446) : _c(0x161D24), on ? _c(0x08202D) : _c(0x0C1117), _c(0x04060A, 0)], [0, .45, 1]),
      );
      if (on) {
        canvas.drawPath(
          _T.sol,
          Paint()..shader = ui.Gradient.radial(const Offset(144, 162), 90, [_c(0x31E7FD, .32), _c(0x31E7FD, 0)]),
        );
      }
      canvas.drawPath(
        _T.bords,
        Paint()
          ..style = PaintingStyle.stroke
          ..strokeWidth = 1.2
          ..shader = ui.Gradient.linear(const Offset(0, 160), const Offset(0, 250), [_c(on ? 0x31E7FD : 0x3A4450, .35), _c(0x31E7FD, 0)]),
      );
    });

    // Les câbles : ils se remplissent depuis le bord à l'allumage, et se
    // vident du départ jusqu'au bout à l'extinction.
    final vc = _v(_Couche.cable);
    final allant = versAllume?.call() ?? true;
    final (debut, fin) = allant ? (0.0, vc) : (1 - vc, 1.0);
    if (fin - debut > 0.001) {
      canvas.saveLayer(zone, Paint()..colorFilter = nuit ? _nuit : null);
      final lueur = _flou(
          Paint()
            ..style = PaintingStyle.stroke
            ..strokeWidth = 7
            ..strokeCap = StrokeCap.round
            ..color = _c(0x31E7FD, .35),
          4);
      final cable = Paint()
        ..style = PaintingStyle.stroke
        ..strokeWidth = 3.6
        ..strokeCap = StrokeCap.round
        ..shader = ui.Gradient.linear(const Offset(0, 160), const Offset(0, 285), [_c(0x8FF4FF), _c(0x22DDFB), _c(0x01B9FD, 0)], [0, .5, 1]);
      for (final m in _T.mesures) {
        final morceau = m.extractPath(m.length * debut, m.length * fin);
        canvas.drawPath(morceau, lueur);
        canvas.drawPath(morceau, cable);
      }
      canvas.restore();
    }

    // Les paquets qui entrent dans le tunnel.
    final vp = on ? _v(_Couche.points) : 0.0;
    if (vp > 0.001) {
      const courbe = Cubic(0.25, 0.1, 0.6, 1);
      for (var i = 0; i < _T.mesures.length; i++) {
        final x = ((t + i) % 3) / 3;
        final k = courbe.transform(x);
        final m = _T.mesures[i];
        final pos = m.getTangentForOffset(m.length * k)!.position;
        final opacite = x < .14 ? x / .14 : (x > .86 ? (1 - x) / .14 : 1.0);
        final echelle = 1.1 + (0.3 - 1.1) * k;
        canvas.save();
        canvas.translate(pos.dx, pos.dy);
        canvas.scale(echelle);
        final a = opacite * vp;
        canvas.drawOval(Rect.fromCenter(center: Offset.zero, width: 26, height: 20), _flou(Paint()..color = _c(0x31E7FD, .35 * a), 4));
        final noyau = Rect.fromCenter(center: Offset.zero, width: 20, height: 15);
        canvas.drawOval(
          noyau,
          Paint()..shader = ui.Gradient.radial(const Offset(-1, -1.5), 12, [_c(0x6FF1FF, a), _c(0x14C8EE, a)]),
        );
        canvas.drawOval(
          noyau,
          Paint()
            ..style = PaintingStyle.stroke
            ..strokeWidth = 1.6
            ..color = _c(0xA8F8FF, a),
        );
        canvas.restore();
      }
    }
    canvas.restore();
  }

  @override
  bool shouldRepaint(covariant _PeintreTunnel old) => old.allume != allume;
}

// ─── La carte du réseau ─────────────────────────────────────────────────

/// Chaque appareil est un nœud relié au serveur par une ligne où circulent
/// des paquets. Les nœuds de droite restent à 110 dp au moins du bord.
class Topologie extends StatefulWidget {
  const Topologie({super.key, required this.appareils, this.grand = false, this.connecte = true});
  final List<Appareil> appareils;
  final bool grand;

  /// Le tunnel de ce téléphone. Coupé, son lien avec le serveur se rompt
  /// et le reste du réseau s'assombrit : on ne le voit plus d'ici.
  final bool connecte;

  /// La taille de l'icône de l'appli, au centre de la carte.
  static double icone(bool grand) => grand ? 56 : 46;

  @override
  State<Topologie> createState() => _TopologieState();
}

/// Désature une image : 0 la laisse en couleur, 1 la met en gris.
ColorFilter _gris(double g) {
  final s = 1 - g;
  const r = 0.2126, v = 0.7152, b = 0.0722;
  return ColorFilter.matrix([
    r + s * (1 - r), v - s * v, b - s * b, 0, 0,
    r - s * r, v + s * (1 - v), b - s * b, 0, 0,
    r - s * r, v - s * v, b + s * (1 - b), 0, 0,
    0, 0, 0, 1, 0,
  ]);
}

class _TopologieState extends State<Topologie> with TickerProviderStateMixin {
  late final _anim = AnimationController(vsync: this, duration: const Duration(seconds: 30))..repeat();

  // 1 : tunnel ouvert ; 0 : coupé. Glisse de l'un à l'autre.
  late final _lien = AnimationController(vsync: this, duration: const Duration(milliseconds: 2800), value: widget.connecte ? 1 : 0);
  late final _etat = CurvedAnimation(parent: _lien, curve: _Lien.reseau);

  @override
  void didUpdateWidget(Topologie old) {
    super.didUpdateWidget(old);
    if (old.connecte != widget.connecte) {
      widget.connecte ? _lien.forward() : _lien.reverse();
    }
  }

  @override
  void dispose() {
    _anim.dispose();
    _etat.dispose();
    _lien.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final t = Topologie.icone(widget.grand);
    return LayoutBuilder(
      builder: (context, k) => Stack(children: [
        Positioned.fill(
          child: RepaintBoundary(
            child: CustomPaint(painter: _PeintreTopologie(_anim, _lien, widget.appareils, widget.grand), size: Size.infinite),
          ),
        ),
        // Le serveur, c'est l'icône de l'appli (même centre que le dessin).
        Positioned(
          left: k.maxWidth / 2 - t / 2,
          top: k.maxHeight * 0.5 - t / 2,
          child: FadeTransition(
            opacity: Tween(begin: 0.35, end: 1.0).animate(_etat),
            // Rond, comme l'anneau qui tourne autour et la lueur dessous :
            // une seule forme, pas trois arrondis qui se contredisent.
            child: Container(
              width: t,
              height: t,
              padding: const EdgeInsets.all(1.2),
              decoration: const BoxDecoration(
                shape: BoxShape.circle,
                gradient: LinearGradient(begin: Alignment.topLeft, end: Alignment.bottomRight, colors: [Couleurs.cyan, Couleurs.bleu]),
              ),
              child: ClipOval(
                child: AnimatedBuilder(
                  animation: _etat,
                  builder: (context, image) => ColorFiltered(colorFilter: _gris(1 - _etat.value), child: image),
                  child: Image.asset('assets/icon/icon.png', fit: BoxFit.cover, filterQuality: FilterQuality.medium),
                ),
              ),
            ),
          ),
        ),
      ]),
    );
  }
}

/// Le minutage de l'allumage, sur la valeur du lien (0 coupé, 1 ouvert).
/// À la coupure, il se joue à l'envers : les paquets s'arrêtent, le réseau
/// se grise, ce téléphone s'éteint, puis son fil se vide jusqu'à lui.
abstract final class _Lien {
  static const fil = Interval(0, 0.35, curve: Curves.easeInOut);
  static const point = Interval(0.3, 0.45, curve: Curves.easeInOut);
  static const reseau = Interval(0.4, 0.8, curve: Curves.easeInOut);
  static const paquets = Interval(0.75, 1, curve: Curves.easeInOut);
}

class _PeintreTopologie extends CustomPainter {
  _PeintreTopologie(this.t, this.lien, this.appareils, this.grand) : super(repaint: Listenable.merge([t, lien]));
  final Animation<double> t;
  final Animation<double> lien;
  final List<Appareil> appareils;
  final bool grand;

  @override
  void paint(Canvas canvas, Size s) {
    final w = s.width, h = s.height;
    final secondes = t.value * 30;
    final v = lien.value;
    final fil = _Lien.fil.transform(v);
    final point = _Lien.point.transform(v);
    final reseau = _Lien.reseau.transform(v);
    final paquets = _Lien.paquets.transform(v);
    final c = Offset(w / 2, h * 0.5);
    // Huit places au plus, dans l'ordre du réseau ; cet appareil n'en sort
    // jamais, il prend la dernière si besoin.
    final tous = appareils.where((a) => a.type != TypeAppareil.serveur).toList();
    final autres = tous.take(8).toList();
    final moi = tous.where((a) => a.moi);
    if (moi.isNotEmpty && !autres.contains(moi.first)) autres[autres.length - 1] = moi.first;
    final places = _places(autres.length, c, w, h);

    // Grille.
    final grille = Paint()..color = Couleurs.cyan.withValues(alpha: 0.045);
    for (var x = 12.0; x < w; x += 24) {
      canvas.drawLine(Offset(x, 0), Offset(x, h), grille);
    }
    for (var y = 12.0; y < h; y += 24) {
      canvas.drawLine(Offset(0, y), Offset(w, y), grille);
    }

    // Le halo du serveur et ses anneaux : ils s'effacent avec le réseau.
    canvas.drawCircle(c, h * 0.48, Paint()..shader = ui.Gradient.radial(c, h * 0.48, [Couleurs.bleu.withValues(alpha: 0.3 * reseau), Couleurs.bleu.withValues(alpha: 0)]));
    canvas.drawCircle(c, h * 0.34, Paint()..style = PaintingStyle.stroke..color = Couleurs.cyan.withValues(alpha: 0.08 * reseau));
    final tourne = Path()..addOval(Rect.fromCircle(center: c, radius: Topologie.icone(grand) / 2 + 10));
    canvas.save();
    canvas.translate(c.dx, c.dy);
    canvas.rotate(t.value * 2 * math.pi);
    canvas.translate(-c.dx, -c.dy);
    _pointilles(canvas, tourne, Paint()..style = PaintingStyle.stroke..color = Couleurs.pointilles.withValues(alpha: 0.5 + 0.5 * reseau), 2, 4);
    canvas.restore();

    final ligne = ui.Gradient.linear(Offset.zero, Offset(w, h), [Couleurs.cyan.withValues(alpha: 0.65), Couleurs.bleu.withValues(alpha: 0.65)]);
    void pointille(Offset p, double a) {
      if (a <= 0) return;
      _pointilles(canvas, Path()..moveTo(c.dx, c.dy)..lineTo(p.dx, p.dy), Paint()..style = PaintingStyle.stroke..color = Couleurs.pointilles.withValues(alpha: a), 3, 4);
    }

    for (var i = 0; i < autres.length; i++) {
      final a = autres[i];
      final p = places[i];
      if (a.moi) {
        // Le fil de ce téléphone se remplit depuis lui vers le serveur, et
        // se vide dans l'autre sens.
        pointille(p, 1);
        if (fil > 0) canvas.drawLine(p, Offset.lerp(p, c, fil)!, Paint()..shader = ligne..strokeWidth = 1.2);
        _paquets(canvas, c, p, secondes, i, paquets);
      } else if (a.enLigne) {
        // Vu d'ici, un appareil en ligne n'est en ligne que si le tunnel
        // l'est : sinon il se grise comme les autres.
        pointille(p, 1 - reseau);
        if (reseau > 0) canvas.drawLine(c, p, Paint()..shader = ligne..strokeWidth = 1.2..color = Colors.white.withValues(alpha: reseau));
        _paquets(canvas, c, p, secondes, i, paquets);
      } else {
        pointille(p, 1);
      }
    }

    // Le serveur : l'icône de l'appli est posée par-dessus (voir Topologie),
    // on ne dessine ici que sa lueur et son nom.
    final demi = Topologie.icone(grand) / 2;
    canvas.drawCircle(c, demi, _flou(Paint()..color = Couleurs.cyan.withValues(alpha: 0.5 * reseau), 10));
    _texte(canvas, 'serveur', '.1', Offset(c.dx, c.dy + demi + 6), TextAlign.center, reseau);

    for (var i = 0; i < autres.length; i++) {
      final a = autres[i];
      _noeud(canvas, a, places[i], c, a.moi ? point : (a.enLigne ? reseau : 0));
    }
  }

  /// Deux paquets sur un lien : un qui monte, un qui descend.
  void _paquets(Canvas canvas, Offset c, Offset p, double secondes, int i, double k) {
    if (k <= 0) return;
    for (final (de, vers, decalage) in [(p, c, i * 0.5), (c, p, i * 0.5 + 1.1)]) {
      final f = ((secondes + decalage) % 2.2) / 2.2;
      final q = Offset.lerp(de, vers, f)!;
      canvas.drawCircle(q, 4.5, _flou(Paint()..color = const Color(0xFFBDF7FF).withValues(alpha: 0.6 * k), 2));
      canvas.drawCircle(q, 2.4, Paint()..color = const Color(0xFFBDF7FF).withValues(alpha: k));
    }
  }

  /// Un appareil : point gris cerclé éteint, point cyan lumineux allumé.
  /// [lumiere] glisse de l'un à l'autre.
  void _noeud(Canvas canvas, Appareil a, Offset p, Offset c, double lumiere) {
    canvas.drawCircle(p, 4.5, Paint()..color = Couleurs.fond);
    canvas.drawCircle(p, 4.5, Paint()..style = PaintingStyle.stroke..strokeWidth = 1.4..color = Couleurs.tertiaire.withValues(alpha: 1 - lumiere));
    if (lumiere > 0) {
      canvas.drawCircle(p, 12, Paint()..color = Couleurs.cyan.withValues(alpha: 0.12 * lumiere));
      canvas.drawCircle(p, 4.5, _flou(Paint()..color = Couleurs.cyan.withValues(alpha: lumiere), 2));
      canvas.drawCircle(p, 4.5, Paint()..color = Couleurs.cyan.withValues(alpha: lumiere));
    }
    // Au-dessus du serveur, le nom passe au-dessus du point : dessous, le
    // lien vers le centre le barrerait.
    final dessus = p.dy < c.dy - 1;
    // Sur la carte, un nom long se raccourcit : « sdk-gphone64-x8… ».
    final n = a.nomAffiche;
    final nom = n.length > 16 ? '${n.substring(0, 15)}…' : n;
    _texte(canvas, nom, a.fin, Offset(p.dx, dessus ? p.dy - 10 : p.dy + 10), TextAlign.center, lumiere, dessus: dessus);
  }

  /// Les [n] appareils, répartis sur une ellipse autour du serveur. Aucun
  /// ne tombe pile en bas (le nom du serveur y est écrit) ni pile en haut :
  /// si le partage régulier y mène, on tourne d'un demi-pas.
  static List<Offset> _places(int n, Offset c, double w, double h) {
    if (n == 0) return const [];
    final rx = math.max(w / 2 - 64, 80.0);
    final ry = h * 0.28;
    final pas = 360 / n;
    var depart = 180.0;
    final versLeBas = (90 - depart) / pas;
    if ((versLeBas - versLeBas.round()).abs() < 1e-6) depart += pas / 2;
    return [
      for (var i = 0; i < n; i++)
        c + Offset(rx * math.cos((depart + i * pas) * math.pi / 180), ry * math.sin((depart + i * pas) * math.pi / 180)),
    ];
  }

  void _texte(Canvas canvas, String nom, String fin, Offset o, TextAlign alignement, double lumiere, {bool dessus = false}) {
    final tp = TextPainter(
      text: TextSpan(children: [
        TextSpan(text: '$nom\n', style: texte(grand ? 12.5 : 11, graisse: 600, couleur: Color.lerp(Couleurs.etiquette, Couleurs.texte, lumiere)!)),
        TextSpan(text: fin, style: mono(11, graisse: 400, couleur: Couleurs.etiquette)),
      ]),
      textAlign: alignement,
      textDirection: TextDirection.ltr,
    )..layout();
    final x = switch (alignement) {
      TextAlign.right => o.dx - tp.width,
      TextAlign.center => o.dx - tp.width / 2,
      _ => o.dx,
    };
    tp.paint(canvas, Offset(x, dessus ? o.dy - tp.height : o.dy));
  }

  void _pointilles(Canvas canvas, Path chemin, Paint p, double tiret, double espace) {
    for (final m in chemin.computeMetrics()) {
      for (var d = 0.0; d < m.length; d += tiret + espace) {
        canvas.drawPath(m.extractPath(d, d + tiret), p);
      }
    }
  }

  @override
  bool shouldRepaint(covariant _PeintreTopologie old) => old.appareils != appareils || old.grand != grand;
}
