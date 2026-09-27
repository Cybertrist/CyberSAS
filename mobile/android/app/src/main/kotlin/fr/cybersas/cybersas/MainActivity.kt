package fr.cybersas.cybersas

import android.content.Intent
import android.net.VpnService
import android.os.Build
import android.os.SystemClock
import android.view.WindowManager
import androidx.biometric.BiometricManager
import androidx.biometric.BiometricManager.Authenticators.BIOMETRIC_STRONG
import androidx.biometric.BiometricPrompt
import androidx.core.content.ContextCompat
import fr.cybersas.pont.Pont
import io.flutter.embedding.android.FlutterFragmentActivity
import io.flutter.embedding.engine.FlutterEngine
import io.flutter.plugin.common.MethodChannel
import java.util.Arrays
import java.util.concurrent.Executors
import javax.crypto.Cipher

// FlutterFragmentActivity : l'invite biométrique d'Android (BiometricPrompt,
// ici et dans local_auth) en a besoin pour s'afficher.
class MainActivity : FlutterFragmentActivity() {
    // Les appels réseau du moteur (inscription, désinscription) ne doivent
    // jamais tourner sur le fil de l'interface.
    private val travail = Executors.newSingleThreadExecutor()
    private var enAttente: MethodChannel.Result? = null
    private var canalMoteur: MethodChannel? = null

    // « Masquer l'écran » (réglage) et le verrou de l'appli : les deux
    // peuvent demander FLAG_SECURE, on le garde tant que l'un le veut.
    private var ecranMasque = false
    private var verrouActif = false

    // Une seule invite d'empreinte à la fois.
    private var inviteOuverte = false

    // Le lien d'invitation (cybersas://rejoindre?...) qui a ouvert l'appli.
    private fun lien(intent: Intent?): String? =
        intent?.data?.takeIf { it.scheme == "cybersas" }?.toString()

    companion object {
        private const val AUTORISATION_VPN = 42
        private const val SANS_EMPREINTE =
            "Aucune empreinte enregistrée sur ce téléphone : ajoute-en une dans les réglages d'Android."
    }

