import 'dart:async';

import 'package:flutter/material.dart';
import 'package:qr_flutter/qr_flutter.dart';
import 'package:share_plus/share_plus.dart';

import '../composants.dart';
import '../donnees.dart';
import '../etat.dart';
import '../moteur.dart';
import '../icones.dart';
import '../theme.dart';

/// Inviter un nouvel appareil : un code à usage unique, valable 10 minutes,
/// en QR et en clair.
class EcranAjout extends StatefulWidget {
  const EcranAjout({super.key});

  @override
  State<EcranAjout> createState() => _EcranAjoutState();
}

class _EcranAjoutState extends State<EcranAjout> {
  late CodeInvitation _invitation = CodeInvitation(EtatReseau.of(context).serveur);
  late final Timer _horloge;

  @override
  void initState() {
    super.initState();
    _horloge = Timer.periodic(const Duration(seconds: 1), (_) => setState(() {}));
  }

  @override
  void dispose() {
    _horloge.cancel();
    super.dispose();
  }

  Duration get _reste {
    final d = _invitation.expire.difference(DateTime.now());
    return d.isNegative ? Duration.zero : d;
  }

  void _renouveler() => setState(() => _invitation = CodeInvitation(_invitation.serveur));

  Future<void> _partager() => SharePlus.instance.share(ShareParams(
        subject: 'Invitation CyberSAS',
        text: 'Rejoins mon réseau CyberSAS.\n'
            'Serveur : ${_invitation.serveur}\n'
            'Code : ${_invitation.code} (usage unique, 10 minutes)\n'
            '${_invitation.charge}',
      ));

  @override
  Widget build(BuildContext context) {
    if (EtatReseau.of(context).reel) return const _AjoutReel();
    final reste = _reste;
    final expiree = reste == Duration.zero;
    final mm = reste.inMinutes.toString().padLeft(2, '0');
    final ss = (reste.inSeconds % 60).toString().padLeft(2, '0');

    return Scaffold(
      body: Fond(
        centre: const Alignment(0, -0.32),
        etendue: const Size(0.7, 0.3),
        child: SafeArea(
          child: Center(
            child: ConstrainedBox(
              constraints: const BoxConstraints(maxWidth: 480),
              child: Padding(
                padding: const EdgeInsets.fromLTRB(16, 6, 16, 20),
                child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
                  const Align(alignment: Alignment.centerLeft, child: BoutonRetour('Appareils')),
                  const SizedBox(height: 14),
                  Expanded(
                    child: SansDefilement(
                      child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, mainAxisSize: MainAxisSize.min, children: [
                        Padding(
                          padding: const EdgeInsets.symmetric(horizontal: 6),
                          child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                            Text('Ajouter un appareil', style: titre(28)),
                            const SizedBox(height: 8),
                            Text('Un code à usage unique, valable 10 minutes.', style: texte(13.5, couleur: Couleurs.secondaire)),
                          ]),
                        ),
                        const SizedBox(height: 14),
                        Bordee(
                          bordure: const LinearGradient(
                            begin: Alignment.topLeft,
                            end: Alignment.bottomRight,
                            colors: [Color(0x9931E7FD), Couleurs.bordure, Color(0x7301B9FD)],
                            stops: [0, 0.45, 1],
                          ),
                          fond: const Color(0xEB090F16),
                          rayon: 24,
                          halo: [BoxShadow(color: Couleurs.cyan.withValues(alpha: 0.6), blurRadius: 34, spreadRadius: -14)],
                          padding: const EdgeInsets.fromLTRB(18, 20, 18, 16),
                          child: Column(children: [
                            AnimatedOpacity(
                              duration: const Duration(milliseconds: 300),
                              opacity: expiree ? 0.25 : 1,
                              child: _Qr(charge: _invitation.charge),
                            ),
                            const SizedBox(height: 14),
                            Text(_invitation.code,
                                style: mono(19, couleur: expiree ? Couleurs.tertiaire : Couleurs.texte, espacement: 1.5)),
                            const SizedBox(height: 5),
                            if (expiree)
                              TextButton(
                                onPressed: _renouveler,
                                style: TextButton.styleFrom(foregroundColor: Couleurs.cyan),
                                child: Text('Code expiré · en créer un autre', style: texte(13, graisse: 600, couleur: Couleurs.cyan)),
                              )
                            else
                              Row(mainAxisAlignment: MainAxisAlignment.center, children: [
                                const Icone(Ico.horloge, couleur: Couleurs.secondaire, taille: 13, trait: 2),
                                const SizedBox(width: 6),
                                Text('Expire dans $mm:$ss', style: texte(12.5, couleur: Couleurs.secondaire)),
                              ]),
                          ]),
                        ),
                        const SizedBox(height: 14),
                        const _Etapes(),
                        const _LigneInvitations(),
                      ]),
                    ),
                  ),
                  const SizedBox(height: 14),
                  BoutonContour(libelle: "Partager l'invitation", ico: Ico.partage, hauteur: 52, onTap: expiree ? _renouveler : _partager),
                ]),
              ),
            ),
          ),
        ),
      ),
    );
  }
}

