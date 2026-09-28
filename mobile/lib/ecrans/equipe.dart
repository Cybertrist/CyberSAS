import 'package:flutter/material.dart';

import '../composants.dart';
import '../donnees.dart';
import '../etat.dart';
import '../icones.dart';
import '../securite.dart';
import '../theme.dart';

/// Admin : l'équipe, c'est-à-dire les comptes Google qui peuvent rejoindre
/// le réseau (equipe.txt sur le serveur). On y ajoute quelqu'un, on change
/// son groupe, on l'en retire. Chaque changement demande le doigt : un
/// téléphone laissé ouvert ne fait entrer personne.
class EcranEquipe extends StatefulWidget {
  const EcranEquipe({super.key});

  @override
  State<EcranEquipe> createState() => _EcranEquipeState();
}

class _EcranEquipeState extends State<EcranEquipe> {
  String? _erreur;
  bool _enCours = false;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) => _lire());
  }

  Future<void> _lire() async {
    if (!mounted) return;
    final e = await EtatReseau.of(context).lireEquipe();
    if (mounted) setState(() => _erreur = e);
  }

  /// Le doigt, puis le changement. [raison] s'affiche dans l'invite.
  Future<void> _changer(String adresse, String groupe, {required String raison, required String fait}) async {
    if (_enCours) return;
    final r = EtatReseau.of(context);
    final messager = ScaffoldMessenger.of(context);
    final id = await confirmerIdentite(raison);
    if (id == Identite.impossible) messager.showSnackBar(const SnackBar(content: Text(sansEmpreinte)));
    if (id != Identite.confirmee) return;
    setState(() => _enCours = true);
    final e = await r.changerMembre(adresse, groupe);
    if (!mounted) return;
    setState(() => _enCours = false);
    messager.showSnackBar(SnackBar(content: Text(e ?? fait)));
  }

  Future<void> _ajouter() async {
    final choix = await _fenetreAjout(context);
    if (choix == null || !mounted) return;
    final (adresse, groupe) = choix;
    await _changer(adresse, groupe,
        raison: "Ajouter $adresse à l'équipe (${_nomGroupe(groupe)})",
        fait: "$adresse entre dans l'équipe : invite ses appareils depuis Appareils.");
  }

  Future<void> _ouvrir(Membre m) async {
    final choix = await _fenetreMembre(context, m);
    if (choix == null || !mounted) return;
    if (choix.isEmpty) {
      if (await _confirmerRetrait(context, m) != true || !mounted) return;
      await _changer(m.adresse, '',
          raison: "Retirer ${m.adresse} de l'équipe", fait: "${m.adresse} retiré : ses appareils sont coupés.");
      return;
    }
    await _changer(m.adresse, choix,
        raison: 'Mettre ${m.adresse} dans le groupe $choix', fait: '${m.adresse} : ${_nomGroupe(choix)}');
  }

  @override
  Widget build(BuildContext context) {
    final r = EtatReseau.of(context);
    final admins = r.membres.where((m) => m.admin).length;
    return Scaffold(
      body: Fond(
        centre: const Alignment(0, -0.72),
        etendue: const Size(0.8, 0.24),
        child: SafeArea(
          child: Center(
            child: ConstrainedBox(
              constraints: const BoxConstraints(maxWidth: 560),
              child: Padding(
                padding: const EdgeInsets.fromLTRB(16, 6, 16, 20),
                child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
                  const Align(alignment: Alignment.centerLeft, child: BoutonRetour('Réglages')),
                  const SizedBox(height: 16),
                  Expanded(
                    child: SansDefilement(
                      child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, mainAxisSize: MainAxisSize.min, children: [
                        Padding(
                          padding: const EdgeInsets.symmetric(horizontal: 4),
                          child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                            Text('Équipe', style: titre(26)),
                            const SizedBox(height: 8),
                            Text(
                              "Les comptes Google qui peuvent rejoindre le réseau. Retirer quelqu'un coupe tous ses appareils.",
                              style: texte(13.5, couleur: Couleurs.secondaire, hauteur: 1.4),
                            ),
                          ]),
                        ),
                        const SizedBox(height: 16),
                        if (_erreur != null) ...[
                          Carte(
                            padding: const EdgeInsets.all(14),
                            fond: Couleurs.rouge.withValues(alpha: 0.06),
                            bord: Couleurs.rouge.withValues(alpha: 0.35),
                            onTap: _lire,
                            child: Text('$_erreur\nTouche pour réessayer.', style: texte(13.5, couleur: Couleurs.rougeClair, hauteur: 1.4)),
                          ),
                          const SizedBox(height: 14),
                        ],
                        Padding(
                          padding: const EdgeInsets.fromLTRB(6, 0, 6, 8),
                          child: Etiquette('${r.membres.length} membre${r.membres.length > 1 ? 's' : ''} · $admins admin${admins > 1 ? 's' : ''}'),
                        ),
                        Carte(
                          child: Column(children: [
                            for (var i = 0; i < r.membres.length; i++)
                              _LigneMembre(
                                m: r.membres[i],
                                dernier: i == r.membres.length - 1,
                                onTap: r.membres[i].moi ? null : () => _ouvrir(r.membres[i]),
                              ),
                          ]),
                        ),
                      ]),
                    ),
                  ),
                  const SizedBox(height: 14),
                  BoutonContour(libelle: _enCours ? 'Enregistrement…' : 'Ajouter un membre', ico: Ico.plus, hauteur: 52, onTap: _enCours ? null : _ajouter),
                ]),
              ),
            ),
          ),
        ),
      ),
    );
  }
}

