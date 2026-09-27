import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../composants.dart';
import '../donnees.dart';
import '../etat.dart';
import '../icones.dart';
import '../securite.dart';
import '../theme.dart';
import 'detail.dart';

/// Le compte, cet appareil, la sécurité de l'appli, le réseau. Pas de
/// « VPN toujours actif » ni de démarrage automatique : le tunnel s'allume
/// et se coupe à la main, depuis l'accueil. Les demandes en attente sont
/// sur la page Appareils, pas ici.
class EcranReglages extends StatelessWidget {
  const EcranReglages({super.key});

  @override
  Widget build(BuildContext context) {
    final r = EtatReseau.of(context);
    final format = formatDe(context);
    final titre0 = Padding(
      padding: const EdgeInsets.symmetric(horizontal: 6),
      child: Text('Réglages', style: titre(format == Format.compact ? 28 : 32)),
    );
    final compte = _Compte(r: r);
    final appareil = _CetAppareil(r: r);
    final securite = _Securite(r: r);
    final reseau = _Reseau(r: r);
    final quitter = _Quitter(r: r);
    const espace = SizedBox(height: 18);

    return switch (format) {
      Format.compact => ListView(
          padding: const EdgeInsets.fromLTRB(16, 12, 16, 12),
          children: [titre0, espace, compte, espace, appareil, espace, securite, espace, reseau, espace, quitter],
        ),
      Format.portrait => Align(
          alignment: Alignment.topCenter,
          child: ConstrainedBox(
            constraints: const BoxConstraints(maxWidth: 620),
            child: ListView(
              padding: const EdgeInsets.fromLTRB(28, 16, 28, 12),
              children: [titre0, espace, compte, espace, appareil, espace, securite, espace, reseau, espace, quitter],
            ),
          ),
        ),
      Format.paysage => SingleChildScrollView(
          padding: const EdgeInsets.fromLTRB(28, 16, 28, 24),
          child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
            titre0,
            espace,
            Row(crossAxisAlignment: CrossAxisAlignment.start, children: [
              Expanded(child: Column(children: [compte, espace, appareil, espace, quitter])),
              const SizedBox(width: 24),
              Expanded(child: Column(children: [securite, espace, reseau])),
            ]),
          ]),
        ),
    };
  }
}

class _Compte extends StatelessWidget {
  const _Compte({required this.r});
  final Reseau r;

  @override
  Widget build(BuildContext context) => Bordee(
        bordure: Bords.reflet,
        fond: const Color(0xED090F16),
        halo: haloCarte(),
        padding: const EdgeInsets.fromLTRB(15, 12, 15, 12),
        child: Row(children: [
          Avatar(lettre: r.compte.isEmpty ? '?' : r.compte[0], taille: 42, plein: true),
          const SizedBox(width: 14),
          Expanded(
            child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
              Text(r.compte, style: texte(16, graisse: 600), maxLines: 1, overflow: TextOverflow.ellipsis),
              const SizedBox(height: 2),
              // Le compte de l'inscription ; la connexion Google n'existe pas encore.
              Text(r.courriel.isNotEmpty ? r.courriel : "Invité par l'admin",
                  style: texte(12.5, couleur: Couleurs.secondaire), maxLines: 1, overflow: TextOverflow.ellipsis),
            ]),
          ),
          if (r.admin) const Puce('ADMIN', couleur: Couleurs.texte, fond: false),
        ]),
      );
}

/// Ce qui identifie ce téléphone sur le réseau : son nom (qu'on peut
/// changer), son adresse, son certificat et l'empreinte de sa clé, celle
/// que l'admin compare avant de signer.
class _CetAppareil extends StatelessWidget {
  const _CetAppareil({required this.r});
  final Reseau r;

