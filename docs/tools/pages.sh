#!/bin/bash
# Les figures fixes des documents de docs/ : le protocole, le verrou, le
# modèle de menace et l'audit. Comme figures.sh pour le README : c'est le
# seul fichier à ouvrir pour changer un texte. Les schémas animés de ces
# documents sont dans docs.js.
#
#   bash docs/tools/pages.sh
source "$(dirname "${BASH_SOURCE[0]}")/rendu.sh"
S="$DOCS/schemas"

# =========================================================== le protocole
banniere_doc "$DOCS/banniere-protocole.png" "CYBERSAS · DOCUMENTATION" "Le protocole" \
  "Tout ce qui se passe entre un appareil, le serveur et les autres appareils, et pourquoi le serveur n'y lit rien." \
  "Noise IK" "X25519" "ChaCha20-Poly1305" "BLAKE2s" "Ed25519"
bandeaux "$DOCS/sections/protocole" "Ce qui est à nous" "Deux couches" "La poignée de main" "Les messages" \
  "Les sessions" "Ce que le tunnel refuse" "L'inondation" "Le filtre" "L'inscription" "Le verrou" "Itinérance et reprise" "Ce qui manque encore"

grille "$S/protocole/primitives.png" 4 \
  "sync_alt|X25519|Les échanges de clés : quatre par poignée de main, deux de chaque côté." \
  "lock|ChaCha20-Poly1305|Le chiffrement authentifié des sessions, et XChaCha20-Poly1305 pour les cookies." \
  "tag|BLAKE2s|Le hachage, les MAC (mac1, mac2, la preuve de possession) et la dérivation des clés." \
  "verified|Ed25519|Les signatures du verrou : certificats, politique, révocations."

octets "$S/protocole/messages.png" \
  "Initiation|type 1 · 148 octets|type:1:g,réservé:3:g,émetteur:4:b,Noise message 1:108:c,mac1:16:v,mac2:16:v" \
  "Réponse|type 2 · 92 octets|type:1:g,réservé:3:g,émetteur:4:b,destinataire:4:b,Noise message 2:48:c,mac1:16:v,mac2:16:v" \
  "Cookie|type 4 · 64 octets|type:1:g,réservé:3:g,destinataire:4:b,nonce:24:g,cookie chiffré:16:c,tag:16:c" \
  "Données|type 3 · variable|type:1:g,réservé:3:g,destinataire:4:b,compteur:8:b,paquet chiffré:n:c,tag:16:c" \
  "Trame de relais|dans les données|0x00:1:g,réservé:1:g,longueur:2:b,numéro d'appareil:4:b,message relayé:n:c"

grille "$S/protocole/refus.png" 4 \
  "replay|Un paquet rejoué|Fenêtre des 2 048 derniers compteurs. Marqué seulement après le tag : un faux ne fait pas avancer la fenêtre." \
  "history|Une initiation rejouée|Son horodatage TAI64N doit être plus récent que la dernière acceptée de ce pair." \
  "key_off|Une clé inconnue|Le répondeur déchiffre la clé statique de l'initiateur : absente de ses pairs, il ne répond pas." \
  "alt_route|Le mauvais chemin|Relayé, il doit venir d'un pair joint par relais, sous le numéro annoncé ; direct, d'un pair direct." \
  "badge|Une adresse usurpée|Un paquet ne sort du tunnel que si sa source est l'adresse du pair qui l'a chiffré." \
  "call_split|Pas pour nous|Sa destination doit être notre adresse : pas de rebond vers le réseau local d'un appareil." \
  "error|Une réponse forgée|Lue sur une copie de l'état Noise : une fausse n'empêche pas la vraie de passer." \
  "filter_alt|Ce que le filtre refuse|Aucune règle, aucun flux ouvert par nous : jeté en silence. Voir plus bas."

octets "$S/protocole/signes.png" \
  "Certificat|un par appareil|contexte:23:g,clé:32:c,adresse:4:b,expiration:8:b,étiquette:n:v,propriétaire:n:v,groupe:n:v" \
  "Politique|versionnée|contexte:22:g,version:8:b,le fichier politique.json:n:c" \
  "Révocations|versionnées|contexte:24:g,version:8:b,clé révoquée:32:c,clé révoquée:32:c,…:n:g"