String _nomGroupe(String g) => g == 'admins' ? 'admin' : 'équipe';

/// Une ligne : l'initiale, l'adresse et le groupe. « admins » en cyan,
/// pour ne pas passer inaperçu ; sa propre ligne ne s'ouvre pas.
class _LigneMembre extends StatelessWidget {
  const _LigneMembre({required this.m, required this.dernier, this.onTap});
  final Membre m;
  final bool dernier;
  final VoidCallback? onTap;

  @override
  Widget build(BuildContext context) => InkWell(
        onTap: onTap,
        child: Container(
          padding: const EdgeInsets.fromLTRB(14, 10, 14, 10),
          decoration: BoxDecoration(border: dernier ? null : const Border(bottom: BorderSide(color: Couleurs.separateur))),
          child: Row(children: [
            Avatar(lettre: prenom(m.adresse)[0], taille: 36, plein: m.moi),
            const SizedBox(width: 12),
            Expanded(
              child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                Text(m.adresse, style: texte(14.5, graisse: 500), maxLines: 1, overflow: TextOverflow.ellipsis),
                const SizedBox(height: 2),
                Text(
                  m.admin ? "admins, droits d'admin" : 'equipe',
                  style: texte(12.5, couleur: m.admin ? Couleurs.cyan : Couleurs.secondaire),
                ),
              ]),
            ),
            const SizedBox(width: 8),
            if (m.moi) const Puce('TOI', couleur: Couleurs.secondaire, fond: false) else const Chevron(),
          ]),
        ),
      );
}

/// Le choix du groupe : deux cases côte à côte. « admins » donne les
/// droits d'admin : écrit en toutes lettres.
class _ChoixGroupe extends StatelessWidget {
  const _ChoixGroupe({required this.valeur, required this.onChanged});
  final String valeur;
  final ValueChanged<String> onChanged;

  @override
  Widget build(BuildContext context) {
    Widget choix(String g, String titre, String detail) {
      final pris = valeur == g;
      return Expanded(
        child: Semantics(
          selected: pris,
          button: true,
          child: Carte(
            rayon: 14,
            fond: pris ? Couleurs.cyan.withValues(alpha: 0.08) : Couleurs.bloc,
            bord: pris ? Couleurs.cyan.withValues(alpha: 0.6) : Couleurs.bordure,
            onTap: () => onChanged(g),
            padding: const EdgeInsets.fromLTRB(12, 10, 12, 10),
            child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
              Text(titre, style: texte(14.5, graisse: 600, couleur: pris ? Couleurs.cyan : Couleurs.texte)),
              const SizedBox(height: 2),
              Text(detail, style: texte(12, couleur: Couleurs.secondaire), maxLines: 1, overflow: TextOverflow.ellipsis),
            ]),
          ),
        ),
      );
    }

    return Row(children: [
      choix('equipe', 'equipe', 'selon la politique'),
      const SizedBox(width: 10),
      choix('admins', 'admins', "droits d'admin"),
    ]);
  }
}