  @override
  Widget build(BuildContext context) {
    final moi = r.moi;
    final jours = moi.certificat.joursRestants(DateTime.now());
    return Groupe(titre: 'Cet appareil', enfants: [
      LigneReglage(
        ico: Ico.etiquette,
        libelle: 'Nom',
        valeur: moi.nomAffiche,
        fin: r.peutRenommer(moi) ? const Chevron() : null,
        onTap: r.peutRenommer(moi) ? () => renommerAppareil(context, moi) : null,
        dense: true,
      ),
      LigneReglage(ico: Ico.repere, libelle: 'Adresse privée', valeur: moi.adresse, valeurMono: true, dense: true),
      LigneReglage(
        ico: Ico.bouclier,
        couleurIco: switch (moi.etatCertificat) {
          EtatCertificat.signe => !moi.certificat.finConnue || jours > 14 ? Couleurs.vert : Couleurs.rouge,
          EtatCertificat.attente => Couleurs.tertiaire,
          _ => Couleurs.rouge,
        },
        libelle: 'Certificat',
        valeur: switch (moi.etatCertificat) {
          EtatCertificat.signe => !moi.certificat.finConnue ? 'signé' : jours > 1 ? 'encore $jours j' : "moins d'un jour",
          EtatCertificat.revoque => 'révoqué',
          EtatCertificat.expire => 'expiré',
          EtatCertificat.attente => r.serveurInjoignable ? 'serveur injoignable' : 'pas encore signé',
        },
        dense: true,
      ),
      LigneReglage(
        ico: Ico.cle,
        libelle: 'Ma clé',
        valeur: moi.certificat.empreinte.join('-'),
        valeurMono: true,
        separateur: false,
        dense: true,
      ),
    ]);
  }
}

class _Securite extends StatelessWidget {
  const _Securite({required this.r});
  final Reseau r;

  Future<void> _verrou(BuildContext context, bool v) async {
    final messager = ScaffoldMessenger.of(context);
    // On vérifie que le doigt (ou le code du téléphone) passe avant
    // d'activer : sinon on s'enfermerait dehors.
    if (v) {
      final ok = await confirmerIdentite("Verrouiller CyberSAS avec l'empreinte", biometrieSeule: false);
      if (ok == Identite.impossible) {
        messager.showSnackBar(const SnackBar(content: Text('Aucune empreinte ni code sur ce téléphone.')));
      }
      if (ok != Identite.confirmee) return;
    }
    await r.reglerVerrou(v);
  }

  /// La clé du verrou, collée ou tapée dans un champ masqué, est vérifiée
  /// (elle doit être celle du verrou de ce réseau), puis rangée dans le
  /// coffre par l'invite d'empreinte d'Android. Quelle que soit l'issue
  /// (rangée, fenêtre fermée, empreinte ratée), le presse-papiers est vidé
  /// s'il contient encore la clé.
  Future<void> _importerVerrou(BuildContext context) async {
    final messager = ScaffoldMessenger.of(context);
    final champ = TextEditingController();
    String? collee;
    String? saisie;
    try {
      saisie = await _demanderVerrou(context, champ, (texte) => collee = texte);
      if (saisie == null || saisie.isEmpty) return;
      final e = await r.importerVerrou(saisie);
      // Invite fermée : on revient, sans rien dire.
      if (e == operationAnnulee) return;
      messager.showSnackBar(SnackBar(content: Text(e ?? 'Clé du verrou rangée : ce téléphone peut signer.')));
    } finally {
      final candidats = {collee, saisie, champ.text.trim()}.whereType<String>().where((s) => s.isNotEmpty).toSet();
      Future<void>.delayed(const Duration(milliseconds: 400), champ.dispose);
      await _oublierPressePapiers(candidats);
    }
  }

  /// Vide le presse-papiers s'il contient encore la clé du verrou, et
  /// seulement dans ce cas : ce que l'admin y a copié depuis ne nous
  /// regarde pas.
  static Future<void> _oublierPressePapiers(Set<String> cles) async {
    if (cles.isEmpty) return;
    try {
      final contenu = (await Clipboard.getData(Clipboard.kTextPlain))?.text?.trim() ?? '';
      if (cles.contains(contenu)) await Clipboard.setData(const ClipboardData(text: ''));
    } on PlatformException {
      // Presse-papiers illisible : rien à faire de plus.
    }
  }

