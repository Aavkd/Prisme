# Phase 0 — fondation locale

Livraison du 6 septembre 2026. Le **critère de sortie logiciel local est vérifié** :
Flutter Windows s'authentifie auprès de Go, lit PostgreSQL et consulte les agents ;
un agent s'enrôle et émet un heartbeat mTLS sans accès à l'hôte.

Prisme et `dev.mantara.prisme_client` restent provisoires. Le nom définitif et le
NO-GO production de la tour restent ouverts. Ce document ne valide ni publication,
passkeys de production, correction du poste Windows, ni intégration des pilotes.

## Livrables et décisions

| Livrable phase 0 | Emplacement / statut |
|---|---|
| Diagnostic tour | `DIAGNOSTIC_TOUR_WINDOWS.md`, préexistant et réutilisé |
| Runtime | ADR 0001 : WSL2 local, VM Linux cible sous conditions matérielles |
| Nom | Provisoire, selon réponse explicite de l'utilisateur |
| Monorepo et conventions | `apps`, `packages`, `cmd`, `internal`, `docs/CONVENTIONS.md` |
| Design tokens | Socle `prisme_ui` existant réutilisé |
| OpenAPI et agent | `contracts/openapi.json`, `contracts/agent-v1.md`, client Dart généré |
| Menaces et secrets | `docs/THREAT_MODEL.md`, génération privée `.dev`, ACL Windows |
| Compose local | `deploy/compose.dev.yml` : PostgreSQL, migration, API et profil agent |
| CI | `.github/workflows/ci.yml` : Go/race/PostgreSQL, OpenAPI, Flutter/Windows |
| Connecteurs IA | `docs/CONNECTOR_QUALIFICATION.md` : interfaces qualifiées, contrôle non activé |
| Secours | ADR 0002 : conception indépendante, implémentation en phase 6 |

## Démarrer sur Windows

Depuis la racine, Docker Desktop en mode Linux actif, Flutter 3.38.5 et Go capables
de télécharger le toolchain 1.26.8 :

```powershell
./tool/dev.ps1
dart run apps/prisme_client/tool/smoke.dart
./tool/run-client.ps1
```

`dev.ps1` crée `.dev` seulement s'il est absent et refuse de remplacer une identité.
Il restreint les ACL de ce dossier à l'utilisateur courant et SYSTEM. Le secret
de connexion est `.dev/bootstrap-secret` : ouvrir localement ce fichier et saisir
sa valeur dans le champ masqué ; ne pas l'afficher dans les sorties partagées.
La CA **publique seulement** est intégrée au lancement du client. Aucun secret
n'est transmis dans les arguments, compilé dans l'application ou conservé sur disque
par Flutter. Les sessions durent 60 minutes, sans renouvellement automatique.

La stack expose uniquement `https://localhost:8443` sur IPv4 loopback. PostgreSQL
reste sur le réseau Docker interne. Le réseau client séparé permet la publication
loopback sous Docker Desktop ; le réseau des données et l'agent restent internes.

Pour l'agent conteneurisé, créer un token de dix minutes, puis lancer le profil :

```powershell
dart run apps/prisme_client/tool/smoke.dart --invite-only
docker compose -f deploy/compose.dev.yml --profile agent up --build -d agent
```

Le volume `prisme-dev_agent_identity` conserve sa clé et son certificat au redémarrage.
Le token n'est plus nécessaire pour une identité déjà enrôlée. Les certificats
agents expirent après 24 heures : renouvellement automatique différé. Pour un nouvel
agent de test, utiliser un nouveau répertoire/volume et un nouveau token ; ne pas
effacer implicitement une identité existante. Les fichiers de serveur expirent
après un mois ; le renouvellement de CA/serveur est également une opération locale
explicite, hors lancement normal du script.

Pour arrêter sans perte de données :

```powershell
./tool/dev.ps1 -Stop
```

Cette commande ne retire pas les volumes et n'agit pas sur les autres stacks.
Aucune commande automatique de purge, réparation Windows ou mutation VPS n'est fournie.
Les services n'ont pas de politique de redémarrage au boot de l'hôte en phase 0.

## Android

Le runner Android est présent avec permission Internet, backup désactivé et identité
de développement. Sur un téléphone autorisé et connecté par ADB :

```powershell
adb reverse tcp:8443 tcp:8443
./tool/run-client.ps1 -Device <identifiant-adb>
```

Le téléphone utilise ainsi `localhost` et le même certificat. Ne pas publier le
port sur le LAN pour contourner ADB. La validation native Android, les licences/
outils SDK et la signature de distribution restent à effectuer ; le runner généré
utilise la signature debug pour ses builds locaux. Aucun APK de production n'est livré.

## Vérifier

```powershell
./tool/verify-foundation.ps1 -Integration
./tool/verify.ps1
dart run apps/prisme_client/tool/smoke.dart
$env:PRISME_SMOKE_DIR = (Resolve-Path .dev).Path
cd apps/prisme_client
flutter test integration_test/local_backend_test.dart -d windows
```

Le test PostgreSQL utilise `deploy/compose.test.yml`, projet `prisme-tests`, port
loopback 55432 et données temporaires en tmpfs. Il crée un schéma isolé, le retire
en fin de test puis arrête ce projet de test. Sans `PRISME_TEST_DATABASE_URL`,
`go test ./...` ignore explicitement le test SQL/TLS : cette commande seule ne
prouve pas l'intégration. CI exécute ce test avec une base éphémère et `-race`.

Régénération des modèles **et méthodes** Dart :

```powershell
dart run tool/generate_api.dart
dart run tool/generate_api.dart --check
```

Le transport TLS, les délais, la gestion des sessions et les erreurs sont dans
`apps/prisme_client/lib/api/connection.dart` ; ils ne sont pas générés et restent
hors du package de présentation. Les statuts, dates et erreurs affichés viennent
du backend réel. Aucune action d'infrastructure n'est disponible en phase 0.

## Preuves acquises

- Migration PostgreSQL relançable, authentification/refus, sessions expirées et
  révoquées, demandes JSON strictes, idempotence, concurrence d'enrôlement, CSR sans
  escalade, mTLS, reconnexion et audit sans secret : tests Go sur PostgreSQL réel.
- API Compose saine ; appel réel par le client Dart généré ; agent Go réellement
  enrôlé, heartbeat consulté, session rejetée après logout.
- Agent Compose observé sous `10001:10001`, racine en lecture seule,
  `cap_drop: ALL`, `no-new-privileges:true`, sans montage hôte ni port entrant.
- Test Flutter **Windows natif réussi** : connexion via champ masqué, lecture
  authentifiée du backend, affichage de l'état, déconnexion.
- Tests Flutter client : connexion, perte réseau, session révoquée à 390 px et
  rejet des origines HTTP/non conformes ; analyse statique sans problème.
- Contrat validé par `openapi-spec-validator 0.7.2`.
- Vérification finale : `go vet`, tests Go et intégration PostgreSQL/TLS réussis ;
  13 tests du socle UI + 9 du catalogue + 2 du client réussis, génération à jour,
  analyse et formatage propres ; build **release Windows x64 réussi**.

Le workflow CI est ajouté, mais aucune exécution GitHub distante n'est revendiquée.
Pas de validation Android physique, audit complet d'accessibilité, qualification
de production 24/7, session IA pilotée, ou secours VPS déployé dans cette phase.