/// Le QR, 176 dp, avec ses coins de visée et la maison au centre.
class _Qr extends StatelessWidget {
  const _Qr({required this.charge});
  final String charge;

  @override
  Widget build(BuildContext context) => CustomPaint(
        foregroundPainter: Coins(),
        child: Container(
          width: 176,
          height: 176,
          padding: const EdgeInsets.all(12),
          decoration: BoxDecoration(
            color: Couleurs.bloc,
            borderRadius: BorderRadius.circular(18),
            border: Border.all(color: Couleurs.separateur),
          ),
          child: Stack(alignment: Alignment.center, children: [
            QrImageView(
              data: charge,
              padding: EdgeInsets.zero,
              errorCorrectionLevel: QrErrorCorrectLevel.H,
              eyeStyle: const QrEyeStyle(eyeShape: QrEyeShape.square, color: Color(0xFFE6FDFF)),
              dataModuleStyle: const QrDataModuleStyle(dataModuleShape: QrDataModuleShape.square, color: Color(0xFFCFF8FF)),
            ),
            Container(
              width: 30,
              height: 30,
              alignment: Alignment.center,
              decoration: BoxDecoration(
                color: const Color(0xFF0A1219),
                borderRadius: BorderRadius.circular(8),
                border: Border.all(color: Couleurs.cyan, width: 1.2),
              ),
              child: const Icone(Ico.maison, taille: 17, trait: 2),
            ),
          ]),
        ),
      );
}

class _Etapes extends StatelessWidget {
  const _Etapes();

  @override
  Widget build(BuildContext context) {
    const etapes = [
      ('Installer CyberSAS', 'Sur le nouvel appareil'),
      ("Ouvrir le lien d'invitation", 'Le nouvel appareil crée sa propre clé'),
      ("Signature sur le téléphone de l'admin", "Après vérification de l'empreinte"),
    ];
    return Carte(
      fond: const Color(0xC70C1117),
      child: Column(children: [
        for (var i = 0; i < etapes.length; i++)
          Container(
            padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 10),
            decoration: BoxDecoration(
              border: i < etapes.length - 1 ? const Border(bottom: BorderSide(color: Couleurs.separateur)) : null,
            ),
            child: Row(children: [
              _Pastille(n: i + 1, etat: i == 0 ? 2 : (i == 1 ? 1 : 0)),
              const SizedBox(width: 12),
              Expanded(
                child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                  Text(etapes[i].$1, style: texte(14, graisse: 500, couleur: i == 2 ? Couleurs.secondaire : Couleurs.texte)),
                  const SizedBox(height: 2),
                  Text(etapes[i].$2, style: texte(12, couleur: Couleurs.tertiaire)),
                ]),
              ),
            ]),
          ),
      ]),
    );
  }
}

/// 2 : fait ; 1 : en cours ; 0 : à venir.
class _Pastille extends StatelessWidget {
  const _Pastille({required this.n, required this.etat});
  final int n;
  final int etat;

