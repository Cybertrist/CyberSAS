import 'package:flutter/material.dart';

import '../composants.dart';
import '../icones.dart';
import '../securite.dart';
import '../theme.dart';

/// L'ouverture, comme dans SmartBudget : le logo entre en grandissant, un
/// reflet le traverse, le nom apparaît, puis l'empreinte est demandée
/// d'elle-même. Fermée ou ratée, un bouton la relance et l'écran dit
/// pourquoi. Le code du téléphone est accepté aussi : un doigt mouillé ne
/// doit pas fermer la porte.
class EcranVerrou extends StatefulWidget {
  const EcranVerrou({super.key, required this.deverrouiller, required this.sansCode});
  final VoidCallback deverrouiller;

  /// Le téléphone n'a ni empreinte ni code : le verrou ne protège rien et
  /// enfermerait dehors. On le retire, et on ouvre.
  final VoidCallback sansCode;

  @override
  State<EcranVerrou> createState() => _EcranVerrouState();
}

class _EcranVerrouState extends State<EcranVerrou> with SingleTickerProviderStateMixin {
  late final _entree = AnimationController(vsync: this, duration: const Duration(milliseconds: 1400))..forward();
  bool _enCours = false;
  String? _erreur;

  @override
  void initState() {
    super.initState();
    // L'empreinte attend la fin de l'entrée : la fenêtre du système par
    // dessus l'animation la couperait en plein milieu.
    Future<void>.delayed(const Duration(milliseconds: 1500), () {
      if (mounted) _demander();
    });
  }

  @override
  void dispose() {
    _entree.dispose();
    super.dispose();
  }

  Future<void> _demander() async {
    if (_enCours) return;
    setState(() {
      _enCours = true;
      _erreur = null;
    });
    final ok = await confirmerIdentite('Déverrouiller CyberSAS', biometrieSeule: false);
    if (!mounted) return;
    if (ok == Identite.confirmee) {
      widget.deverrouiller();
      return;
    }
    if (ok == Identite.impossible) {
      widget.sansCode();
      return;
    }
    setState(() {
      _enCours = false;
      _erreur = ok == Identite.impossible ? "Aucune empreinte ni code sur ce téléphone." : null;
    });
  }

  double _phase(double debut, double fin, [Curve courbe = Curves.easeOutCubic]) =>
      courbe.transform(((_entree.value - debut) / (fin - debut)).clamp(0.0, 1.0));

  @override
  Widget build(BuildContext context) => Scaffold(
        body: Fond(
          centre: const Alignment(0, -0.3),
          etendue: const Size(0.8, 0.4),
          child: SafeArea(
            child: SizedBox.expand(
              child: AnimatedBuilder(
                animation: _entree,
                builder: (context, _) => Column(children: [
                  const Spacer(flex: 3),
                  Opacity(
                    opacity: _phase(0, 0.3),
                    child: Transform.scale(
                      scale: 0.82 + 0.18 * _phase(0, 0.55, Curves.easeOutBack),
                      child: _Logo(reflet: _phase(0.45, 0.95, Curves.easeInOut)),
                    ),
                  ),
                  const SizedBox(height: 30),
                  Opacity(
                    opacity: _phase(0.6, 0.95),
                    child: Transform.translate(
                      offset: Offset(0, 14 * (1 - _phase(0.6, 0.95))),
                      child: Column(children: [
                        const FittedBox(fit: BoxFit.scaleDown, child: Marque(taille: 36)),
                        const SizedBox(height: 10),
                        Text('Relie tes appareils, chiffré de bout en bout.',
                            textAlign: TextAlign.center, style: texte(14, couleur: Couleurs.secondaire)),
                      ]),
                    ),
                  ),
                  const SizedBox(height: 44),
                  Opacity(
                    opacity: _phase(0.75, 1),
                    child: Column(children: [
                      if (_erreur != null) ...[
                        Text(_erreur!, textAlign: TextAlign.center, style: texte(13.5, couleur: Couleurs.rougeClair)),
                        const SizedBox(height: 16),
                      ],
                      SizedBox(
                        width: 240,
                        child: BoutonContour(
                          libelle: _enCours ? 'En attente…' : 'Ouvrir',
                          ico: Ico.empreinte,
                          hauteur: 54,
                          onTap: _enCours ? null : _demander,
                        ),
                      ),
                    ]),
                  ),
                  const Spacer(flex: 3),
                ]),
              ),
            ),
          ),
        ),
      );
}

/// Le vrai logo, en plaque arrondie, et le reflet qui le traverse.
class _Logo extends StatelessWidget {
  const _Logo({required this.reflet});
  final double reflet;

  @override
  Widget build(BuildContext context) => Container(
        width: 132,
        height: 132,
        decoration: BoxDecoration(
          borderRadius: BorderRadius.circular(32),
          boxShadow: [
            const BoxShadow(color: Couleurs.bordure, spreadRadius: 1),
            BoxShadow(color: Couleurs.bleu.withValues(alpha: 0.55), blurRadius: 50, spreadRadius: -10, offset: const Offset(0, 14)),
          ],
        ),
        clipBehavior: Clip.antiAlias,
        child: Stack(fit: StackFit.expand, children: [
          Image.asset('assets/icon/icon.png', fit: BoxFit.cover),
          FractionalTranslation(
            translation: Offset(-1.2 + 2.4 * reflet, 0),
            child: const DecoratedBox(
              decoration: BoxDecoration(
                gradient: LinearGradient(
                  begin: Alignment(-1, -0.4),
                  end: Alignment(1, 0.4),
                  colors: [Color(0x00FFFFFF), Color(0x38FFFFFF), Color(0x00FFFFFF)],
                  stops: [0.3, 0.5, 0.7],
                ),
              ),
            ),
          ),
        ]),
      );
}