    override fun configureFlutterEngine(flutterEngine: FlutterEngine) {
        super.configureFlutterEngine(flutterEngine)
        val messager = flutterEngine.dartExecutor.binaryMessenger
        MethodChannel(messager, "fr.cybersas/ecran").setMethodCallHandler { appel, reponse ->
            when (appel.method) {
                // « Masquer l'écran » : pas de capture, et un aperçu vide dans
                // les applis récentes.
                "masquer" -> {
                    ecranMasque = appel.arguments == true
                    appliquerMasque()
                    reponse.success(null)
                }
                // Le verrou de l'appli est actif : l'aperçu des applis
                // récentes ne doit pas montrer ce qu'il protège.
                "verrou" -> {
                    verrouActif = appel.arguments == true
                    appliquerMasque()
                    reponse.success(null)
                }
                // Le temps écoulé depuis le démarrage du téléphone, en
                // millisecondes : il ne recule pas quand on change l'heure,
                // contrairement à DateTime.now().
                "horloge" -> reponse.success(SystemClock.elapsedRealtime())
                else -> reponse.notImplemented()
            }
        }

        // Le moteur du tunnel (pont/ en Go).
        val canal = MethodChannel(messager, "fr.cybersas/moteur")
        canalMoteur = canal
        canal.setMethodCallHandler { appel, reponse ->
            val dossier = TunnelService.dossier(this)
            when (appel.method) {
                "inscription" -> reponse.success(Pont.inscription(dossier.path))
                "lien" -> reponse.success(lien(intent))
                // Le nom donné au téléphone dans ses réglages, sinon son modèle.
                "nomAppareil" -> reponse.success(
                    android.provider.Settings.Global.getString(contentResolver, android.provider.Settings.Global.DEVICE_NAME)
                        ?.takeIf { it.isNotBlank() } ?: android.os.Build.MODEL
                )
                "etat" -> reponse.success(Pont.etat())
                "rejoindre" -> {
                    val serveur = appel.argument<String>("serveur") ?: ""
                    val verrou = appel.argument<String>("verrou") ?: ""
                    val cle = appel.argument<String>("cle") ?: ""
                    val jeton = appel.argument<String>("jeton") ?: ""
                    // Sans nom choisi, celui du modèle : « SM-F966B ».
                    val nom = (appel.argument<String>("nom") ?: "").ifEmpty { android.os.Build.MODEL }
                    val autorite = appel.argument<String>("autorite") ?: ""
                    enArriere(reponse) {
                        Pont.rejoindre(dossier.path, serveur, verrou, cle, jeton, nom, autorite)
                    }
                }
                "demarrer" -> {
                    // La première fois, Android demande à l'utilisateur
                    // d'autoriser le VPN.
                    val demande = VpnService.prepare(this)
                    if (demande == null) {
                        lancerService()
                        reponse.success(true)
                    } else if (enAttente != null) {
                        // La fenêtre d'autorisation est déjà ouverte : le
                        // premier appel reçoit false, celui-ci attend la
                        // réponse. Aucun Result ne reste sans réponse.
                        enAttente?.success(false)
                        enAttente = reponse
                    } else {
                        enAttente = reponse
                        @Suppress("DEPRECATION")
                        startActivityForResult(demande, AUTORISATION_VPN)
                    }
                }
                "libeller" -> {
                    val cle = appel.argument<String>("cle") ?: ""
                    val libelle = appel.argument<String>("libelle") ?: ""
                    enArriere(reponse) { Pont.libeller(dossier.path, cle, libelle); null }
                }
                // Admin : les appareils, la clé du verrou, les signatures.
                "appareils" -> enArriere(reponse) { Pont.appareils(dossier.path) }
                // L'état du réseau par l'API, quand le tunnel est coupé.
                "reseau" -> enArriere(reponse) { Pont.reseau(dossier.path) }
                "coffrePresent" -> reponse.success(Coffre.present(this))
                "coffreRanger" -> coffreRanger(
                    dossier.path,
                    appel.argument<String>("graine") ?: "",
                    appel.argument<String>("titre") ?: "Ranger la clé du verrou",
                    reponse,
                )
                "coffreEffacer" -> { Coffre.effacer(this); reponse.success(null) }
                // L'empreinte ouvre le coffre pour cette seule opération : la
                // clé en sort et va droit au moteur, sans passer par Flutter.
                "signer" -> {
                    // Les fiches telles que Pont.appareils les a rendues : le
                    // moteur refuse si le serveur a changé un champ depuis.
                    val fiches = appel.argument<String>("fiches") ?: "[]"
                    avecLeVerrou(appel.argument<String>("titre") ?: "Signer", appel.argument<String>("detail") ?: "", reponse) { graine ->
                        Pont.signer(dossier.path, graine, fiches)
                    }
                }
                "revoquer" -> {
                    val cles = appel.argument<String>("cles") ?: ""
                    avecLeVerrou(appel.argument<String>("titre") ?: "Révoquer", appel.argument<String>("detail") ?: "", reponse) { graine ->
                        Pont.revoquer(dossier.path, graine, cles)
                    }
                }
                "inviter" -> {
                    val qui = appel.argument<String>("utilisateur") ?: ""
                    val minutes = appel.argument<Int>("minutes") ?: 10
                    enArriere(reponse) { Pont.inviter(dossier.path, qui, minutes.toLong()) }
                }
                "lienReseau" -> enArriere(reponse) { Pont.lienReseau(dossier.path) }
                "retirer" -> {
                    val cle = appel.argument<String>("cle") ?: ""
                    enArriere(reponse) { Pont.retirer(dossier.path, cle); null }
                }
                "arreter" -> {
                    startService(Intent(this, TunnelService::class.java).setAction(TunnelService.ARRET))
                    reponse.success(null)
                }
                "quitter" -> {
                    startService(Intent(this, TunnelService::class.java).setAction(TunnelService.ARRET))
                    enArriere(reponse) {
                        // Le coffre part même si le moteur n'arrive pas à se
                        // désinscrire : on ne laisse pas la clé du verrou
                        // derrière soi.
                        try {
                            Pont.quitter(dossier.path)
                        } finally {
                            Coffre.effacer(this)
                        }
                        null
                    }
                }
                else -> reponse.notImplemented()
            }
        }
    }

