<div align="center">

<img src="banniere-protocole.png" alt="Document : le protocole. Octet par octet : tout ce qui se passe entre un appareil, le serveur et les autres appareils, et pourquoi le serveur n'y lit rien. Noise IK, X25519, ChaCha20-Poly1305, BLAKE2s, Ed25519." width="100%">

</div>

<br>

Tout ce qui se passe entre un appareil, le serveur et les autres appareils, octet par octet. Le code vit dans `internal/` : `noise`, `tunnel`, `protocole`, `verrou` et `client`.


<a name="ce-qui-est-a-nous"></a>
<img src="sections/protocole/s01.png" alt="01 Ce qui est à nous" width="100%">

Les primitives cryptographiques viennent de la bibliothèque standard de Go et de `golang.org/x/crypto`. On n'écrit jamais ses propres primitives.

<img src="schemas/protocole/primitives.png" alt="Quatre primitives, venues de bibliothèques éprouvées. X25519 pour les échanges de clés, quatre par poignée de main. ChaCha20-Poly1305 pour le chiffrement des sessions, et XChaCha20-Poly1305 pour les cookies. BLAKE2s pour le hachage, les MAC et la dérivation des clés. Ed25519 pour les signatures du verrou." width="100%">

Tout le reste est écrit ici : l'assemblage de la poignée de main, le format des messages, les sessions, le relais, la protection contre le rejeu et l'inondation, le filtre, le verrou.

L'architecture reprend celle de WireGuard, qui a fait ses preuves, et celle de Tailscale pour le relais et le verrou. Mais le code est le nôtre, et il n'est **pas compatible** avec WireGuard : le prologue et le format diffèrent.

<a name="deux-couches"></a>
<img src="sections/protocole/s02.png" alt="02 Deux couches" width="100%">

<img src="schemas/protocole/couches.svg" alt="Les deux couches du protocole. Le téléphone fold8-tristan (A) et la maison (B) ont chacun une session Noise avec le serveur sasd, la couche transport, toujours ouverte en UDP direct. À l'intérieur, A et B ont leur propre session, de bout en bout, ouverte au premier paquet. Un paquet de A vers B voyage dans les deux : le serveur ouvre la trame de relais, lit le destinataire, et la remet à B sans pouvoir ouvrir la session A-B." width="100%">

Deux couches, toutes deux des poignées de main Noise IK :

- **La couche transport** : chaque appareil a une session avec le serveur, en UDP direct. Elle transporte ce qui s'adresse au serveur lui-même (le DNS du réseau) et les trames de relais.
- **La couche de bout en bout** : chaque paire d'appareils qui a le droit de se parler a sa propre session. Un paquet de A vers B est chiffré avec la session A-B, glissé dans une trame de relais, chiffrée à son tour pour le serveur. Le serveur ouvre la trame, lit le numéro du destinataire, et la remet à B dans sa propre session. Le paquet lui-même lui reste illisible : il n'a pas les clés de la session A-B.

Une session de bout en bout ne s'ouvre qu'à la demande, au premier paquet. Celle avec le serveur reste toujours ouverte : c'est par elle que les autres appareils nous joignent.

<a name="la-poignee-de-main"></a>
<img src="sections/protocole/s03.png" alt="03 La poignée de main" width="100%">

<img src="schemas/protocole/poignee.svg" alt="La poignée de main Noise IK, en un aller-retour. Message 1, de l'initiateur au répondeur : e, es, s, ss et un horodatage TAI64N chiffré, 148 octets avec mac1 et mac2. Le répondeur ne répond pas à une clé inconnue. Message 2, en retour : e, ee, se et une charge vide chiffrée, 92 octets. Chaque côté en tire deux clés de session, une par sens, renouvelées toutes les deux minutes. e est la clé éphémère, s la clé statique envoyée chiffrée ; es, ss, ee et se sont des échanges X25519 mêlés au hachage." width="100%">

`Noise_IK_25519_ChaChaPoly_BLAKE2s`, écrit d'après la [spécification Noise](https://noiseprotocol.org/noise.html), révision 34, avec le prologue `CyberSas tunnel v2`.

