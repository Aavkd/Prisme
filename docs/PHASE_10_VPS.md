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

### Exécution (19/10/2026, 15:05 → 15:18 UTC, accord d'Alexy : commandes 0 à 5)

| # | Résultat |
|---|---|
| 0 | Avant : RAM disponible 5 070 Mo, swap 1 025 Mo, `/` 36 %, 41 conteneurs actifs (36 + 5 Prisme), tous `Up`. |
| 1 | `git pull` → `b941784`. |
| 2 | `install-host.sh` OK : groupe `prisme-observation` (10002), compte `prisme-collector` (UID 997 ; groupes 10002 et `docker`), minuteurs actifs, premiers fichiers écrits (`host.prom` : 0 conteneur en problème, 41 en marche sur 42 ; `inventory.json` en `0640`). RAM disponible minimale 4 521 Mo. Avertissement sans effet : la compilation en root sur le dépôt de `ubuntu` n'a pas pu lire le numéro de commit. |
| 3 | `up -d --build --wait` : réseau `prisme_egress` créé, Alloy et API recréés, sains. RAM disponible minimale 5 075 Mo ; swap 1 035 Mo. |
| 4 | Voir ci-dessous. |
| 5 | **Exercice** : drapeau créé à 15:08:26 → incident `PRISME-DRILL` ouvert à 15:09:19 ; supprimé à 15:09:20 → résolu à 15:09:49 ; événements `detected`, `resolved` ; `/etc/prisme` vide ensuite. |

Écart trouvé et corrigé pendant la vérification (même commandes 1 et 3, `de6c0b3`) : le libellé
`instance` des mesures de l'hôte et d'Alloy valait le nom de machine du conteneur Alloy, qui change
à chaque recréation ; pendant 5 minutes, les règles de l'hôte voyaient deux séries et restaient
« inconnues ». Il est fixé à `ovh-mantara`. Alloy monte `config.alloy` **fichier par fichier** :
après `git pull`, il a fallu `up -d --force-recreate --no-deps alloy` pour qu'il lise la nouvelle
version (consigne ajoutée au LISEZMOI). Les anciennes séries ont expiré à 15:16:56.

Vérifications finales (15:17 UTC) :

- VictoriaMetrics : 28 checks de sites, tous réussis ; échéance TLS la plus proche 44,7 jours ;
  **une seule** série disque (`/`, 48,9 Go libres) et de swap ; conteneurs en problème 0 ; âge de
  la collecte hôte ≈ 5 s ; signal d'exercice 0 ; cAdvisor : 41 conteneurs, aucun doublon.
- **62 règles sur 62 saines** ; seul incident : l'exercice, résolu.
- 36 autres conteneurs : mêmes noms et images qu'avant, aucun redémarrage.
- Mémoire de Prisme : Alloy 256 Mo (limite 512), Loki 84, VictoriaMetrics 73, Postgres 34, API 9.
  Alloy : 4 à 13 % d'un cœur. Charge moyenne du VPS 0,6 à 1,0 (4 vCPU).
- Après : RAM disponible 4 887 Mo, swap 1 033 Mo (+8 Mo depuis le relevé 0, stable).

**Critère de l'étape 4** : la collecte et les règles sont en place et vérifiées, la règle de test
s'est déclenchée puis résolue. **Non vérifié** : la vue Serveurs (VPS et conteneurs avec mesures
fraîches) — elle attend l'enrôlement de l'agent, donc la première passkey d'Alexy (commande 6).

## Étape 5 — Menaces SSH (en service ; observation 24 h en cours)

### Relevés en lecture seule (19/10/2026, 15:55 → 16:20 UTC)

Scripts lancés depuis la tour par `ssh ovh-mantara 'bash -s'`, sans rien écrire sur le VPS
(`D:\tools\vps-scripts\ssh-releves*.sh`). RAM disponible 4 678 à 4 855 Mo, swap 1 032 Mo stable.

**Source** : journal `ssh.service` (il remonte au 14/07, donc 30 jours complets). `sshd` n'accepte
que les clés (`passwordauthentication no`, `permitrootlogin no`, `maxauthtries 3`). Depuis
OpenSSH 9.8, toutes les lignes d'échec viennent du programme **`sshd-session`**, pas de `sshd`.