  /// La fenêtre où l'admin colle la clé du verrou. [colle] reçoit ce que le
  /// bouton « Coller » a lu.
  Future<String?> _demanderVerrou(BuildContext context, TextEditingController champ, void Function(String) colle) => showDialog<String>(
      context: context,
      barrierColor: const Color(0xA8020407),
      builder: (context) => Dialog(
        backgroundColor: Colors.transparent,
        insetPadding: const EdgeInsets.symmetric(horizontal: 24),
        child: ConstrainedBox(
          constraints: const BoxConstraints(maxWidth: 420),
          child: Bordee(
            bordure: Bords.reflet,
            fond: const Color(0xFF0A1119),
            rayon: 24,
            padding: const EdgeInsets.fromLTRB(22, 22, 22, 18),
            child: Column(mainAxisSize: MainAxisSize.min, crossAxisAlignment: CrossAxisAlignment.stretch, children: [
              Text('Clé du verrou', style: texte(20, graisse: 600, espacement: -0.4)),
              const SizedBox(height: 6),
              Text(
                "Colle la clé privée du verrou (le fichier etat/verrou/cle du serveur). Elle sera chiffrée dans la puce "
                "du téléphone, et ne servira qu'après ton empreinte.",
                style: texte(13.5, couleur: Couleurs.secondaire, hauteur: 1.4),
              ),
              const SizedBox(height: 16),
              TextField(
                controller: champ,
                autofocus: true,
                obscureText: true,
                autocorrect: false,
                enableSuggestions: false,
                style: mono(15, graisse: 400),
                decoration: InputDecoration(
                  hintText: 'Clé en base64',
                  hintStyle: mono(15, graisse: 400, couleur: Couleurs.tertiaire),
                  filled: true,
                  fillColor: Couleurs.bloc,
                  contentPadding: const EdgeInsets.symmetric(horizontal: 14, vertical: 14),
                  suffixIcon: TextButton(
                    onPressed: () async {
                      final texte = (await Clipboard.getData(Clipboard.kTextPlain))?.text?.trim() ?? '';
                      colle(texte);
                      champ.text = texte;
                    },
                    child: Text('Coller', style: texte(13, graisse: 600, couleur: Couleurs.cyan)),
                  ),
                  enabledBorder: OutlineInputBorder(borderRadius: BorderRadius.circular(14), borderSide: const BorderSide(color: Couleurs.bordure)),
                  focusedBorder: OutlineInputBorder(borderRadius: BorderRadius.circular(14), borderSide: const BorderSide(color: Couleurs.cyan)),
                ),
              ),
              const SizedBox(height: 18),
              Row(children: [
                Expanded(child: BoutonFantome(libelle: 'Annuler', onTap: () => Navigator.pop(context))),
                const SizedBox(width: 10),
                Expanded(
                  child: BoutonFantome(
                    libelle: 'Ranger',
                    couleur: Couleurs.cyan,
                    bord: Couleurs.cyan.withValues(alpha: 0.5),
                    onTap: () => Navigator.pop(context, champ.text.trim()),
                  ),
                ),
              ]),
            ]),
          ),
        ),
      ),
    );

