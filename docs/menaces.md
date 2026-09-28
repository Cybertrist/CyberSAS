<div align="center">

<img src="banniere-menaces.png" alt="Document : le modèle de menace. Contre qui, et jusqu'où : ce que CyberSAS protège, ce qu'un attaquant peut encore faire, et ce qu'il ne promet pas." width="100%">

</div>

<br>

Ce que CyberSAS protège, contre qui, et ce qu'il ne promet pas. Chaque affirmation renvoie à un mécanisme décrit dans [le protocole](protocole.md) ou [le verrou](verrou.md), et la plupart à un test.


<a name="ce-qu-on-protege"></a>
<img src="sections/menaces/s01.png" alt="01 Ce qu'on protège" width="100%">

<img src="schemas/menaces/protege.png" alt="Quatre choses protégées. Le contenu : personne sur le chemin ne le lit, pas même le serveur. Les services de la maison : joignables par les appareils autorisés, sur les ports autorisés, et rien d'autre. L'adresse de la maison : elle n'apparaît nulle part, la maison sort vers le serveur. L'accès de l'équipe : un mot de passe volé, une clé publique connue ou un serveur piraté ne suffisent pas à entrer." width="100%">

<a name="ce-qui-est-expose"></a>
<img src="sections/menaces/s02.png" alt="02 Ce qui est exposé" width="100%">

<img src="schemas/menaces/surface.svg" alt="La surface d'attaque. Quelqu'un sur Internet scanne. Sur le VPS, seul point public : le port UDP 51820 du tunnel reste muet sans mac1 valide ; le port TCP 443 ne sert que vpn., auth. et maison., tout autre nom est coupé avant l'échange de certificat ; le port TCP 80 ne fait que rediriger vers 443. À la maison, aucun port ouvert : rien n'écoute. L'API de sasd n'écoute que sur 127.0.0.1, derrière Nginx." width="100%">

- **UDP 51820**, le tunnel. Il ne répond qu'à une initiation portant un mac1 valide, donc calculé avec la clé publique du serveur, et venant d'une clé inscrite. À tout le reste, il ne répond rien : un scan à l'aveugle ne voit qu'un port muet. Cette clé n'est pas secrète pour autant : l'API la donne à qui la demande, puisqu'un appareil en a besoin avant de s'inscrire. Contre qui la connaît et inonde, ce sont le cookie et la limite de dix poignées de main par seconde qui tiennent, pas le mac1.
- **TCP 443**, pour trois noms : `vpn.` (l'API d'inscription), `auth.` (la connexion Google des pages web) et `maison.` (un service publié). Tout autre nom est coupé avant même l'échange de certificat.
- **TCP 80**, qui ne fait que rediriger.

<a name="contre-qui"></a>
<img src="sections/menaces/s03.png" alt="03 Contre qui" width="100%">

<img src="schemas/menaces/adversaires.png" alt="Six adversaires. Qui écoute le réseau voit des paquets UDP chiffrés, leur taille arrondie à 16 octets, les IP publiques, mais ni le contenu ni l'identité. L'hébergeur du VPS voit qui parle à qui, quand et combien, pas ce qui se dit. Qui rejoue ou modifie des paquets est rejeté en silence. Qui inonde à l'aveugle est jeté au premier hachage ; qui vise vraiment le serveur doit prouver un cookie, puis se limiter à dix poignées de main par seconde. Qui veut entrer doit avoir un jeton Google de l'équipe ou une invitation à usage unique, prouver sa clé privée, puis obtenir la signature de l'admin. Un membre qui va trop loin n'atteint que ce que la politique autorise et se fait couper en cinq secondes." width="100%">

Quelques précisions que les fiches ne disent pas :

- **Qui veut s'approprier l'appareil d'un autre.** Les clés publiques circulent, mais inscrire une clé demande de prouver qu'on détient sa moitié privée. Et une clé inscrite ne change jamais de propriétaire.
- **Un membre qui va trop loin**, volontairement ou parce que son appareil est compromis. Le serveur ne relaie pas vers les appareils sans relation avec lui, et ceux qui en ont filtrent eux-mêmes ce qui entre. Il ne peut pas prendre le nom d'une machine ou du serveur dans le DNS du VPN, ni écrire lui-même les en-têtes d'identité qu'un service publié croit : le port publié n'est ouvert qu'au serveur.
- **Un service de la maison compromis.** Il ne peut se retourner vers aucun appareil : aucune règle ne part de `etiquette:maison`, et les appareils refusent ce qu'il tenterait d'ouvrir chez eux.
- **Un compte Google d'admin volé.** Il permet d'inscrire un appareil, pas d'en faire un appareil d'admin : les routes d'admin exigent un certificat signé par le verrou pour le groupe `admins`.

<a name="si-le-serveur-tombe"></a>
<img src="sections/menaces/s04.png" alt="04 Si le serveur tombe" width="100%">

C'est le cas le plus grave. Grâce au bout en bout et au verrou, il reste borné. Tout ce qui suit suppose le verrou en place ; sans lui, le serveur est cru sur parole.