  @override
  Widget build(BuildContext context) {
    if (etat == 2) {
      return Container(
        width: 26,
        height: 26,
        alignment: Alignment.center,
        decoration: BoxDecoration(
          shape: BoxShape.circle,
          gradient: const LinearGradient(begin: Alignment.topLeft, end: Alignment.bottomRight, colors: [Couleurs.cyan, Couleurs.bleu]),
          boxShadow: [BoxShadow(color: Couleurs.cyan.withValues(alpha: 0.7), blurRadius: 12, spreadRadius: -2)],
        ),
        child: const Icone(Ico.coche, couleur: Couleurs.fond, taille: 14, trait: 2.4),
      );
    }
    final actif = etat == 1;
    return Container(
      width: 26,
      height: 26,
      alignment: Alignment.center,
      decoration: BoxDecoration(
        shape: BoxShape.circle,
        border: Border.all(color: actif ? Couleurs.cyan.withValues(alpha: 0.6) : Couleurs.bordure),
        boxShadow: actif ? [BoxShadow(color: Couleurs.cyan.withValues(alpha: 0.7), blurRadius: 12, spreadRadius: -3)] : null,
      ),
      child: Text('$n', style: texte(12, graisse: 600, couleur: actif ? Couleurs.cyan : Couleurs.tertiaire)),
    );
  }
}

/// La vraie invitation : l'admin dit pour qui et pour combien de temps, le
/// serveur crée la clé, et le téléphone en fait le lien cybersas://.
class _AjoutReel extends StatefulWidget {
  const _AjoutReel();

  @override
  State<_AjoutReel> createState() => _AjoutReelState();
}

class _AjoutReelState extends State<_AjoutReel> {
  late final _qui = TextEditingController(text: EtatReseau.of(context).courriel);
  static const _durees = [(10, '10 min'), (60, '1 h'), (24 * 60, '24 h')];
  int _minutes = 10;
  String _lien = '';
  DateTime _expire = DateTime.now();
  String? _erreur;
  bool _enCours = false;
  Timer? _horloge;