L'initiateur connaît d'avance la clé publique statique de l'autre (le **K** de IK) : le serveur la donne à l'inscription, puis dans l'état du réseau pour les autres appareils. Il envoie la sienne, chiffrée, dans le premier message (le **I**). Un seul aller-retour établit deux clés de session, une par sens.

Ce qui prouve que c'est juste :

- les vecteurs de test officiels du projet Noise (cacophony), pour cette variante exacte : les deux messages, le hachage final et quatre messages de transport correspondent octet pour octet ;
- l'échange dans les deux sens avec [flynn/noise](https://github.com/flynn/noise), une implémentation indépendante ;
- un octet modifié n'importe où dans le premier message le fait refuser.

<a name="les-messages"></a>
<img src="sections/protocole/s04.png" alt="04 Les messages" width="100%">

Tous commencent par un octet de type et trois octets réservés, à zéro. Les entiers sont en petit-boutiste, sauf la longueur d'une trame de relais. Chaque case est dessinée à l'échelle de ses octets.

<img src="schemas/protocole/messages.png" alt="Le format des messages, à l'échelle. Initiation, type 1, 148 octets : type 1, réservé 3, indice de l'émetteur 4, message Noise 1 sur 108, mac1 16, mac2 16. Réponse, type 2, 92 octets : type 1, réservé 3, émetteur 4, destinataire 4, message Noise 2 sur 48, mac1 16, mac2 16. Cookie, type 4, 64 octets : type 1, réservé 3, destinataire 4, nonce 24, cookie chiffré 16 et son tag 16. Données, type 3, longueur variable : type 1, réservé 3, destinataire 4, compteur 8, paquet chiffré, tag 16. Trame de relais, dans les données : 0x00, réservé 1, longueur 2 en grand-boutiste, numéro d'appareil 4, message relayé." width="100%">

- Le clair d'un message de données est soit un paquet IPv4 (premier octet `0x4X`), soit une trame de relais (premier octet `0x00`), soit vide (un maintien).
- Le message d'une trame de relais est une initiation, une réponse ou des données de la couche de bout en bout. Le numéro désigne le destinataire quand la trame va au serveur, et l'expéditeur quand elle en revient. La longueur permet d'écarter le remplissage ajouté par la couche transport.
- Le serveur ne relaie que des initiations, réponses et données : jamais un cookie, jamais une trame dans une trame.
- Les indices sont tirés au hasard par chaque côté. Ils permettent de retrouver une session sans rien révéler de l'identité de l'appareil.

<a name="les-sessions"></a>
<img src="sections/protocole/s05.png" alt="05 Les sessions" width="100%">

<img src="schemas/protocole/sessions.svg" alt="La vie d'une session sur quatre minutes. La session 1 sert de 0 à 3 minutes ; à 2 minutes, l'initiateur fait une nouvelle poignée de main et la session 2 prend le relais ; à 3 minutes, la session 1 ne chiffre ni ne déchiffre plus rien. En continu, un pair qui n'a rien renvoyé en 10 secondes envoie un paquet vide. Le compteur de chaque paquet sert de nonce. Le répondeur n'adopte la session 2 qu'au premier paquet chiffré avec elle." width="100%">

- Le compteur d'un paquet de données sert de nonce ChaCha20-Poly1305 : quatre octets nuls, puis le compteur sur huit octets.
- Le clair est complété par des zéros jusqu'à un multiple de 16 octets. La longueur réelle se lit dans l'en-tête IP, ou dans celui de la trame.
- L'initiateur renouvelle sa session toutes les **deux minutes**. Une session de plus de **trois minutes** ne chiffre et ne déchiffre plus rien.
- Le répondeur n'utilise une nouvelle session qu'après avoir reçu un premier paquet de données chiffré avec elle : c'est la preuve que l'initiateur détient les mêmes clés.
- **MTU** de l'interface : 1 360 octets. Le pire cas, un paquet relayé, tient ainsi dans 1 500 octets même sur IPv6.

<a name="ce-que-le-tunnel-refuse"></a>
<img src="sections/protocole/s06.png" alt="06 Ce que le tunnel refuse" width="100%">