<img src="schemas/menaces/serveur.png" alt="Si le VPS tombe aux mains d'un attaquant. Ce qu'il ne peut pas : lire le trafic entre appareils ; s'intercaler, il lui faudrait un certificat signé par le verrou ; ouvrir un port, chaque appareil calcule ses règles à partir de la politique signée et refuse une version plus ancienne ; faire revenir un banni, la liste de révocation est signée et ne recule jamais, et il ne peut pas faire signer à l'admin une liste de son choix ; faire signer autre chose que ce que l'admin a vu. Ce qu'il peut encore : couper le réseau, d'où l'expiration des certificats à 90 jours ; inscrire ses appareils, que les autres refusent ; lire ce qui s'adresse à lui, le DNS du réseau et les pages publiées par Nginx ; voler ses propres secrets, la clé privée du serveur et le secret du client Google, à régénérer." width="100%">

<img src="schemas/verrou.svg" alt="Un serveur piraté glisse un intrus dans le réseau : le téléphone fold8-tristan vérifie le certificat, ne trouve pas de signature du verrou, et le refuse. L'ordinateur laptop-lea, signé par le verrou, est accepté." width="100%">

Deux points à savoir :

- Un appareil qui n'a jamais vu une nouvelle révocation ne peut pas l'appliquer : c'est la raison d'être de l'expiration des certificats.
- Pour les pages publiées sur `maison.`, le TLS se termine sur le VPS. Passer par le VPN plutôt que par la page publique garde le chiffrement de bout en bout.

<a name="si-le-telephone-est-vole"></a>
<img src="sections/menaces/s05.png" alt="05 Si le téléphone de l'admin est volé" width="100%">

Depuis que l'admin signe depuis son téléphone, ce téléphone compte autant que le serveur. Voici ce qui le protège, et ce qu'il faut faire s'il disparaît.

<img src="schemas/menaces/telephone.png" alt="Si le téléphone de l'admin est volé. Ce qui le protège : le verrou de l'appli, une empreinte à l'ouverture et à nouveau après 30 secondes dehors, compté sur l'horloge du système qu'on ne recule pas ; la clé du verrou, chiffrée par une clé de la puce, qui ne s'ouvre qu'avec une empreinte pour une seule opération et qu'une empreinte ajoutée invalide ; pas de copie, la clé de l'appareil et le coffre sont exclus des sauvegardes et des transferts ; un aperçu vide dans les applis récentes. Ce qu'il faut faire : retirer et révoquer le téléphone depuis un autre appareil d'admin ou depuis l'ordinateur ; changer de verrou si le téléphone était déverrouillé et l'appli ouverte au moment du vol ; garder une copie de la clé du verrou hors ligne." width="100%">

Un voleur qui trouve le téléphone verrouillé n'a rien. S'il le trouve déverrouillé, l'appli lui demande une empreinte ; s'il la trouve ouverte, il peut voir le réseau, créer une invitation, pour un membre déjà dans l'équipe seulement, ou en annuler une ; signer ou révoquer demandent encore le doigt de l'admin, seul à ouvrir la clé du verrou. Changer l'équipe ou retirer un appareil, non : l'appli redemande l'empreinte, mais le serveur n'exige que le jeton d'un appareil d'admin signé, et qui l'a tiré du téléphone s'en passe. Cela ne fait entrer personne dans le VPN, où chaque appareil doit encore être signé, mais la liste des pages web (`emails.txt`, que lit oauth2-proxy pour `maison.`) suit l'équipe sans passer par le verrou : un compte ajouté ainsi, ou par un serveur piraté, ouvre ces pages.

<a name="ce-qu-il-ne-promet-pas"></a>
<img src="sections/menaces/s06.png" alt="06 Ce qu'il ne promet pas" width="100%">

<img src="schemas/menaces/promet-pas.png" alt="Ce que CyberSAS ne promet pas. Pas d'audit humain : le protocole reprend WireGuard, suit les vecteurs de Noise, a résisté à des millions de messages forgés et à trois relectures, mais peu de gens l'ont lu ; pour des données dont la fuite serait grave, WireGuard reste le choix raisonnable. Les métadonnées : le serveur voit qui parle à qui, quand et combien. Google, tiers de confiance pour les inscriptions seulement : en panne, personne n'entre mais ceux qui sont dedans continuent ; un compte volé inscrit un appareil, inutile tant que l'admin ne l'a pas signé. Le premier contact : un appareil à qui l'on ne donne pas la clé du verrou retient la première annoncée ; le lien d'invitation la donne d'avance, et l'empreinte permet de vérifier." width="100%">

Les relectures, leurs constats et ce qui en a été fait : [l'audit](audit.md).

<br>

<div align="center">

<a href="../README.md"><img src="nav/readme.png" alt="Le README : CyberSAS en un coup d’œil." width="49%"></a>
<a href="protocole.md"><img src="nav/protocole.png" alt="Le protocole : les deux couches, Noise IK, les messages à l’échelle, le filtre." width="49%"></a>
<a href="verrou.md"><img src="nav/verrou.png" alt="Le verrou : la chaîne de confiance, le coffre du téléphone, signer et révoquer." width="49%"></a>
<a href="audit.md"><img src="nav/audit.png" alt="L’audit de sécurité : trois relectures, 79 constats." width="49%"></a>

</div>