  @override
  void initState() {
    super.initState();
    // Les invitations déjà envoyées, pour pouvoir en annuler une.
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted) EtatReseau.of(context).chargerInvitations();
    });
  }

  @override
  void dispose() {
    _qui.dispose();
    _horloge?.cancel();
    super.dispose();
  }

  Future<void> _creer() async {
    if (_enCours) return;
    FocusScope.of(context).unfocus();
    setState(() {
      _enCours = true;
      _erreur = null;
    });
    final r = EtatReseau.of(context);
    final (lien, e) = await r.inviter(_qui.text.trim(), _minutes);
    if (!mounted) return;
    if (e == null) r.chargerInvitations();
    setState(() {
      _enCours = false;
      _erreur = e;
      _lien = lien;
      _expire = DateTime.now().add(Duration(minutes: _minutes));
    });
    _horloge?.cancel();
    if (e == null) _horloge = Timer.periodic(const Duration(seconds: 1), (_) => setState(() {}));
  }

  String get _duree => _durees.firstWhere((d) => d.$1 == _minutes).$2;

  /// Le lien permanent du réseau : pas de clé, la personne entre avec son
  /// compte Google de l'équipe, puis l'admin la signe.
  Future<void> _partagerReseau() async {
    final messager = ScaffoldMessenger.of(context);
    final String lien;
    try {
      lien = await Moteur.lienReseau();
    } on ErreurMoteur catch (e) {
      messager.showSnackBar(SnackBar(content: Text(e.message)));
      return;
    }
    await SharePlus.instance.share(ShareParams(
      subject: 'Réseau CyberSAS',
      text: 'Rejoins mon réseau CyberSAS : ouvre ce lien sur ton appareil, où CyberSAS est installé, '
          "puis connecte-toi avec ton compte Google (il doit être dans l'équipe).\n$lien",
    ));
  }

  Future<void> _partager() => SharePlus.instance.share(ShareParams(
        subject: 'Invitation CyberSAS',
        text: 'Rejoins mon réseau CyberSAS : ouvre ce lien sur ton appareil, où CyberSAS est installé '
            '(usage unique, valable $_duree).\n$_lien',
      ));

  @override
  Widget build(BuildContext context) {
    final r = EtatReseau.of(context);
    final reste = _expire.difference(DateTime.now());
    final expiree = _lien.isNotEmpty && reste.isNegative;
    final h = reste.inHours;
    final mm = (reste.inMinutes % 60).toString().padLeft(2, '0');
    final ss = (reste.inSeconds % 60).toString().padLeft(2, '0');

    final Widget corps;
    if (!r.admin) {
      corps = Carte(
        padding: const EdgeInsets.all(18),
        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
          Text("Seul un admin peut inviter", style: texte(16, graisse: 600)),
          const SizedBox(height: 8),
          Text(
            "Demande une invitation à l'admin du réseau : un lien cybersas:// à ouvrir sur le nouvel appareil.",
            style: texte(13.5, couleur: Couleurs.secondaire, hauteur: 1.45),
          ),
        ]),
      );
    } else if (_lien.isEmpty) {
      corps = Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
        Carte(
          padding: const EdgeInsets.fromLTRB(16, 12, 14, 12),
          child: Row(children: [
            const Icone(Ico.etiquette, taille: 20),
            const SizedBox(width: 14),
            Expanded(
              child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                Text("Pour qui (son adresse Google)", style: texte(12.5, couleur: Couleurs.secondaire)),
                const SizedBox(height: 3),
                TextField(
                  controller: _qui,
                  keyboardType: TextInputType.emailAddress,
                  autocorrect: false,
                  style: texte(16, graisse: 500),
                  cursorColor: Couleurs.cyan,
                  decoration: InputDecoration.collapsed(hintText: 'prenom@gmail.com', hintStyle: texte(16, couleur: Couleurs.tertiaire)),
                ),
              ]),
            ),
          ]),
        ),
        const SizedBox(height: 10),
        Padding(
          padding: const EdgeInsets.symmetric(horizontal: 6),
          child: Text("Ton adresse pour un appareil à toi. La personne doit déjà faire partie de l'équipe.",
              style: texte(12.5, couleur: Couleurs.tertiaire, hauteur: 1.4)),
        ),
        const SizedBox(height: 16),
        Padding(
          padding: const EdgeInsets.symmetric(horizontal: 6),
          child: Text('Valable', style: texte(12.5, couleur: Couleurs.secondaire)),
        ),
        const SizedBox(height: 8),
        Row(children: [
          for (final d in _durees) ...[
            Expanded(
              child: Carte(
                rayon: 14,
                fond: d.$1 == _minutes ? Couleurs.cyan.withValues(alpha: 0.12) : Couleurs.carte,
                bord: d.$1 == _minutes ? Couleurs.cyan.withValues(alpha: 0.5) : Couleurs.bordure,
                onTap: () => setState(() => _minutes = d.$1),
                child: SizedBox(
                  height: 42,
                  child: Center(
                    child: Text(d.$2, style: texte(14.5, graisse: 600, couleur: d.$1 == _minutes ? Couleurs.cyan : Couleurs.secondaire)),
                  ),
                ),
              ),
            ),
            if (d != _durees.last) const SizedBox(width: 10),
          ],
        ]),
        if (_erreur != null) ...[
          const SizedBox(height: 14),
          Text(_erreur!, style: texte(13.5, couleur: Couleurs.rougeClair, hauteur: 1.4)),
        ],
      ]);
    } else {
      // Avec l'autorité du labo, le lien est trop long pour un QR lisible :
      // on le partage.
      final qr = _lien.length <= 700;
      corps = Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
        Bordee(
          bordure: const LinearGradient(
            begin: Alignment.topLeft,
            end: Alignment.bottomRight,
            colors: [Color(0x9931E7FD), Couleurs.bordure, Color(0x7301B9FD)],
            stops: [0, 0.45, 1],
          ),
          fond: const Color(0xEB090F16),
          rayon: 24,
          halo: [BoxShadow(color: Couleurs.cyan.withValues(alpha: 0.6), blurRadius: 34, spreadRadius: -14)],
          padding: const EdgeInsets.fromLTRB(18, 20, 18, 16),
          child: Column(children: [
            if (qr)
              AnimatedOpacity(
                duration: const Duration(milliseconds: 300),
                opacity: expiree ? 0.25 : 1,
                child: _Qr(charge: _lien),
              )
            else
              Text('Invitation prête pour ${_qui.text.trim()}',
                  textAlign: TextAlign.center, style: texte(15.5, graisse: 600, couleur: expiree ? Couleurs.tertiaire : Couleurs.texte)),
            const SizedBox(height: 12),
            if (expiree)
              TextButton(
                onPressed: () => setState(() => _lien = ''),
                style: TextButton.styleFrom(foregroundColor: Couleurs.cyan),
                child: Text('Invitation expirée · en créer une autre', style: texte(13, graisse: 600, couleur: Couleurs.cyan)),
              )
            else
              Row(mainAxisAlignment: MainAxisAlignment.center, children: [
                const Icone(Ico.horloge, couleur: Couleurs.secondaire, taille: 13, trait: 2),
                const SizedBox(width: 6),
                Text('Expire dans ${h > 0 ? '$h h ' : ''}$mm:$ss · une seule fois', style: texte(12.5, couleur: Couleurs.secondaire)),
              ]),
          ]),
        ),
        const SizedBox(height: 14),
        const _Etapes(),
      ]);
    }

    final bouton = !r.admin
        ? null
        : _lien.isEmpty
            ? BoutonContour(libelle: _enCours ? 'Création…' : "Créer l'invitation", ico: Ico.plus, hauteur: 52, onTap: _creer)
            : BoutonContour(libelle: "Partager l'invitation", ico: Ico.partage, hauteur: 52, onTap: expiree ? null : _partager);

    return Scaffold(
      body: Fond(
        centre: const Alignment(0, -0.32),
        etendue: const Size(0.7, 0.3),
        child: SafeArea(
          child: Center(
            child: ConstrainedBox(
              constraints: const BoxConstraints(maxWidth: 480),
              child: Padding(
                padding: const EdgeInsets.fromLTRB(16, 6, 16, 20),
                child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
                  const Align(alignment: Alignment.centerLeft, child: BoutonRetour('Appareils')),
                  const SizedBox(height: 14),
                  Expanded(
                    child: SansDefilement(
                      child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, mainAxisSize: MainAxisSize.min, children: [
                        Padding(
                          padding: const EdgeInsets.symmetric(horizontal: 6),
                          child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                            Text('Ajouter un appareil', style: titre(28)),
                            const SizedBox(height: 8),
                            Text('Une invitation à usage unique.', style: texte(13.5, couleur: Couleurs.secondaire)),
                          ]),
                        ),
                        const SizedBox(height: 14),
                        corps,
                        const _LigneInvitations(),
                      ]),
                    ),
                  ),
                  if (bouton != null) ...[const SizedBox(height: 14), bouton],
                  // Pour les comptes Google de l'équipe : un seul lien, qui sert
                  // à tous leurs appareils.
                  if (r.reel && r.admin) ...[
                    const SizedBox(height: 10),
                    BoutonFantome(libelle: 'Partager le lien du réseau (Google)', onTap: _partagerReseau),
                  ],
                ]),
              ),
            ),
          ),
        ),
      ),
    );
  }
}

