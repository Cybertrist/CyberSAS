<div align="center">

<img src="banniere-audit.png" alt="Document : l'audit. Trois relectures, 79 constats : qui a relu quoi, ce qui a été trouvé, ce qui a été corrigé, et ce qui reste." width="100%">

</div>

<br>

Ce document rend compte des relectures de sécurité de CyberSAS : qui a relu quoi, ce qui a été trouvé, ce qui a été corrigé, et ce qui reste. Les deux premières datent du 24 septembre 2026, la troisième du 26.

<img src="schemas/audit/trois.png" alt="Trois audits. Premier audit, le 24 septembre : trois relecteurs indépendants, protocole, serveur et clients, puis une revue de sécurité des corrections ; 46 constats, dont 3 hauts. Deuxième audit, le même jour : les outils du métier, govulncheck, staticcheck, gosec, Semgrep, Trivy, Hadolint, ShellCheck, Gixy et du fuzzing différentiel ; 10 constats. Troisième audit, le 26 septembre : ce qui a changé depuis, les routes d'admin, l'Android natif, la frontière entre l'appli et le moteur ; 23 constats, dont 1 haut." width="100%">


<a name="ce-que-cet-audit-est"></a>
<img src="sections/audit/s01.png" alt="01 Ce que cet audit est" width="100%">

Ce n'est **pas** un audit par un cabinet ou un cryptographe indépendant. Tout
a été fait par des outils et des relecteurs automatiques, sans humain
extérieur. Cela ne remplace pas le regard d'un expert, et ce document ne
prétend pas le contraire.

Ce qui a été fait, en revanche, l'a été sérieusement, et chaque affirmation
ci-dessous est vérifiable dans le dépôt :

