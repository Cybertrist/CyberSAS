import 'dart:async';

import 'package:flutter/material.dart';

import '../composants.dart';
import '../dessins.dart';
import '../donnees.dart';
import '../etat.dart';
import '../icones.dart';
import '../theme.dart';

/// L'accueil : le tunnel, l'interrupteur, et ce qui dit qu'on est protégé.
class EcranAccueil extends StatefulWidget {
  const EcranAccueil({super.key});

  @override
  State<EcranAccueil> createState() => _EcranAccueilState();
}

class _EcranAccueilState extends State<EcranAccueil> {
  // « depuis 2 h 14 » avance avec l'horloge.
  late final Timer _minute = Timer.periodic(const Duration(seconds: 20), (_) => setState(() {}));

  @override
  void dispose() {
    _minute.cancel();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final r = EtatReseau.of(context);
    final format = formatDe(context);
    final entete = _Entete(grand: format != Format.compact);

    switch (format) {
      case Format.paysage:
        return Padding(
          padding: const EdgeInsets.fromLTRB(28, 16, 28, 24),
          child: Column(children: [
            entete,
            const SizedBox(height: 8),
            Expanded(
              child: Row(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
                Expanded(child: _Scene(r: r, largeurCarte: 520)),
                const SizedBox(width: 30),
                SizedBox(
                  width: 440,
                  child: SansDefilement(
                    alignement: Alignment.center,
                    child: Column(mainAxisSize: MainAxisSize.min, children: [
                      _CarteAppareil(r: r, grand: true),
                      const SizedBox(height: 16),
                      _Infos(r: r, complet: true),
                    ]),
                  ),
                ),
              ]),
            ),
          ]),
        );
      case Format.portrait:
        return Padding(
          padding: const EdgeInsets.fromLTRB(28, 16, 28, 12),
          child: Column(children: [
            entete,
            const SizedBox(height: 8),
            Expanded(child: _Scene(r: r, largeurCarte: 560)),
            const SizedBox(height: 16),
            IntrinsicHeight(
              child: Row(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
                Expanded(child: _CarteAppareil(r: r, grand: true)),
                const SizedBox(width: 16),
                Expanded(child: _Infos(r: r)),
              ]),
            ),
          ]),
        );
      case Format.compact:
        return Padding(
          padding: const EdgeInsets.only(top: 6),
          child: Column(children: [
            entete,
            Expanded(child: _Scene(r: r)),
            const SizedBox(height: 12),
            Padding(padding: const EdgeInsets.symmetric(horizontal: 16), child: _CarteAppareil(r: r)),
            const SizedBox(height: 8),
          ]),
        );
    }
  }
}

class _Entete extends StatelessWidget {
  const _Entete({required this.grand});
  final bool grand;

  @override
  Widget build(BuildContext context) => Padding(
        padding: EdgeInsets.symmetric(horizontal: grand ? 0 : 22),
        child: SizedBox(
          height: grand ? 38 : 36,
          // Le logo ne grandit pas avec la police : il tient dans l'en-tête.
          child: Align(
            alignment: Alignment.centerLeft,
            child: FittedBox(fit: BoxFit.scaleDown, child: Marque(taille: grand ? 30 : 24)),
          ),
        ),
      );
}

/// Le tunnel, ses coins de visée, et la carte d'état qui le chevauche.
class _Scene extends StatelessWidget {
  const _Scene({required this.r, this.largeurCarte});
  final Reseau r;
  final double? largeurCarte;