**Définition d'un échec** (une fois par connexion) : `Invalid user … from` (nom inexistant) ou
`Connection closed by | Disconnected from | Connection reset by authenticating user … [preauth]`
(compte existant, clé refusée). Les simples sondages (délai dépassé, négociation impossible,
connexion coupée sans nom) sont comptés à part : ≈ 240 par jour.

| Mesure | 7 jours (12 → 19/10) | 30 jours (19/09 → 19/10) |
|---|---|---|
| Échecs | 17 886 (7 564 noms inexistants, 10 322 comptes existants, dont `root` 9 830 et `ubuntu` 433) | 76 954 |
| Par jour | 2 000 à 3 250 (40 à 115 IP distinctes) | 1 200 à 4 800 |
| 10 min alignées : médiane / 90e / 99e centile / max | 14 / 33 / 97 / 135 | 13 / 35 / 94 / 236 (30/09) |
| Fenêtres de 10 min au-dessus de 50 | 31 | 236 |

Le « ≈ 1 000 par jour » des consignes correspond aux seuls noms inexistants ; avec les comptes
existants, c'est 2 à 3 fois plus. Les pointes viennent d'une seule IP (≈ 90 à 110 échecs en
10 min), que fail2ban devrait couper.

Alertes qu'aurait produites une mesure « échecs des 10 dernières minutes » relevée toutes les
30 s, sur les 30 jours (résolution sous la moitié du seuil) :

| Seuil | sans délai | maintenu 5 min | maintenu 10 min | maintenu 15 min |
|---|---|---|---|---|
| > 50 | 131 | 116 | 65 | 18 |
| > 100 | 41 | 35 | 5 | 1 |
| > 150 | 7 | 3 | **0** | 0 |
| > 200 | 1 | 1 | 0 | 0 |

**Le seuil décidé (> 50 en 10 min) déclencherait ≈ 4 alertes par jour sur le bruit normal.**

**fail2ban et connexions réussies** : un défaut de configuration de fail2ban a été trouvé
(il ne coupe pas les rafales comme prévu) et plusieurs connexions réussies par clé viennent
d'adresses à vérifier. Détails, adresses et commandes **hors dépôt** jusqu'à la correction
(note locale de la tour, `D:\tools\vps-scripts\etape5-notes-sensibles.md`). Une enquête en
lecture seule n'a trouvé aucune trace d'installation (comptes, clés, tâches planifiées, services,
ports d'écoute) ; les connexions vérifiées sont cohérentes avec l'activité d'Alexy sur GitHub
aux mêmes heures. Aucune adresse d'Alexy n'a été bannie ni n'a d'échec en 30 jours.

### Décisions d'Alexy (19/10/2026)

1. Adresses connues : celles qu'il a confirmées ; le fichier d'IP connues reste **sur le VPS
   seulement**, hors dépôt. Trois lieux d'abord non reconnus ont été vérifiés puis confirmés
   par Alexy (voir ci-dessous) ; seules la tour et une ligne fixe sont dans la liste.
2. Connexions mobiles (adresse changeante) : **alerte urgente à chaque connexion**, pas de plage
   autorisée.
3. Clé de déploiement GitHub Actions (adresse changeante) : **alerte urgente à chaque usage**,
   pas d'exception.
4. Seuil d'avertissement : **plus de 150 échecs sur les 10 dernières minutes, maintenu 10 min**,
   résolution sous 75 (0 alerte sur les 30 jours de bruit réel). Remplace le « > 50 » initial.
5. fail2ban : correction validée puis appliquée le 19/10 à 20:23 UTC (voir ci-dessous).

### Connexions d'abord non reconnues (19/10/2026, lecture seule)

Trois lieux de connexion par la clé personnelle d'Alexy n'étaient pas reconnus au premier abord
(un réseau d'entreprise, une ligne fixe Orange, un réseau professionnel ; 5 adresses entre le
11/09 et le 14/10). Enquête en lecture seule avant toute autre action :

- **Aucune trace d'installation** : comptes avec shell (`root`, `ubuntu`, `ke2c-copie` créé le
  18/09), fichiers `authorized_keys` modifiés seulement le 18/09, `sudoers.d` d'origine, aucune
  crontab utilisateur, unités systemd récentes = celles de Prisme, aucun port d'écoute inattendu.
- **Commandes** (journal `sudo`, par tranche horaire) : audits du VPS (`ufw status`, `du`,
  `sshd -T`, journaux), installation de la copie de sauvegarde KE2C vers la tour (la tour s'est
  connectée deux minutes après), mise en place du projet `offres`.
