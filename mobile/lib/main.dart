import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import 'composants.dart';
import 'donnees.dart';
import 'ecrans/accueil.dart';
import 'ecrans/appareils.dart';
import 'ecrans/connexion.dart';
import 'ecrans/reglages.dart';
import 'ecrans/verrou.dart';
import 'etat.dart';
import 'icones.dart';
import 'moteur.dart';
import 'securite.dart';
import 'theme.dart';

/// La démo (--dart-define=DEMO=true) : un réseau d'exemple, rien à relier.
const modeDemo = bool.fromEnvironment('DEMO');

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  SystemChrome.setEnabledSystemUIMode(SystemUiMode.edgeToEdge);
  SystemChrome.setSystemUIOverlayStyle(const SystemUiOverlayStyle(
    statusBarColor: Colors.transparent,
    statusBarIconBrightness: Brightness.light,
    systemNavigationBarColor: Colors.transparent,
    systemNavigationBarIconBrightness: Brightness.light,
  ));
  // La démo (--dart-define=DEMO=true) tourne sur un réseau d'exemple, déjà
  // rejoint ; la vraie appli lit son inscription dans le moteur.
  final reseau = modeDemo ? Reseau(inscrit: true) : Reseau.reel();
  await reseau.chargerReglages();
  if (!modeDemo) {
    Moteur.ecouter();
    await reseau.charger();
  }
  if (reseau.ecranMasque) await masquerEcran(true);
  WidgetsBinding.instance.addObserver(_BarrageRetour());
  runApp(CyberSAS(reseau: reseau));
}

class CyberSAS extends StatelessWidget {
  const CyberSAS({super.key, required this.reseau});
  final Reseau reseau;

  @override
  Widget build(BuildContext context) => EtatReseau(
        reseau: reseau,
        child: MaterialApp(
          title: 'CyberSAS',
          debugShowCheckedModeBanner: false,
          theme: themeCyberSAS(),
          // Le verrou passe au-dessus de tout, fenêtres comprises : une fenêtre
          // ouverte (l'import de la clé) reste en dessous et se retrouve après
          // l'empreinte.
          //
          // La police du téléphone est suivie jusqu'à 1,5 fois sa taille. Les
          // écrans tiennent sans défiler sur 360 dp de large : au-delà, les
          // libellés des lignes (« Adresse », « Certificat ») ne gardent que
          // deux lettres et l'accueil ne montre plus que sa carte. Android 14
          // réduit déjà le grossissement des grands textes (titres, logo) ;
          // 1,5 laisse les textes courants bien plus gros qu'à 1.
          //
          // Le rattrapage rend aux petits boutons (le retour, le crayon) les
          // touchers tombés juste à côté : 48 dp de cible, sans les grossir.
          builder: (context, enfant) => RattrapageTactile(
            child: MediaQuery.withClampedTextScaling(
              maxScaleFactor: 1.5,
              child: _Garde(child: enfant!),
            ),
          ),
          home: const _Racine(),
        ),
      );
}

/// L'écran de verrouillage, en calque sur toute l'appli : au lancement si
/// le réglage est actif, et au retour après plus de 30 s ailleurs.
class _Garde extends StatefulWidget {
  const _Garde({required this.child});
  final Widget child;

  @override
  State<_Garde> createState() => _GardeState();
}

class _GardeState extends State<_Garde> with WidgetsBindingObserver {
  late bool _verrouillee = EtatReseau.of(context).verrouAppli;

  /// Quand l'appli a été quittée, lu sur l'horloge du système (le temps
  /// depuis le démarrage du téléphone) : reculer l'heure du téléphone ne
  /// raccourcit pas l'absence.
  Future<Duration>? _partie;

  /// Un voile opaque sur l'appli dès qu'elle perd la main (volet des
  /// notifications, applis récentes) : l'aperçu ne montre rien.
  bool _voile = false;

  /// Ce qu'on a dit à Android pour l'aperçu des applis récentes.
  bool? _apercuProtege;

