<div align="center">

<img src="banniere-verrou.png" alt="Document : le verrou. Un serveur qu'on n'a pas à croire : la clé qui signe qui entre, où elle vit, et comment s'en servir pas à pas. Ed25519, Keystore, StrongBox, empreinte." width="100%">

</div>

<br>

Le verrou protège le réseau contre son propre serveur. Avec lui, un serveur piraté ne peut ni s'intercaler entre deux appareils, ni leur ouvrir des ports, ni faire revenir un appareil banni. Le principe est décrit dans [le document du protocole](protocole.md) ; celui-ci dit où vit la clé et comment s'en servir.


<a name="la-regle-d-or"></a>
<img src="sections/verrou/s01.png" alt="01 La règle d'or" width="100%">

**La clé privée du verrou ne touche jamais le serveur.** Le serveur ne reçoit que des documents déjà signés, qu'il transmet sans pouvoir les modifier. La clé, elle, vit à deux endroits au plus, tous deux chez l'admin :

<img src="schemas/verrou/deux-places.png" alt="Deux places pour la clé du verrou. Sur le téléphone de l'admin : rangée chiffrée par une clé AES du Keystore, dans la puce StrongBox ; une empreinte l'ouvre pour une seule signature ; c'est là qu'on signe au quotidien, demandes et révocations. Sur l'ordinateur de l'admin : le fichier créé par sas verrou creer, idéalement chiffré et sauvegardé hors ligne ; c'est lui qui signe la politique, et qui sert de copie de secours." width="100%">

Dans le labo, tout tient sur une machine, clé comprise : c'est commode pour essayer, et c'est la seule exception.

<a name="la-chaine-de-confiance"></a>
<img src="sections/verrou/s02.png" alt="02 La chaîne de confiance" width="100%">

<img src="schemas/verrou/confiance.svg" alt="La chaîne de confiance du verrou. La clé du verrou, Ed25519, ne touche jamais le serveur. Elle signe trois sortes de documents : les certificats d'appareil (clé, adresse, groupe, 90 jours), la politique versionnée, et la liste des révocations versionnée. Le serveur sasd les transmet sans pouvoir y toucher : un octet changé et tous les refusent. Chaque appareil, fold8-tristan, laptop-lea et la maison, vérifie la signature puis applique. Chaque signature porte son contexte, pour qu'une signature faite pour l'une ne serve jamais pour une autre." width="100%">

Le serveur est un facteur : il porte les lettres, il ne peut pas les réécrire. Et comme chaque appareil garde la plus haute version vue de la politique et des révocations, il ne peut pas non plus ressortir une vieille lettre plus arrangeante.

<a name="le-coffre-du-telephone"></a>
<img src="sections/verrou/s03.png" alt="03 Le coffre du téléphone" width="100%">

<img src="schemas/verrou/coffre.svg" alt="Le coffre de la clé du verrou sur le téléphone de l'admin, en cinq temps : l'empreinte, par l'invite biométrique forte d'Android ; la clé AES du Keystore, dans la puce StrongBox, utilisable pour cette seule opération ; le fichier verrou.chiffre, déchiffré en mémoire ; le moteur Go, qui signe le document ; puis la clé est effacée, ses tableaux remis à zéro. Une empreinte ajoutée au téléphone invalide la clé AES. Le fichier chiffré copié ailleurs ne vaut rien." width="100%">

Le code est dans `Coffre.kt` et `MainActivity.kt`, côté Android de l'appli.

- **Une empreinte, une opération.** La clé AES du Keystore n'a aucune durée de validité : elle n'accepte que le chiffreur que l'invite biométrique vient d'authentifier. Déverrouiller le téléphone au doigt ne suffit donc pas à ouvrir le coffre dix secondes plus tard.
- **Une empreinte forte.** L'invite n'accepte que la biométrie de classe forte : pas un visage de classe faible, pas le code du téléphone.
- **Ce qu'on signe est affiché avant.** L'invite reprend le nom, l'empreinte, l'adresse et le groupe de l'appareil. Et le moteur ne signe que les fiches montrées : si le serveur en a changé un champ depuis l'affichage, rien n'est signé.
- **Pas de copie.** Le fichier chiffré et la clé de l'appareil sont exclus des sauvegardes Google et des transferts d'un téléphone à l'autre. La clé AES, elle, ne quitte jamais la puce.
- **Ranger la clé** : Réglages, Clé du verrou. On la colle, l'appli vérifie que c'est bien celle du verrou retenu, puis demande l'empreinte et la range. Le presse-papiers est vidé quoi qu'il arrive.

Ce qui reste, et qui est dit tel quel : la clé déchiffrée passe un instant dans une chaîne de caractères de Kotlin et de Go, qu'on ne peut pas effacer de la mémoire. Seuls les tableaux d'octets sont remis à zéro.

<a name="creer-le-verrou"></a>
<img src="sections/verrou/s04.png" alt="04 Créer le verrou" width="100%">

Sur l'ordinateur de l'admin :

```bash
sas verrou creer --fichier ~/.cybersas/verrou
```

La commande affiche la clé publique et son empreinte. La clé publique se pose sur le serveur, dans `etat/verrou/publique`. La clé privée reste où elle est : `sas verrou creer` ne remplace jamais une clé existante. Pour signer depuis le téléphone, on colle ensuite son contenu dans l'appli (Réglages, Clé du verrou).

<a name="signer-un-appareil"></a>
<img src="sections/verrou/s05.png" alt="05 Signer un appareil" width="100%">