grille "$S/protocole/itinerance.png" 4 \
  "swap_horiz|Du Wi-Fi à la 4G|Le serveur suit l'appareil. Seul un paquet authentifié, et jamais vu, peut déplacer son adresse." \
  "restart_alt|15 secondes sans rien|Un initiateur qui envoie sans rien recevoir rouvre une session. Après un redémarrage du serveur, tout revient en moins de 25 secondes." \
  "favorite|10 secondes de silence|Un pair qui reçoit sans rien renvoyer envoie un paquet vide : l'autre sait que la liaison vit." \
  "refresh|Toutes les 10 secondes|Le client relit l'état du réseau, et l'adresse du serveur avec : un changement d'IP ne le perd pas."

grille "$S/protocole/manque.png" 3 \
  "looks_4|IPv4 seulement|À l'intérieur du tunnel. Le transport, lui, passe aussi en IPv6." \
  "hub|Pas de liaison directe|Tout passe par le relais du serveur, chiffré de bout en bout. Plus simple derrière n'importe quelle box, au prix d'un détour." \
  "person_search|Aucun audit humain|Les tests prouvent la conformité à Noise et le refus des attaques connues. Cela ne remplace pas le regard d'un cryptographe."

# ============================================================== le verrou
banniere_doc "$DOCS/banniere-verrou.png" "CYBERSAS · DOCUMENTATION" "Le verrou" \
  "La clé qui signe qui entre, où elle vit, et comment s'en servir pas à pas." \
  "Ed25519" "Keystore" "StrongBox" "empreinte"
bandeaux "$DOCS/sections/verrou" "La règle d'or" "La chaîne de confiance" "Le coffre du téléphone" "Créer le verrou" \
  "Signer un appareil" "Signer la politique" "Bannir un appareil" "Si la clé est perdue ou volée" "La sauvegarde de secours"

grille "$S/verrou/deux-places.png" 2 \
  "smartphone|Sur le téléphone de l'admin|La clé est rangée chiffrée par une clé AES du Keystore, dans la puce StrongBox. Une empreinte l'ouvre pour une seule signature. C'est là qu'on signe au quotidien : demandes, révocations." \
  "computer|Sur l'ordinateur de l'admin|Le fichier créé par <code>sas verrou creer</code>, idéalement chiffré et sauvegardé hors ligne. C'est lui qui signe la politique, et qui sert de copie de secours."

grille "$S/verrou/perte.png" 2 \
  "search_off|Perdue|Il faut en créer une nouvelle, et réinscrire tous les appareils avec <code>--oublier</code> : ils refusent tout changement de verrou. Le téléphone seul, sans copie, gèle toute signature." \
  "gpp_bad|Volée|Même chose, et vite : le voleur peut signer ce qu'il veut tant que les appareils font confiance à l'ancienne. Sur le téléphone, il lui faudrait d'abord le doigt de l'admin."

octets "$S/verrou/secours.png" \
  "Sauvegarde|127 octets en base64|version:1:g,mémoire:4:b,passes:1:b,voies:1:b,sel:16:g,nonce:24:g,clé publique:32:v,graine chiffrée:32:c,tag:16:c"

grille "$S/verrou/secours-limites.png" 2 \
  "verified_user|Ce qu'elle protège|Le téléphone perdu, cassé ou remis à zéro : la sauvegarde et sa phrase redonnent la clé, sans réinscrire personne. Modifiée d'un seul octet, elle ne s'ouvre plus." \
  "warning|Ce qu'elle ne protège pas|Une phrase faible : qui vole le texte essaie des phrases chez lui, sans limite, et Argon2id ne fait que le ralentir. Une phrase oubliée : rien ne la retrouve. Le texte et sa phrase ensemble valent la clé."

# ========================================================== les menaces
banniere_doc "$DOCS/banniere-menaces.png" "CYBERSAS · DOCUMENTATION" "Le modèle de menace" \
  "Ce que CyberSAS protège, ce qu'un attaquant peut encore faire, et ce qu'il ne promet pas."
bandeaux "$DOCS/sections/menaces" "Ce qu'on protège" "Ce qui est exposé" "Contre qui" "Si le serveur tombe" \
  "Si le téléphone de l'admin est volé" "Ce qu'il ne promet pas"