1. **Vérifications de conformité** de la cryptographie, avant tout audit :
   - la poignée de main Noise IK comparée octet pour octet au vecteur de test
     officiel du projet Noise (cacophony) : messages, hachage final et
     messages de transport ;
   - la même poignée de main confrontée, dans les deux sens, à une
     implémentation indépendante ([flynn/noise](https://github.com/flynn/noise)) ;
   - le fuzzing du moteur : 27,5 millions de messages forgés en une minute
     sur le point d'entrée réseau, sans une panique ni un paquet produit.
2. **Trois relecteurs indépendants**, lancés en parallèle, sans contexte sur
   le projet, chacun sur un périmètre, avec la consigne d'attaquer et de
   prouver par un test ce qu'ils avancent :
   - le protocole et la cryptographie (`internal/noise`, `internal/tunnel`) ;
   - le serveur (API, politique, base, DNS, verrou) ;
   - les clients et le déploiement (Docker, Nginx, oauth2-proxy, scripts).
3. **Une revue de sécurité** de toutes les corrections, suivie d'une
   vérification indépendante de chaque constat pour écarter les faux
   positifs.
4. **Les corrections**, chacune accompagnée d'un test de non-régression
   quand c'est possible. Ils s'appellent `TestAudit…` dans le code, ou sont
   cités ci-dessous.

**Résultat** : 46 constats, dont 3 de gravité haute : 11 sur le protocole,
16 sur le serveur (9 constats et 7 durcissements), 17 sur les clients et le
déploiement, et 2 dans la revue des corrections. Tous sont traités :
corrigés, ou acceptés en connaissance de cause et expliqués. Après
correction, 67 tests Go et 18 vérifications de bout en bout dans le labo
passent, sous le détecteur d'accès concurrents de Go.

<a name="relancer-les-preuves"></a>
<img src="sections/audit/s02.png" alt="02 Relancer les preuves" width="100%">

```bash
go test -race ./...                                        # 79 tests, dont ceux des audits
go test -run '^$' -fuzz=FuzzRecevoir -fuzztime=3m ./internal/tunnel
go test -run '^$' -fuzz=FuzzLireMessage1 -fuzztime=3m ./internal/noise
bash scripts/sas.sh essai                                  # 18 vérifications dans le labo
```

Les outils du deuxième audit se relancent de la même façon, chacun dans son
conteneur : voir la section « Deuxième audit ».

<a name="protocole-et-cryptographie"></a>
<img src="sections/audit/s03.png" alt="03 Protocole et cryptographie" width="100%">

Le relecteur juge sains, après vérification : l'assemblage Noise IK
(ordre des jetons, HKDF, nonces, Split), l'absence de réutilisation de nonce,
la fenêtre anti-rejeu, la séparation des chemins direct et relayé, les
trames de relais (pas d'imbrication, pas d'amplification), mac1 et mac2
conformes à WireGuard, la lecture des paquets IPv4, et l'ordre des verrous.
Ses constats :

### T1. La destination des paquets reçus n'était pas vérifiée

- **Gravité** : moyenne à haute. Prouvé.
- **Le problème** : un appareil vérifiait la source d'un paquet reçu et son
  port, mais pas sa destination. Autorisé sur le TCP 80 de la maison, un pair
  pouvait lui envoyer un paquet pour 192.168.1.50:80 : écrit sur l'interface,
  il partait vers le réseau local de la maison.
- **Correction** : le moteur connaît sa propre adresse et rejette tout paquet
  qui ne lui est pas destiné, avant le filtre (`moteur.go`, `recevoirDonnees`).
- **Test** : `TestAuditDestinationVerifiee`.

### T2. Une réponse forgée cassait la poignée de main en cours

- **Gravité** : moyenne. Prouvé.
- **Le problème** : `LireMessage2` mélangeait la réponse dans l'état Noise avant
  d'en vérifier le tag. Une seule réponse forgée, facile à produire pour qui
  observe le réseau et connaît la clé publique du client, corrompait l'état :
  la vraie réponse était ensuite refusée, et le client ne pouvait plus se
  connecter.
- **Correction** : la lecture travaille sur une copie de l'état, qui ne
  remplace l'original qu'en cas de succès, comme dans WireGuard. En direct,
  une réponse doit en plus venir de l'adresse où l'initiation est partie.
- **Test** : `TestReponseForgeeNeCassePasLaPoignee`.

### T3. La limite de débit se contournait en IPv6

- **Gravité** : moyenne. Prouvé.
- **Le problème** : un seau de jetons par adresse. Qui possède un /64
  possède des milliards d'adresses, donc autant de seaux.
- **Correction** : un seau par /64 en IPv6, par /32 en IPv4. Table pleine :
  les seaux inactifs sont oubliés au lieu de refuser les nouveaux venus.
- **Test** : `TestAuditSeauParReseauIPv6`.

### T4. Les poignées de main relayées n'étaient pas limitées

- **Gravité** : moyenne à basse. Prouvé.
- **Le problème** : relayées par le serveur, elles échappent aux cookies. Un
  appareil relié à un autre pouvait lui en envoyer en boucle, et épuiser son
  processeur et sa batterie.
- **Correction** : un seau par pair chez le client, et un par couple
  d'appareils chez le serveur, qui ne relaie plus au-delà.
- **Test** : `TestAuditInitiationsRelayeesLimitees`.

### T5. Un pair pouvait remplir le suivi des connexions d'un autre

- **Gravité** : basse à moyenne.
- **Le problème** : une seule table pour tous les pairs, remplie aussi par
  nos réponses aux connexions qu'une règle laisse entrer. Un pair autorisé
  pouvait la saturer, et les réponses venant des autres étaient refusées.
- **Correction** :
  - un budget d'entrées par pair ;
  - les réponses à ce qu'une règle autorise déjà ne sont plus retenues ;
  - une réponse n'est acceptée que du pair à qui l'on a écrit.
- **Tests** : `TestAuditSuiviBudgetParPair`, `TestAuditReponseAutoriseeNonRetenue`.

### T6. Un flux à sens unique relançait une poignée de main toutes les 15 secondes

- **Gravité** : basse. Prouvé.
- **Correction** : le maintien passif part à la première donnée reçue depuis
  notre dernier envoi, sans se réarmer à chaque paquet.
- **Test** : `TestAuditFluxSensUnique`.

### T7. Après un abandon, les tentatives ne repartaient pas de zéro

- **Gravité** : basse. Prouvé.
- **Test** : `TestAuditDebutTentativesRemis`.

### T8. Un pair retiré pendant sa poignée de main pouvait revenir

- **Gravité** : basse.
- **Correction** : un drapeau `retire`, vérifié avant toute inscription dans
  la table des indices.
- **Test** : `TestAuditIndices`.

### T9. Pas de détection de collision d'indice

- **Gravité** : basse.
- **Correction** : l'indice est retiré au sort tant qu'il est pris ou nul.
- **Test** : `TestAuditIndices`.

### T10. Le suivi ICMP laissait entrer n'importe quel type

- **Gravité** : basse.
- **Correction** : seule la réponse d'écho (type 0) entre en retour d'une
  demande d'écho (type 8).
- **Test** : `TestAuditSuiviICMP`.

### T11. Informations

- **Horodatage trop précis**, qui renseignait sur l'horloge de l'appareil :
  arrondi à 2^24 nanosecondes, comme WireGuard. Corrigé.
- **Un cookie peut être forgé** par un observateur du réseau, et forcer des
  relances : comme dans WireGuard, sans gravité. Accepté.
- **L'itinérance suit l'initiation** avant toute preuve de possession des
  clés : conforme à WireGuard, d'impact faible. Accepté.
- **Clé du répondeur compromise (KCI)** : pas de session possible, la
  promotion exige le premier paquet de données. Accepté.
- **Le numéro d'appareil n'est pas signé** : sans conséquence, l'identité
  reste liée à la clé. Accepté.
- **Documentation en retard sur le code** : mise à jour.

<a name="serveur"></a>
<img src="sections/audit/s04.png" alt="04 Serveur" width="100%">

Le relecteur juge sains : l'absence d'injection SQL (requêtes paramétrées)
et nftables (seulement des adresses et des entiers), les jetons (256 bits,
stockés hachés), l'usage unique des clés d'inscription, le refus des points
faibles de X25519, et la politique elle-même. Ses constats :

### S1. Retirer quelqu'un de l'équipe ne le coupait pas

- **Gravité** : haute. Prouvé.
- **Le problème** : Docker monte un fichier seul par son inode. Or `sas.sh`
  écrit la liste de l'équipe à côté, puis la renomme : le conteneur continuait
  à lire l'ancienne. La personne retirée gardait ses sessions, son jeton
  d'API, et pouvait même inscrire de nouveaux appareils. Côté web, elle était
  bien coupée, ce qui donnait à l'admin l'illusion qu'elle l'était partout.
- **Correction** : les dossiers sont montés, pas les fichiers.
- **Test** : dans le labo, « retirée de l'équipe, la personne perd ses
  appareils en quelques secondes ».

### S2. Une politique cassée bloquait toutes les révocations

- **Gravité** : moyenne. Prouvé.
- **Le problème** : une faute de frappe dans `politique.json` arrêtait la
  synchronisation avant la purge. Les appareils des personnes retirées
  restaient actifs.
- **Correction** : les retraits passent d'abord, quoi qu'il arrive à la
  politique. Une politique illisible est remplacée par la dernière lue avec
  succès.
- **Test** : `TestAuditPolitiqueCasseeNeBloquePasLesRetraits`.

### S3. Un appareil pouvait prendre le nom « serveur » ou « maison »

- **Gravité** : moyenne. Prouvé.
- **Le problème** : les noms DNS du VPN se prenaient au premier arrivé. Un
  membre qui nommait son appareil « serveur » recevait ce que les admins
  envoyaient au serveur, leurs identifiants SSH par exemple.
- **Correction** :
  - des noms réservés (`serveur`, `vpn`, `auth`), posés en dernier dans le DNS ;
  - les noms des machines fixés par l'admin dans la clé d'inscription, et
    refusés s'ils sont pris ;
  - les noms des appareils personnels toujours suffixés de leur propriétaire
    (`portable-alice`).
- **Test** : `TestAuditNomsReserves`.

### S4. Une équipe illisible un instant effaçait tous les appareils personnels

- **Gravité** : moyenne. Prouvé.
- **Correction** :
  - la dernière équipe lue avec succès reste en vigueur ;
  - une purge qui retirerait d'un coup plus de la moitié des appareils est
    refusée et signalée. Les appareils concernés sont coupés, mais pas
    effacés.
- **Tests** : `TestAuditEquipeIllisibleNEfacePas`, `TestAuditPurgeMassiveRefusee`.

### S5. Les signatures du verrou ne s'annulaient jamais

- **Gravité** : basse à moyenne. Même constat que C3.
- **Correction** : voir C2 et C3.

### S6. Un pare-feu en échec laissait un état incohérent

- **Gravité** : basse. Prouvé.
- **Correction** : le pare-feu est appliqué en premier. S'il échoue, aucun
  nouvel appareil n'est activé ; seuls les retraits s'appliquent.
- **Test** : `TestAuditPareFeuEnEchec`.

### S7. Les numéros et les adresses étaient réutilisés

- **Gravité** : basse. Prouvé.
- **Le problème** : le dernier appareil retiré et le suivant inscrit
  recevaient le même numéro et la même adresse. Un pair qui n'avait pas encore
  rafraîchi ses règles les appliquait au nouveau venu.
- **Correction** : numéros jamais redonnés (`AUTOINCREMENT`), adresses
  attribuées en tourniquet.
- **Test** : `TestAuditNumeroEtAdresseNonReutilises`.

### S8. La preuve de possession n'était pas liée au justificatif

- **Gravité** : basse.
- **Le problème** : la preuve couvrait la clé et l'horodatage, pas le jeton
  Google ni la clé d'inscription. Et une demande capturée se rejouait pendant
  cinq minutes.
- **Correction** : la preuve couvre toute la demande (justificatif, nom,
  système). Une preuve qui a servi à une inscription réussie est retenue et
  refusée ensuite.
- **Tests** : `TestPreuve`, `TestRejeux`, `TestInscriptionEtPreuve`.

### S9. Une clé d'inscription était perdue quand l'inscription échouait

- **Gravité** : basse. Prouvé.
- **Correction** : la clé est consommée dans la même transaction que
  l'inscription.
- **Test** : `TestAuditCleNonConsommeeSurRefus`.

### Durcissements

- **DNS** : chaque appareil ne résout que les noms des appareils avec qui il
  est relié. Les autres n'existent pas pour lui. Test : `TestAuditDNSFiltre`.
- **État du réseau** : une machine ne voit plus les adresses email des
  personnes, ni le système ou l'expiration des autres appareils. Test :
  `TestReseauNeMontreQueLesPairsRelies`.
- **Google** :
  - l'identifiant permanent du compte (`sub`) est fixé à la première
    inscription, pour qu'une adresse recyclée n'ouvre pas l'accès de l'ancien
    titulaire ;
  - la partie autorisée du jeton (`azp`) est vérifiée.
- **SQLite** : transactions immédiates, pour que deux inscriptions
  simultanées ne prennent pas la même adresse.
- **Pare-feu** : une adresse du VPN ne se joint que par l'interface du VPN.
- **Nom et système** d'un appareil : bornés, et sans caractère de contrôle.
- **API** : délais de lecture et d'écriture.

<a name="clients-et-deploiement"></a>
<img src="sections/audit/s05.png" alt="05 Clients et déploiement" width="100%">

Le relecteur juge sains : Nginx qui coupe les noms inconnus, le refus des
redirections vers un autre domaine, oauth2-proxy tombé qui refuse au lieu de
laisser passer, et l'absence de secret dans l'historique git. Ses constats :

### C1. La signature du verrou se faisait à l'aveugle

- **Gravité** : haute.
- **Le problème** : `sas.sh signer` signait tout appareil non signé que le
  serveur listait. Un serveur piraté n'avait qu'à inscrire sa machine : la
  prochaine signature de routine la validait. Et la clé du verrou était créée
  sur le serveur lui-même.
- **Correction** :
  - l'admin ne signe que les clés qu'il désigne, lues sur les appareils
    eux-mêmes (`sas etat`, ou l'écran de l'appli) ;
  - la clé du verrou ne se crée plus sur le serveur hors du labo, et le script
    refuse de s'en servir s'il en trouve une ;
  - tout texte venu du serveur est nettoyé avant affichage, pour qu'un
    serveur ne puisse pas maquiller le terminal de l'admin.
- **Procédure** : [`verrou.md`](verrou.md).

### C2. La politique n'était pas signée : un serveur piraté pouvait ouvrir tous les ports

- **Gravité** : haute. Prouvé.
- **Le problème** : les règles d'entrée venaient du serveur sans signature. Un
  serveur piraté pouvait se donner « tout » en entrée sur chaque appareil, avec
  ou sans verrou.
- **Correction** : le verrou change de nature.
  - l'admin signe la politique elle-même, avec un numéro de version ;
  - chaque certificat d'appareil couvre désormais son étiquette, son
    propriétaire, son groupe et une date d'expiration ;
  - avec un verrou, chaque appareil **calcule lui-même** ses règles d'entrée
    à partir de la politique signée et des certificats signés ;
  - tout ce que le serveur dit sans signature est ignoré.
- **Test** : `TestVerrouReglesCalculeesLocalement`. Un serveur qui s'ouvre tous
  les ports n'obtient que ce que la politique signée lui donne.

### C3. Aucune révocation

- **Gravité** : moyenne.
- **Correction** :
  - une liste de révocation signée par le verrou et numérotée ; chaque
    appareil garde la plus récente vue, et n'accepte jamais une version plus
    ancienne ;
  - les certificats expirent au bout de 90 jours ;
  - de même, une politique plus ancienne que la dernière vue est refusée.
- **Test** : `TestVerrouRevocationsEtRetourEnArriere`.

### C4. Un pair annoncé en IPv6 faisait planter tout client verrouillé

- **Gravité** : moyenne. Prouvé.
- **Correction** : seules les adresses IPv4 du réseau sont acceptées, et la
  signature d'une adresse non IPv4 rend une erreur au lieu de planter.
- **Tests** : `TestVerrouAdressesInvalides`, `TestAdresseIPv6Refusee`.

### C5. Les en-têtes d'identité étaient falsifiables en direct

- **Gravité** : moyenne. Prouvé.
- **Le problème** : Nginx transmet l'adresse Google dans `X-Email`. Mais le même
  service était joignable directement dans le VPN, où n'importe quel membre
  pouvait écrire cet en-tête lui-même.
- **Correction** : Nginx publie un port réservé au serveur (8081), que la
  politique n'ouvre à personne d'autre. Seul ce port peut croire `X-Email`.
- **Test** : dans le labo, « le port publié de la maison est fermé au poste ».

### C6. Se réinscrire effaçait la clé du serveur et le verrou retenus

- **Gravité** : moyenne.
- **Correction** : la réinscription refuse un autre serveur ou un autre
  verrou, y compris la disparition du verrou, sauf avec `--oublier`.

### C7. Le client acceptait http:// en clair

- **Gravité** : moyenne. Prouvé.
- **Correction** : `https://` obligatoire. Aucune redirection n'est suivie,
  pour que le jeton ne parte pas ailleurs.
- **Test** : `TestAPIRefuseHTTP`.

### C8. Le cookie de session partait vers les services publiés

- **Gravité** : moyenne.
- **Correction** :
  - Nginx retire le cookie d'oauth2-proxy avant de transmettre la requête ;
  - hors labo, le script impose un sous-domaine dédié (`sas.exemple.fr`), pour
    que le cookie ne parte pas vers les autres sites du domaine principal.

### C9. L'autorité du labo pouvait signer pour n'importe quelle adresse IP

- **Gravité** : moyenne. Prouvé.
- **Correction** : les contraintes de noms excluent toute adresse IPv4 et
  IPv6.

### C10 à C17. Durcissements du déploiement

- **Secrets lisibles par tous** sur l'hôte : `umask 077`, et oauth2-proxy
  tourne sous l'utilisateur de l'admin.
- **Création de la clé du verrou** : atomique (`O_EXCL`), jamais par-dessus une
  clé existante.
- **Conteneurs** :
  - aucune capacité par défaut, seules celles nécessaires rendues ;
  - pas d'élévation de privilèges, systèmes de fichiers en lecture seule ;
  - images épinglées par empreinte ;
  - plus de routage IP sur le serveur.
- **Nginx** :
  - seul le WebSocket peut changer de protocole ;
  - tampons ramenés aux valeurs par défaut ;
  - en-têtes posés explicitement vers oauth2-proxy.
- **Secrets en argument de commande** : le secret Google se tape au clavier,
  la clé d'inscription passe par `SAS_CLE`.
- **Le script d'essai en production** : refusé hors labo, et `TLS` n'a plus de
  valeur par défaut.
- **Robustesse du client** :
  - réponses de l'API limitées à 1 Mo ;
  - état écrit sur disque avec synchronisation ;
  - texte du serveur nettoyé avant affichage ;
  - `ip` appelé par son chemin absolu.

<a name="revue-des-corrections"></a>
<img src="sections/audit/s06.png" alt="06 Revue des corrections" width="100%">

Une revue de sécurité a relu toutes les corrections, et un vérificateur
indépendant a contrôlé chacun de ses constats.

### R1. Une clé révoquée pouvait revenir sous une autre écriture

- **Gravité** : moyenne, haute dans le modèle du verrou. Confirmé par un test
  (8 sur 10).
- **Le problème** : une clé de 32 octets a plusieurs écritures base64 valides,
  car le décodeur standard de Go ignore les bits de bourrage. La révocation
  comparait les clés sous forme de texte. Un serveur piraté pouvait donc
  réannoncer un appareil révoqué avec une autre écriture de sa clé : absente,
  en texte, de la liste de révocation, mais identique une fois décodée.
- **Correction** : un seul décodeur, [`internal/b64`](../internal/b64), qui
  n'accepte une chaîne que si la réencoder redonne la même. Il est employé
  partout où une clé, une signature ou une preuve arrive de l'extérieur. Et
  les révocations se comparent sur les octets.
- **Tests** : `TestRevocationEcritureNonCanonique`, `TestUneSeuleEcriture`.

### R2. La même faiblesse dans le cache anti-rejeu des inscriptions

- **Jugé non exploitable en pratique** (3 sur 10) : il faudrait lire le corps
  d'une inscription réussie, qui ne circule en clair que sur l'interface
  locale du serveur.
- Corrigé quand même, par le même décodeur canonique.

<a name="trouve-en-corrigeant"></a>
<img src="sections/audit/s07.png" alt="07 Trouvé en corrigeant" width="100%">

- **Un client réinscrit gardait son ancienne adresse** sur l'interface, en plus
  de la nouvelle, et continuait d'émettre avec. Les pairs rejetaient ces
  paquets comme usurpés : le filtre a fait son travail, mais le client était
  coupé. Corrigé : l'interface est vidée avant de recevoir sa nouvelle
  adresse.
- **La première version du cache anti-rejeu** retenait aussi les demandes
  refusées : réessayer une inscription juste après un refus devenait
  impossible. Corrigé : seules les preuves d'inscriptions réussies sont
  retenues.

<a name="deuxieme-audit"></a>
<img src="sections/audit/s08.png" alt="08 Deuxième audit : les outils" width="100%">

Le premier audit reposait sur des relecteurs. Le second passe tout le dépôt
aux outils qu'emploient les équipes de sécurité, chacun lancé dans un
conteneur, sans rien installer sur le poste. Chaque constat a été lu, puis
corrigé ou écarté avec sa raison.

### Les outils

- **govulncheck** (Go) : les vulnérabilités connues des dépendances, mais
  seulement celles que le code appelle vraiment.
- **staticcheck** : l'analyse statique de référence pour Go.
- **gosec** : les motifs dangereux en Go (débordements, erreurs ignorées,
  chemins, commandes).
- **Semgrep** : 91 règles Go, Dockerfile et Nginx.
- **Trivy** : les vulnérabilités de l'image Docker, sa configuration, et la
  recherche de secrets oubliés.
- **Hadolint** : les bonnes pratiques du Dockerfile.
- **ShellCheck** : les pièges de Bash dans `scripts/sas.sh`.
- **Gixy** : les erreurs de configuration de Nginx, sur la configuration
  réellement chargée (`nginx -T`), pas sur les modèles.
- **Fuzzing long** : huit cibles, trois minutes chacune, dont quatre
  nouvelles (ci-dessous).

### Ce qu'ils ont trouvé, et ce qui a été corrigé

- **O1. Une longueur sur deux octets pouvait déborder dans les certificats**
  (gosec G115). Le message signé d'un certificat écrit chaque texte précédé
  de sa longueur sur deux octets. Un groupe de plus de 65 535 octets aurait
  vu sa longueur tronquée, et deux certificats différents auraient pu donner
  le même message signé. Rien ne permettait d'écrire un tel groupe, mais la
  signature ne doit pas dépendre de cette chance : tout texte de plus de
  255 octets est maintenant refusé (`verrou.MaxChamp`). Test :
  `TestCertificat`.
- **O2. La même longueur dans les trames relayées** (gosec G115). Sans
  conséquence, puisque le chiffrement refusait déjà tout message de plus de
  1 408 octets, mais la borne est maintenant écrite là où la longueur est
  posée. Test : `TestTrameRelais`.
- **O3. Le numéro d'appareil était l'identifiant de la base, tronqué à
  quatre octets** (gosec G115). Au-delà de quatre milliards d'inscriptions,
  deux appareils auraient partagé un numéro, et le serveur aurait relayé
  vers le mauvais. Inatteignable en pratique ; refusé quand même à
  l'inscription (`ErrNumerosEpuises`).
- **O4. Des erreurs ignorées là où elles comptent** (gosec G104, 38
  constats triés un par un) :
  - un appareil dont l'effacement échouait était journalisé comme retiré ;
    l'échec est maintenant journalisé comme tel, et l'effacement retenté ;
  - `sas quitter` répondait « désinscrit » même si le serveur n'avait pas
    pu effacer l'appareil : le serveur renvoie maintenant une erreur, et le
    client dit si la clé privée n'a pas pu être effacée du disque ;
  - une erreur de lecture pendant le choix d'une adresse pouvait faire
    redonner une adresse déjà prise ; l'inscription s'arrête désormais ;
  - un `SAS_PORT` mal écrit donnait en silence le port par défaut ; sasd
    refuse maintenant de démarrer ;
  - la fermeture du fichier d'état, après écriture, est vérifiée.
- **O5. Les redirections et les en-têtes reprenaient l'en-tête Host du
  client** (Semgrep, Gixy). Nginx transmettait `$host`, ce que le client a
  écrit, aux services et à oauth2-proxy. Chaque bloc n'accepte qu'un nom
  exact, donc la valeur était déjà contrainte, mais elle est maintenant
  fixée par Nginx (`$server_name`). Le port 80, lui, acceptait
  `*.domaine` et redirigeait n'importe quel sous-domaine vers lui-même ; il
  ne connaît plus que les trois noms servis, et coupe les autres sans
  répondre. Vérifié à la main : `evil.domaine` n'obtient plus rien.
- **O6. Une mise à jour de Nginx n'était jamais appliquée.** Trouvé en
  vérifiant O5 : Nginx ne lit ses modèles qu'en démarrant, et
  `docker compose up` ne le relançait pas quand seuls les fichiers montés
  changeaient. Une correction de sécurité de la configuration serait donc
  restée sans effet. `sas.sh demarrer` passe maintenant l'empreinte du
  dossier `nginx/` au conteneur, qui est recréé dès qu'elle change.
- **O7. Le serveur par défaut négociait TLS sans réglages explicites**
  (Gixy). Avant de lire le nom demandé, Nginx négocie avec les réglages du
  serveur par défaut, qui n'incluait pas `tls.conf`. Nginx 1.30 se limite
  déjà à TLS 1.2 et 1.3 par défaut ; c'est maintenant écrit pour tous.
- **O8. Paquets Alpine non figés** (Hadolint DL3018). Les versions sont
  maintenant fixées, en laissant passer les révisions de sécurité.
- **O9. Le script** (ShellCheck) : une variable globale portait le même nom
  qu'un tableau local, et une construction `a && b || c` pouvait lancer `c`
  à tort. Corrigés ; ShellCheck ne relève plus rien.
- **O10. Style** (staticcheck ST1005) : un message d'erreur commençait par
  une majuscule.

### Écartés, avec leur raison

- **« Le conteneur tourne en root »** (Semgrep, Trivy DS-0002). Vérifié :
  sous un autre utilisateur, les capacités données par compose ne sont pas
  effectives, et `no-new-privileges` interdit de les poser sur le fichier.
  sasd doit créer une interface et écrire des règles nftables. Ce root n'a
  que les capacités listées dans `compose.yaml`, sur un système de fichiers
  en lecture seule. Expliqué dans le Dockerfile.
- **« Pas de HEALTHCHECK »** (Trivy DS-0026). La même image sert au client,
  qui n'a pas d'API. La vérification de sasd est dans `compose.yaml`.
- **Chemins et commandes « variables »** (gosec G204, G304, G703). Ce sont
  les chemins de la configuration et le chemin absolu de `ip`, jamais une
  donnée venue du réseau.
- **Droits 0700 sur un dossier** (gosec G302). Un dossier a besoin du droit
  d'exécution pour être traversé ; 0700 reste réservé à son propriétaire.
- **Conversions d'heures en entiers non signés** (gosec G115). Horodatages
  de 2026, loin de toute limite.
- **Erreurs ignorées restantes** (gosec G104) : envois UDP et DNS au mieux,
  fermetures sur un chemin déjà en erreur, affichage à l'écran, lecture du
  corps d'une erreur HTTP. Aucune ne change une décision de sécurité.
- **`worker_rlimit_nofile`** (Gixy). Réglage de charge du `nginx.conf` de
  l'image officielle, sans effet sur la sécurité.
- **govulncheck** relève GO-2026-5932 dans `golang.org/x/crypto/openpgp`,
  paquet que CyberSAS n'importe pas. Trivy ne trouve aucune vulnérabilité
  dans l'image, ni aucun secret dans le dépôt.

### Le fuzzing, poussé plus loin

Quatre nouvelles cibles s'ajoutent aux quatre du moteur :

- **`FuzzLireMessage1` et `FuzzLireMessage2`** : du fuzzing *différentiel*.
  Chaque suite d'octets est lue à la fois par notre poignée de main et par
  flynn/noise. Les deux doivent accepter et refuser exactement les mêmes
  messages, et lire la même chose. Et un message accepté doit être celui
  que l'appareil a vraiment écrit : sans sa clé, aucun ne passe.
- **`FuzzDecoder`** : le décodeur base64 n'accepte qu'une écriture par
  valeur, et accepte toujours celle-là.
- **`FuzzPolitique`** : une politique quelconque, même absurde, ne fait
  jamais tomber le serveur, et n'ouvre jamais de flux vers une adresse qui
  n'est pas un appareil.

Résultat, trois minutes par cible, **148 millions d'entrées au total, sans
une panique ni un désaccord** :

- lecture d'un paquet IPv4 : 20,8 millions ;
- lecture d'une trame relayée : 18,9 millions ;
- fenêtre anti-rejeu : 20,6 millions ;
- point d'entrée réseau du moteur : 20,7 millions ;
- message 1 de la poignée de main, contre flynn/noise : 6,2 millions ;
- message 2, contre flynn/noise : 1,0 million (chaque essai refait une
  poignée de main complète) ;
- décodeur base64 : 38,6 millions ;
- politique : 21,2 millions.

<a name="troisieme-audit"></a>
<img src="sections/audit/s09.png" alt="09 Troisième audit : l'appli" width="100%">

Deux jours après les deux premiers, l'appli Android a pris le vrai tunnel, puis les pouvoirs de l'admin : signer une demande, refuser, inviter, renommer, retirer, révoquer, avec la clé du verrou rangée dans la puce du téléphone. Environ 10 000 lignes que personne n'avait relues. Le 26 septembre 2026, trois relecteurs indépendants ont repris la même méthode, chacun sur un périmètre :

- le serveur et ses nouvelles routes (`internal/serveur/admin.go`, la base, le pont Go) ;
- l'Android natif : le manifeste, le coffre du verrou (`Coffre.kt`), le service VPN, le verrou de l'appli ;
- la frontière entre l'appli et le moteur : ce que le téléphone signe, le lien d'invitation, les révocations.

**Résultat** : 23 constats, dont 1 haut et 5 moyens. 22 sont corrigés, un est accepté et expliqué. Les tests Go passent de 73 à 79, plus 8 cibles de fuzzing, sous le détecteur d’accès concurrents.

### A1. Révoquer depuis l'appli signait une liste que le serveur avait choisie

- **Gravité** : haute. Prouvé par deux relecteurs, chacun avec un faux serveur.
- **Le problème** : pour révoquer un appareil, l'appli partait de la liste servie par le serveur, sans en vérifier la signature, puis signait le tout avec la clé du verrou. Un serveur piraté pouvait y glisser les clés d'appareils légitimes, qui se retrouvaient bannis pour de bon, signés par l'admin lui-même. Il pouvait aussi servir la version maximale : la suivante repassait à zéro, et plus aucune révocation n'était possible sans changer de verrou. `sas verrou revoquer` avait le même défaut.
- **Correction** : `client.AllongerRevocations` part de ce que l'admin sait sûr (la liste qu'il a retenue) et n'y ajoute la liste servie que si sa signature par le verrou est juste. Une version au plafond est refusée, jamais repassée à zéro. La ligne de commande refuse une liste actuelle mal signée, et affiche toute la liste qu'elle signe.
- **Tests** : `TestRevoquerIgnoreUneListeForgee`, `TestAllongerRevocationsPlafond`.

### A2. L'admin signait des champs qu'il n'avait jamais vus

- **Gravité** : moyenne, haute dans le modèle du verrou. Prouvé.
- **Le problème** : l'écran des demandes montrait le nom et l'empreinte. Au moment de signer, l'appli relisait le serveur et signait l'adresse, le groupe et le propriétaire de cette seconde lecture. La clé était la bonne, mais un serveur piraté pouvait faire signer `groupe admins` ou l'adresse de la maison au portable d'un membre. C'était la signature à l'aveugle du constat C1, revenue par l'appli.
- **Correction** : la carte de demande affiche ce qui sera signé (adresse, groupe, propriétaire, 90 jours). L'appli renvoie au moteur les fiches exactes qu'elle a montrées ; il relit le serveur et refuse si un seul champ diffère. L'adresse doit être dans le réseau retenu, ni celle du serveur, ni celle d'un autre appareil signé.
- **Tests** : `TestSignerCeQuiEstMontre`, `TestSignerAdresse`.

### A3. Un appareil jamais signé avait les pouvoirs d'admin

- **Gravité** : moyenne. Prouvé.
- **Le problème** : les routes d'admin ne regardaient que le compte Google du propriétaire. Un compte d'admin volé inscrivait un appareil qui, sans aucun certificat, pouvait lister le réseau, retirer des appareils et créer des invitations. Le modèle de menace disait le contraire.
- **Correction** : avec un verrou, il faut en plus un certificat en cours de validité, signé pour le groupe `admins`.
- **Test** : `TestRoutesAdmin`.

### A4. La clé de l'appareil partait dans les sauvegardes

- **Gravité** : moyenne.
- **Le problème** : rien n'excluait `etat.json`, qui contient la clé privée X25519 de l'appareil, de la sauvegarde Google ni du transfert d'un téléphone à l'autre. Deux téléphones pouvaient se retrouver avec la même identité.
- **Correction** : `allowBackup="false"`, et des règles d'extraction qui excluent le dossier du moteur, en sauvegarde comme en transfert.

### A5. L'empreinte n'était pas liée à la signature

- **Gravité** : moyenne.
- **Le problème** : la clé du coffre restait utilisable dix secondes après n'importe quelle authentification biométrique, y compris le déverrouillage du téléphone. L'empreinte demandée par l'appli n'était qu'une barrière de l'interface.
- **Correction** : une empreinte par opération, avec l'invite native d'Android liée au chiffreur du coffre (`CryptoObject`), en biométrie forte seulement. Ranger une nouvelle clé ne détruit plus l'ancienne avant d'avoir réussi ; une empreinte ajoutée au téléphone vide le coffre avec un message clair.

### A6. Un lien d'invitation forgé pouvait router tout l'Internet du téléphone

- **Gravité** : moyenne à basse. Prouvé.
- **Le problème** : le serveur choisissait seul le réseau routé dans le tunnel et le domaine de recherche. Un faux serveur répondait `0.0.0.0/0` : tout le trafic IPv4 du téléphone partait chez lui.
- **Correction** : une plage privée seulement, pas plus large qu'un /16, et un domaine sous `.internal`.
- **Test** : `TestReseauAcceptable`.

### A7 à A17. Les constats bas

- **A7. Écriture de `revocations.json`** : deux envois simultanés pouvaient faire reculer la liste ou mêler leurs octets. Écriture désormais exclusive, dans un fichier temporaire propre, synchronisée avant le renommage. Test : `TestRevocationsSimultanees`.
- **A8. Un crash sous Android 9 et 10** : le coffre utilisait une fonction d'Android 11. L'appli demande désormais Android 11 au moins.
- **A9. Le verrou de l'appli se contournait en reculant l'horloge** : le délai de grâce se compte maintenant sur l'horloge du système, qui ne recule pas.
- **A10. L'aperçu des applis récentes montrait l'appli déverrouillée** : il est masqué tant que le verrou de l'appli est actif, et un voile couvre l'appli dès qu'elle passe en arrière-plan.
- **A11. Le clavier atteignait les boutons sous le verrou** : le focus y est exclu, et le retour est bloqué tant que l'écran est verrouillé.
- **A12. La clé du verrou restait dans le presse-papiers** si l'import était annulé : il est vidé quoi qu'il arrive.
- **A13. Retirer et révoquer ne montraient que le nom**, que le serveur choisit : le dialogue et l'invite affichent aussi l'empreinte et l'adresse.
- **A14. Quitter pendant une synchronisation** pouvait réécrire `etat.json` juste après son effacement : `Quitter` attend la fin réelle du moteur, efface un fichier illisible, et une panique du moteur devient une erreur affichée au lieu de tuer l'appli.
- **A15. Les invitations** : les clés expirées n'étaient jamais purgées. Ménage à chaque création, et 50 clés vivantes au plus.
- **A16. Le lien d'invitation** : un lien sans clé du verrou ou avec une autorité de certification personnalisée est maintenant signalé clairement à l'écran de connexion.
- **A17. Nginx coupait à 16 Ko** ce que sasd accepte jusqu'à 256 Ko : les routes des certificats et des révocations montent à 256 Ko.

### Informations traitées

- **La clé du verrou en mémoire** : elle est manipulée en tableaux d'octets remis à zéro. Les copies en chaîne de caractères, imposées par le canal Flutter et par gomobile, ne peuvent pas être effacées : c'est écrit dans le code et dans [`verrou.md`](verrou.md).
- **Une publication sans clé de signature** était signée en silence avec la clé de débogage : la construction échoue désormais.
- **Un second démarrage du tunnel** pendant l'autorisation VPN laissait le premier appel sans réponse ; **quitter** pouvait laisser le coffre en place si le moteur échouait ; **signer une liste vide** rendait 0 sans erreur. Corrigés tous trois.

### Accepté et expliqué

- **A18. La clé privée de l'appareil est en clair dans son dossier.** Elle n'est enveloppée par aucune clé de la puce. Elle reste protégée par le bac à sable d'Android, le chiffrement du stockage et des droits réservés à l'appli, et elle ne part plus dans aucune sauvegarde (A4). L'envelopper demande de changer le format de stockage partagé avec le client Linux : c'est noté pour plus tard.

### Vérifié et trouvé sain

Le service VPN n'est joignable que par le système ; le lien `cybersas://` n'est traité que sur l'écran de connexion et demande un appui ; les canaux entre Flutter et Kotlin ne sont joignables que depuis l'appli ; aucun journal ne contient de clé, de jeton ni de graine ; le client refuse `http://`, ne suit aucune redirection et ne fait confiance qu'aux racines du système ; l'empreinte affichée et la clé signée viennent de la même chaîne, décodée de façon canonique ; les invitations sont consommées une seule fois, de façon atomique ; les routes d'admin refusent un non-admin (403) et relisent l'équipe à chaque requête.

<a name="ce-qui-reste"></a>
<img src="sections/audit/s10.png" alt="10 Ce qui reste" width="100%">

- **La clé privée de chaque appareil** n'est pas enveloppée par la puce du téléphone (constat A18) : protégée par Android, exclue des sauvegardes.
- **La clé du verrou n'a pas de secours** : perdre le téléphone de l'admin sans copie hors ligne gèle toute signature.
- **Aucun audit humain.** C'est la prochaine étape sérieuse avant de confier au
  VPN des données dont la fuite serait grave.
- **Les métadonnées** : le serveur voit qui parle à qui, quand et combien.
- **Les pages publiées par Nginx** sont déchiffrées sur le serveur, puisque le
  TLS s'y termine. Passer par le VPN garde le chiffrement de bout en bout.
- **Le premier contact** : un appareil à qui l'on ne donne pas la clé du verrou
  d'avance retient la première annoncée.
- **Google**, tiers de confiance pour les inscriptions.
- **La limite de débit de Nginx** voit toutes les connexions sous la même
  adresse quand Docker passe par son mandataire (IPv6 publié, par exemple). À
  régler au déploiement (`userland-proxy: false`).
- **L'adresse de retour après connexion web** n'est pas encodée : un « & »
  dans l'adresse d'origine la tronque. C'est un défaut fonctionnel, sans
  conséquence de sécurité, puisque la liste des domaines permis tient.
- **Pas de liaison directe** entre appareils, et **IPv4 seulement** dans le
  tunnel.

<br>

<div align="center">

<a href="../README.md"><img src="nav/readme.png" alt="Le README : CyberSAS en un coup d’œil." width="49%"></a>
<a href="protocole.md"><img src="nav/protocole.png" alt="Le protocole : les deux couches, Noise IK, les messages à l’échelle, le filtre." width="49%"></a>
<a href="verrou.md"><img src="nav/verrou.png" alt="Le verrou : la chaîne de confiance, le coffre du téléphone, signer et révoquer." width="49%"></a>
<a href="menaces.md"><img src="nav/menaces.png" alt="Le modèle de menace : ce qui est exposé, le serveur piraté, le téléphone volé." width="49%"></a>

</div>