  @override
  Widget build(BuildContext context) {
    final compact = largeurCarte == null;
    return LayoutBuilder(builder: (context, c) {
      // Le tunnel garde ses proportions ; au-delà de 520 dp de haut, il
      // cesse de grandir pour laisser respirer.
      return Stack(children: [
        Positioned(
          left: compact ? 8 : 0,
          right: compact ? 8 : 0,
          top: 4,
          bottom: compact ? 0 : 24,
          child: Stack(fit: StackFit.expand, children: [
            // Ouvert mais sans serveur après le délai : le tunnel s'éteint,
            // rien ne passe.
            Tunnel(allume: r.connecte && !r.tunnelEnPanne),
            Positioned(top: 12, left: 14, child: _coin(true)),
            Positioned(top: 12, right: 14, child: _coin(false)),
            Positioned(
              top: 30,
              left: 18,
              child: Text(r.plage, style: mono(11, graisse: 400, couleur: Couleurs.etiquette, espacement: 1.1)),
            ),
          ]),
        ),
        Positioned(
          left: compact ? 16 : 0,
          right: compact ? 16 : 0,
          bottom: 0,
          child: Center(
            child: ConstrainedBox(
              constraints: BoxConstraints(maxWidth: largeurCarte ?? double.infinity),
              child: _CarteEtat(r: r, grand: !compact),
            ),
          ),
        ),
      ]);
    });
  }

  Widget _coin(bool gauche) => Container(
        width: 12,
        height: 12,
        decoration: BoxDecoration(
          border: Border(
            top: BorderSide(color: Couleurs.cyan.withValues(alpha: 0.5)),
            left: gauche ? BorderSide(color: Couleurs.cyan.withValues(alpha: 0.5)) : BorderSide.none,
            right: gauche ? BorderSide.none : BorderSide(color: Couleurs.cyan.withValues(alpha: 0.5)),
          ),
        ),
      );
}

/// « TUNNEL SAS · Connecté depuis 2 h 14 », et l'interrupteur principal.
class _CarteEtat extends StatelessWidget {
  const _CarteEtat({required this.r, required this.grand});
  final Reseau r;
  final bool grand;

  @override
  Widget build(BuildContext context) {
    final on = r.connecte;
    final attente = r.enTransition;
    // Pas encore signé par l'admin : les autres appareils le refuseraient,
    // l'interrupteur reste fermé à clé.
    final bloque = r.nonSigne && !on;
    final erreur = r.erreurAffichee;
    final (etat, sous) = switch ((on, attente)) {
      // Sans réponse du serveur, on ne sait pas s'il a été signé entre-temps.
      (false, false) when r.serveurInjoignable && erreur.isEmpty => ('Hors ligne', 'serveur injoignable'),
      (false, false) when bloque => (
          'Verrouillé',
          switch (r.moi.etatCertificat) {
            EtatCertificat.revoque => 'appareil révoqué',
            EtatCertificat.expire => 'certificat expiré',
            _ => 'en attente de signature',
          }
        ),
      (true, true) => ('Connexion…', 'ouverture du tunnel'),
      (false, true) => ('Coupure…', 'fermeture du tunnel'),
      // Tunnel ouvert, mais pas (encore) de session avec le serveur.
      (true, false) when !r.serveurJoint =>
        ('Connexion…', r.tunnelEnPanne || erreur.isNotEmpty ? 'serveur injoignable' : 'recherche du serveur'),
      (true, false) => ('Connecté', 'depuis ${duree(DateTime.now().difference(r.debutConnexion))}'),
      (false, false) => ('Déconnecté', erreur.isNotEmpty ? erreur : 'tunnel coupé'),
    };
    return AnimatedSwitcher(
      duration: const Duration(milliseconds: 300),
      child: Bordee(
        key: ValueKey(on),
        bordure: LinearGradient(
          colors: on
              ? [Couleurs.cyan.withValues(alpha: 0.6), Couleurs.bordure, Couleurs.bleu.withValues(alpha: 0.45)]
              : const [Couleurs.bordure, Couleurs.bordure, Couleurs.bordure],
          stops: const [0, 0.55, 1],
        ),
        rayon: grand ? 22 : 20,
        halo: on ? haloCarte() : null,
        padding: EdgeInsets.fromLTRB(grand ? 20 : 18, grand ? 11 : 9, grand ? 12 : 10, grand ? 11 : 9),
        child: Row(children: [
          Expanded(
            child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
              const Etiquette('Tunnel SAS'),
              const SizedBox(height: 4),
              Row(crossAxisAlignment: CrossAxisAlignment.baseline, textBaseline: TextBaseline.alphabetic, children: [
                if (on && !attente && r.serveurJoint)
                  ShaderMask(
                    shaderCallback: (b) => Couleurs.degrade.createShader(b),
                    child: Text(etat, style: texte(21, graisse: 600, espacement: -0.42, couleur: Colors.white)),
                  )
                else
                  Text(etat, style: texte(21, graisse: 600, espacement: -0.42, couleur: Couleurs.secondaire)),
                const SizedBox(width: 9),
                Flexible(
                  child: Text(
                    sous,
                    style: texte(12.5, couleur: on ? Couleurs.secondaire : Couleurs.tertiaire),
                    maxLines: 2,
                    overflow: TextOverflow.ellipsis,
                  ),
                ),
              ]),
            ]),
          ),
          if (bloque) ...[
            const Icone(Ico.cadenas, couleur: Couleurs.tertiaire, taille: 18),
            const SizedBox(width: 2),
          ],
          Interrupteur(valeur: on, onChanged: attente || bloque ? null : r.basculer, largeur: grand ? 56 : 54, hauteur: grand ? 34 : 32),
        ]),
      ),
    );
  }
}

