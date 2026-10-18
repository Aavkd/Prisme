# Phase 10 — Mise en service de Prisme sur le VPS (journal)

Mission : Prisme en **supervision et alertes uniquement** sur `https://prisme.mantara.fr`
(VPS OVH `ovh-mantara`), menaces SSH, notifications S24, watchdog sur la tour et le
portable. Consignes : [PROMPT_DEPLOIEMENT_VPS.md](PROMPT_DEPLOIEMENT_VPS.md).
Une entrée par étape : fait, vérifié, reste. Rien n'est affirmé sans preuve.

## Prérequis — tour `DESKTOP-UCQ8SRH` (18/10/2026, lecture seule)

| Élément | Constat |
|---|---|
| Dépôt | `D:\MANTARA\Prisme`, branche `main`, à jour avec `origin/main` (`5e052c1`, contient `0f595db`). Un clone fait par erreur dans `D:\Prisme` (non modifié) est à supprimer par Alexy. |
| Disques | `C:` ≈ 2,4 Go libres (rien n'y est installé), `D:` ≈ 100 Go libres. |
| Outils présents | `git` 2.52, Docker Desktop 29.5.3 (moteur Linux/WSL2, données dans `D:\DOCKER\DockerDesktopWSL`), `gh` 2.102.0 **déjà installé et connecté** (compte `Aavkd`), contrairement au relevé initial. |
| Outils installés (portables, `D:\tools`) | Go 1.26.8 (SHA-256 vérifié), Flutter 3.38.5 / Dart 3.10.4 (clone du tag officiel), PostgreSQL 17.11 (binaires EnterpriseDB), Restic 0.19.1 (SHA-256 vérifié). Aucun installateur, aucune variable système : `. D:\tools\prisme-env.ps1` (ou `source /d/tools/prisme-env.sh`) place caches Go/pub et fichiers temporaires sur `D:`. |
| Accès VPS | `ssh -o BatchMode=yes ovh-mantara true` réussit depuis la tour. |
| Secrets Firebase | `D:\PrismeSecrets\` contient `google-services.json` et le compte de service ; ACL non héritées, `speee` et Système seulement. Fichiers non ouverts, non copiés. |

## Étape 1 — Préparation sur le poste

### Fait

- **CI GitHub rouge depuis le 18/10** (4 jobs sur 12 depuis `1966abb`) : causes trouvées et corrigées.
  - `TestCollectorExcludesSecretsAndReportsDockerFailure` (Linux) : `platform.platform()` lance
    `uname -p` en sous-processus sous Linux (et peut lancer `ver` sous Windows chargé) ; le faux
    `Popen` du test refusait tout ce qui n'est pas `docker`. Cause confirmée en rejouant l'ancien
    test sous Linux (`AssertionError`). Correctif : le test interroge `platform` avant de remplacer
    `Popen` (résultat mis en cache) ; le contrôle « seulement Docker » reste strict. Le collecteur
    réel n'est pas modifié.
  - `TestPhaseTenOnboardingPreviewAndFirstDeployment` (Linux et qualification) : le test attendait
    8 appels au pilote de déploiement, mais `observation` passe par `Driver.Verify` et `promote`
    reste interne au moteur : 6 appels sont corrects. Le test vérifie maintenant la séquence exacte
    et l'événement `observation` réussi. Code du moteur inchangé.
  - Job `observability` : le conteneur `migrate` (UID 10001) ne pouvait pas lire les secrets créés
    en `0600` par `dev-init` au nom de l'utilisateur du runner (montages de fichiers Compose).
    Correctif CI : groupe `10001` en lecture sur `.dev`. **Même piège en production**, voir plus bas.
  - Job `flutter` : `apps/prisme_client/test/onboarding_test.dart` reformaté par `dart format`.
- **Deuxième passage de la CI** (`d8c213e`) : `backend`, `qualification` et `android` repassent au
  vert ; deux nouveaux problèmes, sans lien avec les correctifs précédents, sont corrigés :
  - Alloy (UID 473) s'arrêtait : son volume nommé était monté sur `/var/lib/prisme-alloy`, chemin
    absent de l'image, donc créé au nom de root (`mkdir /var/lib/prisme-alloy/remotecfg: permission
    denied`, reproduit localement). Volume et `--storage.path` déplacés sur `/var/lib/alloy/data`,
    qui appartient à `alloy` dans l'image : Docker recopie ce propriétaire dans un volume neuf.
    **Concerne aussi la production.** La CI affiche désormais les journaux des conteneurs si
    l'exercice d'observabilité échoue.
  - `flutter build windows` du client : l'image `windows-latest` est passée à Visual Studio 2026
    (MSVC 14.51), où `<experimental/coroutine>`, encore utilisé par le plugin `local_auth_windows`,
    devient une erreur. `apps/prisme_client/windows/CMakeLists.txt` définit
    `_SILENCE_EXPERIMENTAL_COROUTINE_DEPRECATION_WARNINGS` (solution temporaire documentée par
    Microsoft) ; à retirer quand le plugin passera aux coroutines C++20.
- **Watchdog** (code + tests Go) :
  - `urgent` (défaut `false`) : renseigne `Payload.Urgent` (priorité FCM haute) **et** fait passer
    panne et récupération à travers les horaires silencieux (`Due(now, urgent, 0)`, auparavant
    `false` écrit en dur : un `urgent` seul n'aurait pas traversé la nuit).
  - `instance` (défaut vide) : libellé `^[a-z0-9][a-z0-9-]{0,15}$` ajouté au titre
    (`Prisme · tour`). Toute autre valeur rend la configuration invalide.
  - Android affichait toujours le titre fixe « Prisme » et ignorait `title` : `PrismeMessagingService`
    accepte désormais le titre **uniquement** pour la route `/watchdog` et s'il respecte la même
    expression ; sinon « Prisme ». Les titres d'incident restent fixes.
  - Choix conservé : la sévérité du watchdog reste `warning` et Android garde le canal ordinaire
    pour `/watchdog` (la perte du control plane ne prouve pas la panne d'un site, phase 5).
    Le son par défaut du canal ordinaire suffit à prévenir la nuit, hors mode Ne pas déranger.
  - `deploy/notifications/watchdog.example.json` et `docs/PHASE_5.md` mis à jour.
- Fins de ligne : l'ancien clone de la tour avait 76 fichiers en CRLF (antérieurs à
  `.gitattributes`), ce qui faisait échouer `sync_tokens.dart --check` localement. Fichiers
  réextraits depuis Git en LF, contenu identique.
- Binaires (dans `.dev/bin/`, ignoré par Git, avec `SHA256SUMS`) : `linux-amd64/{api,agent,probe,inventory}`,
  `windows-amd64/watchdog.exe`. Construits avec `CGO_ENABLED=0 -trimpath -ldflags="-s -w"`.

### Vérifié

Résultats réels sur la tour, le 18/10/2026 au soir :

| Contrôle | Résultat |
|---|---|
| `gofmt -l cmd internal`, `go vet ./...`, `go build ./...` | propres |
| `go test -count=1 ./...` avec PostgreSQL 17.11 local (`127.0.0.1:55432`, base `prisme_test`) | **15 paquets OK**, dont `internal/api` (≈ 200 s) ; tests « live » Compose/Caddy/charge ignorés faute de variables d'environnement, comme prévu |
| Tests watchdog (`-run Watchdog`) | 10/10 OK, dont 3 nouveaux (urgent + horaires silencieux + titre, valeurs par défaut, libellés refusés) |
| Test d'inventaire sous Linux (conteneur `golang:1.26.8`, Python 3.13) | OK ×3 ; l'ancienne version échoue (`AssertionError`) |
| `tool/test-backups-native.ps1` (PostgreSQL + Restic portables) | OK, dont `TestPhaseSevenLiveResticPostgresAndFiles` |
| `docker compose … config --quiet` (dev, observabilité, GitHub, notifications) | OK |
| `alloy validate` (`grafana/alloy:v1.19.2`) | OK |
| `tool/verify.ps1`, partie `packages/prisme_ui` (Windows) | analyse sans problème, 13 + 9 tests OK |
| `tool/verify.ps1`, partie `apps/prisme_client` | **non exécutable sous Windows ici** : `flutter pub get` exige le « Mode développeur » (liens symboliques des plugins), réglage système non modifié. Rejoué sous Linux (conteneur `ubuntu:24.04` + Flutter 3.38.5 officiel, sans `.dev` monté) : `generate_api --check` OK, formatage 0 changement, analyse sans problème, **44 tests OK** |
| `flutter build windows` / APK | non faits (pas de Mode développeur, pas de SDK Android sur la tour) ; le job CI `android` compile l'APK |
| Test `-race` | non fait sous Windows (exige cgo) ; couvert par le job CI `backend` |
| Kotlin (`PrismeMessagingService`) | non compilé localement ; job CI `android` **vert** sur `d8c213e` |
| Pile d'observabilité locale (Docker Desktop, `compose.dev` + `compose.observability`, Alloy corrigé) | tous les conteneurs `healthy` ; `tool/test-observability.py` OK : échantillons frais via Alloy, panne contrôlée → incident, rétablissement → résolution, 7 lignes de logs. Pile arrêtée ensuite (volumes conservés) |

### Différences dev → prod (à traiter avant ou pendant l'étape 2)

1. **Mode** : `PRISME_MODE=identity`, `PRISME_AUTH_ORIGIN=https://prisme.mantara.fr` (RP ID
   `prisme.mantara.fr`, définitif avant toute passkey durable), `PRISME_IDENTITY_SECRET_FILE`
   au lieu de `PRISME_DEV_SECRET_FILE`. Migration explicite `api -migrate` (déjà le cas en dev).