grille "$S/menaces/protege.png" 4 \
  "lock|Le contenu|Personne sur le chemin ne le lit, pas même le serveur." \
  "home|Les services de la maison|Joignables par les appareils autorisés, sur les ports autorisés, et rien d'autre." \
  "visibility_off|L'adresse de la maison|Elle n'apparaît nulle part : la maison sort vers le serveur." \
  "group|L'accès de l'équipe|Un mot de passe volé, une clé publique connue ou un serveur piraté ne suffisent pas à entrer."

grille "$S/menaces/adversaires.png" 3 \
  "wifi|Qui écoute le réseau|Wi-Fi d'un café, opérateur. Il voit des paquets UDP chiffrés, leur taille arrondie à 16 octets, les IP publiques. Ni le contenu, ni l'identité : la clé statique voyage chiffrée." \
  "dns|L'hébergeur du VPS|Il voit qui parle à qui, quand, et combien. Pas ce qui se dit : le serveur relaie avec une clé qu'il n'a pas. Le labo le vérifie par une capture sur le serveur." \
  "replay|Qui rejoue ou modifie|Un paquet modifié échoue à son tag, un rejoué porte un compteur déjà vu, une initiation rejouée un horodatage trop vieux. Tout est jeté en silence." \
  "waves|Qui inonde|Un scan à l'aveugle est jeté au premier hachage. Qui vise vraiment le serveur, sous charge : un cookie à prouver, puis dix poignées de main par seconde." \
  "door_front|Qui veut entrer|Il lui faut un jeton Google de l'équipe ou une invitation à usage unique, prouver qu'il tient sa clé privée, puis la signature de l'admin." \
  "person_alert|Un membre qui va trop loin|Il n'atteint que ce que la politique autorise, ne prend ni l'adresse ni le nom d'un autre, ne rebondit pas vers un réseau local. Le retirer le coupe en cinq secondes."

colonnes "$S/menaces/serveur.png" \
  "verified_user|Ce qu'il ne peut pas" \
  "Lire¦ le trafic entre appareils.|S'intercaler¦ : il lui faudrait un certificat signé par le verrou, dont la clé n'a jamais touché le serveur.|Ouvrir un port¦ : chaque appareil calcule ses règles à partir de la politique signée, et refuse une version plus ancienne.|Faire revenir un banni¦ : la liste de révocation est signée et ne recule jamais. Il ne peut pas non plus faire signer à l'admin une liste de son choix.|Faire signer autre chose¦ que ce que l'admin a vu : la moindre différence entre l'affichage et la signature, et rien n'est signé." \
  "gpp_maybe|Ce qu'il peut encore" \
  "Couper¦ le réseau : ne plus relayer, ou ne plus transmettre les mises à jour signées. D'où l'expiration des certificats, à 90 jours.|Inscrire ses appareils¦, que les autres refusent sans certificat.|Lire ce qui s'adresse à lui¦ : le DNS du réseau, et les pages publiées par Nginx, dont le TLS se termine sur le VPS.|Voler ses propres secrets¦ : la clé privée du serveur et le secret du client Google, à régénérer ensuite."

colonnes "$S/menaces/telephone.png" \
  "lock|Ce qui le protège" \
  "Le verrou de l'appli¦ : une empreinte à l'ouverture, et à nouveau après 30 secondes dehors, compté sur l'horloge du système, qu'on ne recule pas.|La clé du verrou¦ dort chiffrée par une clé de la puce. Elle ne s'ouvre qu'avec une empreinte, pour une seule opération. Une empreinte ajoutée au téléphone l'invalide.|Pas de copie¦ : la clé de l'appareil et le coffre sont exclus des sauvegardes et des transferts d'Android.|Un aperçu vide¦ dans les applis récentes tant que le verrou de l'appli est actif." \
  "report|Ce qu'il faut faire" \
  "Retirer et révoquer¦ le téléphone depuis un autre appareil d'admin, ou en ligne de commande depuis l'ordinateur.|Changer de verrou¦ si le téléphone était déverrouillé et l'appli ouverte au moment du vol.|Garder une copie¦ de la clé du verrou hors ligne : sans elle, perdre le téléphone gèle toute signature."

