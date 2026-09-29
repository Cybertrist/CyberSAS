import 'dart:async';
import 'dart:convert';

import 'package:crypto/crypto.dart' show sha256;
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_svg/flutter_svg.dart';

import '../composants.dart';
import '../etat.dart';
import '../google.dart';
import '../icones.dart';
import '../moteur.dart';
import '../theme.dart';

/// Rejoindre un réseau. L'admin envoie un lien (https://vpn.…/rejoindre#…,
/// ou cybersas://rejoindre?…) qui porte
/// l'adresse du serveur et la clé publique du verrou : une invitation, avec
/// une clé d'inscription à usage unique, ou le lien du réseau, sans clé,
/// où l'on se connecte ensuite avec son compte Google de l'équipe. Ouvert
/// sur le téléphone, il amène ici ; on peut aussi le coller.
class EcranConnexion extends StatefulWidget {
  const EcranConnexion({super.key});

  @override
  State<EcranConnexion> createState() => _EcranConnexionState();
}

/// L'empreinte d'une clé, comme le serveur et sas l'affichent : les 12
/// premiers caractères de son SHA-256 en base64, en trois blocs.
String empreinteCle(String cleBase64) {
  try {
    final e = base64.encode(sha256.convert(base64.decode(cleBase64)).bytes).substring(0, 12);
    return '${e.substring(0, 4)}-${e.substring(4, 8)}-${e.substring(8, 12)}';
  } on FormatException {
    return 'illisible';
  }
}

class _EcranConnexionState extends State<EcranConnexion> {
  Invitation? _invitation;
  bool _enCours = false;
  String? _erreur;
  StreamSubscription<Invitation>? _liens;
  final _nom = TextEditingController();

  @override
  void initState() {
    super.initState();
    Moteur.nomAppareil().then((n) {
      if (mounted && _nom.text.isEmpty) _nom.text = n;
    });
    Moteur.lienInitial().then((i) {
      if (i != null && mounted) setState(() => _invitation = i);
    });
    _liens = Moteur.liens.stream.listen((i) => setState(() {
          _invitation = i;
          _erreur = null;
        }));
  }

  @override
  void dispose() {
    _liens?.cancel();
    _nom.dispose();
    super.dispose();
  }

  Future<void> _coller() async {
    final d = await Clipboard.getData(Clipboard.kTextPlain);
    final i = Invitation.lire(d?.text ?? '');
    setState(() {
      _invitation = i;
      _erreur = i == null ? "Ce n'est pas un lien CyberSAS : copie le lien de l'admin en entier." : null;
    });
  }

  Future<void> _rejoindre() async {
    final i = _invitation;
    if (i == null || _enCours) return;
    setState(() {
      _enCours = true;
      _erreur = null;
    });
    final r = EtatReseau.of(context);
    var jeton = '';
    if (i.parGoogle) {
      final (j, e) = await jetonGoogle();
      if (!mounted) return;
      if (j == null) {
        // Fenêtre fermée : on revient sans rien dire.
        setState(() {
          _enCours = false;
          _erreur = e;
        });
        return;
      }
      jeton = j;
    }
    final erreur = await r.rejoindre(i, nom: _nom.text.trim(), jeton: jeton);
    // Le jeton a servi : Google redemandera le compte la prochaine fois.
    if (i.parGoogle) await oublierGoogle();
    if (!mounted) return;
    setState(() {
      _enCours = false;
      _erreur = erreur;
    });
  }

  @override
  Widget build(BuildContext context) {
    final i = _invitation;
    return Scaffold(
      resizeToAvoidBottomInset: true,
      body: Fond(
        centre: const Alignment(0, -0.56),
        etendue: const Size(0.7, 0.34),
        child: SafeArea(
          child: Center(
            child: ConstrainedBox(
              constraints: const BoxConstraints(maxWidth: 440),
              child: Padding(
                padding: const EdgeInsets.fromLTRB(22, 24, 22, 22),
                child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
                  Expanded(
                    child: SansDefilement(
                      alignement: Alignment.topCenter,
                      child: Column(mainAxisSize: MainAxisSize.min, crossAxisAlignment: CrossAxisAlignment.stretch, children: [
                        const _Logo(),
                        const SizedBox(height: 20),
                        // Le logo ne dépasse jamais la largeur, même en grande police.
                        const Center(child: FittedBox(fit: BoxFit.scaleDown, child: Marque(taille: 38))),
                        const SizedBox(height: 10),
                        Text('Relie tes appareils entre eux, chiffré de bout en bout.',
                            textAlign: TextAlign.center, style: texte(13.5, couleur: Couleurs.secondaire)),
                        const SizedBox(height: 30),
                        if (i == null) const _Explication() else ...[
                          _CarteInvitation(i: i),
                          const SizedBox(height: 12),
                          _ChampNom(ctrl: _nom),
                        ],
                        if (_erreur != null) ...[
                          const SizedBox(height: 14),
                          Text(_erreur!, textAlign: TextAlign.center, style: texte(13.5, couleur: Couleurs.rougeClair, hauteur: 1.4)),
                        ],
                      ]),
                    ),
                  ),
                  const SizedBox(height: 16),
                  if (i == null)
                    BoutonContour(libelle: 'Coller le lien', ico: Ico.cle, hauteur: 54, onTap: _coller)
                  else if (i.parGoogle)
                    _BoutonGoogle(libelle: _enCours ? 'Inscription…' : 'Continuer avec Google', onTap: _enCours ? null : _rejoindre)
                  else
                    BoutonContour(
                      libelle: _enCours ? 'Inscription…' : 'Rejoindre le réseau',
                      ico: Ico.appareils,
                      hauteur: 54,
                      onTap: _enCours ? null : _rejoindre,
                    ),
                  const SizedBox(height: 12),
                  if (i == null)
                    // Google ne dit pas à quel réseau aller : il faut d'abord
                    // le lien de l'admin, qui donne le serveur et le verrou.
                    _BoutonGoogle(
                      onTap: () => ScaffoldMessenger.of(context).showSnackBar(const SnackBar(
                        content: Text("Ouvre d'abord le lien du réseau que l'admin t'a envoyé, puis continue avec Google."),
                      )),
                    )
                  else
                    BoutonFantome(libelle: 'Annuler', onTap: _enCours ? null : () => setState(() => _invitation = null)),
                ]),
              ),
            ),
          ),
        ),
      ),
    );
  }
}

