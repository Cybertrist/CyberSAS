import 'package:flutter/material.dart';

import '../composants.dart';
import '../donnees.dart';
import '../etat.dart';
import '../icones.dart';
import '../securite.dart';
import '../theme.dart';
import 'reglages.dart';

/// Admin : les appareils qui attendent la signature du verrou. On compare
/// l'empreinte de la clé avec celle affichée sur l'appareil, puis on signe
/// avec le doigt, par l'invite biométrique du téléphone.
class EcranDemandes extends StatelessWidget {
  const EcranDemandes({super.key});

  @override
  Widget build(BuildContext context) {
    final r = EtatReseau.of(context);
    final n = r.demandes.length;
    return Scaffold(
      body: Fond(
        centre: const Alignment(0, -0.72),
        etendue: const Size(0.8, 0.24),
        child: SafeArea(
          child: Center(
            child: ConstrainedBox(
              constraints: const BoxConstraints(maxWidth: 560),
              child: ListView(
                padding: const EdgeInsets.fromLTRB(16, 6, 16, 16),
                children: [
                  const Align(alignment: Alignment.centerLeft, child: BoutonRetour('Appareils')),
                  const SizedBox(height: 16),
                  Padding(
                    padding: const EdgeInsets.symmetric(horizontal: 4),
                    child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                      Text('Demandes en attente', style: titre(26)),
                      const SizedBox(height: 8),
                      Text(
                        "Vérifie que l'empreinte est la même que sur l'appareil, relis ce que tu signes, puis pose ton doigt.",
                        style: texte(13.5, couleur: Couleurs.secondaire, hauteur: 1.4),
                      ),
                    ]),
                  ),
                  // Admin, mais sans la clé du verrou : on voit les demandes, on
                  // ne peut pas les signer.
                  if (r.reel && !r.cleVerrouPresente && r.demandes.isNotEmpty) ...[
                    const SizedBox(height: 14),
                    Carte(
                      padding: const EdgeInsets.all(16),
                      fond: Couleurs.rouge.withValues(alpha: 0.06),
                      bord: Couleurs.rouge.withValues(alpha: 0.35),
                      child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                        Row(children: [
                          const Icone(Ico.cadenas, couleur: Couleurs.rougeClair, taille: 18),
                          const SizedBox(width: 10),
                          Expanded(child: Text("Ton téléphone n'a pas la clé du verrou", style: texte(15, graisse: 600))),
                        ]),
                        const SizedBox(height: 6),
                        Text("Sans elle, tu vois les demandes mais tu ne peux pas les signer.",
                            style: texte(13.5, couleur: Couleurs.secondaire, hauteur: 1.4)),
                        const SizedBox(height: 12),
                        BoutonContour(libelle: 'Ranger la clé', ico: Ico.cle, hauteur: 44, onTap: () => rangerCleVerrou(context)),
                      ]),
                    ),
                  ],
                  for (final d in r.demandes) ...[
                    const SizedBox(height: 14),
                    _CarteDemande(d: d),
                  ],
                  if (n == 0) ...[
                    const SizedBox(height: 48),
                    const Center(child: Icone(Ico.coche, couleur: Couleurs.vert, taille: 34, lueur: true)),
                    const SizedBox(height: 12),
                    Text('Aucune demande en attente.', textAlign: TextAlign.center, style: texte(15, couleur: Couleurs.secondaire)),
                  ],
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }
}

class _CarteDemande extends StatelessWidget {
  const _CarteDemande({required this.d});
  final Demande d;

  @override
  Widget build(BuildContext context) {
    final r = EtatReseau.of(context);
    return Carte(
      rayon: 22,
      padding: const EdgeInsets.all(14),
      child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
        Row(children: [
          CaseIcone(d.type.ico, couleur: Couleurs.bleu, etat: EtatIcone.attente, taille: 42),
          const SizedBox(width: 12),
          Expanded(
            child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
              Text(d.nom, style: texte(16, graisse: 600)),
              const SizedBox(height: 3),
              Text('${d.compte} · ${d.type.libelle}',
                  style: texte(12.5, couleur: Couleurs.secondaire), maxLines: 1, overflow: TextOverflow.ellipsis),
            ]),
          ),
        ]),
        const SizedBox(height: 14),
        const Etiquette('Empreinte de sa clé'),
        const SizedBox(height: 8),
        Empreinte(d.empreinte, couleur: Couleurs.texte),
        const SizedBox(height: 14),
        _CeQuiEstSigne(d: d),
        const SizedBox(height: 14),
        Row(children: [
          Expanded(
            child: BoutonFantome(
              libelle: 'Refuser',
              couleur: Couleurs.rougeClair,
              bord: Couleurs.rouge.withValues(alpha: 0.4),
              onTap: () async {
                // Pas besoin de la clé du verrou pour refuser, mais du doigt,
                // comme pour signer : un téléphone laissé ouvert ne vide pas
                // la liste.
                final messager = ScaffoldMessenger.of(context);
                final id = await confirmerIdentite('Refuser ${d.nom}');
                if (id == Identite.impossible) messager.showSnackBar(const SnackBar(content: Text(sansEmpreinte)));
                if (id != Identite.confirmee) return;
                final e = await r.traiter(d);
                messager.showSnackBar(SnackBar(content: Text(e ?? '${d.nom} refusé')));
              },
            ),
          ),
          const SizedBox(width: 10),
          Expanded(child: _BoutonSigner(d: d)),
        ]),
      ]),
    );
  }
}

