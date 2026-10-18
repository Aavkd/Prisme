# Phase 5 — Mobile, notifications et actions

Livraison locale des 8–9 septembre 2026. Schéma PostgreSQL **7**, OpenAPI **0.6.0**.
Le parcours sur **Samsung S24 physique** est vérifié avec Firebase réel : panne
HTTP locale → incident confirmé → FCM → ouverture de la bonne fiche → signature
Android après empreinte → approbation atomique d'un travail de test.

## Fonctionnement livré

- Abonnement Android par appareil authentifié, permission système, token renouvelé
  à la connexion suivante, désactivation explicite et révocation. Les tokens ne
  sont jamais retournés par les APIs de consultation ni inscrits dans l'audit.
- File PostgreSQL persistante, exclusion concurrente, déduplication des états,
  regroupement des changements d'un même incident, résolution, retry borné,
  respect de `Retry-After`, désactivation d'un token FCM `UNREGISTERED`.
- Horaires silencieux propres à l'appareil, fuseau IANA avec transitions DST.
  Une règle doit être explicitement configurée et porter `urgent: true`, avec
  sévérité critique, pour interrompre la nuit. L'inactivité seule ne devient pas
  urgente. Escalade facultative, délai explicite, cinq relances maximum ;
  acquittement/diagnostic/monitoring arrêtent les relances ordinaires.
- Messages FCM HTTP v1 minimaux : IDs opaques incident/événement, sévérité, état,
  titre fixe, route validée et indicateur urgent. Ni hostname, ni log, ni secret,
  ni plan de déploiement. Les états détaillés se relisent après authentification.
- Réception Android native en premier plan ou arrière-plan, notification privée,
  remplacement par incident, déduplication d'événement, groupes et canaux Android.
  Un lien `prisme://incidents/<id-opaque>` ouvre le même parcours. Un lien ne
  valide ni n'exécute jamais une action. Le mode Ne pas déranger reste souverain.
- Fiche issue d'une notification avec état actuel, chronologie, acquittement et
  diagnostic soumis à révision. Les autres contrôles restent dans Déploiements.
- Clé EC P-256 Android Keystore, signature SHA-256/ECDSA ASN.1 avec
  `BiometricPrompt.CryptoObject`, `AUTH_BIOMETRIC_STRONG` et autorisation pour
  chaque signature. Aucun repli PIN pour les signatures Android. Enrôlement
  uniquement depuis une session passkey de moins de cinq minutes.
- Vérification Windows WebAuthn via navigateur système, `userVerification=required`.
  Elle peut utiliser Windows Hello selon la passkey ; le serveur ne prétend pas
  distinguer empreinte et PIN WebAuthn. Le navigateur ne reçoit aucun bearer.
- Challenge de deux minutes lié à session, appareil, travail, révision et plan.
  Expiration, révocation, phrase et baseline sont recontrôlés ; preuve et décision
  sont consommées dans une transaction. Aucun simple booléen envoyé par le client
  n'autorise la production. L'annulation d'un prompt ne produit pas d'approbation.
- Cache opt-in avec `flutter_secure_storage` (Keystore Android / protection Windows),
  séparé par origine et appareil, limité à 50 résumés par domaine : machines,
  incidents et déploiements. Déverrouillage local, dates explicites et vue dédiée
  en lecture seule ; aucune session, clé fournisseur, longue sortie ni commande.
  Une déconnexion explicite ou une révocation constatée efface ce cache.
- Watchdog indépendant `cmd/watchdog` : processus et fichier d'état distincts,
  checks HTTPS bornés, confirmation panne/récupération, notification minimale de
  perte de visibilité, anti-doublon et reprise. La perte du control plane ne prouve
  jamais la panne d'un site. Aucun bouton de secours fonctionnel n'est simulé.

## Configuration

Les intégrations restent désactivées sans configuration. Les fichiers Firebase
fournis pour les essais et leurs copies Android sont ignorés par Git. La clé
privée de compte de service ne doit jamais entrer dans l'APK ou une image Docker.
Le contexte Docker est déjà limité aux sources Go.

