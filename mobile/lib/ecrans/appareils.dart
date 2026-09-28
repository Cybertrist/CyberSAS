import 'package:flutter/material.dart';

import '../composants.dart';
import '../dessins.dart';
import '../donnees.dart';
import '../etat.dart';
import '../icones.dart';
import '../theme.dart';
import 'ajout.dart';
import 'demandes.dart';
import 'detail.dart';

/// Les appareils du réseau, rangés par propriétaire. Sur le Fold déplié,
/// la liste et le détail de l'appareil choisi sont côte à côte.
class EcranAppareils extends StatelessWidget {
  const EcranAppareils({super.key});

  @override
  Widget build(BuildContext context) {
    final r = EtatReseau.of(context);
    return switch (formatDe(context)) {
      Format.compact => _Compact(r: r),
      Format.paysage => _Paysage(r: r),
      Format.portrait => _Portrait(r: r),
    };
  }
}

void ouvrirAjout(BuildContext context) => Navigator.of(context).push(versEcran(const EcranAjout()));
void ouvrirDemandes(BuildContext context) => Navigator.of(context).push(versEcran(const EcranDemandes()));

/// Une transition discrète : fondu et léger glissement.
Route<void> versEcran(Widget ecran) => PageRouteBuilder(
      transitionDuration: const Duration(milliseconds: 280),
      reverseTransitionDuration: const Duration(milliseconds: 220),
      pageBuilder: (_, _, _) => ecran,
      transitionsBuilder: (_, a, _, enfant) {
        final c = CurvedAnimation(parent: a, curve: Curves.easeOutCubic);
        return FadeTransition(
          opacity: c,
          child: SlideTransition(position: Tween(begin: const Offset(0.04, 0), end: Offset.zero).animate(c), child: enfant),
        );
      },
    );


/// « Appareils », et dessous la plage du réseau et qui est en ligne.
class _Titre extends StatelessWidget {
  const _Titre({required this.r, this.taille = 30});
  final Reseau r;
  final double taille;

  @override
  Widget build(BuildContext context) => Padding(
        padding: const EdgeInsets.symmetric(horizontal: 6),
        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
          Text('Appareils', style: titre(taille)),
          const SizedBox(height: 8),
          Text.rich(
            TextSpan(children: [
              TextSpan(text: r.plage),
              const TextSpan(text: '  ·  '),
              if (r.enService) ...[
                TextSpan(text: '${r.enLigne} en ligne', style: mono(12.5, graisse: 400, couleur: Couleurs.vert)),
                TextSpan(text: ' sur ${r.appareils.length}'),
              ] else
                TextSpan(text: 'tunnel coupé', style: mono(12.5, graisse: 400, couleur: Couleurs.rouge)),
            ]),
            style: mono(12.5, graisse: 400, couleur: Couleurs.etiquette),
          ),
        ]),
      );
}

/// La carte du réseau.
class _CarteReseau extends StatelessWidget {
  const _CarteReseau({required this.r, required this.hauteur, this.grand = false});
  final Reseau r;
  final double hauteur;
  final bool grand;

  @override
  Widget build(BuildContext context) => Container(
        height: hauteur,
        clipBehavior: Clip.antiAlias,
        decoration: BoxDecoration(
          color: const Color(0x99060F16),
          borderRadius: BorderRadius.circular(22),
          border: Border.all(color: Couleurs.bordure),
        ),
        child: Topologie(appareils: r.appareils, grand: grand, connecte: r.enService),
      );
}

/// « 2 demandes à signer · Voir › », pour l'admin.
class BandeauDemandes extends StatelessWidget {
  const BandeauDemandes({super.key, required this.n});
  final int n;

  @override
  Widget build(BuildContext context) => Carte(
        rayon: 16,
        fond: Couleurs.bleu.withValues(alpha: 0.08),
        bord: Couleurs.bleu.withValues(alpha: 0.4),
        onTap: () => ouvrirDemandes(context),
        padding: const EdgeInsets.fromLTRB(12, 0, 12, 0),
        child: SizedBox(
          height: 48,
          child: Row(children: [
            Compteur(n),
            const SizedBox(width: 11),
            Expanded(
              child: Text(
                'demande${n > 1 ? 's' : ''} à signer',
                style: texte(14.5, graisse: 500),
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
              ),
            ),
            Text('Voir', style: texte(14, graisse: 600, couleur: Couleurs.bleu)),
            const SizedBox(width: 2),
            const Icone(Ico.chevron, couleur: Couleurs.bleu, taille: 15, trait: 2),
          ]),
        ),
      );
}