2. **Observabilité bloquante** : hors mode `development`, `observability.NewSource` exige
   HTTPS vers VictoriaMetrics et Loki ; avec `http://victoriametrics:8428` l'API **refuse de
   démarrer**. Options : (a) autoriser HTTP seulement vers un nom de service Docker sans point,
   sur réseau `internal`, par une variable explicite (petit changement de code + tests ; même
   logique que PostgreSQL `sslmode=disable` sur le réseau isolé) ; (b) TLS devant les deux stores
   (plus lourd). **Décision d'Alexy requise** — recommandation : (a).
3. **PKI interne** : `dev-init` crée une CA « development » valable 1 an et un certificat serveur
   valable **1 mois** (SAN `localhost`, `api`), sans outil de renouvellement. En production :
   CA dédiée générée sur le VPS, certificat serveur avec SAN `prisme-api` (nom vu par Caddy),
   durée plus longue et commande de réémission depuis la CA existante (à écrire), plus un rappel.
   Caddy : `reverse_proxy https://prisme-api:8443` avec `tls_trust_pool file <ca.pem>`.
4. **Droits des secrets sous Linux** : les conteneurs tournent en UID/GID 10001 ; les fichiers
   montés comme secrets Compose doivent être lisibles par ce compte (ex. `root:10001` en `0640`
   dans un dossier `0750`), jamais en lecture pour tous.