<img src="schemas/protocole/refus.png" alt="Huit refus. Un paquet rejoué : fenêtre des 2 048 derniers compteurs, marqués seulement après vérification du tag. Une initiation rejouée : son horodatage doit être plus récent que la dernière acceptée. Une clé inconnue : pas de réponse. Le mauvais chemin : un message relayé doit venir d'un pair joint par relais sous le numéro annoncé, un message direct d'un pair direct. Une adresse usurpée : un paquet ne sort que si sa source est l'adresse du pair qui l'a chiffré. Pas pour nous : sa destination doit être notre adresse. Une réponse forgée : lue sur une copie de l'état Noise. Et ce que le filtre refuse." width="100%">

Deux de ces refus méritent un mot de plus :

- **Le mauvais chemin.** Le serveur ne peut pas faire passer un appareil pour un autre : il relaierait sous un numéro, mais la clé de la session ne correspondrait pas.
- **Pas pour nous.** Sans cette vérification, un pair autorisé sur un port pourrait se servir d'un appareil comme routeur vers son réseau local. C'était le constat T1 du premier audit.

<a name="l-inondation"></a>
<img src="sections/protocole/s07.png" alt="07 L'inondation" width="100%">

<img src="schemas/protocole/inondation.svg" alt="Trois barrières contre l'inondation, de la moins chère à la plus chère. Un flot d'initiations arrive. Première barrière, mac1 : ce qui ne vise pas ce serveur (scan à l'aveugle, paquet au hasard) est jeté pour le prix d'un hachage ; la clé du serveur n'est pas secrète, cette barrière n'arrête pas un attaquant décidé. Deuxième, sous charge : le serveur exige un mac2 calculé avec un cookie, qui prouve que l'expéditeur reçoit bien les paquets envoyés à son adresse ; sinon il ne répond qu'un cookie. Troisième : dix poignées de main par seconde au plus, par réseau /32 en IPv4 et /64 en IPv6. Ce qui passe arrive à sasd. Les poignées de main relayées sont limitées par couple d'appareils chez le serveur et par pair chez le client." width="100%">

Repris de WireGuard. Une poignée de main coûte deux échanges X25519 : on trie avant.

- **mac1** : un MAC BLAKE2s de 16 octets sur le message, avec une clé tirée de la clé publique du destinataire. Il arrête pour le prix d'un hachage tout ce qui ne vise pas ce serveur : scans à l'aveugle, paquets au hasard. Il n'arrête pas un attaquant décidé : la clé publique du serveur n'est pas secrète, l'API la donne avant l'inscription (étape 2 plus bas). Les deux barrières suivantes sont là pour lui.
- **Sous charge** (plus de 200 poignées de main par seconde), le serveur exige aussi un **mac2** valide, calculé avec un cookie. Sinon, il répond par un message cookie : un MAC de l'adresse IP et du port de l'expéditeur, sous un secret renouvelé toutes les deux minutes, chiffré en XChaCha20-Poly1305 avec le mac1 reçu en données associées. Seul celui qui reçoit vraiment les paquets envoyés à son adresse peut s'en servir.
- Une adresse qui a prouvé la possession de son IP garde droit à **dix poignées de main par seconde**. Le seau est celui de son réseau : /32 en IPv4, /64 en IPv6, pour que posséder un /64 ne donne pas des milliards de seaux.

<a name="le-filtre"></a>
<img src="sections/protocole/s08.png" alt="08 Le filtre" width="100%">

<img src="schemas/protocole/filtre.svg" alt="Le filtre, chez l'appareil qui reçoit. Un paquet entrant, déjà déchiffré, passe deux questions : une règle l'autorise-t-elle pour ce pair, ce protocole et ce port ? Sinon, répond-il à un flux que l'appareil a lui-même ouvert vers ce pair ? Oui à l'une : il entre. Non aux deux : il est jeté en silence. Trois exemples : TCP 443 autorisé par une règle entre ; TCP 22 sans règle est jeté ; UDP 53, réponse à une question de l'appareil, entre. Les flux sont suivis cinq minutes en TCP, une en UDP, trente secondes pour un écho, avec un budget par pair." width="100%">

Le serveur ne voit plus les paquets entre appareils : il ne peut plus appliquer les règles d'accès. C'est donc l'appareil qui reçoit qui filtre.

