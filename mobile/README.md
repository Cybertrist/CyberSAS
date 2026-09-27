# L'appli Android de CyberSAS

L'appli qui rejoint le réseau, ouvre le tunnel et, pour l'admin, signe qui entre. Flutter pour l'interface, le moteur Go du tunnel embarqué ([`../pont`](../pont)), et le service VPN d'Android. Pensée pour le téléphone comme pour le Fold déplié.

Ce qu'elle fait, en images : [le README du dépôt](../README.md). Comment elle garde la clé du verrou : [`docs/verrou.md`](../docs/verrou.md).

## Ce qu'il y a dedans

- `lib/main.dart` : le démarrage, la navigation, le verrou de l'appli (empreinte à l'ouverture, délai de grâce compté sur l'horloge du système).
- `lib/ecrans/` : un fichier par écran. `connexion` (rejoindre par un lien d'invitation), `accueil` (le tunnel et l'interrupteur), `appareils` (la carte du réseau), `detail` (un appareil, renommer, retirer, révoquer), `demandes` (signer ce qu'on voit), `ajout` (inviter), `reglages`, `verrou`.
- `lib/donnees.dart` : l'état de l'appli, le vrai réseau ou le réseau d'exemple.
- `lib/moteur.dart` : le canal vers Kotlin, et de là vers le moteur Go.
- `lib/composants.dart`, `lib/dessins.dart`, `lib/theme.dart`, `lib/icones.dart` : la charte (cyan `#31E7FD`, bleu `#01B9FD`, fonds sombres) et le tunnel du logo, animé.
- `android/app/src/main/kotlin/fr/cybersas/cybersas/` : `MainActivity.kt` (les canaux, l'invite biométrique liée au coffre), `TunnelService.kt` (le service VPN), `Coffre.kt` (la clé du verrou, chiffrée par le Keystore).

## Construire

Le moteur d'abord, après toute modification du code Go du client (il faut Go, le SDK et le NDK Android, et `gobind` dans le `PATH`) :

```bash
bash scripts/moteur.sh      # depuis la racine : mobile/android/app/libs/moteur.aar
```

Puis l'appli, en APK par architecture : un APK universel aurait un `versionCode` plus petit, et Android refuserait la mise à jour.

```bash
flutter build apk --release --split-per-abi --target-platform android-arm64
```

La version de publication est signée avec la clé de `android/key.properties`, qui n'est pas dans le dépôt. Sans ce fichier, la construction échoue plutôt que de signer en silence avec la clé de débogage : Android n'installe une mise à jour que par-dessus une version signée de la même clé.

La version se change à deux endroits : `pubspec.yaml` et `versionAppli` dans `lib/donnees.dart`.

## La démo

`--dart-define=DEMO=true` construit une autre appli, installable à côté de la vraie (`fr.cybersas.cybersas.demo`, « CyberSAS démo ») : un réseau d'exemple, aucun serveur, aucune clé.

## Les captures

Les captures du README sont des tests : chaque écran, sur téléphone, sur le Fold plié et déplié.

```bash
flutter test --update-goldens test/captures_test.dart
```

Elles atterrissent dans `test/captures/`, d'où `docs/tools/captures.sh` compose les planches.
