<div align="center">

<img src="docs/banniere.png" alt="CyberSAS : mon propre réseau privé, écrit de bout en bout. Un serveur qui relaie sans lire, une clé qui ne quitte jamais le téléphone. Go, Noise IK, ChaCha20-Poly1305, Ed25519, nftables, Flutter." width="100%">

</div>

<br>

Mon propre VPN, de bout en bout : le protocole du tunnel, le serveur, les applis. Il n'ouvre aucun port à la maison, son serveur relaie des paquets qu'il ne peut pas lire, et c'est le téléphone de l'admin, avec son doigt, qui décide qui entre.

> **En cours.** Le serveur, le tunnel et le client Linux tournent dans le labo et passent leurs vérifications. L'appli Android ouvre un vrai tunnel sur le Fold, on la rejoint avec son compte Google, et l'admin gère le réseau et l'équipe depuis son téléphone. Voir [la feuille de route](#la-feuille-de-route).

<img src="docs/schemas/tunnel.svg" alt="Le tunnel du logo de CyberSAS, animé comme dans l'appli : il s'allume couche par couche, les paquets entrent, puis il s'éteint. À gauche : Noise IK, puis ChaCha20-Poly1305, clés renouvelées toutes les deux minutes. À droite, l'état : coupé, connexion, connecté, coupure." width="100%">

<img src="docs/sections/s01.png" alt="01 Ce que c'est" width="100%">

Héberger un service chez soi, c'est d'habitude ouvrir des ports sur sa box et laisser son adresse IP à la vue de tous. Partager un serveur avec quelques personnes, c'est souvent un mot de passe commun qui circule par message.

Tailscale règle ça très bien, mais c'est leur serveur, leur appli et leur protocole. CyberSAS est l'exercice inverse : tout écrire soi-même, sauf les primitives cryptographiques, que personne de sérieux n'écrit.

<img src="docs/schemas/promesses.png" alt="Six promesses. Un seul point d'entrée : un petit serveur public, aucun port ouvert à la maison. Chiffré de bout en bout : le serveur relaie des paquets qu'il ne peut pas ouvrir. Un serveur qu'on n'a pas à croire : chaque appareil porte un certificat signé par la clé du verrou. La clé dans la puce du téléphone : chiffrée par la puce sécurisée du téléphone de l'admin, une empreinte l'ouvre pour une seule signature. Un compte Google, aucun mot de passe. Tout écrit à la main, sauf les primitives cryptographiques." width="100%">

Ce n'est pas un VPN pour naviguer caché : il relie tes appareils entre eux. Ta navigation sur Internet ne passe pas par lui.

<img src="docs/sections/s02.png" alt="02 L'appli" width="100%">

Sur le téléphone, une barre flottante en bas, et le tunnel du logo qui s'allume et s'éteint avec l'interrupteur.

<img src="docs/schemas/captures-telephone.png" alt="Douze écrans sur téléphone. Rejoindre : coller le lien d'invitation de l'admin, ou continuer avec Google. L'accueil : le tunnel animé et l'interrupteur. Tunnel coupé : le logo n'est plus qu'un fantôme bleu nuit. Les appareils : la carte du réseau et les demandes à signer. Inviter : un lien à usage unique à partager. Un appareil : son nom sur le réseau, ses ports tirés de la politique signée, son certificat, l'empreinte de sa clé, Ouvrir dans le navigateur et Révoquer. Les demandes : l'empreinte, puis ce qui sera signé, adresse, groupe et 90 jours, puis le doigt. Les réglages : renommer, verrouiller l'appli, masquer l'écran, la clé du verrou, et l'équipe pour l'admin. Les invitations en cours : pour qui, jusqu'à quand, et Annuler. L'équipe : les comptes Google et leur groupe. Ajouter un membre : son adresse et son groupe, puis le doigt. Un membre : changer son groupe, ou le retirer de l'équipe." width="100%">

Sur le Fold déplié, un rail à gauche. Toucher un appareil fait glisser la liste à gauche et ouvre son détail à droite.

<img src="docs/schemas/captures-deplie.png" alt="Quatre écrans sur le Fold déplié. L'accueil : le tunnel à gauche, l'appareil et le réseau à droite. La carte du réseau en grand à gauche avec les demandes, les machines à droite. Toucher un appareil fait glisser la liste à gauche et ouvre son détail à droite. Les réglages en deux colonnes." width="100%">

L'appli vit dans le dossier `mobile/`. Elle embarque le moteur Go du tunnel (`pont/`) et passe par le service VPN d'Android. Les captures ci-dessus viennent de son mode démo, sur un réseau d'exemple.

Les ports ouverts d'un appareil se déduisent de la politique signée, celle que le tunnel applique. Quand le 80 ou le 443 en fait partie, un bouton ouvre son service dans le navigateur. Les cibles à toucher font au moins 48 dp, et les écrans tiennent jusqu'à une police une fois et demie plus grande.