5. **Agent et mTLS** : l'API authentifie les agents par certificat client. Derrière Caddy, la
   connexion TLS s'arrête à Caddy : l'agent du VPS doit joindre l'API **directement** (réseau
   Docker interne ou `127.0.0.1` uniquement), jamais via `prisme.mantara.fr`.
6. **Réseaux et ports** : supprimer `ports: 127.0.0.1:8443` ; API sur `control` (interne) + `edge`
   (alias `prisme-api`) ; base, stores et Alloy sur `control` seulement.
7. **Alloy** : libellés `server=docker-desktop` / `environment=development` → `ovh-mantara` /
   `production` ; supprimer la cible `local-pilot` et le service `pilot` ; filtre de découverte
   limité à `prisme-dev` à revoir (cAdvisor sur les 35 conteneurs, **sans** collecter leurs logs
   par défaut) ; `discovery.relabel` pointe `alloy:12345` (nom de service à garder).
8. **Règles** : `Rule.Validate` n'accepte que les 9 métriques brutes de `RawMetric` ; les
   métriques dérivées déjà codées (`host_disk_percent`, `host_memory_percent`, …) sont refusées
   dans une règle. À corriger (avec tests) avant les règles disque/RAM de l'étape 4 et les règles
   SSH de l'étape 5.
9. **Notifications du control plane** : l'API a aussi besoin d'un compte de service FCM
   (`PRISME_NOTIFICATION_CONFIG`, `urgent` par règle) pour les incidents SSH ; en plus de celui
   du watchdog. Les deux limités à l'envoi FCM.
10. **Client** : construire avec `PRISME_IDENTITY=true` et `PRISME_API_URL=https://prisme.mantara.fr`.
    Le changement Android (titre du watchdog) impose un nouvel APK pour le S24 ; la tour n'a ni
    SDK Android ni JDK : à construire sur le portable ou à installer sur `D:` (plusieurs Go).
11. **Limites** : `mem_limit`/`pids_limit` du dev conviennent (API 256 Mo, Postgres 384 Mo,
    VictoriaMetrics 256 Mo, Loki 384 Mo, Alloy 512 Mo ≈ 1,8 Go). Monter `/` de l'hôte dans Alloy
    est nécessaire au collecteur `unix` mais reste en lecture seule.

### Reste

- Laisser la CI GitHub confirmer les deux derniers correctifs (Alloy, build Windows).
- Décision sur le point 2 (observabilité HTTP interne) avant d'écrire le compose de production.

## En attente d'accord

Aucune action sur le VPS, le DNS ou Firebase n'a été faite. L'étape 2 sera proposée ici
avec ses commandes exactes avant toute exécution.