    // FLAG_SECURE tant que « Masquer l'écran » est actif. Sous verrou, sur
    // Android 13 et plus, seul l'aperçu des applis récentes est retiré (les
    // captures restent possibles, l'écran de verrou ne montre rien) ; avant
    // Android 13, faute de mieux, FLAG_SECURE aussi.
    private fun appliquerMasque() {
        val moderne = Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU
        if (moderne) setRecentsScreenshotEnabled(!verrouActif)
        if (ecranMasque || (verrouActif && !moderne)) {
            window.addFlags(WindowManager.LayoutParams.FLAG_SECURE)
        } else {
            window.clearFlags(WindowManager.LayoutParams.FLAG_SECURE)
        }
    }

    // Range la clé du verrou collée par l'admin. La graine est vérifiée
    // (c'est bien celle du verrou de ce réseau) avant d'ouvrir l'invite :
    // pas d'empreinte pour une clé fausse.
    //
    // La graine arrive de Flutter en String (le canal n'a pas mieux), et le
    // moteur Go la prend en String aussi : ces copies-là ne peuvent pas être
    // effacées, elles attendent le ramasse-miettes. Les tableaux d'octets,
    // eux, sont remis à zéro dès qu'ils ont servi.
    private fun coffreRanger(dossier: String, graineTexte: String, titre: String, reponse: MethodChannel.Result) {
        val graine = graineTexte.trim()
        if (!empreintePossible(reponse)) return
        travail.execute {
            val empreinte: String
            val chiffreur: Coffre.Chiffreur
            try {
                empreinte = Pont.verifierVerrou(dossier, graine)
                chiffreur = Coffre.chiffreur(this)
            } catch (t: Throwable) {
                runOnUiThread { erreur(reponse, t) }
                return@execute
            }
            runOnUiThread {
                inviter(titre, "Elle sera chiffrée dans la puce du téléphone.", chiffreur.cipher, reponse,
                    abandon = { Coffre.abandonner(chiffreur) }) { cipher ->
                    enArriere(reponse) {
                        val octets = graine.toByteArray()
                        try {
                            Coffre.ranger(this, chiffreur, cipher, octets)
                        } catch (t: Throwable) {
                            // Échec avant le remplacement du fichier :
                            // l'ancien coffre reste tel quel.
                            Coffre.abandonner(chiffreur)
                            throw t
                        } finally {
                            Arrays.fill(octets, 0)
                        }
                        empreinte
                    }
                }
            }
        }
    }

    // Ouvre le coffre pour une seule opération : invite d'empreinte avec le
    // déchiffreur, puis [operation] hors du fil de l'interface avec la graine.
    private fun avecLeVerrou(titre: String, detail: String, reponse: MethodChannel.Result, operation: (String) -> Any?) {
        if (!empreintePossible(reponse)) return
        val (dechiffreur, chiffre) = try {
            Coffre.dechiffreur(this)
        } catch (t: Throwable) {
            erreur(reponse, t)
            return
        }
        inviter(titre, detail, dechiffreur, reponse) { cipher ->
            enArriere(reponse) {
                val octets = Coffre.lire(cipher, chiffre)
                try {
                    operation(String(octets))
                } finally {
                    Arrays.fill(octets, 0)
                }
            }
        }
    }

    private fun empreintePossible(reponse: MethodChannel.Result): Boolean {
        if (BiometricManager.from(this).canAuthenticate(BIOMETRIC_STRONG) == BiometricManager.BIOMETRIC_SUCCESS) return true
        reponse.error("impossible", SANS_EMPREINTE, null)
        return false
    }