**Depuis le téléphone**, c'est le chemin normal : la demande apparaît dans l'onglet Appareils. L'admin compare l'empreinte avec celle affichée sur le nouvel appareil, relit ce qu'il signe (adresse, groupe, propriétaire, 90 jours), puis pose son doigt.

<img src="schemas/verrou.svg" alt="Un serveur piraté glisse un intrus dans le réseau : le téléphone fold8-tristan vérifie le certificat, ne trouve pas de signature du verrou, et le refuse. L'ordinateur laptop-lea, signé par le verrou, est accepté." width="100%">

**Depuis l'ordinateur**, pour une machine sans écran comme le serveur de la maison :

1. L'admin crée une clé d'inscription sur le serveur : `docker compose exec sasd sasd cle --etiquette maison --nom maison`.
2. L'appareil s'inscrit, en recevant la clé publique du verrou d'avance : `SAS_CLE=sas-... sas rejoindre --serveur https://vpn.exemple.fr --verrou <clé publique du verrou>`. Il affiche sa propre clé publique et son empreinte.
3. L'admin lit cette clé **sur l'appareil lui-même** (à l'écran, ou par `sas etat`), jamais sur le serveur. Puis il signe cette clé-là, et elle seule :

```bash
ssh vps 'docker compose exec -T sasd sasd appareils --json' \
  | sas verrou signer --fichier ~/.cybersas/verrou --cle <clé publique de l'appareil> \
  | ssh vps 'docker compose exec -T sasd sasd signatures'
```

`sas verrou signer` affiche ce qu'il signe : nom, adresse, étiquette ou propriétaire, groupe et empreinte. Si l'empreinte ne correspond pas à celle affichée par l'appareil, il ne faut rien envoyer.

Les certificats valent 90 jours (`--duree` pour changer). Il faut les renouveler avant, de la même façon.

<a name="signer-la-politique"></a>
<img src="sections/verrou/s06.png" alt="06 Signer la politique" width="100%">

À chaque modification de `politique/politique.json`, augmenter son champ `version`, puis :

```bash
sas verrou politique --fichier ~/.cybersas/verrou < politique/politique.json > politique/politique.sig
```

Poser les deux fichiers sur le serveur. Les appareils refusent une politique mal signée, ou plus ancienne que la dernière qu'ils ont vue.

<a name="bannir-un-appareil"></a>
<img src="sections/verrou/s07.png" alt="07 Bannir un appareil" width="100%">

<img src="schemas/revocation.svg" alt="Révoquer un appareil depuis l'appli. Sur son téléphone fold8-tristan, l'admin touche Révoquer portable-test ; après son empreinte, le téléphone signe la liste de révocation v4 avec la clé du verrou. Le serveur sasd vérifie la signature et que v4 est plus récente que v3, la garde dans revocations.json et la transmet. La maison et laptop-lea retiennent la v4 ; portable-test est coupé. Les appareils n'acceptent jamais une liste plus ancienne." width="100%">

**Retirer** un appareil le coupe tout de suite, mais son certificat reste valide jusqu'à expiration : retiré seulement, il pourrait se réinscrire. Pour un appareil volé ou perdu, il faut le **révoquer**.

**Depuis le téléphone** : le détail de l'appareil, Révoquer. Le dialogue montre son empreinte et son adresse, pas seulement son nom, que le serveur choisit. Après l'empreinte, le téléphone part de la liste qu'il a retenue, y ajoute celle que sert le serveur **seulement si elle est signée par le verrou**, ajoute l'appareil, signe la version suivante et l'envoie. Le serveur vérifie la signature, que la version est plus récente, et que rien n'a été oublié, puis la garde dans `revocations.json`.

**Depuis l'ordinateur** :

```bash
ssh vps 'docker compose exec -T sasd sasd revocations' \
  | sas verrou revoquer --fichier ~/.cybersas/verrou --cle <clé publique de l'appareil> \
  | ssh vps 'cat > etat/sasd/revocations.json'
```

`sas verrou revoquer` refuse une liste actuelle qui n'est pas signée par ce verrou, et affiche toute la liste qu'il signe, pas seulement ce qui s'ajoute.

La liste est numérotée : les appareils gardent la plus récente vue, et n'acceptent jamais d'en revenir à une plus ancienne.

<a name="si-la-cle-est-perdue-ou-volee"></a>
<img src="sections/verrou/s08.png" alt="08 Si la clé est perdue ou volée" width="100%">

<img src="schemas/verrou/perte.png" alt="Perdue : il faut en créer une nouvelle et réinscrire tous les appareils avec --oublier, puisqu'ils refusent tout changement de verrou ; le téléphone seul, sans copie, gèle toute signature. Volée : même chose, et vite, car le voleur peut signer ce qu'il veut tant que les appareils font confiance à l'ancienne ; sur le téléphone, il lui faudrait d'abord le doigt de l'admin." width="100%">

C'est pour cela qu'un secours pour la clé du verrou est dans la feuille de route : en attendant, garder une copie chiffrée hors ligne du fichier de l'ordinateur.

<br>

<div align="center">

<a href="../README.md"><img src="nav/readme.png" alt="Le README : CyberSAS en un coup d’œil." width="49%"></a>
<a href="protocole.md"><img src="nav/protocole.png" alt="Le protocole : les deux couches, Noise IK, les messages à l’échelle, le filtre." width="49%"></a>
<a href="menaces.md"><img src="nav/menaces.png" alt="Le modèle de menace : ce qui est exposé, le serveur piraté, le téléphone volé." width="49%"></a>
<a href="audit.md"><img src="nav/audit.png" alt="L’audit de sécurité : trois relectures, 79 constats." width="49%"></a>

</div>