/// « + Ajouter un appareil », comme sur la maquette : fond sombre, bordure
/// en dégradé cyan → bleu et halo.
class BoutonAjouter extends StatelessWidget {
  const BoutonAjouter({super.key, this.hauteur = 48});
  final double hauteur;

  @override
  Widget build(BuildContext context) {
    final r = EtatReseau.of(context);
    // Seul un admin invite : pour les autres, le bouton ne mènerait qu'à
    // un refus.
    if (r.reel && !r.admin) return const SizedBox.shrink();
    return _bouton(context);
  }

  Widget _bouton(BuildContext context) => Bordee(
        bordure: Bords.cyan,
        fond: const Color(0xF20A121A),
        rayon: 16,
        halo: [BoxShadow(color: Couleurs.cyan.withValues(alpha: 0.7), blurRadius: 22, spreadRadius: -10)],
        onTap: () => ouvrirAjout(context),
        child: SizedBox(
          height: hauteur - 2,
          child: Row(mainAxisAlignment: MainAxisAlignment.center, children: [
            const Icone(Ico.plus, taille: 19, lueur: true),
            const SizedBox(width: 10),
            Text('Ajouter un appareil', style: texte(14.5, graisse: 600)),
          ]),
        ),
      );
}

String _titreGroupe(Reseau r, String p) {
  if (p == 'admin') return 'Machines du réseau';
  if (p == r.courriel.toLowerCase() || p == r.compte.toLowerCase()) return 'Mes appareils';
  if (p.isEmpty) return 'Autres appareils';
  final n = prenom(p);
  // « d'Essai », « d'Ana » ; « de Léa ».
  return RegExp(r'^[aeiouyhàâéèêîôûAEIOUYHÀÂÉÈÊÎÔÛ]').hasMatch(n) ? "Appareils d'$n" : 'Appareils de $n';
}

// ─── Compact : téléphone et écran extérieur ─────────────────────────────

/// Ici la page défile : la liste grandit avec le réseau, on ne la
/// rétrécit pas pour la faire tenir.
class _Compact extends StatelessWidget {
  const _Compact({required this.r});
  final Reseau r;

  @override
  Widget build(BuildContext context) {
    final groupes = r.parProprietaire;
    return ListView(
      padding: const EdgeInsets.fromLTRB(16, 12, 16, 12),
      children: [
        _Titre(r: r, taille: 28),
        const SizedBox(height: 14),
        _CarteReseau(r: r, hauteur: 210),
        if (r.admin && r.demandes.isNotEmpty) ...[const SizedBox(height: 10), BandeauDemandes(n: r.demandes.length)],
        for (final g in groupes.entries) ...[
          const SizedBox(height: 16),
          _Liste(
            r: r,
            titre: _titreGroupe(r, g.key),
            appareils: g.value,
            onTap: (a) => Navigator.of(context).push(versEcran(EcranDetail(adresse: a.adresse))),
          ),
        ],
        const SizedBox(height: 16),
        const BoutonAjouter(),
      ],
    );
  }
}

// ─── La liste, en lignes ────────────────────────────────────────────────

class _Liste extends StatelessWidget {
  const _Liste({required this.r, required this.titre, required this.appareils, required this.onTap, this.choix = false, this.adresseChoisie});
  final Reseau r;
  final String titre;
  final List<Appareil> appareils;
  final void Function(Appareil) onTap;

  /// Fold déplié : la ligne choisie est marquée, son détail est à côté.
  final bool choix;

  /// L'appareil marqué ; à défaut, celui choisi dans le réseau.
  final String? adresseChoisie;

  @override
  Widget build(BuildContext context) => Column(crossAxisAlignment: CrossAxisAlignment.stretch, mainAxisSize: MainAxisSize.min, children: [
        Padding(padding: const EdgeInsets.fromLTRB(6, 0, 6, 8), child: Etiquette(titre)),
        Carte(
          child: Column(children: [
            for (var i = 0; i < appareils.length; i++)
              _Ligne(
                a: appareils[i],
                choisi: choix && appareils[i].adresse == (adresseChoisie ?? r.appareil(r.selection).adresse),
                chevron: !choix,
                dernier: i == appareils.length - 1,
                onTap: () => onTap(appareils[i]),
              ),
          ]),
        ),
      ]);
}