  /// Revenir dans l'appli moins de 30 s après l'avoir quittée ne la
  /// reverrouille pas.
  static const _grace = Duration(seconds: 30);

  /// Sans le canal (tests), une horloge monotone du processus.
  static final _chrono = Stopwatch()..start();

  static Future<Duration> _maintenant() async => await horlogeSysteme() ?? _chrono.elapsed;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
  }

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    final r = EtatReseau.of(context);
    final proteger = r.verrouAppli && r.inscrit;
    if (proteger != _apercuProtege) {
      _apercuProtege = proteger;
      protegerApercu(proteger);
    }
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    super.dispose();
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState etat) {
    final r = EtatReseau.of(context);
    final garde = r.verrouAppli && r.inscrit;
    switch (etat) {
      case AppLifecycleState.inactive || AppLifecycleState.hidden || AppLifecycleState.paused:
        if (garde && !_voile) setState(() => _voile = true);
        if (etat != AppLifecycleState.inactive) _partie ??= _maintenant();
      case AppLifecycleState.resumed:
        _revenir(garde);
      case AppLifecycleState.detached:
        break;
    }
  }

  /// De retour : reverrouille si l'absence a dépassé la grâce, ou si
  /// l'horloge a reculé (impossible avec elapsedRealtime, sauf tricherie
  /// ou redémarrage), puis lève le voile.
  Future<void> _revenir(bool garde) async {
    final partie = _partie;
    _partie = null;
    var reverrouiller = false;
    if (partie != null && garde) {
      final ecart = await _maintenant() - await partie;
      reverrouiller = ecart.isNegative || ecart > _grace;
    }
    if (!mounted) return;
    // Le clavier se referme : il ne pousse pas l'écran de verrouillage.
    if (reverrouiller) FocusManager.instance.primaryFocus?.unfocus();
    setState(() {
      if (reverrouiller) _verrouillee = true;
      _voile = false;
    });
  }

  @override
  Widget build(BuildContext context) {
    final r = EtatReseau.of(context);
    final verrou = _verrouillee && r.verrouAppli && r.inscrit;
    _BarrageRetour.actif = verrou;
    return Stack(children: [
      // Sous le verrou, rien ne répond au toucher, au clavier ni au lecteur
      // d'écran.
      ExcludeFocus(
        excluding: verrou,
        child: ExcludeSemantics(excluding: verrou, child: IgnorePointer(ignoring: verrou, child: widget.child)),
      ),
      if (verrou)
        Positioned.fill(
          child: MediaQuery.removeViewInsets(
            context: context,
            removeBottom: true,
            // Le geste retour est arrêté par _BarrageRetour : un PopScope,
            // ici au-dessus du Navigator, n'aurait pas de route à retenir.
            child: EcranVerrou(
              deverrouiller: () => setState(() => _verrouillee = false),
              sansCode: () {
                r.reglerVerrou(false);
                setState(() => _verrouillee = false);
              },
            ),
          ),
        ),
      if (_voile && r.verrouAppli && r.inscrit) const Positioned.fill(child: ColoredBox(color: Couleurs.fond)),
    ]);
  }
}

/// Sous le verrou, le bouton ou le geste retour ne fait rien : sans lui, il
/// fermerait la fenêtre ouverte sous l'écran de verrouillage (l'import de
/// la clé, un renommage). Enregistré avant runApp, il passe avant le
/// Navigator de MaterialApp (Flutter consulte les observateurs dans
/// l'ordre).
class _BarrageRetour with WidgetsBindingObserver {
  static bool actif = false;

  @override
  Future<bool> didPopRoute() async => actif;
}

/// Pas encore de réseau : l'écran de connexion. Sinon, l'appli.
class _Racine extends StatefulWidget {
  const _Racine();

  @override
  State<_Racine> createState() => _RacineState();
}

class _RacineState extends State<_Racine> {
  bool? _petitEcran;