Android : placer le `google-services.json` correspondant à
`dev.mantara.prisme_client` dans `apps/prisme_client/android/app/`, puis construire
le client. Le plugin Google Services est appliqué seulement si le fichier existe.
Le minimum Android est API 30. La dépendance de stockage 10.0.0 est compatible
avec la chaîne Flutter 3.38.5 ; la version 11 nécessitait un SDK Android 37 dont
l'identifiant de plateforme n'était pas compatible avec l'environnement testé.

Serveur : copier [le modèle](../deploy/notifications/config.example.json) dans un
fichier privé. Renseigner `credentials_file` et les IDs exacts des règles
d'observabilité qualifiées. Les horaires et l'activation sont configurés dans
**Notifications** sur chaque appareil. Une règle absente n'envoie aucune alerte.
`group_seconds` est compris entre 0 et 900 ; une escalade activée exige un délai
de 60 à 86 400 secondes. Les seuils des vrais pilotes restent à valider.

Définir `PRISME_NOTIFICATION_CONFIG` pour l'API, après migration explicite
`api -migrate`. Pour Compose local, préparer `.dev/notifications.json` avec
`credentials_file: /run/secrets/firebase_service_account` et placer une copie
privée du compte de service dans `.dev/firebase-service-account.json`, puis :

```powershell
docker compose -f deploy/compose.dev.yml -f deploy/compose.notifications.yml up --build --wait api
```