/// Ce que le verrou inscrira dans le certificat, tel que le serveur le
/// propose : l'admin le voit avant de poser le doigt. Un serveur piraté qui
/// glisserait « admins » ou l'adresse d'une autre machine se voit ici.
class _CeQuiEstSigne extends StatelessWidget {
  const _CeQuiEstSigne({required this.d});
  final Demande d;

  @override
  Widget build(BuildContext context) {
    Widget ligne(String libelle, String valeur, {bool monoValeur = false, Color couleur = Couleurs.texte}) => Padding(
          padding: const EdgeInsets.symmetric(vertical: 4),
          child: Row(children: [
            Text(libelle, style: texte(13, couleur: Couleurs.secondaire)),
            const SizedBox(width: 12),
            Expanded(
              child: Text(
                valeur,
                textAlign: TextAlign.right,
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: monoValeur ? mono(13, graisse: 500, couleur: couleur) : texte(13, graisse: 600, couleur: couleur),
              ),
            ),
          ]),
        );
    final admins = d.groupe == 'admins';
    return Container(
      padding: const EdgeInsets.fromLTRB(12, 8, 12, 8),
      decoration: BoxDecoration(
        color: Couleurs.bloc,
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: Couleurs.bordure),
      ),
      child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
        const Etiquette('Ce que tu signes'),
        const SizedBox(height: 4),
        ligne('Adresse', d.adresse.isEmpty ? '?' : d.adresse, monoValeur: true),
        // « admins » donne les droits d'admin : en cyan, pour ne pas passer
        // inaperçu.
        ligne('Groupe', d.groupe.isEmpty ? '?' : (admins ? "admins, droits d'admin" : d.groupe),
            couleur: admins ? Couleurs.cyan : Couleurs.texte),
        ligne(d.etiquette.isNotEmpty ? 'Machine' : 'Propriétaire', d.etiquette.isNotEmpty ? d.etiquette : d.titulaire),
        ligne('Validité', '${Demande.dureeSignature} jours'),
      ]),
    );
  }
}

/// « Signer » : l'invite d'empreinte d'Android s'ouvre tout de suite, sans
/// fenêtre intermédiaire.
class _BoutonSigner extends StatefulWidget {
  const _BoutonSigner({required this.d});
  final Demande d;

  @override
  State<_BoutonSigner> createState() => _BoutonSignerState();
}

class _BoutonSignerState extends State<_BoutonSigner> {
  bool _enCours = false;

  // La clé du verrou est dans le coffre du téléphone (Coffre.kt), sous une
  // clé du Keystore qui ne s'ouvre que pour une opération autorisée par
  // l'empreinte. L'invite vient d'Android (MainActivity), liée à cette
  // signature-là : pas d'invite ici, elle n'ouvrirait rien.
  Future<void> _signer() async {
    if (_enCours) return;
    final r = EtatReseau.of(context);
    final messager = ScaffoldMessenger.of(context);
    setState(() => _enCours = true);
    final d = widget.d;
    if (r.reel && !r.cleVerrouPresente) {
      setState(() => _enCours = false);
      messager.showSnackBar(const SnackBar(
        content: Text("La clé du verrou n'est pas sur ce téléphone : Réglages, Clé du verrou."),
      ));
      return;
    }
    final e = await r.signer(d);
    // Invite fermée : on revient, sans rien dire.
    if (e != operationAnnulee) {
      messager.showSnackBar(SnackBar(content: Text(e ?? '${d.nom} signé : il rejoint le réseau')));
    }
    if (mounted) setState(() => _enCours = false);
  }

  @override
  Widget build(BuildContext context) {
    final r = EtatReseau.of(context);
    // Sans la clé du verrou : grisé, avec un cadenas.
    final sansCle = r.reel && !r.cleVerrouPresente;
    final c = sansCle ? Couleurs.tertiaire : Couleurs.vert;
    return Carte(
      rayon: 14,
      fond: c.withValues(alpha: sansCle ? 0.06 : 0.12),
      bord: c.withValues(alpha: 0.5),
      onTap: sansCle ? null : _signer,
      child: SizedBox(
        height: 42,
        child: Row(mainAxisAlignment: MainAxisAlignment.center, children: [
          Icone(sansCle ? Ico.cadenas : Ico.empreinte, couleur: c, taille: 18),
          const SizedBox(width: 9),
          Text('Signer', style: texte(15, graisse: 600, couleur: c)),
        ]),
      ),
    );
  }
}
