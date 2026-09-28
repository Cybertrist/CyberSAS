#!/bin/bash
# Les planches de captures : le téléphone, puis le Fold déplié.
#
# Les sources sont les captures des tests de l'appli, faites sur le réseau
# d'exemple : aucune vraie adresse, aucune vraie clé. Pour les refaire :
#   cd mobile && flutter test --update-goldens test/captures_test.dart
#
#   bash docs/tools/captures.sh
source "$(dirname "${BASH_SOURCE[0]}")/rendu.sh"
mkdir -p "$DOCS/schemas"
SRC="file:///$RACINE/mobile/test/captures"

# ecran <classe> <fichier> <titre> <légende>
ecran () { printf '<figure class="%s"><div class="cadre"><img src="%s/%s.png"></div><figcaption><b>%s</b>%s</figcaption></figure>' "$1" "$SRC" "$2" "$3" "$4"; }

style () { cat <<'HTML'
<style>
.w{padding:26px 48px;display:grid;gap:26px 22px}
figure{display:flex;flex-direction:column;gap:12px}
.cadre{border-radius:26px;padding:7px;background:linear-gradient(160deg,#2A3340,#10151C 45%,#1B232E);
  box-shadow:0 20px 40px #0009,0 0 0 1px #2F3A47,inset 0 0 0 1px #FFFFFF10}
.cadre img{display:block;width:100%;border-radius:20px}
.deplie .cadre{border-radius:22px}.deplie .cadre img{border-radius:16px}
figcaption{font-family:'Space Grotesk',sans-serif;font-size:13px;line-height:1.45;color:var(--texte);padding:0 6px}
figcaption b{display:block;font-size:14.5px;color:var(--titre);margin-bottom:2px}
</style></head><body>
HTML
}

{ entete 1280; style; cat <<HTML
<div class="w" style="grid-template-columns:repeat(4,1fr)">
$(ecran telephone telephone-connexion 'Rejoindre' 'Coller le lien d’invitation de l’admin, ou continuer avec Google.')
$(ecran telephone telephone-accueil 'L’accueil' 'Le tunnel du logo, animé, et l’interrupteur. Ce qui protège l’appareil, en dessous.')
$(ecran telephone telephone-accueil-eteint 'Tunnel coupé' 'Le logo s’éteint jusqu’à n’être plus qu’un fantôme bleu nuit.')
$(ecran telephone telephone-appareils 'Les appareils' 'La carte du réseau, qui est en ligne, les demandes à signer.')
$(ecran telephone telephone-ajout 'Inviter' 'Un lien à usage unique, à partager au nouvel appareil.')
$(ecran telephone telephone-detail 'Un appareil' 'Son nom sur le réseau, ses ports tirés de la politique signée, son certificat, l’empreinte de sa clé.')
$(ecran telephone telephone-demandes 'Les demandes' 'L’empreinte, puis ce qui sera signé : adresse, groupe, 90 jours. Le doigt, enfin.')
$(ecran telephone telephone-reglages 'Les réglages' 'Renommer l’appareil, verrouiller l’appli, masquer l’écran. Et l’équipe, pour l’admin.')
$(ecran telephone telephone-invitations 'Les invitations en cours' 'Celles qui n’ont pas encore servi, pour qui et jusqu’à quand. Annulée, une invitation ne fait plus entrer personne.')
$(ecran telephone telephone-equipe 'L’équipe' 'Les comptes Google qui peuvent rejoindre le réseau, chacun dans son groupe.')
$(ecran telephone telephone-equipe-ajout 'Ajouter un membre' 'Son adresse Google et son groupe, puis le doigt. Ses appareils devront encore être signés.')
$(ecran telephone telephone-equipe-membre 'Un membre' 'Changer son groupe ou le retirer de l’équipe, ce qui coupe tous ses appareils.')
</div>
HTML
pied; } > "$D/html/captures-telephone.html"
rendre captures-telephone.html "$DOCS/schemas/captures-telephone.png"

{ entete 1280; style; cat <<HTML
<div class="w" style="grid-template-columns:repeat(2,1fr)">
$(ecran deplie fold-deplie-paysage-accueil 'L’accueil, déplié' 'Le tunnel à gauche, l’appareil et le réseau à droite.')
$(ecran deplie fold-deplie-paysage-appareils 'La carte du réseau' 'En grand à gauche, avec les demandes ; les machines à droite.')
$(ecran deplie fold-deplie-paysage-appareils-detail 'Toucher un appareil…' '…fait glisser la liste à gauche, et son détail s’ouvre à droite.')
$(ecran deplie fold-deplie-paysage-reglages 'Les réglages' 'En deux colonnes, sans défiler.')
</div>
HTML
pied; } > "$D/html/captures-deplie.html"
rendre captures-deplie.html "$DOCS/schemas/captures-deplie.png"