/// Une ligne : l'icône du type, le nom et l'adresse, les ports ouverts, et
/// un point vert (en ligne) ou gris (hors ligne).
class _Ligne extends StatelessWidget {
  const _Ligne({required this.a, required this.choisi, required this.chevron, required this.dernier, required this.onTap});
  final Appareil a;
  final bool choisi;
  final bool chevron;
  final bool dernier;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) => InkWell(
        onTap: onTap,
        child: AnimatedContainer(
          duration: const Duration(milliseconds: 200),
          padding: const EdgeInsets.fromLTRB(14, 11, 12, 11),
          decoration: BoxDecoration(
            color: choisi ? Couleurs.cyan.withValues(alpha: 0.06) : Colors.transparent,
            border: Border(
              left: BorderSide(color: choisi ? Couleurs.cyan : Colors.transparent, width: 2),
              bottom: dernier ? BorderSide.none : const BorderSide(color: Couleurs.separateur),
            ),
          ),
          child: Row(children: [
            CaseIcone(a.type.ico, couleur: a.couleur, etat: a.enLigne && EtatReseau.of(context).enService ? EtatIcone.enLigne : EtatIcone.horsLigne, taille: 38),
            const SizedBox(width: 13),
            Expanded(
              child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                Row(children: [
                  Flexible(
                    child: Text(a.nomAffiche,
                        style: texte(15, graisse: 600, couleur: a.enLigne ? Couleurs.texte : Couleurs.secondaire),
                        maxLines: 1,
                        overflow: TextOverflow.ellipsis),
                  ),
                  if (a.moi) ...[const SizedBox(width: 7), const Puce('Vous', couleur: Couleurs.texte, fond: false, monoPolice: false)],
                  // Écarté par ce téléphone : pas (encore) signé, expiré ou révoqué.
                  if (!a.signe) ...[const SizedBox(width: 7), const Puce('NON SIGNÉ', couleur: Couleurs.rouge, fond: false)],
                ]),
                const SizedBox(height: 3),
                Text(a.adresse, style: mono(12.5, graisse: 400, couleur: Couleurs.etiquette)),
              ]),
            ),
            // Dans la liste, seulement les ports TCP précis : « tout ouvert »
            // sur chaque ligne d'un admin ne dirait rien.
            for (final p in a.ports.where((p) => p.startsWith('tcp:')).take(3)) ...[const SizedBox(width: 4), PucePort(p)],
            const SizedBox(width: 10),
            // Tunnel coupé : d'ici, on ne sait plus qui est en ligne.
            _Point(a.enLigne && EtatReseau.of(context).enService),
            if (chevron) ...[const SizedBox(width: 6), const Chevron()],
          ]),
        ),
      );
}

class _Point extends StatelessWidget {
  const _Point(this.enLigne);
  final bool enLigne;

  @override
  Widget build(BuildContext context) => Semantics(
        label: enLigne ? 'En ligne' : 'Hors ligne',
        child: Container(
          width: 8,
          height: 8,
          decoration: BoxDecoration(
            shape: BoxShape.circle,
            color: enLigne ? Couleurs.vert : Colors.transparent,
            border: enLigne ? null : Border.all(color: Couleurs.tertiaire, width: 1.4),
            boxShadow: enLigne ? [BoxShadow(color: Couleurs.vert.withValues(alpha: 0.6), blurRadius: 6)] : null,
          ),
        ),
      );
}

// ─── Déplié : la liste, et le détail à côté ─────────────────────────────

/// Fold déplié en paysage, en deux temps :
/// - au repos, la carte du réseau en grand à gauche (avec les demandes),
///   la liste des appareils à droite ;
/// - un appareil touché, la liste glisse à gauche et son détail s'ouvre à
///   droite. « Carte du réseau » ou le retour d'Android revient au repos.
class _Paysage extends StatefulWidget {
  const _Paysage({required this.r});
  final Reseau r;

  @override
  State<_Paysage> createState() => _PaysageState();
}

class _PaysageState extends State<_Paysage> {
  /// L'adresse de l'appareil ouvert (l'adresse, pas le nom : il peut être
  /// renommé pendant qu'on le regarde).
  String? _ouvert;

  /// Le sens du dernier décalage : vers le détail, ou retour à la carte.
  bool _enAvant = true;

  void _ouvrir(Appareil a) => setState(() {
        _enAvant = true;
        _ouvert = a.adresse;
      });
  void _fermer() => setState(() {
        _enAvant = false;
        _ouvert = null;
      });