<img src="docs/sections/s03.png" alt="03 Comment ça marche" width="100%">

<img src="docs/schemas/relais.svg" alt="Un paquet part du téléphone fold8-tristan, en clair : GET / HTTP/1.1. Il voyage dans deux enveloppes. Le serveur sasd ouvre l'extérieure, la sienne, et n'y lit que le numéro du destinataire ; l'intérieure, la session de bout en bout, reste fermée : il n'en voit que des octets chiffrés. Il la remet dans une nouvelle enveloppe pour la maison, qui la déchiffre." width="100%">

1. L'admin partage le lien du réseau, qui porte l'adresse du serveur et la clé publique du verrou. Sur le nouvel appareil, on l'ouvre puis on se connecte avec son compte Google : seuls ceux de l'équipe passent. Sans compte Google, l'admin envoie plutôt une invitation, un lien à usage unique. Ces liens sont des adresses https du serveur (`https://vpn.<domaine>/rejoindre#…`) : WhatsApp, Gmail ou un SMS les rendent cliquables, et Android les ouvre directement dans l'appli. Tout ce qu'ils portent est après le `#`, que le navigateur n'envoie jamais au serveur.
2. Le nouvel appareil génère sa paire de clés. La clé privée ne le quitte jamais.
3. Il s'inscrit auprès de sasd avec la preuve qu'il détient la clé privée. sasd lui attribue une adresse, `10.77.0.x`.
4. L'admin voit la demande sur son téléphone, compare l'empreinte avec celle du nouvel appareil, et signe son certificat avec son doigt.
5. L'appareil ouvre une session avec le serveur, puis une session de bout en bout avec chaque appareil qu'il a le droit de joindre, à la demande. À partir de là, l'API ne sert plus à rien : le tunnel ne se fie qu'aux clés.

Tout le protocole, octet par octet et en schémas, est dans [le document du protocole](docs/protocole.md).

<img src="docs/sections/s04.png" alt="04 Rejoindre le réseau" width="100%">

<img src="docs/schemas/inscription.svg" alt="Un nouvel appareil rejoint le réseau, en dix messages entre trois acteurs : le nouvel appareil, le serveur sasd et le téléphone de l'admin. 01 l'admin crée une invitation pour alice@gmail.com, valable une heure ; 02 le serveur rend une clé d'inscription à usage unique ; 03 l'admin envoie le lien https://vpn…/rejoindre avec le serveur, la clé et la clé du verrou ; 04 l'appareil crée sa paire de clés, la privée ne sort pas ; 05 il s'inscrit avec une preuve de possession ; 06 le serveur présente la demande à l'admin avec l'empreinte Qm7X-tR2k-9vLp ; 07 l'admin compare l'empreinte et signe au doigt ; 08 le certificat signé part au serveur ; 09 le serveur donne le réseau et les certificats à l'appareil ; 10 l'appareil est dans le réseau, en 10.77.0.3, pour 90 jours." width="100%">

L'empreinte, ce sont les douze premiers caractères de la clé publique du nouvel appareil. Elle s'affiche des deux côtés : si elles sont identiques, personne ne s'est glissé entre les deux. Sous l'empreinte, l'admin lit ce qu'il signe : l'adresse, le groupe, la durée. Si le serveur change un seul de ces champs avant la signature, rien n'est signé.

Une machine sans écran, comme le serveur de la maison, s'inscrit avec le client Linux et une clé à usage unique donnée par l'admin.

<img src="docs/schemas/verrou.svg" alt="Un serveur piraté glisse un intrus dans le réseau : le téléphone fold8-tristan vérifie le certificat, ne trouve pas de signature du verrou, et le refuse. L'ordinateur laptop-lea, signé par le verrou, est accepté." width="100%">

Le verrou, sa chaîne de confiance et le coffre du téléphone ont [leur document](docs/verrou.md).

<img src="docs/sections/s05.png" alt="05 Qui peut aller où" width="100%">

La politique vit dans `politique/politique.json`, signée par l'admin. Tout est fermé par défaut.

<img src="docs/schemas/regles.png" alt="Six règles. Les admins atteignent tout le réseau. Chacun atteint ses propres appareils, jamais ceux des autres. L'équipe atteint les services web de la maison, pas son SSH. Seul le serveur atteint le port publié de la maison. La maison ne peut ouvrir de connexion vers personne. La politique est signée par la clé du verrou : le serveur ne peut pas la changer sans que ça se voie." width="100%">

Chaque appareil applique lui-même ce qui le concerne : le serveur ne voit pas le trafic entre appareils, et ne peut pas changer les règles sans que ça se voie.