grille "$S/menaces/promet-pas.png" 2 \
  "person_search|Pas d'audit humain|Le protocole reprend WireGuard, suit les vecteurs de Noise, a résisté à des millions de messages forgés et à trois relectures. Mais peu de gens l'ont lu. Pour des données dont la fuite serait grave, WireGuard reste le choix raisonnable." \
  "monitoring|Les métadonnées|Le serveur voit qui parle à qui, quand, et combien." \
  "account_circle|Google, tiers de confiance|Pour les inscriptions seulement. En panne, personne n'entre, mais ceux qui sont dedans continuent. Un compte volé inscrit un appareil, inutile tant que l'admin ne l'a pas signé." \
  "handshake|Le premier contact|Un appareil à qui l'on ne donne pas la clé du verrou retient la première annoncée. Le lien d'invitation la donne d'avance ; l'empreinte permet de vérifier."

# ============================================================== l'audit
banniere_doc "$DOCS/banniere-audit.png" "CYBERSAS · DOCUMENTATION" "L’audit de sécurité" \
  "Trois relectures, 79 constats : qui a relu quoi, ce qui a été trouvé, et ce qui en a été fait." \
  "3 relecteurs" "8 outils" "fuzzing" "tests de non-régression"
bandeaux "$DOCS/sections/audit" "Ce que cet audit est" "Relancer les preuves" "Protocole et cryptographie" "Serveur" \
  "Clients et déploiement" "Revue des corrections" "Trouvé en corrigeant" "Deuxième audit : les outils" \
  "Troisième audit : l'appli" "Ce qui reste"

grille "$S/audit/trois.png" 3 \
  "groups|Premier audit · 24/09|Trois relecteurs indépendants (protocole, serveur, clients) puis une revue de sécurité des corrections. <b>46 constats, dont 3 hauts.</b>" \
  "build|Deuxième audit · 24/09|Les outils du métier : govulncheck, staticcheck, gosec, Semgrep, Trivy, Hadolint, ShellCheck, Gixy, et du fuzzing différentiel. <b>10 constats.</b>" \
  "smartphone|Troisième audit · 26/09|Ce qui a changé depuis : les routes d'admin, l'Android natif, la frontière entre l'appli et le moteur. <b>23 constats, dont 1 haut.</b>"

# ================================================ les cartes de navigation
# Deux par ligne, sous le README et au bas de chaque document.
N="$DOCS/nav"
carte_lien "$N/readme.png" "home" "Le README" "CyberSAS en un coup d'œil : ce que c'est, l'appli, le labo."
carte_lien "$N/protocole.png" "sync_alt" "Le protocole" "Les deux couches, Noise IK, les messages à l'échelle, le filtre."
carte_lien "$N/verrou.png" "key" "Le verrou" "La chaîne de confiance, le coffre du téléphone, signer et révoquer."
carte_lien "$N/menaces.png" "shield" "Le modèle de menace" "Ce qui est exposé, le serveur piraté, le téléphone volé."
carte_lien "$N/audit.png" "fact_check" "L'audit de sécurité" "Trois relectures, 79 constats, et ce qui a été fait de chacun."

grille "$S/audit/outils.png" 3 \
  "bug_report|govulncheck|Les vulnérabilités connues des dépendances, seulement celles que le code appelle vraiment." \
  "fact_check|staticcheck|L'analyse statique de référence pour Go." \
  "warning|gosec|Les motifs dangereux en Go : débordements, erreurs ignorées, chemins, commandes." \
  "rule|Semgrep|91 règles Go, Dockerfile et Nginx." \
  "inventory_2|Trivy|Les vulnérabilités de l'image Docker, sa configuration, et les secrets oubliés." \
  "deployed_code|Hadolint|Les bonnes pratiques du Dockerfile." \
  "terminal|ShellCheck|Les pièges de Bash dans <code>scripts/sas.sh</code>." \
  "dns|Gixy|Les erreurs de configuration de Nginx, sur la configuration réellement chargée." \
  "shuffle|Fuzzing long|Huit cibles, trois minutes chacune, dont quatre nouvelles, différentielles contre flynn/noise."