/// L'adresse privée de l'appareil et ce qui le protège.
class _CarteAppareil extends StatelessWidget {
  const _CarteAppareil({required this.r, this.grand = false});
  final Reseau r;
  final bool grand;

  @override
  Widget build(BuildContext context) {
    final moi = r.moi;
    final grandePolice = MediaQuery.textScalerOf(context).scale(10) > 10;
    Widget garantie(Ico ico, Color c, String titre, String sous) => Row(children: [
          Icone(ico, couleur: c, taille: 19, lueur: true),
          const SizedBox(width: 10),
          Flexible(
            child: Column(crossAxisAlignment: CrossAxisAlignment.start, mainAxisSize: MainAxisSize.min, children: [
              Text(titre, style: texte(13, graisse: 500, hauteur: 1.25)),
              const SizedBox(height: 2),
              Text(sous, style: texte(11.5, couleur: Couleurs.secondaire), maxLines: 1, overflow: TextOverflow.fade, softWrap: false),
            ]),
          ),
        ]);

    return Container(
      decoration: BoxDecoration(
        color: const Color(0xD10A1017),
        borderRadius: BorderRadius.circular(grand ? 22 : 20),
        border: Border.all(color: Couleurs.bordure),
      ),
      child: Column(mainAxisSize: MainAxisSize.min, children: [
        Container(
          padding: EdgeInsets.fromLTRB(18, grand ? 16 : 13, 18, grand ? 14 : 11),
          decoration: const BoxDecoration(border: Border(bottom: BorderSide(color: Couleurs.separateur))),
          child: Row(crossAxisAlignment: CrossAxisAlignment.end, children: [
            // Avec une grande police, l'adresse se réduit pour laisser au
            // nom deux cinquièmes de la ligne.
            Flexible(
              flex: grandePolice ? 3 : 0,
              child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                const Etiquette('Adresse privée'),
                const SizedBox(height: 5),
                FittedBox(
                  fit: BoxFit.scaleDown,
                  alignment: Alignment.centerLeft,
                  child: Text(moi.adresse, style: mono(grand ? 26 : 20)),
                ),
              ]),
            ),
            const SizedBox(width: 16),
            // Un nom long se coupe au lieu de sortir de la carte.
            Expanded(
              flex: 2,
              child: Column(crossAxisAlignment: CrossAxisAlignment.end, children: [
                const Etiquette('Cet appareil'),
                const SizedBox(height: 5),
                Text(moi.nomAffiche,
                    style: texte(grand ? 15 : 14, graisse: 500), maxLines: 1, overflow: TextOverflow.ellipsis, textAlign: TextAlign.right),
              ]),
            ),
          ]),
        ),
        Padding(
          padding: EdgeInsets.fromLTRB(16, grand ? 14 : 11, 14, grand ? 15 : 12),
          child: Row(children: [
            Expanded(child: garantie(Ico.cadenas, Couleurs.cyan, 'Chiffré de bout en bout', 'Protocole ${r.protocole}')),
            const SizedBox(width: 10),
            // Pas encore signé : l'admin compare cette empreinte avant de signer.
            Expanded(
              child: switch (moi.etatCertificat) {
                // Signé : on prévient deux semaines avant l'expiration.
                EtatCertificat.signe when moi.certificat.finConnue && moi.certificat.joursRestants(DateTime.now()) <= 14 =>
                  garantie(Ico.bouclier, Couleurs.rouge, 'Expire dans ${moi.certificat.joursRestants(DateTime.now())} j', 'à renouveler'),
                EtatCertificat.signe => garantie(Ico.bouclier, Couleurs.cyan, 'Verrou vérifié', "Signé par l'admin"),
                EtatCertificat.revoque => garantie(Ico.bouclier, Couleurs.rouge, 'Appareil révoqué', "par l'admin"),
                EtatCertificat.expire => garantie(Ico.bouclier, Couleurs.rouge, 'Certificat expiré', 'à renouveler'),
                _ when r.serveurInjoignable => garantie(Ico.bouclier, Couleurs.tertiaire, 'Serveur injoignable', 'signature à vérifier'),
                _ => garantie(Ico.empreinte, Couleurs.rouge, 'En attente de signature', moi.certificat.empreinte.join('-')),
              },
            ),
          ]),
        ),
      ]),
    );
  }
}

