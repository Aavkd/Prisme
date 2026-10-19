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

- **CI GitHub rouge depuis le 18/10** (4 jobs sur 11 depuis `1966abb`) : causes trouvées et corrigées.
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
- **Troisième et quatrième passages** : l'exercice d'observabilité lisait le graphique une seule
  fois, or VictoriaMetrics masque les 30 dernières secondes des requêtes de plage et l'incident
  s'ouvre ≈ 27 s après la panne sur le runner : le script réessaie désormais (métriques et logs)
  et relit la révision avant l'acquittement. Ensuite, cAdvisor ne voyait aucun conteneur sur le
  runner : celui-ci utilisait encore le stockage `overlay2`, où cAdvisor doit lire
  `/var/lib/docker/image/overlay2/layerdb` (réservé à root). **Le VPS (Docker 29.6.1) et Docker
  Desktop utilisent le stockage containerd** (`overlayfs`, `io.containerd.snapshotter.v1`) : la CI
  active donc ce stockage et donne à Alloy le groupe du socket containerd, comme il faudra le
  faire en production. **CI entièrement verte (11/11) sur `11c25bb`.**
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
7. **Alloy** : sur le VPS, `/run/containerd/containerd.sock` est `root:root 0660` et
   `/var/run/docker.sock` appartient au groupe `docker` (986) : Alloy (UID 473) aura besoin des
   groupes `986` **et** `0` (`PRISME_DOCKER_SOCKET_GID=986`, `PRISME_CONTAINERD_SOCKET_GID=0`).
   Le groupe root reste un privilège à valider par Alexy, même si l'accès au socket Docker est
   déjà équivalent à root. Libellés `server=docker-desktop` / `environment=development` → `ovh-mantara` /
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

- Travail local encore à faire avant l'étape 2 (sans effet sur le VPS) : compose de production,
  commande de réémission du certificat serveur (point 3), correctif du point 2 selon la décision,
  correctif `RawMetric` (point 8), config Alloy de production.
- État de la tour en fin d'étape : Docker Desktop laissé démarré ; PostgreSQL de test arrêté
  (données dans `D:\tools\pgtest`) ; pile `prisme-dev` arrêtée (volumes conservés) ; `D:` ≈ 97 Go
  libres ; aucune erreur disque constatée. RAM de la tour : ≈ 1,8 Go libres sur 16 Go pendant les
  essais, dont ≈ 6,6 Go retenus par la VM de Docker Desktop (`vmmem`).

### Décisions d'Alexy (19/10/2026)

- Point 2 : **option (a)**, HTTP seulement vers un nom de service Docker interne.
- Point 7 : **groupe root accordé à Alloy** pour le socket containerd.

## Étape 2 — Installation sur le VPS (préparée en local, en attente d'accord)

### Fait (local, sans effet sur le VPS)

- **Option (a)** : `observability.NewInternalSource` n'accepte le HTTP qu'avec
  `PRISME_TELEMETRY_INTERNAL_HTTP=1` et vers un nom de service sans point
  (`^[a-z][a-z0-9-]{0,62}$`) ; IP, noms DNS, identifiants, chemins et requêtes restent refusés
  (test avec 10 adresses dangereuses). HTTPS inchangé.
- **PKI de production** : `cmd/pki` (`init`, `renew-server`, `ca-base64`) et image `pki` du
  Dockerfile. CA dédiée « Prisme control plane CA » de 5 ans, certificat API de 397 jours pour
  `prisme-api` et `localhost`, réémission sans toucher la CA ni les agents, refus de remplacer une
  PKI existante. Seul le certificat **public** de la CA peut être affiché (pour Caddy).