  /// Retirer la clé du verrou de ce téléphone : il ne pourra plus signer.
  /// La clé elle-même reste sur le serveur du verrou (etat/verrou/cle).
  Future<void> _retirerVerrou(BuildContext context) async {
    final messager = ScaffoldMessenger.of(context);
    final oui = await showDialog<bool>(
      context: context,
      barrierColor: const Color(0xA8020407),
      builder: (context) => Dialog(
        backgroundColor: Colors.transparent,
        insetPadding: const EdgeInsets.symmetric(horizontal: 24),
        child: ConstrainedBox(
          constraints: const BoxConstraints(maxWidth: 400),
          child: Bordee(
            bordure: Bords.accent(Couleurs.rouge),
            fond: const Color(0xFF0A1119),
            rayon: 24,
            padding: const EdgeInsets.fromLTRB(22, 22, 22, 18),
            child: Column(mainAxisSize: MainAxisSize.min, crossAxisAlignment: CrossAxisAlignment.stretch, children: [
              Text('Retirer la clé du verrou ?', style: texte(20, graisse: 600, espacement: -0.4)),
              const SizedBox(height: 8),
              Text(
                "Ce téléphone ne pourra plus signer les nouveaux appareils. Tu restes admin et sur le réseau ; pour signer à nouveau, il faudra ranger la clé une autre fois.",
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
        ),
      ),
    );
    if (oui != true) return;
    await r.oublierVerrou();
    messager.showSnackBar(const SnackBar(content: Text('Clé du verrou retirée de ce téléphone.')));
  }

  Future<void> _ecran(bool v) async {
    await r.reglerEcran(v);
    await masquerEcran(v);
  }

  @override
  Widget build(BuildContext context) => Groupe(titre: 'Sécurité', enfants: [
        LigneReglage(
          ico: Ico.empreinte,
          libelle: "Verrouiller l'appli",
          sousTitre: "Empreinte demandée à l'ouverture",
          fin: Interrupteur(valeur: r.verrouAppli, onChanged: (v) => _verrou(context, v), largeur: 44, hauteur: 26, libelle: "Verrouiller l'appli"),
          dense: true,
        ),
        LigneReglage(
          ico: Ico.oeil,
          libelle: "Masquer l'écran",
          sousTitre: 'Pas de capture, aperçu vide dans les applis récentes',
          fin: Interrupteur(valeur: r.ecranMasque, onChanged: _ecran, largeur: 44, hauteur: 26, libelle: "Masquer l'écran"),
          separateur: r.admin,
          dense: true,
        ),
        if (r.admin)
          LigneReglage(
            ico: Ico.puce,
            libelle: 'Clé du verrou',
            sousTitre: !r.reel || r.cleVerrouPresente
                ? "Elle signe les nouveaux appareils, gardée dans la puce du téléphone, après ton empreinte.${r.reel ? " Touche ici pour la retirer." : ""}"
                : "Pas encore sur ce téléphone : touche ici pour la ranger.",
            fin: r.reel ? const Chevron() : null,
            onTap: !r.reel ? null : r.cleVerrouPresente ? () => _retirerVerrou(context) : () => _importerVerrou(context),
            separateur: false,
            dense: true,
          ),
      ]);
}

/// Ranger la clé du verrou, depuis n'importe quel écran (les demandes).
Future<void> rangerCleVerrou(BuildContext context) => _Securite(r: EtatReseau.of(context))._importerVerrou(context);

class _Reseau extends StatelessWidget {
  const _Reseau({required this.r});
  final Reseau r;

  @override
  Widget build(BuildContext context) => Groupe(titre: 'Réseau', enfants: [
        LigneReglage(ico: Ico.serveurLigne, libelle: 'Serveur', valeur: r.serveur, dense: true),
        LigneReglage(ico: Ico.globe, libelle: 'Plage', valeur: r.plage, valeurMono: true, dense: true),
        LigneReglage(ico: Ico.cadenas, libelle: 'Chiffrement', valeur: r.protocole, dense: true),
        const LigneReglage(ico: Ico.info, libelle: 'Version', valeur: versionAppli, valeurMono: true, separateur: false, dense: true),
      ]);
}

class _Quitter extends StatelessWidget {
  const _Quitter({required this.r});
  final Reseau r;

  @override
  Widget build(BuildContext context) => Carte(
        child: LigneReglage(
          ico: Ico.quitter,
          couleurIco: Couleurs.rouge,
          couleurTexte: Couleurs.rougeClair,
          libelle: 'Quitter le réseau',
          separateur: false,
          onTap: () => _quitter(context),
        ),
      );

  Future<void> _quitter(BuildContext context) async {
    final oui = await showDialog<bool>(
      context: context,
      barrierColor: const Color(0xA8020407),
      builder: (context) => Dialog(
        backgroundColor: Colors.transparent,
        insetPadding: const EdgeInsets.symmetric(horizontal: 24),
        child: ConstrainedBox(
          constraints: const BoxConstraints(maxWidth: 400),
          child: Bordee(
            bordure: Bords.accent(Couleurs.rouge),
            fond: const Color(0xFF0A1119),
            rayon: 24,
            padding: const EdgeInsets.fromLTRB(22, 22, 22, 18),
            child: Column(mainAxisSize: MainAxisSize.min, crossAxisAlignment: CrossAxisAlignment.stretch, children: [
              Text('Quitter le réseau ?', style: texte(20, graisse: 600, espacement: -0.4)),
              const SizedBox(height: 8),
              Text(
                "Cet appareil oublie sa clé et son certificat. Pour revenir, l'admin devra le signer à nouveau.",
                style: texte(14, couleur: Couleurs.secondaire, hauteur: 1.4),
              ),
              const SizedBox(height: 20),
              Row(children: [
                Expanded(child: BoutonFantome(libelle: 'Annuler', onTap: () => Navigator.pop(context, false))),
                const SizedBox(width: 10),
                Expanded(
                  child: BoutonFantome(
                    libelle: 'Quitter',
                    couleur: Couleurs.rougeClair,
                    bord: Couleurs.rouge.withValues(alpha: 0.45),
                    onTap: () => Navigator.pop(context, true),
                  ),
                ),
              ]),
            ]),
          ),
        ),
      ),
    );
    if (oui != true || !context.mounted) return;
    final messager = ScaffoldMessenger.of(context);
    final erreur = await r.quitter();
    if (erreur != null) messager.showSnackBar(SnackBar(content: Text(erreur)));
  }
}