Les mutations production nécessitent toujours un manifeste activé, les préconditions
phase 4 et une session passkey. Android demande d'abord **Enregistrer la clé
biométrique** dans Notifications. Un appareil dont les biométries ont changé peut
nécessiter un nouvel enrôlement de clé. Cet enrôlement remplace uniquement une clé
signalée définitivement invalide par Android ; une clé encore utilisable est
conservée. La signature ne remplace jamais elle-même la clé et ne propose aucun
repli moins fort. Voir [l'invalidation Android Keystore](https://developer.android.com/reference/android/security/keystore/KeyPermanentlyInvalidatedException).

Watchdog : [configuration](../deploy/notifications/watchdog.example.json),
[unité systemd](../deploy/systemd/prisme-watchdog.service). Le provisionner sur
une machine indépendante avec répertoire d'état privé, accès HTTPS direct au
check de readiness, ses propres credentials FCM, des horaires silencieux indépendants et une liste de tokens explicitement
autorisée. Cette liste indépendante doit être mise à jour lors d'une rotation ou
révocation ; elle ne dépend pas de la base indisponible du control plane. La
qualification Cloudflare Access du chemin de santé et le déploiement sur VPS
restent ouverts. Ne pas lancer plusieurs watchdogs sur le même fichier d'état.

Le watchdog persiste chaque transition confirmée avant l'envoi. Une erreur pour
un destinataire n'empêche pas les autres de recevoir panne ou récupération ; les
retries conservent le même ID d'événement. Un nouvel état confirmé remplace les
envois anciens encore en attente, y compris pendant les horaires silencieux.
Un destinataire peut ainsi recevoir la récupération même si l'alerte de panne ne
lui est jamais parvenue. `enabled: false` reste respecté sans fuseau explicite ;
le fuseau par défaut est UTC.

## Vérifications et preuves

```powershell
go test ./...
go vet ./...
./tool/test-integration.ps1
dart run tool/generate_api.dart --check
cd apps/prisme_client
flutter analyze --no-pub
flutter test --no-pub
flutter build apk --debug
```

Essai physique explicite depuis la racine :

```powershell
./tool/test-mobile.ps1 -Device <serial-adb> -FCMCredentialsFile <fichier-prive.json>
```

Le test utilise un schéma SQL jetable, un serveur TLS temporaire et une panne HTTP
sur loopback. Une session initiale est créée par un authentifiant WebAuthn virtuel
de test ; elle est transférée uniquement dans le sandbox Android, puis supprimée.
La clé biométrique et sa signature sont **réelles sur le téléphone**. L'utilisateur
ouvre la notification réelle et confirme l'empreinte. La preuve est vérifiée côté
Go ; le job doit être `ready` avec une seule preuve consommée. Aucun worker de
déploiement n'est lancé par cet essai et aucun service pilote n'est modifié.

Résultat du 8 septembre : `TestPhaseFiveLiveAndroid` **PASS**, 123,45 s, sur le
S24 connecté et le projet Firebase fourni. La notification a ouvert la bonne fiche,
la signature Android a autorisé la décision et le stockage chiffré natif a été
relu après création d'une nouvelle instance. Le test de cache natif isole le
stockage en substituant seulement le dialogue de déverrouillage ; son contrôle
logiciel est testé séparément. Les journaux locaux sans secrets sont dans
`.dev/mobile-backend.log`. Le test retire son abonnement et ses entrées privées.

Les régressions couvrent DST, urgence explicite, acquittement, retries, token
expiré, session révoquée, signature forgée, réutilisation, autre travail, expiration,
absence de vérification utilisateur WebAuthn et session de développement. Les
tests Flutter couvrent le filtrage des liens, le cache cloisonné, l'effacement,
l'absence de commandes en cache, et les écrans 390/1 440 px.

Le 9 septembre : **24 tests Flutter réussis**, analyse sans problème, régressions
Go/PostgreSQL réussies, `go vet` et compilation Go réussis. Le contrat OpenAPI est
valide et le client généré à jour. Les compilations natives Windows et Android
ont réussi ; les builds de développement utilisent uniquement la CA locale
publique et demandent les identifiants à la connexion.

Le 10 septembre : corrections de l'enrôlement après invalidation biométrique et
des livraisons partielles du watchdog. Les régressions Go et `go vet` réussissent ;
**29 tests Flutter réussissent**, analyse sans problème. L'APK de développement
est recompilé avec succès et la CA locale publique. Les tests watchdog
exercent l'échec du premier ou dernier destinataire, les retries avec ID stable,
la récupération partielle suivie d'une nouvelle panne, la reprise d'un ancien
fichier d'état et la désactivation sans fuseau. Le test vérifie aussi que la
transition est sur disque avant l'envoi externe. Le S24 n'est plus présent dans
ADB lors de cette reprise : l'invalidation par ajout/suppression d'empreinte
n'a pas été exercée physiquement et reste à qualifier.

## Limites de qualification

Le critère d'interaction S24 est exercé sur une panne **locale réelle**, pas sur
un incident de production. Passkeys physiques sur le domaine final, publication
signée, Windows Hello natif et comportements S24 en Doze, après arrêt forcé,
avec notifications refusées et pendant les horaires silencieux restent à qualifier.
FCM peut retarder/perdre une livraison ; `sent` signifie acceptation fournisseur,
pas lecture. Les doublons après une acceptation suivie d'une panne SQL sont
neutralisés par l'ID d'événement Android, sans promesse de livraison exactement une fois.

Le serveur vérifie la possession de la clé Android enrôlée par une session passkey.
Il ne valide pas encore une chaîne d'attestation matérielle Android : l'exigence
biométrique dépend de l'application native de confiance, pas d'une attestation du
terminal distant. La rétention disque des historiques opérationnels reste le
chantier global antérieur. Le watchdog n'est pas installé sur le VPS. Le canal
de secours, secrets et fichiers restent phase 6. Le pilotage Codex/Claude proposé
reste soumis aux qualifications précédentes. Le NO-GO production de la tour demeure.

Références officielles consultées : [FCM HTTP v1](https://firebase.google.com/docs/cloud-messaging/send/v1-api),
[réception Android](https://firebase.google.com/docs/cloud-messaging/android/receive-messages),
[CryptoObject et biométrie](https://developer.android.com/identity/sign-in/biometric-auth),
[stockage sécurisé](https://pub.dev/packages/flutter_secure_storage).