- **Sans verrou**, avec les règles que le serveur lui transmet. **Avec verrou**, avec les règles qu'il **calcule lui-même** à partir de la politique signée et des certificats signés : le serveur ne peut alors ni ouvrir un port, ni changer à qui une règle s'applique.
- Ce qu'un appareil envoie n'est jamais filtré.
- Seule la réponse d'écho peut revenir d'une demande d'écho ICMP.
- Répondre à ce qu'une règle laisse déjà entrer ne consomme rien du budget d'un pair : il ne peut pas remplir la table au détriment des autres.
- Un fragment qui n'est pas le premier n'a pas d'en-tête de transport : il ne passe qu'avec une règle « tout ».

En plus du filtre, le serveur ne relaie qu'entre deux appareils que la politique relie, dans un sens au moins. Deux appareils sans relation ne peuvent même pas se faire une poignée de main.

<a name="l-inscription"></a>
<img src="sections/protocole/s09.png" alt="09 L'inscription" width="100%">

<img src="schemas/inscription.svg" alt="Un nouvel appareil rejoint le réseau, en dix messages entre trois acteurs : le nouvel appareil, le serveur sasd et le téléphone de l'admin. 01 l'admin crée une invitation pour alice@gmail.com, valable une heure ; 02 le serveur rend une clé d'inscription à usage unique ; 03 l'admin envoie le lien cybersas:// avec le serveur, la clé et la clé du verrou ; 04 l'appareil crée sa paire de clés, la privée ne sort pas ; 05 il s'inscrit avec une preuve de possession ; 06 le serveur présente la demande à l'admin avec l'empreinte Qm7X-tR2k-9vLp ; 07 l'admin compare l'empreinte et signe au doigt ; 08 le certificat signé part au serveur ; 09 le serveur donne le réseau et les certificats à l'appareil ; 10 l'appareil est dans le réseau, en 10.77.0.3, pour 90 jours." width="100%">