    // L'invite d'empreinte d'Android, liée à [cipher] (CryptoObject) : la
    // clé du Keystore ne s'ouvre que pour ce Cipher-là, et seulement après
    // ce doigt-là. Annulée : erreur « annulee », que Flutter passe sous
    // silence.
    private fun inviter(
        titre: String,
        detail: String,
        cipher: Cipher,
        reponse: MethodChannel.Result,
        abandon: () -> Unit = {},
        suite: (Cipher) -> Unit,
    ) {
        if (inviteOuverte) {
            abandon()
            reponse.error("annulee", "Une empreinte est déjà demandée.", null)
            return
        }
        val infos = BiometricPrompt.PromptInfo.Builder()
            .setTitle(titre)
            .apply { if (detail.isNotBlank()) setDescription(detail) }
            .setAllowedAuthenticators(BIOMETRIC_STRONG)
            .setNegativeButtonText("Annuler")
            .setConfirmationRequired(true)
            .build()
        val invite = BiometricPrompt(this, ContextCompat.getMainExecutor(this), object : BiometricPrompt.AuthenticationCallback() {
            override fun onAuthenticationSucceeded(resultat: BiometricPrompt.AuthenticationResult) {
                inviteOuverte = false
                val autorise = resultat.cryptoObject?.cipher
                if (autorise == null) {
                    abandon()
                    reponse.error("empreinte", "L'empreinte n'a pas ouvert le coffre.", null)
                    return
                }
                suite(autorise)
            }

            override fun onAuthenticationError(code: Int, message: CharSequence) {
                inviteOuverte = false
                abandon()
                when (code) {
                    BiometricPrompt.ERROR_NEGATIVE_BUTTON,
                    BiometricPrompt.ERROR_USER_CANCELED,
                    BiometricPrompt.ERROR_CANCELED -> reponse.error("annulee", message.toString(), null)
                    BiometricPrompt.ERROR_NO_BIOMETRICS,
                    BiometricPrompt.ERROR_HW_NOT_PRESENT,
                    BiometricPrompt.ERROR_HW_UNAVAILABLE -> reponse.error("impossible", SANS_EMPREINTE, null)
                    else -> reponse.error("empreinte", message.toString(), null)
                }
            }

            // Un doigt non reconnu : l'invite reste ouverte et redemande.
            override fun onAuthenticationFailed() {}
        })
        inviteOuverte = true
        invite.authenticate(infos, BiometricPrompt.CryptoObject(cipher))
    }

    // Un lien ouvert pendant que l'appli tourne déjà.
    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        setIntent(intent)
        lien(intent)?.let { canalMoteur?.invokeMethod("lien", it) }
    }

    private fun lancerService() {
        startService(Intent(this, TunnelService::class.java))
    }

    @Deprecated("startActivityForResult suffit ici : une seule demande possible à la fois")
    override fun onActivityResult(requestCode: Int, resultCode: Int, data: Intent?) {
        super.onActivityResult(requestCode, resultCode, data)
        if (requestCode != AUTORISATION_VPN) return
        val r = enAttente ?: return
        enAttente = null
        if (resultCode == RESULT_OK) {
            lancerService()
            r.success(true)
        } else {
            r.success(false)
        }
    }

    // Le coffre ne s'ouvre plus (empreinte ajoutée au téléphone) : code
    // « coffre », pour que Flutter oublie qu'il avait la clé.
    private fun erreur(reponse: MethodChannel.Result, t: Throwable) {
        val code = if (t is Coffre.Invalide) "coffre" else "moteur"
        reponse.error(code, t.message ?: t.toString(), null)
    }

    // Exécute [bloc] hors du fil de l'interface, et rend son résultat (ou
    // son erreur) à Flutter sur le fil de l'interface. Throwable : une
    // erreur du moteur (UnsatisfiedLinkError, OutOfMemoryError) ne doit pas
    // laisser Flutter attendre une réponse qui ne viendra jamais.
    private fun enArriere(reponse: MethodChannel.Result, bloc: () -> Any?) {
        travail.execute {
            try {
                val r = bloc()
                runOnUiThread { reponse.success(r) }
            } catch (t: Throwable) {
                runOnUiThread { erreur(reponse, t) }
            }
        }
    }
}