- **Recoupement GitHub** : le dépôt `Aavkd/mantara-offres` et sa clé de déploiement ont été créés
  sur le compte d'Alexy **à la même minute** que la clé et le clone correspondants sur le VPS ;
  commits d'Alexy aux mêmes heures les autres jours.
- Alexy a **confirmé** que les trois lieux sont les siens. Aucune clé à remplacer. Ils restent
  hors de la liste des adresses connues : une connexion depuis ces lieux ouvrira une alerte
  urgente, qu'il reconnaîtra.

### Correction de fail2ban (19/10/2026, 20:23 UTC, accord d'Alexy)

**Défaut** (aujourd'hui corrigé) : le fichier du paquet `jail.d/defaults-debian.conf` imposait
`journalmatch = _SYSTEMD_UNIT=ssh.service + _COMM=sshd`, alors que les échecs sont écrits par
`sshd-session` : fail2ban n'en voyait aucun (0 sur 1 491 depuis son installation). De plus,
`mantara-sshd.local` avait été écrit 6 s après son démarrage et jamais rechargé (ban de 10 min
au lieu d'1 h).

Commandes exécutées (annoncées à l'identique avant le « oui » ; adresses masquées ici) :

```sh
sudo cp -p /etc/fail2ban/jail.d/mantara-sshd.local /etc/fail2ban/mantara-sshd.local.bak-20261019
printf '%s\n' 'journalmatch = _SYSTEMD_UNIT=ssh.service + _COMM=sshd + _COMM=sshd-session' 'ignoreip = 127.0.0.1/8 ::1 <tour> <ligne fixe>' | sudo tee -a /etc/fail2ban/jail.d/mantara-sshd.local
sudo fail2ban-client -t && sudo fail2ban-client reload
```

Résultat : test de configuration réussi, rechargement OK ; filtre
`_SYSTEMD_UNIT=ssh.service + _COMM=sshd + _COMM=sshd-session`, `bantime` 3 600 s, deux adresses
d'Alexy ignorées (en plus de `127.0.0.0/8` et `::1`). 42 conteneurs identiques avant et après,
RAM disponible 5 284 Mo, swap 17 Mo. Retour arrière : remettre la copie, puis
`sudo fail2ban-client reload`. Débannir : `sudo fail2ban-client set sshd unbanip <adresse>`.

Vérification : à 20:31 UTC, fail2ban avait détecté **8 échecs sur les 8** comptés dans le journal
sur les 10 dernières minutes (essai à blanc du collecteur ci-dessous). À 20:33 UTC, 10 min après le
rechargement : 7 échecs venant de 4 adresses, aucune n'atteignant 5 échecs, donc aucun ban attendu ; RAM
disponible 5 273 Mo, swap 17 Mo.

### Redémarrage du VPS (19/10/2026, 20:05 UTC)

Redémarrage programmé par un autre chantier, avec l'accord d'Alexy (« Redémarrage prévu pour
charger le noyau 7.0.0-38 »), sans lien avec Prisme. Noyau 7.0.0-38, plus de redémarrage en
attente. **Prisme est reparti seul** : 5 services démarrés (API et PostgreSQL `healthy`), minuteurs
actifs, collecte hôte fraîche, `/readyz` → 200. Swap revenu à 17 Mo.

### Préparation locale (sans effet sur le VPS)

- **`deploy/production/host/ssh_metrics.py`** (Python 3 standard), lancé toutes les 30 s par
  `prisme-ssh-metrics.timer` sous `prisme-collector`, **sans accès à Docker**
  (`InaccessiblePaths`) et avec le groupe `adm` pour cette seule unité (le compte n'est pas
  modifié). Il lit le journal de `ssh.service`, `systemctl is-active fail2ban` et
  `/var/log/fail2ban.log` (+ `.1`), et écrit `ssh.prom` dans le dossier déjà lu par Alloy
  (aucune modification d'Alloy). Mesures : échecs sur 10 min, connexions réussies et connexions
  depuis une adresse inconnue sur 30 min, fail2ban actif, adresses bannies, détections de
  fail2ban sur 10 min, fail2ban « aveugle » (actif mais aucune détection pour 20 échecs ou plus),
  exercices, heure de génération. **Aucune adresse dans les mesures** (donnée personnelle,
  cardinalité) : une connexion inconnue est décrite dans le journal de l'unité.
- Liste des adresses connues : `/etc/prisme/ssh-known-ips`, hors dépôt ; sans elle, l'unité ne
  démarre pas (`ConditionPathExists`) et une liste invalide fait échouer la collecte. En cas
  d'échec, `ssh.prom` n'est pas réécrit : son âge déclenche `VPS-SSH-COLLECTOR`.
- **Exercices** : quatre drapeaux dans `/etc/prisme` (`drill-ssh-unknown-login`,
  `drill-fail2ban-down`, `drill-ssh-failures`, `drill-fail2ban-blind`), qui ne peuvent
  qu'ajouter des alarmes.
- Code : 7 métriques ajoutées à `RawMetric`, transformation `prisme_ssh_metrics_age_seconds` ;
  **67 règles** (5 nouvelles) :

  | Règle | Seuil (ouverture / résolution) | Délai | Sévérité |
  |---|---|---|---|
  | `VPS-SSH-UNKNOWN-LOGIN` | connexions inconnues sur 30 min > 0 / = 0 | 2 relevés / 1 min | critical (urgent à l'étape 6) |
  | `VPS-FAIL2BAN-DOWN` | fail2ban actif < 1 / = 1 | 1 min / 1 min | critical (urgent à l'étape 6) |
  | `VPS-SSH-FAILURES` | échecs sur 10 min > 150 / ≤ 75 | 10 min / 5 min | warning |
  | `VPS-FAIL2BAN-BLIND` | aveugle > 0 / = 0 | 15 min / 5 min | warning (proposée en plus) |
  | `VPS-SSH-COLLECTOR` | âge de la collecte SSH > 180 s / < 90 s | 1 min / 1 min | warning |

- **Tests** : 9 tests Python (formats réels du journal et de fail2ban, adresses connues et
  IPv6, liste invalide refusée, bans après redémarrage de fail2ban, aveuglement, exercices) ; 2
  tests Go qui chargent **la vraie configuration de production** et rejouent les relevés : le
  pire bruit mesuré (141 pendant 30 min, pointe à 236) n'ouvre rien, 151 pendant 9 min non plus,
  une attaque à 400 ouvre à 10 min pile et se résout 5 min après ; connexion inconnue ouverte en
  15 s, résolue 1 min après la fin de la fenêtre ; rechargement de fail2ban (45 s) sans incident,
  arrêt détecté en 1 min.
- **Essai à blanc sur le VPS** (lecture seule, sortie à l'écran, rien d'écrit) avec `setpriv`
  sous `prisme-collector` + `adm` : échecs 8, fail2ban 8 détections, 12 connexions dont 0
  inconnue, fail2ban actif, 0 ban (identique à `fail2ban-client`). Sans `adm` : refus
  (`journal unreadable`), rien n'est écrit.

### Commandes proposées (aucune n'est lancée sans le « oui » d'Alexy)

`P` = `docker compose --env-file deploy/production/ovh-mantara.env -f deploy/production/compose.yml`,
depuis `/opt/projects/prisme`. Adresses masquées ici, données en clair à Alexy dans le chat.

| # | Commande | Effet |
|---|---|---|
| 0 | relevé RAM/swap/conteneurs (lecture seule) | Référence. |
| 1 | `git pull --ff-only` | Met à jour le clone. |
| 2 | `printf '%s\n' '# Adresses SSH connues, validées par Alexy le 19/10/2026' '<tour>' '<ligne fixe>' \| sudo tee /etc/prisme/ssh-known-ips && sudo chown root:prisme-observation /etc/prisme/ssh-known-ips && sudo chmod 0640 /etc/prisme/ssh-known-ips` | Crée la liste des adresses connues (hors dépôt). |
| 3 | `sudo sh deploy/production/install-host.sh` | Installe `ssh_metrics.py` et `prisme-ssh-metrics.{service,timer}`, active le minuteur ; réinstalle les deux autres collecteurs à l'identique. |
| 4 | `sudo $P up -d --build --wait` puis `sudo $P up -d --force-recreate --no-deps --wait api` | Recompile l'API (nouvelles métriques) et lui fait relire `observability.json` (67 règles). Alloy n'a pas besoin d'être recréé. |
| 5 | vérifications en lecture seule | `ssh.prom`, 9 séries dans VictoriaMetrics, 67 règles saines, autres conteneurs intacts. |
| 6 | `sudo touch` des quatre drapeaux d'exercice, attendre ≈ 16 min, puis `sudo rm` des quatre | Critère de l'étape 5 : quatre incidents ouverts puis résolus sur métriques synthétiques, sans toucher à SSH ni à fail2ban. Aucune notification encore (étape 6). |
| 7 | observation 24 h (lecture seule) | Aucun faux positif. |

Retour arrière : `sudo systemctl disable --now prisme-ssh-metrics.timer` et
`sudo rm /var/lib/prisme-metrics/ssh.prom` (les règles passent à « inconnu »).

### Exécution (19/10/2026, 20:57 → 21:20 UTC, accord d'Alexy : commandes 0 à 6)

Scripts : `D:\tools\vps-scripts\etape5-installation.sh` (0 à 4), `etape5-verif.sh` (5),
`etape5-exercice.sh` (6), `etape5-observation.sh` (7).

| # | Résultat |
|---|---|
| 0 | Avant : 42 conteneurs actifs, RAM disponible ≈ 5,2 Go, swap 17 Mo. |
| 1 | `git pull` → `e4ec993`. |
| 2 | `/etc/prisme/ssh-known-ips` : 2 adresses (tour, ligne fixe), `root:prisme-observation` `0640`. |
| 3 | `install-host.sh` : `ssh_metrics.py` et `prisme-ssh-metrics.{service,timer}` installés, minuteur actif ; première exécution réussie (code 0) ; les deux autres collecteurs réinstallés à l'identique. |
| 4 | API recompilée puis recréée (`migrate` terminé en 0, API `healthy`). RAM disponible minimale **4 750 Mo**, swap max 17 Mo ; après : 5 201 Mo, 42 conteneurs, aucun nom ni image changé hors Prisme. |
| 5 | Voir ci-dessous. |
| 6 | Exercice : voir ci-dessous. |

Vérifications (commande 5, 20:58 UTC) :

- Collecteur SSH toutes les 30 s, sans erreur ; `ssh.prom` : 6 échecs sur 10 min, 4 connexions
  dont 0 inconnue, fail2ban actif, **2 adresses bannies** (fail2ban bloque de nouveau les
  récidivistes), 10 détections de fail2ban sur 10 min, aveugle 0.
- VictoriaMetrics : les 9 mesures, **une série chacune** (portée `vps-host`).
- **67 règles sur 67 saines**, 0 incident ouvert.

Exercice (commande 6) — drapeaux créés à 20:58:44, supprimés à 21:14:05 :

| Règle | Ouverte | Délai | Résolue | Délai après suppression |
|---|---|---|---|---|
| `VPS-SSH-UNKNOWN-LOGIN` | 20:59:17 | 33 s | 21:15:32 | 1 min 27 s |
| `VPS-FAIL2BAN-DOWN` | 20:59:57 | 1 min 13 s | 21:15:32 | 1 min 27 s |
| `VPS-SSH-FAILURES` | 21:09:02 | 10 min 18 s | 21:19:32 | 5 min 27 s |
| `VPS-FAIL2BAN-BLIND` | 21:14:02 | 15 min 18 s | 21:19:32 | 5 min 27 s |

Événements `detected` puis `resolved` pour chacune ; `/etc/prisme` ne contient plus que
`ssh-known-ips` ; ensuite 67 règles saines, 0 incident ouvert, RAM disponible 5 165 Mo, swap
17 Mo. Aucune notification envoyée (pas encore configurées : étape 6).

Premier passage de l'observation (21:20 UTC) : 0 incident SSH depuis l'exercice, aucune
connexion inconnue, aucune erreur de collecte, maximum de 7 échecs sur 10 min hors exercice ;
fail2ban : 3 bans au total depuis le redémarrage.

**Critère de l'étape 5** : incidents créés et résolus sur métriques synthétiques — **atteint**.
**Non vérifié** : 24 h d'observation sans faux positif (relancer `etape5-observation.sh` après le
20/10 à 21:20 UTC). Les deux règles critiques ne deviendront `urgent` qu'avec la configuration
des notifications (étape 6).

## Étape 6 — Alertes S24 et veilleurs (préparée en local, en attente d'Alexy)

### Constats (lecture du code, 19/10/2026)

- L'API lit `PRISME_NOTIFICATION_CONFIG` et **refuse de démarrer** si la clé FCM manque :
  les notifications sont donc un complément de Compose à part, activé seulement quand
  `secrets/fcm-api.json` existe. Une règle absente de la configuration n'envoie **rien**.
- Horaires silencieux : réglés **par appareil** dans l'application (écran Notifications), défaut
  22:00–07:00. Une alerte ne les traverse que si sa règle est `urgent` **et** critique.
- L'API joint `fcm.googleapis.com` par le réseau `edge` (non interne, vérifié sur le VPS) ;
  `control` reste sans sortie.
- Le veilleur garde sa **propre liste de jetons** (le control plane peut être en panne) : jetons
  lus dans `notification_devices` (appareils actifs, non révoqués), jamais affichés. Son état ne
  contient que des empreintes de jetons.
- Envoi FCM : permission `cloudmessaging.messages.create` (rôle prédéfini « Firebase Cloud
  Messaging API Admin », qui ajoute seulement les abonnements aux sujets et les statistiques
  de livraison ; un rôle personnalisé réduit à cette permission est proposé en premier).
- La session de l'agent sur la tour **n'est pas administrateur** : la tâche planifiée (compte
  SERVICE LOCAL) est installée par Alexy dans un PowerShell administrateur, sur les deux PC.

### Fait (local, sans effet sur le VPS, Firebase ou les PC)

- `deploy/production/notifications.json` : les **67 règles** notifient, `group_seconds` 60,
  aucune relance ; **urgentes : `VPS-SSH-UNKNOWN-LOGIN` et `VPS-FAIL2BAN-DOWN`** (décision du
  19/10). Test Go : chaque règle d'`observability.json` a sa ligne, les urgentes sont
  critiques et sont exactement ces deux-là, aucune relance sans décision.
- `deploy/production/compose.notifications.yml` : variable, fichier monté et secret
  `firebase_service_account` pour l'API **seulement** (`migrate` ne reçoit pas la clé) ; fusion
  vérifiée avec `docker compose config` ; ajouté à la CI.
- `deploy/watchdog/windows/install-watchdog.ps1` (PowerShell 5.1, ASCII) : installation,
  `-Status`, `-RefreshTokens`, `-Uninstall`, `-DryRun`. Dossier à héritage coupé (SYSTEM,
  Administrateurs, SERVICE LOCAL en lecture, écriture dans `state` seulement), tâche
  `\Prisme\Prisme watchdog` sous SERVICE LOCAL (démarrage + toutes les 5 min, une instance,
  relance 1 min, durée illimitée), `urgent: true`, seuil 3, horaires 22 h–7 h Paris. Refuse une
  clé `firebase-adminsdk` et un libellé d'instance hors `^[a-z0-9][a-z0-9-]{0,15}$` (y compris
  les majuscules, que `ValidatePattern` laissait passer : défaut trouvé en essai). JSON écrit
  **sans BOM** (Go refuse un BOM : vérifié avec le vrai programme).
- Essais sur la tour (non administrateur, rien installé) : syntaxe PowerShell sans erreur ;
  essai à blanc complet ; refus de la clé `firebase-adminsdk`, des majuscules et du statut sans
  droits ; le vrai `watchdog.exe` (recompilé, SHA-256 `29c8a4ad…aeb`) a lu la configuration
  produite et, avec une clé RSA jetable générée localement, tourné 45 s : 2 contrôles réussis de
  `https://prisme.mantara.fr/readyz`, aucun envoi, état écrit et relu comme par `-Status`.
  Fichiers d'essai et clé jetable supprimés.
- Procédure pas à pas pour Alexy : [ETAPE_6_PROCEDURE.md](ETAPE_6_PROCEDURE.md) (passkey,
  Firebase, S24, clé de l'API, veilleurs, essais, révocation de l'ancienne clé complète,
  limites) ; mode d'emploi du veilleur : `deploy/watchdog/windows/LISEZMOI.md`.

### Reste (dans l'ordre)

1. Alexy : première passkey (A), comptes Firebase limités et 3 clés (B), APK et S24 (C).
2. Agent, après « oui » : clé de l'API sur le VPS et recréation de l'API (D).
3. Alexy : veilleurs de la tour (E) et du portable (F).
4. Ensemble : essais du critère (G), puis révocation de l'ancienne clé complète (H).