/// Le nom de cet appareil sur le réseau. Le serveur y ajoute celui de son
/// propriétaire : « fold8 » devient « fold8-tristan ».
class _ChampNom extends StatelessWidget {
  const _ChampNom({required this.ctrl});
  final TextEditingController ctrl;

  @override
  Widget build(BuildContext context) => Carte(
        padding: const EdgeInsets.fromLTRB(16, 10, 14, 10),
        child: Row(children: [
          const Icone(Ico.etiquette, taille: 20),
          const SizedBox(width: 14),
          Expanded(
            child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
              Text('Nom de cet appareil', style: texte(12.5, couleur: Couleurs.secondaire)),
              const SizedBox(height: 3),
              TextField(
                controller: ctrl,
                maxLength: 30,
                autocorrect: false,
                style: texte(16, graisse: 500),
                cursorColor: Couleurs.cyan,
                decoration: InputDecoration.collapsed(hintText: 'fold8', hintStyle: texte(16, couleur: Couleurs.tertiaire))
                    .copyWith(counterText: ''),
              ),
            ]),
          ),
        ]),
      );
}

/// Sans invitation : ce qu'il faut faire.
class _Explication extends StatelessWidget {
  const _Explication();

  @override
  Widget build(BuildContext context) => Carte(
        padding: const EdgeInsets.all(18),
        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
          Text("Il te faut le lien de l'admin", style: texte(16, graisse: 600)),
          const SizedBox(height: 8),
          Text(
            "C'est un lien vers le serveur, en https : le lien du réseau, avec lequel tu te connectes ensuite avec ton compte Google, "
            "ou une invitation à usage unique. Ouvre-le sur ce téléphone, ou copie-le puis colle-le ici.",
            style: texte(13.5, couleur: Couleurs.secondaire, hauteur: 1.45),
          ),
        ]),
      );
}

/// L'invitation reçue : où elle mène, et le verrou qu'elle annonce.
class _CarteInvitation extends StatelessWidget {
  const _CarteInvitation({required this.i});
  final Invitation i;

  @override
  Widget build(BuildContext context) {
    Widget ligne(Ico ico, String libelle, String valeur, {bool monoValeur = false}) => Padding(
          padding: const EdgeInsets.symmetric(vertical: 7),
          child: Row(children: [
            Icone(ico, taille: 18),
            const SizedBox(width: 12),
            Text(libelle, style: texte(14, couleur: Couleurs.secondaire)),
            const Spacer(),
            Flexible(
              child: Text(valeur,
                  textAlign: TextAlign.right,
                  overflow: TextOverflow.ellipsis,
                  style: monoValeur ? mono(13.5, graisse: 400) : texte(14, graisse: 600)),
            ),
          ]),
        );
    return Bordee(
      bordure: Bords.reflet,
      fond: const Color(0xED090F16),
      halo: haloCarte(),
      padding: const EdgeInsets.fromLTRB(18, 14, 18, 14),
      child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
        Etiquette(i.parGoogle ? 'Lien du réseau' : 'Invitation reçue', couleur: Couleurs.cyan),
        const SizedBox(height: 6),
        ligne(Ico.serveurLigne, 'Serveur', i.hote),
        if (i.parGoogle) ligne(Ico.cle, 'Entrée', 'compte Google') else ligne(Ico.cle, "Clé d'inscription", 'usage unique'),
        ligne(Ico.bouclier, 'Verrou', i.verrou.isEmpty ? 'aucun' : empreinteCle(i.verrou), monoValeur: true),
        if (i.autorite.isNotEmpty) ligne(Ico.info, 'Certificat', 'autorité du labo'),
        // Sans verrou dans le lien, l'appareil croira le premier qu'on lui
        // annonce : un serveur piraté pourrait lui donner le sien.
        if (i.verrou.isEmpty)
          const _Avertissement(
            "Ce lien ne donne pas la clé du verrou : l'appareil retiendra la première qu'on lui annonce. "
            "Demande à l'admin un lien complet, ou compare l'empreinte du verrou avec la sienne dès que tu es inscrit.",
          )
        else
          Padding(
            padding: const EdgeInsets.only(top: 4, bottom: 2),
            child: Text(
              "Compare cette empreinte du verrou avec celle que l'admin voit de son côté : si elle diffère, n'y va pas.",
              style: texte(12.5, couleur: Couleurs.secondaire, hauteur: 1.4),
            ),
          ),
        // Une autorité propre au lien : le certificat du serveur n'est pas
        // vérifié par les autorités publiques, mais par celle-ci.
        if (i.autorite.isNotEmpty)
          const _Avertissement(
            "Autorité de certification personnalisée (labo) : ce lien dit à l'appareil quel certificat croire pour ce "
            "serveur. N'accepte que si l'invitation vient bien de ton admin.",
          ),
        const SizedBox(height: 6),
        Text(
          i.parGoogle
              ? "Seul un compte Google de l'équipe peut entrer. Cet appareil crée sa clé ici, elle n'en sortira pas, "
                  "puis l'admin devra le signer."
              : "Cet appareil crée sa clé ici, elle n'en sortira pas. L'admin devra ensuite le signer.",
          style: texte(12.5, couleur: Couleurs.tertiaire, hauteur: 1.4),
        ),
      ]),
    );
  }
}