  Widget _listes({required bool marquer}) {
    final r = widget.r;
    return SingleChildScrollView(
      child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
        for (final g in r.parProprietaire.entries) ...[
          if (g.key != r.parProprietaire.keys.first) const SizedBox(height: 16),
          _Liste(
            r: r,
            titre: _titreGroupe(r, g.key),
            appareils: g.value,
            choix: marquer,
            adresseChoisie: _ouvert,
            onTap: _ouvrir,
          ),
        ],
      ]),
    );
  }

  @override
  Widget build(BuildContext context) {
    final r = widget.r;
    final ouvert = _ouvert;
    final Widget gauche;
    final Widget droite;
    if (ouvert == null) {
      gauche = Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
        _Titre(r: r, taille: 30),
        const SizedBox(height: 14),
        Expanded(child: _CarteReseau(r: r, hauteur: double.infinity, grand: true)),
        if (r.admin && r.demandes.isNotEmpty) ...[const SizedBox(height: 12), BandeauDemandes(n: r.demandes.length)],
      ]);
      droite = Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
        // La place du bouton « Carte du réseau » : la liste reste à la même
        // hauteur quand elle glisse à gauche.
        const SizedBox(height: 54),
        Expanded(child: _listes(marquer: false)),
        const SizedBox(height: 12),
        const BoutonAjouter(),
      ]);
    } else {
      gauche = Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
        Align(alignment: Alignment.centerLeft, child: BoutonRetour('Carte du réseau', onTap: _fermer)),
        const SizedBox(height: 16),
        Expanded(child: _listes(marquer: true)),
        const SizedBox(height: 12),
        const BoutonAjouter(),
      ]);
      final a = r.parAdresse(ouvert);
      droite = a == null ? const AppareilParti() : PanneauDetail(a: a, disposition: Disposition.colonne);
    }

    return PopScope(
      canPop: ouvert == null,
      onPopInvokedWithResult: (fait, _) {
        if (!fait) _fermer();
      },
      child: Padding(
        padding: const EdgeInsets.fromLTRB(24, 16, 24, 20),
        child: ClipRect(
          child: AnimatedSwitcher(
          duration: const Duration(milliseconds: 320),
          switchInCurve: Curves.easeOutCubic,
          switchOutCurve: Curves.easeInCubic,
          // Comme les volets de SmartBudget : toute la rangée se décale d'une
          // colonne. En avant, la nouvelle arrive par la droite et l'ancienne
          // part à gauche ; au retour, l'inverse. La liste glisse ainsi pile
          // à sa nouvelle place, d'où les deux colonnes de même largeur.
          transitionBuilder: (enfant, a) {
            final entrant = enfant.key == ValueKey(ouvert == null);
            final sens = (_enAvant == entrant) ? 1.0 : -1.0;
            return SlideTransition(
              position: Tween(begin: Offset(sens / 2, 0), end: Offset.zero).animate(a),
              child: FadeTransition(opacity: a, child: enfant),
            );
          },
          layoutBuilder: (actuel, anciens) => Stack(children: [...anciens, ?actuel]),
          child: Row(
            key: ValueKey(ouvert == null),
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Expanded(child: gauche),
              const SizedBox(width: 22),
              Expanded(child: droite),
            ],
          ),
        ),
        ),
      ),
    );
  }
}

/// Fold déplié en portrait : tout défile, les deux groupes côte à côte,
/// et le détail de l'appareil choisi en bas.
class _Portrait extends StatelessWidget {
  const _Portrait({required this.r});
  final Reseau r;

  @override
  Widget build(BuildContext context) {
    final groupes = r.parProprietaire.entries.toList();
    return ListView(
      padding: const EdgeInsets.fromLTRB(24, 16, 24, 16),
      children: [
        _Titre(r: r, taille: 30),
        const SizedBox(height: 14),
        _CarteReseau(r: r, hauteur: 190, grand: true),
        const SizedBox(height: 16),
        for (var i = 0; i < groupes.length; i += 2) ...[
          if (i > 0) const SizedBox(height: 14),
          Row(crossAxisAlignment: CrossAxisAlignment.start, children: [
            for (final j in [i, i + 1]) ...[
              if (j > i) const SizedBox(width: 14),
              Expanded(
                child: j < groupes.length
                    ? _Liste(r: r, titre: _titreGroupe(r, groupes[j].key), appareils: groupes[j].value, choix: true, onTap: (a) => r.choisir(a.nom))
                    : const SizedBox(),
              ),
            ],
          ]),
        ],
        const SizedBox(height: 14),
        Row(children: [
          if (r.admin && r.demandes.isNotEmpty) ...[
            Expanded(child: BandeauDemandes(n: r.demandes.length)),
            const SizedBox(width: 14),
          ],
          const Expanded(child: BoutonAjouter()),
        ]),
        const SizedBox(height: 16),
        PanneauDetail(a: r.appareil(r.selection), disposition: Disposition.deuxColonnes),
      ],
    );
  }
}