<img src="docs/sections/s06.png" alt="06 L'équipe" width="100%">

L'équipe, c'est la liste des comptes Google qui ont le droit de rejoindre le réseau, chacun dans un groupe : `admins` ou `equipe`. Le reste se fait depuis le téléphone de l'admin.

<img src="docs/schemas/equipe.png" alt="Six gestes. Inviter : l'admin choisit un membre de l'équipe et une durée, 10 minutes, une heure ou un jour, et l'appli partage un lien https://vpn…/rejoindre à usage unique, qui ouvre l'appli d'un appui et porte la clé du verrou. Signer : la demande arrive avec l'empreinte, l'adresse et le groupe ; l'admin compare puis signe avec son doigt ; si le serveur change un champ entre-temps, rien n'est signé. Renommer : chacun ses appareils, l'admin tous ; le nom affiché ne fait pas partie du certificat. Retirer : un appareil dont on ne veut plus est coupé tout de suite, mais pourrait se réinscrire. Révoquer : pour un appareil volé, la liste de révocation est signée sur le téléphone de l'admin, et aucun appareil ne revient à une liste plus ancienne. L'équipe elle-même : la liste des comptes vit sur le serveur, dans equipe.txt, et l'admin la règle depuis l'appli, écran Équipe, ou en ligne de commande." width="100%">

<img src="docs/schemas/revocation.svg" alt="Révoquer un appareil depuis l'appli. Sur son téléphone fold8-tristan, l'admin touche Révoquer portable-test ; après son empreinte, le téléphone signe la liste de révocation v4 avec la clé du verrou. Le serveur sasd vérifie la signature et que v4 est plus récente que v3, la garde dans revocations.json et la transmet. La maison et laptop-lea retiennent la v4 ; portable-test est coupé. Les appareils n'acceptent jamais une liste plus ancienne." width="100%">

L'équipe elle-même se règle depuis l'appli de l'admin, dans Réglages, Équipe : ajouter un compte Google, changer son groupe, le retirer, chaque fois avec le doigt. Le serveur refuse qu'un admin change son propre accès, ou retire le dernier admin. La ligne de commande fait la même chose, sur le même fichier :

```bash
bash scripts/sas.sh membre alice@gmail.com equipe
bash scripts/sas.sh retirer alice@gmail.com
```

Retirer quelqu'un de l'équipe coupe tous ses appareils, et ses services web, en cinq secondes au plus.

<img src="docs/sections/s07.png" alt="07 Essayer le labo" width="100%">

Il faut Docker et Bash (Git Bash suffit sous Windows).

```bash
cp .env.exemple .env            # y mettre son adresse Google dans ADMIN_EMAIL
bash scripts/sas.sh init        # secrets, autorité du labo, verrou, liste d'accès
bash scripts/sas.sh demarrer    # construit, signe la politique, lance le serveur
bash scripts/sas.sh labo        # une fausse maison, un poste d'essai, signés
bash scripts/sas.sh essai       # dix-huit vérifications, de bout en bout
```

L'essai vérifie entre autres que :

- une inscription sans preuve de possession est refusée ;
- le poste atteint le service de la maison, mais ni son port publié ni son port d'admin ;
- la maison ne peut pas se retourner vers le poste ;
- une capture réseau sur le serveur voit en clair ce que le serveur fait lui-même, mais jamais ce que le poste envoie à la maison ;
- un appareil que le serveur inscrit sans certificat est refusé par le poste ;
- une personne retirée de l'équipe perd ses appareils en quelques secondes ;
- après un redémarrage du serveur, les appareils reviennent seuls.

Pour inviter un téléphone : `bash scripts/sas.sh invitation <adresse Google>` donne le lien à ouvrir sur lui, puis `bash scripts/sas.sh signer <sa clé publique>` signe sa demande depuis l'ordinateur, si on ne le fait pas depuis l'appli. Une invitation qui n'a pas encore servi se voit avec `bash scripts/sas.sh invitations` et s'annule avec `bash scripts/sas.sh annuler <id>`, ou depuis l'écran d'ajout de l'appli.

Les tests du code se lancent à part : `go test -race ./...` (102 tests, dont les vecteurs officiels de Noise, et 8 cibles de fuzzing).

