import 'package:flutter/material.dart';
import 'package:share_plus/share_plus.dart';

import '../composants.dart';
import '../donnees.dart';
import '../etat.dart';
import '../icones.dart';
import '../theme.dart';

/// La sauvegarde de secours de la clé du verrou, depuis les Réglages : une
/// phrase de passe tapée deux fois, l'empreinte qui ouvre le coffre pour
/// cette seule opération, puis le texte chiffré, à partager hors du
/// téléphone. La clé elle-même ne passe jamais par ici : seul le texte
/// chiffré remonte du moteur.
Future<void> sauvegarderSecours(BuildContext context) async {
  final r = EtatReseau.of(context);
  final messager = ScaffoldMessenger.of(context);
  final phrase = await showDialog<String>(
    context: context,
    barrierColor: const Color(0xA8020407),
    builder: (_) => const FenetrePhrase(),
  );
  if (phrase == null) return;
  final (:texte, :erreur) = await r.sauvegarderVerrou(phrase);
  if (texte == null) {
    // Invite fermée : on revient, sans rien dire.
    if (erreur != null && erreur != operationAnnulee) messager.showSnackBar(SnackBar(content: Text(erreur)));
    return;
  }
  if (!context.mounted) return;
  await showDialog<void>(
    context: context,
    barrierColor: const Color(0xA8020407),
    builder: (_) => FenetreSecours(sauvegarde: texte, empreinte: r.empreinteVerrou),
  );
}

/// Ce que dit la phrase en cours de saisie : ce qui manque, ou qu'elle va.
({String message, Color couleur, bool ok}) _avis(String phrase, String encore) {
  final n = phrase.runes.length;
  if (n == 0) return (message: '$phraseMin caractères au moins.', couleur: Couleurs.secondaire, ok: false);
  if (n < phraseMin) {
    final reste = phraseMin - n;
    return (message: 'Encore $reste caractère${reste > 1 ? 's' : ''} au moins.', couleur: Couleurs.rougeClair, ok: false);
  }
  if (encore.isEmpty) return (message: 'Tape-la une seconde fois.', couleur: Couleurs.secondaire, ok: false);
  if (encore != phrase) return (message: 'Les deux phrases diffèrent.', couleur: Couleurs.rougeClair, ok: false);
  return (message: 'Phrase acceptée.', couleur: Couleurs.vert, ok: true);
}

/// La phrase de passe, deux fois, dans des champs masqués. Rend la phrase,
/// ou null si l'admin renonce.
class FenetrePhrase extends StatefulWidget {
  const FenetrePhrase({super.key});

  @override
  State<FenetrePhrase> createState() => _FenetrePhraseState();
}

class _FenetrePhraseState extends State<FenetrePhrase> {
  final _phrase = TextEditingController();
  final _encore = TextEditingController();

  @override
  void dispose() {
    _phrase.dispose();
    _encore.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final avis = _avis(_phrase.text, _encore.text);
    void valider() {
      if (avis.ok) Navigator.pop(context, _phrase.text);
    }

    return Fenetre(enfants: [
      Text('Sauvegarde de secours', style: texte(20, graisse: 600, espacement: -0.4)),
      const SizedBox(height: 6),
      Text(
        "La clé du verrou, chiffrée par une phrase que toi seul connais. Si ce téléphone disparaît, "
        "cette sauvegarde et sa phrase suffisent pour signer à nouveau, sans réinscrire le réseau.",
        style: texte(13.5, couleur: Couleurs.secondaire, hauteur: 1.4),
      ),
      const SizedBox(height: 16),
      ChampMasque(
        controleur: _phrase,
        indication: 'Phrase de passe',
        autofocus: true,
        onChanged: (_) => setState(() {}),
      ),
      const SizedBox(height: 10),
      ChampMasque(
        controleur: _encore,
        indication: 'Encore une fois',
        action: TextInputAction.done,
        onChanged: (_) => setState(() {}),
        onSubmitted: (_) => valider(),
      ),
      const SizedBox(height: 8),
      Text(avis.message, style: texte(13, couleur: avis.couleur)),
      const SizedBox(height: 10),
      Text(
        "Quelques mots pris au hasard valent mieux qu'un mot compliqué. Oubliée, la phrase ne se retrouve pas : "
        "personne ne peut ouvrir la sauvegarde sans elle.",
        style: texte(12.5, couleur: Couleurs.tertiaire, hauteur: 1.4),
      ),
      const SizedBox(height: 18),
      Row(children: [
        Expanded(child: BoutonFantome(libelle: 'Annuler', onTap: () => Navigator.pop(context))),
        const SizedBox(width: 10),
        Expanded(
          child: BoutonFantome(
            libelle: 'Chiffrer',
            couleur: avis.ok ? Couleurs.cyan : Couleurs.tertiaire,
            bord: avis.ok ? Couleurs.cyan.withValues(alpha: 0.5) : Couleurs.bordure,
            onTap: avis.ok ? valider : null,
          ),
        ),
      ]),
    ]);
  }
}