/// Un point de l'invitation qui mérite l'attention, en rouge.
class _Avertissement extends StatelessWidget {
  const _Avertissement(this.message);
  final String message;

  @override
  Widget build(BuildContext context) => Container(
        margin: const EdgeInsets.only(top: 8),
        padding: const EdgeInsets.fromLTRB(12, 10, 12, 10),
        decoration: BoxDecoration(
          color: Couleurs.rouge.withValues(alpha: 0.06),
          borderRadius: BorderRadius.circular(12),
          border: Border.all(color: Couleurs.rouge.withValues(alpha: 0.35)),
        ),
        child: Row(crossAxisAlignment: CrossAxisAlignment.start, children: [
          const Padding(
            padding: EdgeInsets.only(top: 1),
            child: Icone(Ico.info, couleur: Couleurs.rougeClair, taille: 16),
          ),
          const SizedBox(width: 10),
          Expanded(child: Text(message, style: texte(12.5, couleur: Couleurs.texte, hauteur: 1.4))),
        ]),
      );
}

class _Logo extends StatelessWidget {
  const _Logo();

  @override
  Widget build(BuildContext context) => Center(
        child: SizedBox(
          width: 132,
          height: 132,
          child: Stack(clipBehavior: Clip.none, children: [
            Positioned(
              left: -44,
              top: -44,
              right: -44,
              bottom: -44,
              child: DecoratedBox(
                decoration: BoxDecoration(
                  shape: BoxShape.circle,
                  gradient: RadialGradient(colors: [Couleurs.bleu.withValues(alpha: 0.3), Couleurs.bleu.withValues(alpha: 0)], stops: const [0, 0.65]),
                ),
              ),
            ),
            Container(
              decoration: BoxDecoration(
                borderRadius: BorderRadius.circular(30),
                boxShadow: [
                  const BoxShadow(color: Couleurs.bordure, spreadRadius: 1),
                  BoxShadow(color: Couleurs.bleu.withValues(alpha: 0.6), blurRadius: 50, spreadRadius: -12, offset: const Offset(0, 16)),
                ],
              ),
              clipBehavior: Clip.antiAlias,
              child: Image.asset('assets/icon/icon.png', width: 132, height: 132, fit: BoxFit.cover),
            ),
          ]),
        ),
      );
}

class _BoutonGoogle extends StatelessWidget {
  const _BoutonGoogle({required this.onTap, this.libelle = 'Continuer avec Google'});
  final VoidCallback? onTap;
  final String libelle;

  @override
  Widget build(BuildContext context) => DecoratedBox(
        decoration: BoxDecoration(
          borderRadius: BorderRadius.circular(18),
          boxShadow: [BoxShadow(color: Couleurs.cyan.withValues(alpha: 0.45), blurRadius: 30, spreadRadius: -12, offset: const Offset(0, 10))],
        ),
        child: Material(
          color: Couleurs.texte,
          borderRadius: BorderRadius.circular(18),
          clipBehavior: Clip.antiAlias,
          child: InkWell(
            onTap: onTap,
            child: SizedBox(
              height: 54,
              child: Row(mainAxisAlignment: MainAxisAlignment.center, children: [
                SvgPicture.string(logoGoogle, width: 20, height: 20),
                const SizedBox(width: 12),
                Flexible(
                  child: Text(libelle,
                      style: texte(16, graisse: 600, couleur: Couleurs.fond), maxLines: 1, overflow: TextOverflow.ellipsis),
                ),
              ]),
            ),
          ),
        ),
      );
}