  /// Sur un petit écran (téléphone, écran extérieur du Fold), l'appli reste
  /// debout : couchée, elle serait minuscule. Déplié, elle tourne librement.
  /// On se règle à chaque changement d'écran (plier, déplier).
  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    final petit = MediaQuery.sizeOf(context).shortestSide < 600;
    if (petit == _petitEcran) return;
    _petitEcran = petit;
    SystemChrome.setPreferredOrientations(
      petit ? const [DeviceOrientation.portraitUp] : const <DeviceOrientation>[],
    );
  }

  @override
  Widget build(BuildContext context) {
    final r = EtatReseau.of(context);
    final Widget ecran;
    if (!r.inscrit) {
      ecran = const EcranConnexion();
    } else {
      ecran = const Coquille();
    }
    return AnimatedSwitcher(duration: const Duration(milliseconds: 350), child: ecran);
  }
}

/// Les trois onglets, et la façon de naviguer selon l'écran :
/// - compact (téléphone, écran extérieur du Fold) : barre flottante en bas ;
/// - déplié en portrait (3/4) : une barre de 480 dp, centrée ;
/// - déplié en paysage (4/3) : un rail vertical de 96 dp à gauche.
class Coquille extends StatefulWidget {
  const Coquille({super.key});

  @override
  State<Coquille> createState() => _CoquilleState();
}

class _CoquilleState extends State<Coquille> {
  int _onglet = 0;

  static const _onglets = [(Ico.accueil, 'Accueil'), (Ico.appareils, 'Appareils'), (Ico.reglages, 'Réglages')];

  // Où luit le fond, écran par écran (comme dans les maquettes).
  static const _lueurs = [
    (Alignment(0, -0.4), Size(0.8, 0.34)),
    (Alignment(0, -0.52), Size(0.8, 0.24)),
    (Alignment(0.4, -0.76), Size(0.7, 0.22)),
  ];

  void _choisir(int i) => setState(() => _onglet = i);

  @override
  Widget build(BuildContext context) {
    final r = EtatReseau.of(context);
    final format = formatDe(context);
    final pages = [
      const EcranAccueil(),
      const EcranAppareils(),
      const EcranReglages(),
    ];
    final contenu = IndexedStack(index: _onglet, children: pages);
    final alerte = r.admin && r.demandes.isNotEmpty;
    final (centre, etendue) = format == Format.paysage
        ? (const Alignment(-0.32, -0.2), const Size(0.4, 0.4))
        : _lueurs[_onglet];

    return Scaffold(
      body: Fond(
        centre: centre,
        etendue: etendue,
        child: switch (format) {
          Format.paysage => SafeArea(
              child: Row(children: [
                _Rail(onglet: _onglet, onglets: _onglets, alerte: alerte, choisir: _choisir),
                Expanded(child: contenu),
              ]),
            ),
          Format.portrait => SafeArea(
              child: Column(children: [
                Expanded(child: contenu),
                Padding(
                  padding: const EdgeInsets.fromLTRB(24, 8, 24, 14),
                  child: SizedBox(
                    width: double.infinity,
                    child: _Barre(onglet: _onglet, onglets: _onglets, alerte: alerte, choisir: _choisir),
                  ),
                ),
              ]),
            ),
          Format.compact => SafeArea(
              child: Column(children: [
                Expanded(child: contenu),
                Padding(
                  padding: const EdgeInsets.fromLTRB(16, 8, 16, 12),
                  child: _Barre(onglet: _onglet, onglets: _onglets, alerte: alerte, choisir: _choisir),
                ),
              ]),
            ),
        },
      ),
    );
  }
}

typedef _Onglet = (Ico, String);

/// La barre flottante : 66 dp, rayon 24, fond flouté, bordure #2A333D.
class _Barre extends StatelessWidget {
  const _Barre({required this.onglet, required this.onglets, required this.alerte, required this.choisir});
  final int onglet;
  final List<_Onglet> onglets;
  final bool alerte;
  final void Function(int) choisir;