1. L'appareil tire sa paire de clés X25519. La clé privée ne le quitte jamais, pas même dans une sauvegarde Android.
2. Il demande la clé publique du serveur (`GET /api/v1/serveur`).
3. Il calcule une **preuve de possession** : un HMAC-BLAKE2s, avec pour clé le secret X25519 entre sa clé et celle du serveur, sur toute la demande (les deux clés publiques, l'horodatage, le justificatif, le nom et le système). Seul le détenteur de la clé privée peut la produire, et elle ne se recolle pas à une autre demande. Le serveur refuse une preuve fausse, vieille de plus de cinq minutes, ou qui a déjà servi.
4. Il l'envoie avec un justificatif : une clé d'inscription à usage unique (le lien d'invitation), ou un jeton d'identité Google.
5. Le serveur vérifie le justificatif (pour Google : signature, émetteur, expiration, destinataire, partie autorisée, adresse vérifiée, présence dans l'équipe). À la première inscription d'une adresse, il retient l'identifiant permanent du compte Google : une adresse recyclée n'ouvre pas l'accès de l'ancien titulaire. Il attribue une adresse `10.77.0.x` et un numéro, jamais redonnés tout de suite à un autre appareil.

Le lien d'invitation porte la clé publique du verrou : le nouvel appareil la connaît avant même son premier contact avec le serveur. Et le serveur qu'il annonce ne peut router qu'une plage privée, pas plus large qu'un /16, avec un domaine sous `.internal` : un lien forgé ne détourne pas tout l'Internet du téléphone.

Une clé déjà inscrite ne change jamais de propriétaire ni d'étiquette. Le nom d'une machine est fixé par l'admin ; celui d'un appareil personnel porte toujours le nom de son propriétaire (`portable-alice`), pour que personne ne prenne celui d'une machine ou du serveur.

Toutes les clés, signatures et preuves reçues sont décodées par un décodeur base64 **canonique** (`internal/b64`) : une valeur n'a qu'une écriture acceptée. Le décodeur standard en accepte plusieurs, et un serveur piraté s'en serait servi pour faire passer une clé révoquée.

<a name="le-verrou"></a>
<img src="sections/protocole/s10.png" alt="10 Le verrou" width="100%">

Le chiffrement de bout en bout empêche le serveur de lire. Mais c'est lui qui distribue les clés publiques et les règles : un serveur piraté pourrait annoncer sa propre clé sous le nom d'un appareil, s'ouvrir les ports de tous, ou faire revenir un appareil banni. Le verrou ferme ces portes.

L'admin a une clé de signature **Ed25519**, gardée hors du serveur. Il signe trois sortes de documents, chacune avec son propre contexte, pour qu'une signature faite pour l'une ne serve jamais pour une autre. Chaque texte est précédé de sa longueur.

<img src="schemas/protocole/signes.png" alt="Les trois documents signés par le verrou, à l'échelle. Le certificat, un par appareil : le contexte « CyberSas certificat v2 » sur 23 octets, la clé 32, l'adresse IPv4 4, l'expiration 8, puis l'étiquette, le propriétaire et le groupe. La politique : le contexte « CyberSas politique v1 » sur 22 octets, la version 8, puis le fichier politique.json. Les révocations : le contexte « CyberSas revocations v1 » sur 24 octets, la version 8, puis les clés révoquées, 32 octets chacune." width="100%">

- **Le certificat** contient tout ce qui décide des règles d'un appareil : le serveur ne peut ni déplacer une clé signée vers une autre adresse, ni changer son étiquette ou son groupe. Il expire au bout de 90 jours.
- **La politique** est signée octet pour octet, avec son numéro de version.
- **La liste des clés révoquées** est triée, avec son numéro de version.

Chaque appareil retient la clé publique du verrou à l'inscription. Ensuite :

- il refuse tout pair sans certificat valide, expiré, ou révoqué, même présenté par le serveur ;
- il calcule ses règles d'entrée lui-même, à partir de la politique signée et des certificats, dont le sien : sans certificat pour lui-même, rien n'entre ;
- il n'accepte jamais une politique ou une liste de révocation plus ancienne que la dernière vue, et garde les versions vues sur disque ;
- il refuse tout changement ou toute disparition du verrou, et de la clé du serveur, même en se réinscrivant, sauf si on le lui demande explicitement.

Comment on s'en sert, où vit la clé et comment le téléphone la protège : [le document du verrou](verrou.md).

<a name="itinerance-et-reprise"></a>
<img src="sections/protocole/s11.png" alt="11 Itinérance et reprise" width="100%">

<img src="schemas/protocole/itinerance.png" alt="Itinérance et reprise. Du Wi-Fi à la 4G : le serveur suit l'appareil, et seul un paquet authentifié et jamais vu peut déplacer son adresse. 15 secondes sans rien : un initiateur qui envoie sans rien recevoir rouvre une session ; après un redémarrage du serveur, tout revient en moins de 25 secondes. 10 secondes de silence : un pair qui reçoit sans rien renvoyer envoie un paquet vide. Toutes les 10 secondes, le client relit l'état du réseau et l'adresse du serveur." width="100%">

<a name="ce-qui-manque-encore"></a>
<img src="sections/protocole/s12.png" alt="12 Ce qui manque encore" width="100%">

<img src="schemas/protocole/manque.png" alt="Ce qui manque encore. IPv4 seulement à l'intérieur du tunnel ; le transport, lui, passe aussi en IPv6. Pas de liaison directe : tout passe par le relais du serveur, chiffré de bout en bout, plus simple derrière n'importe quelle box, au prix d'un détour. Aucun audit humain : les tests prouvent la conformité à Noise et le refus des attaques connues, cela ne remplace pas le regard d'un cryptographe." width="100%">

Les relectures automatiques, leurs constats et ce qui en a été fait sont dans [l'audit](audit.md).

<br>

<div align="center">

<a href="../README.md"><img src="nav/readme.png" alt="Le README : CyberSAS en un coup d’œil." width="49%"></a>
<a href="verrou.md"><img src="nav/verrou.png" alt="Le verrou : la chaîne de confiance, le coffre du téléphone, signer et révoquer." width="49%"></a>
<a href="menaces.md"><img src="nav/menaces.png" alt="Le modèle de menace : ce qui est exposé, le serveur piraté, le téléphone volé." width="49%"></a>
<a href="audit.md"><img src="nav/audit.png" alt="L’audit de sécurité : trois relectures, 79 constats." width="49%"></a>

</div>
