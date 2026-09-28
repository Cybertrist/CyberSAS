#!/bin/bash
# Toutes les figures fixes du README : la bannière, les bandeaux de
# section, les grilles, la feuille de route et les limites. C’est le seul
# fichier à ouvrir pour changer un texte.
#
#   bash docs/tools/figures.sh
source "$(dirname "${BASH_SOURCE[0]}")/rendu.sh"
mkdir -p "$DOCS/sections" "$DOCS/schemas"
LOGO="file:///$DOCS/logo.png"

# ------------------------------------------------------------- bannière
# Le logo est plein cadre, sans badge : la plaque arrondie et son contour
# sont tracés ici, comme sur la carte du profil.
{ entete 1280; cat <<HTML
<style>
.w{width:1280px;height:340px;position:relative;overflow:hidden;
   background:radial-gradient(55% 140% at 10% 0%,#31E7FD24 0%,transparent 60%),linear-gradient(135deg,#0D1117 0%,#0D1117 55%,#04060A 100%)}
.grille{position:absolute;inset:0;opacity:.4;
  background-image:linear-gradient(#31E7FD12 1px,transparent 1px),linear-gradient(90deg,#31E7FD12 1px,transparent 1px);
  background-size:46px 46px;-webkit-mask-image:radial-gradient(70% 100% at 8% 50%,#000 0%,transparent 72%)}
.cat{position:absolute;top:26px;right:30px;display:flex;gap:8px}
.cat b{font-family:'JetBrains Mono',monospace;font-weight:500;font-size:12px;letter-spacing:2.2px;
  color:#31E7FDE0;border:1px solid #31E7FD46;background:#31E7FD12;border-radius:5px;padding:7px 13px}
.cat i{font-style:normal;font-family:'JetBrains Mono',monospace;font-weight:500;font-size:12px;letter-spacing:2.2px;
  color:#94A3B0;border:1px solid #2A333D;background:#0C1117;border-radius:5px;padding:7px 13px}
.in{position:absolute;inset:0;display:flex;align-items:center;gap:48px;padding:0 66px}
.logo{width:150px;height:150px;flex-shrink:0;border-radius:36px;border:1.5px solid #31E7FD55;
  box-shadow:0 0 34px #31E7FD40,0 14px 30px #000A}
h1{font-family:Syne,sans-serif;font-weight:800;font-size:60px;line-height:1;letter-spacing:-1.5px}
h1 em{font-style:normal;color:var(--cyan)}
.sl{font-family:'Space Grotesk',sans-serif;font-size:22px;line-height:1.55;margin-top:18px;color:#94A3B0;max-width:860px}
.sl b{font-weight:500;color:#E6EDF3}.sl em{font-style:normal;color:var(--cyan)}
.pl{display:flex;gap:8px;margin-top:18px}
.pl span{font-family:'Space Grotesk',sans-serif;font-size:12.5px;font-weight:500;letter-spacing:.6px;
  color:#31E7FDD0;border:1px solid #31E7FD3A;background:#31E7FD0E;border-radius:6px;padding:6px 11px}
.ln{position:absolute;left:0;right:0;bottom:0;height:3px;background:linear-gradient(90deg,#31E7FD 0%,#01B9FD 40%,transparent 90%)}
</style></head><body>
<div class="w"><div class="grille"></div>
<div class="cat"><b>VPN</b><b>GO</b><b>ANDROID</b><i>EN COURS</i></div>
<div class="in"><img class="logo" src="$LOGO">
<div><h1>Cyber<em>SAS</em></h1>
<p class="sl"><b>Mon propre réseau privé, écrit de bout en bout.</b><br>Un serveur qui <em>relaie sans lire</em>, une clé qui ne quitte <em>jamais</em> le téléphone.</p>
<div class="pl"><span>Go</span><span>Noise IK</span><span>ChaCha20-Poly1305</span><span>Ed25519</span><span>nftables</span><span>Flutter</span></div></div></div>
<div class="ln"></div></div>
HTML
pied; } > "$D/html/banniere.html"
rendre banniere.html "$DOCS/banniere.png"

# -------------------------------------------------------------- bandeaux
bandeaux "$DOCS/sections" "Ce que c'est" "L'appli" "Comment ça marche" "Rejoindre le réseau" "Qui peut aller où" \
  "L'équipe" "Essayer le labo" "Ce qu'il y a dedans" "La sécurité, et ses limites" "La feuille de route"

# ------------------------------------------------------------- les grilles
grille "$DOCS/schemas/promesses.png" 3 \
  "hub|Un seul point d'entrée|Un petit serveur public, et c'est tout. À la maison, aucun port ouvert : la machine sort vers le serveur par le tunnel." \
  "lock|Chiffré de bout en bout|Entre deux appareils, le serveur relaie des paquets qu'il ne peut pas ouvrir. Il n'a pas les clés." \
  "verified_user|Un serveur qu'on n'a pas à croire|Chaque appareil porte un certificat signé par la clé du verrou. Un serveur piraté ne peut ni s'intercaler, ni faire entrer un intrus." \
  "fingerprint|La clé dans la puce du téléphone|La clé du verrou dort chiffrée par la puce sécurisée du téléphone de l'admin. Une empreinte l'ouvre pour une seule signature." \
  "account_circle|Un compte Google, aucun mot de passe|On rejoint le réseau avec son compte Google. Rien n'est créé, stocké ni partagé ici." \
  "code|Tout écrit à la main|Le protocole, le serveur, les clients et l'appli. Seules les primitives cryptographiques viennent de bibliothèques éprouvées."

grille "$DOCS/schemas/regles.png" 3 \
  "admin_panel_settings|Les admins|Atteignent tout le réseau." \
  "devices|Chacun ses appareils|Chacun atteint ses propres appareils, jamais ceux des autres." \
  "language|L'équipe et la maison|L'équipe atteint les services web de la maison, pas son SSH." \
  "dns|Le port publié|Seul le serveur atteint le port que la maison publie sur Internet." \
  "block|La maison ne se retourne pas|La maison ne peut ouvrir de connexion vers personne." \
  "gpp_good|Signée par l'admin|La politique est signée par la clé du verrou : le serveur ne peut pas la changer sans que ça se voie."

grille "$DOCS/schemas/equipe.png" 3 \
  "person_add|Inviter|L'admin choisit un membre de l'équipe et une durée : 10 minutes, une heure ou un jour. L'appli partage un lien <code>cybersas://</code> à usage unique, qui porte la clé du verrou." \
  "draw|Signer|La demande arrive sur le téléphone de l'admin avec l'empreinte, l'adresse et le groupe. Il compare, puis signe avec son doigt. Si le serveur change un champ entre-temps, rien n'est signé." \
  "edit|Renommer|Chacun renomme ses appareils ; l'admin, tous. Le nom affiché ne fait pas partie du certificat : l'empreinte et l'adresse restent la référence." \
  "logout|Retirer|Un appareil dont on ne veut plus est coupé tout de suite. Retiré seulement, il pourrait se réinscrire." \
  "gpp_bad|Révoquer|Pour un appareil volé : la liste de révocation est signée sur le téléphone de l'admin, et aucun appareil ne revient jamais à une liste plus ancienne." \
  "group|L'équipe elle-même|La liste des comptes Google qui ont le droit d'entrer vit sur le serveur, dans <code>equipe.txt</code>. L'admin la règle depuis l'appli, écran Équipe, ou en ligne de commande."

grille "$DOCS/schemas/dedans.png" 3 \
  "sync_alt|Le protocole|<code>internal/noise</code> et <code>internal/tunnel</code> : poignée de main Noise IK, identique octet pour octet au vecteur officiel, puis ChaCha20-Poly1305, renouvelé toutes les deux minutes." \
  "dns|sasd, le serveur|<code>cmd/sasd</code> : le tunnel, le relais chiffré, l'API d'inscription en HTTPS, le pare-feu nftables et le DNS du réseau (<code>maison.sas.internal</code>)." \
  "terminal|sas, le client Linux|<code>cmd/sas</code> : pour les machines sans écran comme le serveur de la maison, et l'outil de l'admin pour le verrou." \
  "key|Le verrou|<code>internal/verrou</code> : certificats d'appareil avec expiration, politique signée et versionnée, révocations signées." \
  "smartphone|L'appli Android|<code>mobile/</code> : Flutter, pour le téléphone comme pour le Fold déplié. Le moteur Go embarqué (<code>pont/</code>), le service VPN d'Android, et l'admin au doigt." \
  "shield|Nginx et oauth2-proxy|Pour publier un service de la maison sur une page web, derrière la même connexion Google."

# ------------------------------------------------------- feuille de route
# etape <état> <titre> <texte> ; état : fait, cours, venir, discuter
etape () {
  local ch lib
  case "$1" in
    fait) ch=v; lib='FAIT';; cours) ch=c; lib='EN COURS';; venir) ch=g; lib='À VENIR';; discuter) ch=o; lib='À DISCUTER';;
  esac
  printf '<div class="e %s"><i></i><div class="t"><h3>%s</h3><p>%s</p></div><b>%s</b></div>' "$ch" "$2" "$3" "$lib"
}
{ entete 1280; cat <<HTML
<style>
.w{padding:24px 56px;display:flex;flex-direction:column;gap:10px;position:relative}
.w:before{content:'';position:absolute;left:79px;top:40px;bottom:40px;width:2px;background:linear-gradient(#3DDC97,#3DDC97 60%,#2A333D 70%)}
.e{display:flex;align-items:center;gap:22px;position:relative}
.e i{width:48px;height:48px;flex-shrink:0;border-radius:50%;background:#0D1117;border:2px solid var(--a);box-shadow:0 0 18px color-mix(in srgb,var(--a) 40%,transparent);position:relative;z-index:1}
.e i:after{content:'';position:absolute;inset:14px;border-radius:50%;background:var(--a)}
.t{flex:1;background:var(--carte);border:1px solid var(--bord);border-radius:14px;padding:14px 18px}
h3{font-family:'Space Grotesk',sans-serif;font-size:16px;font-weight:700;margin-bottom:4px}
p{font-family:'Space Grotesk',sans-serif;font-size:13.5px;line-height:1.5;color:var(--texte)}
b{font-family:'JetBrains Mono',monospace;font-size:11.5px;font-weight:500;letter-spacing:1.8px;color:var(--a);
  border:1px solid color-mix(in srgb,var(--a) 40%,transparent);background:color-mix(in srgb,var(--a) 8%,transparent);border-radius:6px;padding:6px 11px;width:128px;text-align:center}
.v{--a:#3DDC97}.c{--a:#31E7FD}.g{--a:#5C6A7A}.o{--a:#94A3B0}
.g i:after,.o i:after{background:transparent}
</style></head><body><div class="w">
$(etape fait 'Le tunnel, le serveur et le verrou' 'Noise IK, relais chiffré de bout en bout, certificats, politique et révocations signés. 79 tests Go, 8 cibles de fuzzing, 18 vérifications de bout en bout dans le labo.')
$(etape fait 'Trois audits' '46 constats de trois relecteurs et d’une revue de sécurité, 10 aux outils du métier, puis 23 sur l’appli et l’admin à distance. Tous traités, un seul accepté et expliqué.')
$(etape fait 'L’appli Android, avec le vrai tunnel' 'Le moteur Go embarqué et le service VPN d’Android : sur le Fold, l’interrupteur ouvre un vrai tunnel vers le labo.')
$(etape fait 'L’admin depuis le téléphone' 'Signer une demande, refuser, inviter par un lien, renommer, retirer, révoquer. La clé du verrou dort dans la puce, une empreinte par signature.')
$(etape fait 'La connexion Google dans l’appli' 'Le lien du réseau, puis l’invite native d’Android : un compte de l’équipe, une touche. Le serveur vérifie le jeton, l’admin signe.')
$(etape fait 'L’équipe depuis l’appli' 'Ajouter un compte Google, changer son groupe, le retirer, chaque fois avec le doigt ; le serveur garde toujours un admin. Les invitations qui n’ont pas servi se voient et s’annulent.')
$(etape fait 'Une appli juste, et lisible par tous' 'Les ports ouverts et le bouton Ouvrir dans le navigateur se déduisent de la politique signée. Des cibles de 48 dp, et des écrans qui tiennent jusqu’à une police une fois et demie plus grande.')
$(etape venir 'Le serveur en ligne' 'D’abord sur un ordinateur à la maison pour les essais, puis sur un VPS avec un certificat Let’s Encrypt et une nouvelle clé du verrou.')
$(etape venir 'Un secours pour la clé du verrou' 'Perdre le téléphone de l’admin ne doit pas geler le réseau : une seconde clé, gardée hors ligne.')
$(etape discuter 'L’appli Windows' 'Le moteur tourne déjà sur PC ; il manque le pilote réseau et l’interface.')
$(etape discuter 'Un site, et une console web' 'Une vitrine pour présenter CyberSAS, et une console pour voir le réseau à distance, sans jamais pouvoir signer.')
</div>
HTML
pied; } > "$D/html/feuille.html"
rendre feuille.html "$DOCS/schemas/feuille.png"

# -------------------------------------------------------------- limites
colonnes "$DOCS/schemas/limites.png" \
  "verified_user|Ce qui est vrai" \
  "Le serveur ne lit pas¦ ce que deux appareils s’envoient : une capture réseau sur le serveur ne voit que du chiffré, c’est l’un des essais du labo.|Un serveur piraté ne fait entrer personne.¦ Sans certificat signé par le verrou, un appareil est refusé par les autres.|Retirer quelqu’un de l’équipe¦ coupe ses appareils en cinq secondes au plus.|Trois audits¦, 79 constats : 78 corrigés, chacun avec son test quand c’est possible, un accepté et expliqué." \
  "info|Ce qui ne l’est pas" \
  "Pas d’audit humain.¦ Pour des données dont la fuite serait grave, WireGuard reste le choix raisonnable.|Le serveur voit les métadonnées¦ : qui parle à qui, quand, et combien.|Ce n’est pas un VPN pour naviguer.¦ Il relie tes appareils entre eux ; ta navigation sur Internet ne passe pas par lui.|La clé du verrou n’a pas de secours¦ pour l’instant : perdre le téléphone de l’admin gèle les signatures. C’est un prochain gros sujet."