Pour brancher la connexion Google des pages web, créer dans la [console Google Cloud](https://console.cloud.google.com/apis/credentials) un ID client OAuth de type **Application Web**, avec comme URI de redirection `https://auth.<domaine>/oauth2/callback`, puis :

```bash
bash scripts/sas.sh google <id>.apps.googleusercontent.com   # le secret est demandé
```

<img src="docs/sections/s08.png" alt="08 Ce qu'il y a dedans" width="100%">

<img src="docs/schemas/architecture.svg" alt="L'architecture. Au milieu, le VPS, seul point public : sasd (tunnel, relais, API, pare-feu, DNS), Nginx sur le port 443 pour vpn., auth. et maison., et oauth2-proxy pour la connexion Google des pages. À gauche, le téléphone fold8-tristan avec l'appli Android et laptop-lea avec le client sas ouvrent leur tunnel vers sasd en UDP 51820. À droite, la maison sort elle aussi vers le VPS : aucun port ouvert chez elle. Un paquet du téléphone vers la maison est relayé par sasd, chiffré de bout en bout. sasd et oauth2-proxy parlent à Google." width="100%">

<img src="docs/schemas/dedans.png" alt="Six briques. Le protocole, internal/noise et internal/tunnel : Noise IK identique au vecteur officiel, puis ChaCha20-Poly1305 renouvelé toutes les deux minutes. sasd, le serveur : tunnel, relais chiffré, API d'inscription, pare-feu nftables, DNS du réseau. sas, le client Linux, pour les machines sans écran et l'outil de l'admin pour le verrou. Le verrou, internal/verrou : certificats avec expiration, politique signée, révocations signées. L'appli Android, mobile/, en Flutter, pour le téléphone et le Fold, avec le moteur Go embarqué, le service VPN d'Android et l'admin au doigt. Nginx et oauth2-proxy, pour publier un service de la maison derrière la connexion Google." width="100%">

<img src="docs/sections/s09.png" alt="09 La sécurité, et ses limites" width="100%">

CyberSAS a été relu trois fois. D'abord par trois relecteurs indépendants et une revue de sécurité : 46 constats, dont 3 hauts. Puis aux outils du métier (govulncheck, staticcheck, gosec, Semgrep, Trivy, Hadolint, ShellCheck, Gixy, fuzzing différentiel contre flynn/noise) : 10 de plus. Enfin sur l'appli et l'admin à distance : 23, dont un haut, une révocation depuis l'appli qui reprenait sans la vérifier la liste du serveur. Tout est traité, chaque fois avec son test quand c'est possible, sauf un constat accepté et expliqué.

<img src="docs/schemas/limites.png" alt="Ce qui est vrai : le serveur ne lit pas ce que deux appareils s'envoient ; un serveur piraté ne fait entrer personne ; retirer quelqu'un coupe ses appareils en cinq secondes ; trois audits, 79 constats, 78 corrigés et un accepté et expliqué. Ce qui ne l'est pas : pas d'audit humain, WireGuard reste le choix raisonnable pour des données sensibles ; le serveur voit les métadonnées ; ce n'est pas un VPN pour naviguer ; la sauvegarde du verrou ne vaut que sa phrase." width="100%">

Pour aller plus loin, quatre documents, dessinés comme ce README :

<div align="center">

<a href="docs/protocole.md"><img src="docs/nav/protocole.png" alt="Le protocole : les deux couches, Noise IK, les messages à l’échelle, le filtre." width="49%"></a>
<a href="docs/verrou.md"><img src="docs/nav/verrou.png" alt="Le verrou : la chaîne de confiance, le coffre du téléphone, signer et révoquer." width="49%"></a>
<a href="docs/menaces.md"><img src="docs/nav/menaces.png" alt="Le modèle de menace : ce qui est exposé, le serveur piraté, le téléphone volé." width="49%"></a>
<a href="docs/audit.md"><img src="docs/nav/audit.png" alt="L’audit de sécurité : trois relectures, 79 constats." width="49%"></a>

</div>

<a name="la-feuille-de-route"></a>
<img src="docs/sections/s10.png" alt="10 La feuille de route" width="100%">

<img src="docs/schemas/feuille.png" alt="La feuille de route. Fait : le tunnel, le serveur et le verrou, 102 tests Go, 8 cibles de fuzzing et 18 vérifications de bout en bout ; trois audits ; l'appli Android avec le vrai tunnel ; l'admin depuis le téléphone, signer, refuser, inviter, renommer, retirer, révoquer ; la connexion Google dans l'appli ; l'équipe depuis l'appli, ajouter, changer de groupe, retirer, avec toujours un admin, et les invitations en cours qu'on annule ; une appli juste et lisible, les ports tirés de la politique signée, des cibles de 48 dp et les grandes polices ; un secours pour la clé du verrou, chiffré par une phrase. À venir : le serveur en ligne, d'abord à la maison puis sur un VPS. À discuter : l'appli Windows ; un site vitrine et une console web qui ne peut pas signer." width="100%">

Les figures de ce README sont dessinées par les scripts de `docs/tools` : aucune ne sort d'un logiciel de dessin, et chaque animation est vérifiée image par image avant d'être publiée.

<br>

<div align="center">
<sub>Tristan Joncour · élève ingénieur en cyberdéfense à l'ENSIBS</sub>
</div>