/// La fenêtre des fenêtres de cet écran : centrée, sombre, bordée.
Widget _fenetre(BuildContext context, {required Widget child, Gradient? bordure}) => Dialog(
      backgroundColor: Colors.transparent,
      insetPadding: const EdgeInsets.symmetric(horizontal: 24),
      child: ConstrainedBox(
        constraints: const BoxConstraints(maxWidth: 420),
        child: Bordee(
          bordure: bordure ?? Bords.reflet,
          fond: const Color(0xFF0A1119),
          rayon: 24,
          padding: const EdgeInsets.fromLTRB(22, 22, 22, 18),
          child: child,
        ),
      ),
    );

/// Ajouter quelqu'un : son adresse Google et son groupe. Rend (adresse,
/// groupe), ou null.
Future<(String, String)?> _fenetreAjout(BuildContext context) async {
  final champ = TextEditingController();
  var groupe = 'equipe';
  String? erreur;
  final r = await showDialog<(String, String)>(
    context: context,
    barrierColor: const Color(0xA8020407),
    builder: (context) => StatefulBuilder(
      builder: (context, setState) {
        void valider() {
          final a = champ.text.trim().toLowerCase();
          if (!Membre.adresseValide(a)) {
            setState(() => erreur = 'Une adresse Google complète : prenom@gmail.com');
            return;
          }
          Navigator.pop(context, (a, groupe));
        }

        return _fenetre(
          context,
          child: Column(mainAxisSize: MainAxisSize.min, crossAxisAlignment: CrossAxisAlignment.stretch, children: [
            Text('Ajouter un membre', style: texte(20, graisse: 600, espacement: -0.4)),
            const SizedBox(height: 6),
            Text(
              "Son compte Google pourra rejoindre le réseau. Ses appareils devront encore être signés.",
              style: texte(13.5, couleur: Couleurs.secondaire, hauteur: 1.4),
            ),
            const SizedBox(height: 16),
            TextField(
              controller: champ,
              autofocus: true,
              autocorrect: false,
              enableSuggestions: false,
              keyboardType: TextInputType.emailAddress,
              textInputAction: TextInputAction.done,
              onSubmitted: (_) => valider(),
              style: texte(15),
              decoration: InputDecoration(
                hintText: 'adresse@gmail.com',
                hintStyle: texte(15, couleur: Couleurs.tertiaire),
                errorText: erreur,
                errorStyle: texte(12.5, couleur: Couleurs.rougeClair),
                filled: true,
                fillColor: Couleurs.bloc,
                contentPadding: const EdgeInsets.symmetric(horizontal: 14, vertical: 14),
                enabledBorder: OutlineInputBorder(borderRadius: BorderRadius.circular(14), borderSide: const BorderSide(color: Couleurs.bordure)),
                focusedBorder: OutlineInputBorder(borderRadius: BorderRadius.circular(14), borderSide: const BorderSide(color: Couleurs.cyan)),
                errorBorder: OutlineInputBorder(borderRadius: BorderRadius.circular(14), borderSide: const BorderSide(color: Couleurs.rouge)),
                focusedErrorBorder: OutlineInputBorder(borderRadius: BorderRadius.circular(14), borderSide: const BorderSide(color: Couleurs.rouge)),
              ),
            ),
            const SizedBox(height: 14),
            const Etiquette('Groupe'),
            const SizedBox(height: 8),
            _ChoixGroupe(valeur: groupe, onChanged: (g) => setState(() => groupe = g)),
            const SizedBox(height: 18),
            Row(children: [
              Expanded(child: BoutonFantome(libelle: 'Annuler', onTap: () => Navigator.pop(context))),
              const SizedBox(width: 10),
              Expanded(
                child: BoutonFantome(
                  libelle: 'Ajouter',
                  couleur: Couleurs.cyan,
                  bord: Couleurs.cyan.withValues(alpha: 0.5),
                  onTap: valider,
                ),
              ),
            ]),
          ]),
        );
      },
    ),
  );
  // La fenêtre s'efface encore un instant avec le champ : on attend.
  Future<void>.delayed(const Duration(milliseconds: 400), champ.dispose);
  return r;
}