- **`deploy/production/`** : `compose.yml` (projet `prisme`, aucun port publié, réseau interne
  `control`, API seule sur `edge` avec l'alias `prisme-api`, journaux Docker bornés à 3 × 10 Mo,
  `restart: unless-stopped`), `ovh-mantara.env` (non secret), `config.alloy` (hôte, Alloy,
  conteneurs Prisme ; pas encore les 35 conteneurs ni les sites, prévus à l'étape 4),
  `observability.json` (3 portées, aucune règle), `init-secrets.sh`, `renew-server-cert.sh`,
  `edge-bloc.caddy`, `LISEZMOI.md`. La CI valide désormais ce compose, cette config Alloy et les
  deux scripts.
- **Simulation complète sur Docker Desktop** (secrets jetables, réseau `edge` local, Caddy 2.11.4
  comme sur le VPS), qui a trouvé et fait corriger trois défauts :
  - l'outil `pki` montait son dossier sur `/pki`, qui est le programme lui-même → `/secrets` ;
  - **course au démarrage de PostgreSQL** : pendant l'initialisation, un serveur temporaire
    n'écoute que sur le socket local, `pg_isready` le déclarait prêt et `migrate` échouait en TCP
    une seconde trop tôt. Santé testée en TCP (`-h 127.0.0.1`), en production et en dev ;
  - les fichiers écrits par l'outil `pki` sont en `0600` root : l'API ne les lit qu'après l'étape
    de droits `root:10001` / `0640` du script (reproduite).
- Résultats de la simulation : 5 services sains, `migrate` terminé en 0, aucun port publié ; à
  travers Caddy, `/readyz` → 200 `{"status":"ready"}`, `/auth` → 200, `/v1/me`, `/v1/servers`,
  `/v1/incidents` → 401 en anonyme ; avec une autre CA de confiance, Caddy répond **502**
  (« certificate signed by unknown authority ») ; métriques hôte, Alloy et conteneur API
  présentes dans VictoriaMetrics, logs de l'API dans Loki ; mémoire réelle ≈ 340 Mo
  (API 7, Postgres 40, VictoriaMetrics 83, Loki 68, Alloy 142). Simulation démontée ensuite.

### À décider avant l'étape 3 (exposition)

**Limitation de débit derrière Caddy** : l'API compte les requêtes anonymes par adresse IP
source (120/min). Derrière Caddy, tous les anonymes ont l'adresse de Caddy : un robot à plus de
120 requêtes/min bloquerait la connexion par passkey d'Alexy (les sessions ouvertes et `/readyz`
restent épargnés). Correctif proposé : ne croire l'en-tête `X-Forwarded-For` que si la connexion
vient du réseau `edge` (Caddy remplace cet en-tête par l'IP réelle du client). À valider.

### Commandes proposées (aucune n'est lancée sans le « oui » d'Alexy)

Toutes depuis la tour. `P` désigne `docker compose --env-file deploy/production/ovh-mantara.env -f deploy/production/compose.yml`.

| # | Commande | Effet |
|---|---|---|
| 0 | `ssh ovh-mantara 'free -m; df -h /; docker ps --format "{{.Names}} {{.Status}}"'` | Relevé avant (lecture seule). |
| 1 | `ssh ovh-mantara 'ssh-keygen -q -t ed25519 -N "" -C prisme-deploy@ovh-mantara -f ~/.ssh/deploy_prisme && printf "\nHost github-prisme\n  HostName github.com\n  User git\n  IdentityFile ~/.ssh/deploy_prisme\n  IdentitiesOnly yes\n" >> ~/.ssh/config'` | Nouvelle clé dédiée sur le VPS, bloc ajouté à la fin de `~/.ssh/config` (rien d'autre modifié). |
| 2 | `ssh ovh-mantara 'cat ~/.ssh/deploy_prisme.pub' > D:\tools\tmp\deploy_prisme.pub` puis `gh repo deploy-key add D:\tools\tmp\deploy_prisme.pub --repo Aavkd/Prisme --title "ovh-mantara (lecture seule)"` | Clé **publique** déclarée sur GitHub en lecture seule (action externe visible). |
| 3 | `ssh ovh-mantara 'git clone -q --filter=blob:none --sparse git@github-prisme:Aavkd/Prisme.git /opt/projects/prisme && cd /opt/projects/prisme && git sparse-checkout set deploy cmd internal'` | Crée `/opt/projects/prisme` (clone partiel). |
| 4 | `ssh ovh-mantara 'cd /opt/projects/prisme && sudo sh deploy/production/init-secrets.sh'` | Crée `secrets/` (`root:10001`, `0640`) ; compile l'outil `pki` (≈ 2 min, ≈ 1 Go de RAM pendant la compilation, image Go téléchargée). Rien n'est affiché. |
| 5 | `ssh ovh-mantara 'cd /opt/projects/prisme && sudo $P up -d --build --wait'` | Télécharge Postgres, VictoriaMetrics, Loki, Alloy (≈ 1,5 Go de disque) et démarre la pile `prisme`. Aucun port publié, DNS et Caddy **non touchés** : rien n'est exposé. |
| 6 | Vérifications en lecture seule : état et ports des conteneurs `prisme`, `docker stats`, `free -m`, liste des 35 autres conteneurs comparée au relevé 0, requête VictoriaMetrics/Loki par un conteneur `curl` jetable sur `prisme_control`. | Critère de l'étape 2. |

Retour arrière si besoin : `sudo $P down` (conserve les volumes). Supprimer `/opt/projects/prisme`
ou la clé de déploiement seulement avec l'accord d'Alexy.

### Exécution (19/10/2026, 00:25 → 00:39 UTC, accord d'Alexy)

| # | Résultat |
|---|---|
| 0 | Avant : RAM disponible 5 292 Mo, swap 1 014 Mo, `/` 24 Go utilisés (48 libres), **36** conteneurs actifs (35 + `thielen-maquettes-web-1`, nouveau site déployé en parallèle par ailleurs, créé à 00:11). |
| 1 | `~/.ssh/deploy_prisme` (ed25519, `0600`) et alias `github-prisme` ajouté en fin de `~/.ssh/config` ; copie de sauvegarde `~/.ssh/config.bak-20261019-0025xx` faite avant. |
| 2 | Clé de déploiement GitHub n° 166885280 « ovh-mantara (lecture seule) », `read-only`. |
| 3 | `/opt/projects/prisme` : clone partiel (`deploy`, `cmd`, `internal` + fichiers racine), `8c42c65`, 2,4 Mo, arbre Git propre. |
| 4 | `init-secrets.sh` réussi en ≈ 1 min : `secrets/` `root:10001`, dossiers `0750`, fichiers `0640` ; `ubuntu` ne peut pas lister le dossier ; rien affiché. RAM disponible minimale pendant la compilation : **4 752 Mo** ; swap max 1 014 Mo. |
| 5 | `up -d --build --wait` réussi : postgres, victoriametrics, loki, alloy, api `healthy`/démarrés, `migrate` terminé en **0**. RAM disponible minimale : 5 136 Mo ; swap 1 027 Mo. |
| 6 | Voir ci-dessous. |

Vérifications (étape 6) :

- **Aucun port publié** par Prisme (les `5432/tcp`, `3100/tcp`, `8428/tcp` affichés sont seulement
  exposés sur le réseau interne). La seule écoute hôte sur les ports testés est
  `127.0.0.1:5432`, qui appartient à `arpent-postgres-1` (préexistant).
- API : `Prisme API listening with TLS (identity)` ; réseaux `edge` (alias `prisme-api`) et
  `prisme_control`. Rien n'est exposé : DNS et Caddy non touchés.
- Mémoire réelle de Prisme ≈ 200 Mo (API 4, Alloy 72, Postgres 59, Loki 34, VictoriaMetrics 33)
  pour 1,8 Go de limites.
- Alloy : une seule erreur, bénigne (`/run/udev/data` absent, propriétés udev des disques) ;
  **cAdvisor lit bien les conteneurs via containerd** avec les groupes 986 et 0.
- VictoriaMetrics : mesures de l'hôte (`/` à 35,6 %), d'Alloy et du conteneur API (≈ 5 Mo) ;
  Loki : logs de l'API reçus (requêtes par un conteneur `curl` jetable sur `prisme_control`).
- **Autres conteneurs** : mêmes 36 noms et images qu'avant, tous `Up`. `thielen-maquettes-web-1`
  a redémarré à **00:25:12** par son propre projet (dossier modifié à 00:25), avant la première
  commande modifiante de cette étape (relevé 0 à 00:25:04) : activité extérieure, sans lien.
- Après : RAM disponible **5 078 Mo** (−214 Mo), swap **1 026 Mo** (+12 Mo, stable), `/` 26 Go
  utilisés (+2 Go d'images et de cache de compilation), 41 conteneurs actifs.
- Incident de méthode, sans effet sur le VPS : mon enveloppe de relevé mémoire de l'étape 4 a vu
  la fin de son script lue par `docker compose run` ; la boucle de relevé est restée active
  ≈ 10 min, puis a été arrêtée et son fichier `/tmp` supprimé. Corrigé (entrée `/dev/null`).

**Critère de l'étape 2 atteint** : conteneurs sains, aucun port publié, RAM de Prisme sous 2 Go,
autres conteneurs intacts.

## Étape 3 — Exposition `prisme.mantara.fr` (préparation)

### Limitation de débit derrière Caddy (correctif validé par Alexy le 19/10/2026)

- `Server.TrustedProxy` / `PRISME_TRUSTED_PROXY_CIDR` : seul un réseau **privé** d'au moins /8
  est accepté (sinon l'API refuse de démarrer). Si le pair TCP en fait partie, l'adresse du client
  est le **dernier** élément de `X-Forwarded-For` (Caddy le remplace par l'IP de son client) ;
  valeur invalide ou absente → adresse du pair. Hors de ce réseau, ou sans réglage, l'en-tête
  n'est jamais lu. La même adresse sert à la limitation de débit et à `audit_events.remote_address`.
- Production : `PRISME_TRUSTED_PROXY_CIDR=172.22.0.0/16` (réseau `edge` du VPS) dans
  `ovh-mantara.env`.
- Tests Go : quotas séparés par client derrière le proxy, dernier saut retenu, en-tête invalide
  → adresse du proxy, en-têtes ignorés hors réseau et sans réglage ; suite complète OK avec
  PostgreSQL.
- Simulation (Docker Desktop, réseau `edge` `10.222.0.0/24`, Caddy 2.11.4) :

  | Client | Résultat |
  |---|---|
  | A, sur `edge`, via Caddy, `X-Forwarded-For` falsifié différent à chaque requête | 120 × 401 puis 429 : Caddy écrase l'en-tête, A garde son quota |
  | B, sur `edge`, via Caddy, pendant que A est bloqué | **401** (avant le correctif : 429, quota partagé) |
  | C, sur `prisme_control` (hors `edge`), direct sur l'API, en-tête falsifié | 120 × 401 puis 429 : en-tête ignoré |
  | Audit des rejets | adresses réelles de A (`10.222.0.4`), B (`.5`) et C (`172.24.0.7`) ; jamais celle de Caddy ni une adresse falsifiée |

- **Risque résiduel accepté** : un conteneur déjà présent sur `edge` (les sites clients) peut
  joindre `prisme-api` directement et choisir son `X-Forwarded-For`, donc obtenir de nouveaux
  quotas anonymes. Les plafonds globaux (6 000 requêtes/min, 32 simultanées) et
  l'authentification restent inchangés.

### DNS (19/10/2026)

Vérifié depuis le VPS : `prisme.mantara.fr` répond encore `64.29.17.65` et `216.198.79.1` sur
1.1.1.1, 8.8.8.8 **et** les serveurs Cloudflare qui font autorité. C'est l'enregistrement
générique `*.mantara.fr` (un sous-domaine inexistant donne les mêmes adresses) : l'enregistrement
explicite **A `prisme` → 51.255.163.21, nuage gris** n'existe pas encore. Bloc Caddy **non activé**.

### Exécution (19/10/2026, accord d'Alexy : A tout de suite, B dès que le DNS résout)

**A — mise à jour de la pile (10:54 UTC)** : `git pull` (`7f203cc`) puis `up -d --build --wait`.
Seuls `migrate` (terminé en 0) et `api` recréés ; API `healthy`, `PRISME_TRUSTED_PROXY_CIDR=172.22.0.0/16`
chargé. RAM disponible minimale pendant la compilation 4 457 Mo, swap 1 028 Mo ; 36 autres
conteneurs identiques, aucun redémarré.

**DNS** : surveillé chaque minute depuis le VPS ; enregistrement `prisme` visible à **13:17:02 UTC**
sur 1.1.1.1 et 8.8.8.8, puis revérifié sur les deux serveurs Cloudflare qui font autorité
(`51.255.163.21`, aucun AAAA).

**B — Caddy (13:17:46 UTC)** :

- Référence avant : codes HTTP des 27 adresses servies par le Caddy mutualisé (joint en local).
- Sauvegarde `/opt/projects/edge/Caddyfile.bak-20261019-131746` (512 lignes) ; bloc ajouté en
  écrivant **en place** (539 lignes) ; certificat public de la CA du VPS (empreinte SHA-256
  `629F3A2C…5F99`) ; `caddy validate` → `Valid configuration` ; `caddy reload` OK. Le bloc avait
  aussi été validé à l'avance en local avec le Caddyfile du VPS.
- Certificat Let's Encrypt obtenu par HTTP-01 en ≈ 3 s : émetteur `Let's Encrypt YE2`, valable
  jusqu'au **17/01/2027**, SAN `prisme.mantara.fr`.

Vérifications depuis la tour, à travers Internet :

| Requête | Résultat |
|---|---|
| `/readyz`, `/healthz` | 200 (`{"status":"ready"}`) |
| `/auth` (page de connexion passkey) | 200 |
| `/v1/me`, `/v1/servers`, `/v1/incidents`, `/v1/observability`, `/v1/devices` en anonyme | **401**, aucune donnée |
| `http://` | 308 vers `https://` |
| En-têtes | `Strict-Transport-Security: max-age=31536000; includeSubDomains`, `X-Content-Type-Options`, `X-Frame-Options`, `Referrer-Policy` ; pas d'en-tête `Server` |

- Les 27 autres adresses répondent **exactement** comme avant le rechargement ; nouvelle adresse
  `prisme.mantara.fr/` → 404 (l'API ne sert rien à la racine).
- Audit : les rejets portent les **vraies adresses** des visiteurs (dont l'IP publique de la tour),
  jamais celle de Caddy (`172.22.0.4`). Quelques minutes après la publication du certificat, deux
  robots avaient déjà sondé l'API (33 et 13 requêtes rejetées) : le correctif de limitation de
  débit était nécessaire.
- RAM disponible après : 4 967 Mo, swap 1 026 Mo.

### Reste pour clore l'étape 3 (actions d'Alexy)

1. Lire le secret d'initialisation **soi-même** sur le VPS, sans le copier ailleurs :
   `ssh ovh-mantara 'sudo cat /opt/projects/prisme/secrets/identity-secret'`.
2. Lancer le client sur le portable (Flutter 3.38.5), depuis `apps/prisme_client` :
   `flutter run -d windows --dart-define=PRISME_IDENTITY=true --dart-define=PRISME_API_URL=https://prisme.mantara.fr`
   (aucune CA à fournir : le certificat Let's Encrypt est reconnu par le système). Bouton passkey
   → page `/auth` dans le navigateur → « Créer ou récupérer » → secret d'initialisation → passkey.
   Conserver hors de l'appareil les codes de secours proposés.
3. S24 : ajouter l'appareil depuis la session du portable (ticket d'ajout), avec un APK construit
   pour la même URL. Le nouvel APK (titre du watchdog) se construit sur le portable, la tour n'a pas
   le SDK Android.

Tant que ces connexions ne sont pas faites, le critère « connexion passkey OK depuis le portable et
le S24 » reste **non vérifié**.

### Toujours à faire par Alexy

- Supprimer le clone fait par erreur : `Remove-Item -Recurse -Force D:\Prisme`.

## Étape 4 — Collecte réelle du VPS (préparée en local, en attente d'accord)

### Conception (contraintes du moteur de règles)

Une règle lit **une seule série brute** de la liste blanche, avec ses horodatages d'origine ;
une donnée absente donne l'état « inconnu », jamais un incident. D'où :

- **Correctif `RawMetric`** : `RuleMetric` accepte une série brute ou sa transformation
  **échantillon par échantillon** (horodatage conservé). Ajouts : `node_filesystem_avail_bytes`,
  `node_memory_SwapFree_bytes`, `prisme_containers_problem`, `prisme_drill`, et deux
  transformations : `probe_ssl_expiry_days` (jours avant expiration TLS) et
  `prisme_host_metrics_age_seconds` (âge du fichier écrit sur l'hôte : un fichier lu par le
  collecteur `textfile` porte l'heure de lecture, une vieille valeur passerait sinon pour
  fraîche). Les taux et ratios (`host_cpu_percent`, `host_disk_percent`…) restent réservés aux
  graphiques : leurs horodatages sont synthétiques. Tests Go ajoutés.
- **Conteneurs arrêtés** : cAdvisor ne voit que les conteneurs en marche ; un conteneur arrêté
  disparaîtrait sans alerte. `host_metrics.py` (Python 3 standard, `docker ps`/`docker inspect`
  seulement, toutes les 30 s) compte les conteneurs arrêtés de façon inattendue (politique de
  redémarrage ou code ≠ 0), en redémarrage, en mauvaise santé (en marche seulement) ou
  redémarrés par Docker depuis moins de 10 min. Écrit en Python : `date` du VPS est la
  version Rust (uutils). Tests unitaires Python (5), exécutés aussi par la CI.
- **Disque** : seule la série `mountpoint="/"` est gardée (le VPS a aussi `/boot` et
  `/boot/efi`) ; règle vérifiée sur des exemples.
- **Sites** : 28 adresses servies par le Caddy mutualisé (relevé du 19/10, dont
  `thielen.mantara.fr` et 4 redirections `www`), check HTTPS toutes les 30 s par le DNS public
  (Cloudflare pour les domaines proxifiés, comme arpent.online), redirections non suivies.
  Réussite : 2xx, 3xx, 401, 403 ; échec : 404, 5xx, délai, TLS invalide. `prisme.mantara.fr`
  est vérifié sur `/readyz`. Alloy rejoint pour cela un réseau `egress` dont il est le seul
  membre (jamais `edge`, où son interface HTTP serait visible des sites).
- **Conteneurs (cAdvisor)** : tous les conteneurs, libellés par projet/service Compose (ou
  libellés `prisme.*`), métriques CPU, mémoire, réseau, OOM ; logs limités à Prisme.
- **Agent d'inventaire** : conteneur `agent` (profil `agent`, sans socket Docker) qui lit
  l'inventaire écrit sur l'hôte par `prisme-inventory.timer` (compte `prisme-collector`) et
  joint l'API en direct (`prisme-api` ajouté comme alias sur le réseau interne : le mTLS
  s'arrête à Caddy). Enrôlement après la première passkey d'Alexy.

Règles (62, dont 56 pour les sites) — sévérités sans `urgent` (réservé à l'étape 5) :

| Règle | Seuil (ouverture / résolution) | Délai | Sévérité |
|---|---|---|---|
| `VPS-RAM-LOW` | RAM disponible < 1 Gio / > 1,5 Gio | 5 min | warning |
| `VPS-DISK-ROOT` | `/` libre < 15 % / > 18 % (85 % / 82 % occupés) | 10 min | warning |
| `VPS-SWAP-HIGH` | swap libre < 25 % / > 37,5 % (aujourd'hui ≈ 50 % utilisé) | 10 min | warning |
| `VPS-CONTAINERS` | conteneurs en problème > 0 / = 0 | 1 min / 2 min | warning |
| `VPS-HOST-COLLECTOR` | âge de la collecte hôte > 180 s / < 90 s | 1 min | warning |
| `PRISME-DRILL` | signal d'exercice > 0 / = 0 | immédiat | information |
| `SITE-<adresse>-HTTP` (×28) | check en échec | 90 s / 60 s | critical |
| `SITE-<adresse>-TLS` (×28) | certificat < 14 j / > 21 j | 5 min | warning |

### Simulation locale (Docker Desktop, 19/10/2026)

- `host_metrics.py` exécuté pour de vrai contre le Docker local : a trouvé un défaut (un
  conteneur arrêté proprement gardait l'état de santé `unhealthy` de son dernier passage et était
  compté) → santé prise en compte pour les seuls conteneurs en marche ; test ajouté.
- VictoriaMetrics : 28 checks de sites **tous réussis** (dont 401 roquebrune, 302
  prototype-sad, arpent.online par Cloudflare, redirections `www`), échéance TLS la plus proche
  44,8 jours ; swap, conteneurs en problème (5 vrais conteneurs locaux arrêtés en erreur),
  âge de la collecte (≈ 38 s) et signal d'exercice présents ; cAdvisor : 20 conteneurs, aucun
  doublon. Aucune série disque sur Docker Desktop (pas de `/` dans sa VM) : attendu, le VPS en a.
- **Exercice** : drapeau créé à 14:00:35 → incident `PRISME-DRILL` ouvert à 14:01:01 ;
  supprimé à 14:01:14 → résolu à 14:02:01 ; événements `detected`, `resolved`.
- États : 60 règles saines, `VPS-CONTAINERS` en échec (à raison), `VPS-DISK-ROOT` inconnue
  (attendu en local). Alloy : 217 Mo (limite 512), 5 % CPU.

### Commandes proposées (aucune n'est lancée sans le « oui » d'Alexy)

`P` = `docker compose --env-file deploy/production/ovh-mantara.env -f deploy/production/compose.yml`,
depuis `/opt/projects/prisme`.

| # | Commande | Effet |
|---|---|---|
| 0 | relevé RAM/swap/conteneurs (lecture seule) | Référence. |
| 1 | `git pull --ff-only` | Met à jour le clone. |
| 2 | `sudo sh deploy/production/install-host.sh` | Crée le groupe `prisme-observation` (10002) et le compte système `prisme-collector` (groupes `docker`, `prisme-observation`), installe `/usr/local/bin/prisme-inventory`, `/usr/local/lib/prisme/host_metrics.py`, `/etc/prisme`, 4 unités systemd, active 2 minuteurs. |
| 3 | `sudo $P up -d --build --wait` | Recrée `alloy` (nouvelle config, réseau `egress`) et `api` (nouvelles règles). |
| 4 | vérifications en lecture seule | Une série disque, swap, conteneurs en problème (attendu : 0), 28 checks, mesures des ≈ 42 conteneurs, état des 62 règles, mémoire d'Alloy, autres conteneurs intacts. |
| 5 | `sudo touch /etc/prisme/drill`, attendre l'incident, puis `sudo rm /etc/prisme/drill` | Critère « une règle de test se déclenche puis se résout », sans toucher à aucun service. |
| 6 | (après la passkey d'Alexy et son jeton) `sudo $P --profile agent up -d --build --wait agent` | Enrôle l'agent ; le VPS et ses conteneurs apparaissent dans la vue Serveurs. |