/// Les réglages du réseau, en lecture seule.
class _Infos extends StatelessWidget {
  const _Infos({required this.r, this.complet = false});
  final Reseau r;
  final bool complet;

  @override
  Widget build(BuildContext context) {
    final lignes = [
      if (complet)
        (Ico.activite, Couleurs.cyan, 'Connecté depuis', r.enService ? duree(DateTime.now().difference(r.debutConnexion)) : 'pas connecté'),
      if (complet) (Ico.appareils, Couleurs.cyan, 'Appareils joignables', r.enService ? '${r.enLigne} sur ${r.appareils.length}' : '0 sur ${r.appareils.length}'),
      (Ico.globe, Couleurs.cyan, 'Réseau', r.plage),
      (Ico.cadenas, Couleurs.cyan, 'Protocole', r.protocole),
      (Ico.serveurLigne, Couleurs.cyan, 'Serveur', r.serveur),
    ];
    return Container(
      decoration: BoxDecoration(
        color: const Color(0xD10A1017),
        borderRadius: BorderRadius.circular(22),
        border: Border.all(color: Couleurs.bordure),
      ),
      child: Column(mainAxisSize: MainAxisSize.min, mainAxisAlignment: MainAxisAlignment.spaceEvenly, children: [
        for (var i = 0; i < lignes.length; i++)
          Container(
            padding: const EdgeInsets.symmetric(horizontal: 18, vertical: 14),
            decoration: BoxDecoration(
              border: i < lignes.length - 1 ? const Border(bottom: BorderSide(color: Couleurs.separateur)) : null,
            ),
            child: Row(children: [
              Icone(lignes[i].$1, couleur: lignes[i].$2, taille: 18, lueur: true),
              const SizedBox(width: 12),
              Expanded(child: Text(lignes[i].$3, style: texte(14.5, couleur: Couleurs.secondaire))),
              Text(lignes[i].$4, style: mono(14.5)),
            ]),
          ),
      ]),
    );
  }
}