/// La fiche d'un membre : changer son groupe, ou le retirer. Rend le
/// nouveau groupe, '' pour retirer, ou null.
Future<String?> _fenetreMembre(BuildContext context, Membre m) => showDialog<String>(
      context: context,
      barrierColor: const Color(0xA8020407),
      builder: (context) {
        var groupe = m.groupe;
        return StatefulBuilder(
          builder: (context, setState) => _fenetre(
            context,
            child: Column(mainAxisSize: MainAxisSize.min, crossAxisAlignment: CrossAxisAlignment.stretch, children: [
              Text(prenom(m.adresse), style: texte(20, graisse: 600, espacement: -0.4)),
              const SizedBox(height: 4),
              Text(m.adresse, style: mono(13, graisse: 400, couleur: Couleurs.secondaire), maxLines: 1, overflow: TextOverflow.ellipsis),
              const SizedBox(height: 16),
              const Etiquette('Groupe'),
              const SizedBox(height: 8),
              _ChoixGroupe(valeur: groupe, onChanged: (g) => setState(() => groupe = g)),
              const SizedBox(height: 18),
              Row(children: [
                Expanded(child: BoutonFantome(libelle: 'Annuler', onTap: () => Navigator.pop(context))),
                const SizedBox(width: 10),
                Expanded(
                  child: BoutonFantome(
                    libelle: 'Changer',
                    couleur: groupe == m.groupe ? Couleurs.tertiaire : Couleurs.cyan,
                    bord: groupe == m.groupe ? Couleurs.bordure : Couleurs.cyan.withValues(alpha: 0.5),
                    onTap: groupe == m.groupe ? null : () => Navigator.pop(context, groupe),
                  ),
                ),
              ]),
              const SizedBox(height: 10),
              BoutonFantome(
                libelle: "Retirer de l'équipe",
                couleur: Couleurs.rougeClair,
                bord: Couleurs.rouge.withValues(alpha: 0.4),
                onTap: () => Navigator.pop(context, ''),
              ),
            ]),
          ),
        );
      },
    );

/// Retirer quelqu'un de l'équipe : on le redit, puis le doigt.
Future<bool?> _confirmerRetrait(BuildContext context, Membre m) => showDialog<bool>(
      context: context,
      barrierColor: const Color(0xA8020407),
      builder: (context) => _fenetre(
        context,
        bordure: Bords.accent(Couleurs.rouge),
        child: Column(mainAxisSize: MainAxisSize.min, crossAxisAlignment: CrossAxisAlignment.stretch, children: [
          Text('Retirer ${prenom(m.adresse)} ?', style: texte(20, graisse: 600, espacement: -0.4)),
          const SizedBox(height: 4),
          Text(m.adresse, style: mono(13, graisse: 400, couleur: Couleurs.secondaire), maxLines: 1, overflow: TextOverflow.ellipsis),
          const SizedBox(height: 12),
          Text(
            "Ce compte ne pourra plus rejoindre le réseau, ni ouvrir les services web. Tous ses appareils sont coupés dans les secondes qui suivent. "
            "S'ils étaient signés, révoque-les aussi : le verrou les refusera même si le serveur était piraté.",
            style: texte(14, couleur: Couleurs.secondaire, hauteur: 1.4),
          ),
          const SizedBox(height: 20),
          Row(children: [
            Expanded(child: BoutonFantome(libelle: 'Annuler', onTap: () => Navigator.pop(context, false))),
            const SizedBox(width: 10),
            Expanded(
              child: BoutonFantome(
                libelle: 'Retirer',
                couleur: Couleurs.rougeClair,
                bord: Couleurs.rouge.withValues(alpha: 0.45),
                onTap: () => Navigator.pop(context, true),
              ),
            ),
          ]),
        ]),
      ),
    );