/// « 2 invitations en cours › », seulement pour un admin et s'il y en a :
/// ouvre la liste, où chacune s'annule.
class _LigneInvitations extends StatelessWidget {
  const _LigneInvitations();

  @override
  Widget build(BuildContext context) {
    final r = EtatReseau.of(context);
    final n = r.invitations.length;
    if (!r.admin || n == 0) return const SizedBox.shrink();
    return Padding(
      padding: const EdgeInsets.only(top: 14),
      child: Carte(
        child: LigneReglage(
          ico: Ico.horloge,
          libelle: n == 1 ? '1 invitation en cours' : '$n invitations en cours',
          separateur: false,
          fin: const Chevron(),
          onTap: () => montrerInvitations(context),
        ),
      ),
    );
  }
}

/// Les invitations pas encore utilisées : pour qui, expire dans combien de
/// temps, et Annuler. Une fenêtre centrée qui suit la liste.
Future<void> montrerInvitations(BuildContext context) async {
  final r = EtatReseau.of(context);
  final messager = ScaffoldMessenger.of(context);
  r.chargerInvitations();
  await showDialog<void>(
    context: context,
    barrierColor: const Color(0xA8020407),
    builder: (context) => Dialog(
      backgroundColor: Colors.transparent,
      insetPadding: const EdgeInsets.symmetric(horizontal: 24, vertical: 40),
      child: ConstrainedBox(
        constraints: const BoxConstraints(maxWidth: 400),
        child: Bordee(
          bordure: Bords.reflet,
          fond: const Color(0xFF0A1119),
          rayon: 24,
          padding: const EdgeInsets.fromLTRB(22, 22, 22, 18),
          child: ListenableBuilder(
            listenable: r,
            builder: (context, _) => Column(mainAxisSize: MainAxisSize.min, crossAxisAlignment: CrossAxisAlignment.stretch, children: [
              Text('Invitations en cours', style: texte(20, graisse: 600, espacement: -0.4)),
              const SizedBox(height: 6),
              Text(
                r.invitations.isEmpty
                    ? 'Aucune : toutes ont servi ou expiré.'
                    : 'Pas encore utilisées. Une invitation annulée ne fait plus entrer personne.',
                style: texte(13.5, couleur: Couleurs.secondaire, hauteur: 1.4),
              ),
              if (r.invitations.isNotEmpty) ...[
                const SizedBox(height: 12),
                Flexible(
                  child: SingleChildScrollView(
                    child: Column(children: [
                      for (final i in r.invitations)
                        Container(
                          padding: const EdgeInsets.symmetric(vertical: 8),
                          decoration: BoxDecoration(
                            border: i == r.invitations.last ? null : const Border(bottom: BorderSide(color: Couleurs.separateur)),
                          ),
                          child: Row(children: [
                            Icone(i.machine ? Ico.serveur : Ico.etiquette, taille: 18),
                            const SizedBox(width: 12),
                            Expanded(
                              child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                                Text(i.machine ? 'Machine « ${i.pour} »' : i.pour,
                                    maxLines: 1, overflow: TextOverflow.ellipsis, style: texte(14.5, graisse: 600)),
                                const SizedBox(height: 2),
                                Text('Expire dans ${i.reste}', style: texte(12.5, couleur: Couleurs.secondaire)),
                              ]),
                            ),
                            const SizedBox(width: 4),
                            TextButton(
                              onPressed: () => _annulerInvitation(context, i, messager),
                              style: TextButton.styleFrom(
                                foregroundColor: Couleurs.rougeClair,
                                padding: const EdgeInsets.symmetric(horizontal: 8),
                                minimumSize: const Size(0, 40),
                              ),
                              child: Text('Annuler', style: texte(13.5, graisse: 600, couleur: Couleurs.rougeClair)),
                            ),
                          ]),
                        ),
                    ]),
                  ),
                ),
              ],
              const SizedBox(height: 16),
              BoutonFantome(libelle: 'Fermer', onTap: () => Navigator.pop(context)),
            ]),
          ),
        ),
      ),
    ),
  );
}

/// Confirme, puis annule [i] sur le serveur.
Future<void> _annulerInvitation(BuildContext context, InvitationEnCours i, ScaffoldMessengerState messager) async {
  final r = EtatReseau.of(context);
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
            Text("Annuler l'invitation ?", style: texte(20, graisse: 600, espacement: -0.4)),
            const SizedBox(height: 12),
            Text(
              i.machine
                  ? "La clé d'inscription de la machine « ${i.pour} » ne servira plus. Il en faudra une nouvelle pour l'ajouter."
                  : "Le lien envoyé à ${i.pour} ne fera plus entrer personne. Pour l'inviter à nouveau, il faudra un nouveau lien.",
              style: texte(14, couleur: Couleurs.secondaire, hauteur: 1.4),
            ),
            const SizedBox(height: 20),
            Row(children: [
              Expanded(child: BoutonFantome(libelle: 'Garder', onTap: () => Navigator.pop(context, false))),
              const SizedBox(width: 10),
              Expanded(
                child: BoutonFantome(
                  libelle: 'Annuler',
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
  final e = await r.annulerInvitation(i);
  messager.showSnackBar(SnackBar(content: Text(e ?? 'Invitation annulée')));
}