/// La sauvegarde est prête : où la ranger, ce qu'elle protège, et le
/// partage (gestionnaire de mots de passe, courriel à soi, fichier).
class FenetreSecours extends StatelessWidget {
  const FenetreSecours({super.key, required this.sauvegarde, required this.empreinte});
  final String sauvegarde;

  /// L'empreinte du verrou sauvegardé.
  final String empreinte;

  Future<void> _partager() => SharePlus.instance.share(ShareParams(
        subject: 'Sauvegarde de secours CyberSAS, verrou $empreinte',
        text: sauvegarde,
      ));

  @override
  Widget build(BuildContext context) => Fenetre(enfants: [
        Text('Sauvegarde prête', style: texte(20, graisse: 600, espacement: -0.4)),
        const SizedBox(height: 12),
        Container(
          padding: const EdgeInsets.fromLTRB(14, 12, 14, 12),
          decoration: BoxDecoration(
            color: Couleurs.bloc,
            borderRadius: BorderRadius.circular(14),
            border: Border.all(color: Couleurs.bordure),
          ),
          child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
            Row(children: [
              const Icone(Ico.cle, couleur: Couleurs.cyan, taille: 17, lueur: true),
              const SizedBox(width: 8),
              Text('Verrou ', style: texte(13.5, couleur: Couleurs.secondaire)),
              Flexible(child: Text(empreinte, style: mono(13.5), maxLines: 1, overflow: TextOverflow.ellipsis)),
            ]),
            const SizedBox(height: 8),
            Text(sauvegarde, style: mono(12, graisse: 400, couleur: Couleurs.etiquette), maxLines: 2, overflow: TextOverflow.ellipsis),
          ]),
        ),
        const SizedBox(height: 14),
        Row(crossAxisAlignment: CrossAxisAlignment.start, children: [
          const Padding(padding: EdgeInsets.only(top: 1), child: Icone(Ico.info, couleur: Couleurs.rouge, taille: 18)),
          const SizedBox(width: 10),
          Expanded(
            child: Text(
              "Range-la hors de ce téléphone : gestionnaire de mots de passe, clé USB, papier au coffre. "
              "Une copie qui reste ici disparaît avec lui.",
              style: texte(13.5, hauteur: 1.4),
            ),
          ),
        ]),
        const SizedBox(height: 10),
        Text(
          "Elle ne vaut que ce que vaut ta phrase : qui vole ce texte peut essayer des phrases sans limite. "
          "Ne range jamais la phrase au même endroit.",
          style: texte(13, couleur: Couleurs.secondaire, hauteur: 1.4),
        ),
        const SizedBox(height: 18),
        Row(children: [
          Expanded(child: BoutonFantome(libelle: 'Fermer', onTap: () => Navigator.pop(context))),
          const SizedBox(width: 10),
          Expanded(
            child: BoutonFantome(
              libelle: 'Partager',
              couleur: Couleurs.cyan,
              bord: Couleurs.cyan.withValues(alpha: 0.5),
              onTap: _partager,
            ),
          ),
        ]),
      ]);
}
