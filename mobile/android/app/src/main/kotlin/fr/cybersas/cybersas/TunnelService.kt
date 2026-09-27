package fr.cybersas.cybersas

import android.content.Context
import android.content.Intent
import android.net.VpnService
import android.system.OsConstants
import android.util.Log
import fr.cybersas.pont.Pont
import fr.cybersas.pont.Protecteur
import org.json.JSONObject
import java.io.File

// Le tunnel CyberSAS, tenu par le moteur Go (pont/, compilé par gomobile).
//
// Seul le réseau privé (10.77.0.0/24) passe par l'interface : la navigation
// sur Internet reste en dehors. Le DNS du réseau répond à *.sas.internal et
// transmet le reste à un résolveur public.
class TunnelService : VpnService() {
    companion object {
        const val ARRET = "fr.cybersas.ARRET"
        private const val MTU = 1360 // tunnel.MTU

        fun dossier(c: Context): File = File(c.filesDir, "sas")
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        if (intent?.action == ARRET) {
            Pont.arreter()
            stopSelf()
            return START_NOT_STICKY
        }
        // Hors du fil principal : c'est aussi celui de Flutter, et le moteur
        // y ferait du réseau.
        Thread {
            try {
                demarrer()
            } catch (e: Exception) {
                Log.e("CyberSAS", "tunnel impossible", e)
                stopSelf()
            }
        }.start()
        return START_NOT_STICKY
    }

    private fun demarrer() {
        val infos = JSONObject(Pont.inscription(dossier(this).path).ifEmpty { throw IllegalStateException("pas inscrit") })
        val (reseau, bits) = infos.getString("reseau").split("/")
        // Le serveur est la première adresse du réseau : c'est aussi le DNS.
        val dns = reseau.substringBeforeLast('.') + ".1"
        val interfaceTun = Builder()
            .setSession("CyberSAS")
            .setMtu(MTU)
            .addAddress(infos.getString("adresse"), bits.toInt())
            .addRoute(reseau, bits.toInt())
            .addDnsServer(dns)
            .addSearchDomain(infos.optString("domaine", "sas.internal"))
            // L'appli elle-même reste hors du tunnel : pour l'établir, le
            // moteur doit résoudre et joindre le serveur par le vrai réseau.
            // Sans cela, sa résolution de nom partirait vers le DNS du
            // réseau, joignable seulement par le tunnel qu'il cherche à ouvrir.
            .addDisallowedApplication(packageName)
            // Le réseau privé est en IPv4 : sans ceci, Android bloquerait tout
            // l'IPv6 du téléphone tant que le tunnel est ouvert.
            .allowFamily(OsConstants.AF_INET6)
            .establish() ?: throw IllegalStateException("autorisation VPN retirée")
        val fd = interfaceTun.detachFd()
        Pont.demarrer(fd.toLong(), dossier(this).path, object : Protecteur {
            override fun proteger(fd: Long): Boolean = protect(fd.toInt())
        })
    }

    // Un autre VPN a pris la place, ou l'utilisateur a retiré l'autorisation.
    override fun onRevoke() {
        Pont.arreter()
        stopSelf()
    }

    override fun onDestroy() {
        Pont.arreter()
        super.onDestroy()
    }
}