  @override
  Widget build(BuildContext context) => Container(
        height: 66,
        decoration: BoxDecoration(
          color: Couleurs.barre,
          borderRadius: BorderRadius.circular(24),
          border: Border.all(color: Couleurs.bordure),
          boxShadow: const [BoxShadow(color: Color(0x80000000), blurRadius: 30, offset: Offset(0, 10))],
        ),
        child: Row(children: [
          for (var i = 0; i < onglets.length; i++)
            Expanded(
              child: _Bouton(
                o: onglets[i],
                actif: i == onglet,
                pastille: i == 1 && alerte,
                onTap: () => choisir(i),
              ),
            ),
        ]),
      );
}

/// Le rail du Fold déplié en paysage.
class _Rail extends StatelessWidget {
  const _Rail({required this.onglet, required this.onglets, required this.alerte, required this.choisir});
  final int onglet;
  final List<_Onglet> onglets;
  final bool alerte;
  final void Function(int) choisir;

  @override
  Widget build(BuildContext context) => Container(
        width: 96,
        decoration: const BoxDecoration(border: Border(right: BorderSide(color: Couleurs.separateur))),
        padding: const EdgeInsets.fromLTRB(12, 40, 12, 20),
        child: Column(children: [
          for (var i = 0; i < onglets.length; i++)
            SizedBox(
              height: 70,
              child: _Bouton(o: onglets[i], actif: i == onglet, pastille: i == 1 && alerte, rail: true, onTap: () => choisir(i)),
            ),
        ]),
      );
}

/// Un onglet : l'icône dans une pastille douce quand il est choisi, le
/// libellé dessous (ou à côté sur la barre du Fold déplié). Un seul effet
/// pour l'onglet actif, pas trois.
class _Bouton extends StatelessWidget {
  const _Bouton({required this.o, required this.actif, required this.pastille, required this.onTap, this.rail = false});
  final _Onglet o;
  final bool actif;
  final bool pastille;
  final bool rail;
  final VoidCallback onTap;

  static const _duree = Duration(milliseconds: 220);

  @override
  Widget build(BuildContext context) {
    final icone = Stack(clipBehavior: Clip.none, children: [
      Icone(o.$1, couleur: actif ? Couleurs.cyan : Couleurs.secondaire, taille: 22),
      if (pastille)
        Positioned(
          top: -1,
          right: -2,
          child: Container(
            width: 9,
            height: 9,
            decoration: BoxDecoration(
              shape: BoxShape.circle,
              color: Couleurs.rouge,
              border: Border.all(color: const Color(0xFF080D13), width: 1.5),
            ),
          ),
        ),
    ]);
    // Le libellé grandit avec la police du téléphone, jusqu'à 1,3 fois :
    // au-delà, il sortirait de la barre de 66 dp. L'icône et le lecteur
    // d'écran disent le reste.
    final libelle = MediaQuery.withClampedTextScaling(
      maxScaleFactor: 1.3,
      child: Text(o.$2,
          style: texte(12, graisse: actif ? 600 : 500, couleur: actif ? Couleurs.texte : Couleurs.secondaire)),
    );
    final fond = BoxDecoration(
      color: actif ? Couleurs.cyan.withValues(alpha: 0.12) : Couleurs.cyan.withValues(alpha: 0),
      borderRadius: BorderRadius.circular(16),
    );

    final corps = Column(mainAxisAlignment: MainAxisAlignment.center, mainAxisSize: MainAxisSize.min, children: [
        AnimatedContainer(
          duration: _duree,
          curve: Curves.easeOutCubic,
          width: rail ? 56 : 58,
          height: 32,
          alignment: Alignment.center,
          decoration: fond,
          child: icone,
        ),
        const SizedBox(height: 4),
        libelle,
      ]);
    return Semantics(
      button: true,
      selected: actif,
      label: pastille ? '${o.$2}, demandes en attente' : null,
      child: GestureDetector(onTap: onTap, behavior: HitTestBehavior.opaque, child: corps),
    );
  }
}
