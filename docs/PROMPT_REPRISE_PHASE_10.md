# Prompt de reprise — Phase 10 (Prisme sur le VPS), état au 19/10/2026 15:20 UTC

> À donner à un agent de code ouvert sur la tour `DESKTOP-UCQ8SRH`, dans `D:\MANTARA\Prisme`.
> Il prend la suite d'une session qui a mené les étapes 1 à 4 de
> [PROMPT_DEPLOIEMENT_VPS.md](PROMPT_DEPLOIEMENT_VPS.md). Cette session est terminée.

## 1. À lire d'abord, dans cet ordre

1. [PROMPT_DEPLOIEMENT_VPS.md](PROMPT_DEPLOIEMENT_VPS.md) : la mission, les décisions et les **règles non
   négociables** (§5), qui restent en vigueur.
2. [PHASE_10_VPS.md](PHASE_10_VPS.md) : le journal complet (fait, vérifié, décisions, commandes exactes,
   résultats). Le tenir à jour, une entrée par étape.
3. [deploy/production/LISEZMOI.md](../deploy/production/LISEZMOI.md) : le mode d'emploi de la production.

Alexy n'est pas développeur. Parler **français**, simplement, annoncer avant d'agir.

## 2. Où et comment travailler

- **Dépôt** : `D:\MANTARA\Prisme`, branche `main`, à jour avec `origin/main` (`959e708` + ce fichier).
  **Jamais** à la racine de `D:`. Un clone fait par erreur, `D:\Prisme`, est à supprimer par Alexy
  (`Remove-Item -Recurse -Force D:\Prisme`) : ne pas y travailler.
- **Outils portables** dans `D:\tools` (rien sur `C:`, 2,4 Go libres seulement) : Go 1.26.8, Flutter
  3.38.5, PostgreSQL 17.11, Restic 0.19.1. Activer avec `. D:\tools\prisme-env.ps1` (PowerShell) ou
  `source /d/tools/prisme-env.sh` (Git Bash). Base de test : `D:\tools\pgtest\data`, à démarrer avec
  `pg_ctl` via `Start-Process` (port `55432`, base `prisme_test`), puis
  `PRISME_TEST_DATABASE_URL=postgres://postgres@127.0.0.1:55432/prisme_test?sslmode=disable`.
- **Docker Desktop** (données sur `D:`) sert aux simulations locales de la pile de production
  (secrets jetables dans `secrets/`, ignoré par Git, réseau `edge` local en `10.222.0.0/24` car
  `172.22.0.0/16` est déjà pris localement ; toujours démonter ensuite).
- **VPS** : `ssh ovh-mantara` (utilisateur `ubuntu`, sudo sans mot de passe). Depuis la tour,
  `ssh -o BatchMode=yes ovh-mantara '…'` ; scripts longs via `ssh ovh-mantara 'bash -s' < script`.
  Scripts utiles déjà écrits dans `D:\tools\vps-scripts\` (relevé mémoire `run-with-mem.sh`,
  vérifications `check-step4.sh`, exercice `drill.sh`, codes HTTP des sites `site-codes.sh`).
- **Commits** : message en anglais, par étape logique, terminés par la ligne `Co-Authored-By` de
  **votre** modèle ; `git -c user.name=Alexy -c user.email=traorealexy@gmail.com commit …` (nom non
  configuré sur la tour). **Pousser à chaque étape** (`origin/main` autorisé : le disque `D:` est
  dégradé) et vérifier la CI : `gh run watch <id> --exit-status` ; elle est **verte 11/11**.
- **Toute action qui modifie le VPS, le DNS, Firebase ou une machine d'Alexy** : annoncer les
  commandes exactes et leur effet dans le journal et le chat, attendre son « oui ». Lecture seule :
  libre. S'arrêter au moindre écart : RAM disponible du VPS sous ≈ 2 Go, swap qui grossit nettement,
  autre conteneur affecté.

## 3. État au moment de la reprise

| Étape | État |
|---|---|
| 1 Préparation | **Faite.** CI réparée (7 causes), watchdog `urgent` + `instance` (Go + Android), binaires. |
| 2 Installation VPS | **Faite.** Pile `prisme` dans `/opt/projects/prisme` (5 services sains, aucun port publié). |
| 3 Exposition | **Faite côté serveur** : `https://prisme.mantara.fr` (Let's Encrypt jusqu'au 17/01/2027), anonyme = `/readyz`, `/auth` et 401. **Reste** : première passkey d'Alexy (portable puis S24) — non vérifiée. |
| 4 Collecte réelle | **Faite** : 28 sites, 41 conteneurs, disque/RAM/swap, 62 règles saines, exercice réussi. **Reste** : commande 6 (agent d'inventaire), qui attend la passkey et un jeton d'enrôlement. |
| 5 Menaces SSH | **Préparée et testée en local** (collecteur `ssh_metrics.py`, 5 règles, essai à blanc réussi sur le VPS). fail2ban corrigé le 19/10 (accord). **Reste** : commandes 1 à 7 du journal (accord d'Alexy), dont l'exercice et 24 h d'observation. |
| 6 FCM S24 + watchdog | À faire. |
| 7 Qualification + sauvegarde | À faire. |
| 8 Documentation + mémoire | À faire (`README.md`, dépôt `Aavkd/mantara-docs`). |

### Décisions d'Alexy déjà prises (ne pas les rediscuter)

- Télémétrie interne en HTTP **seulement** vers un nom de service Docker
  (`PRISME_TELEMETRY_INTERNAL_HTTP=1`, `observability.NewInternalSource`).
- Groupe root (0) accordé à Alloy pour le socket containerd, en plus du groupe `docker` (986).
- `X-Forwarded-For` cru **seulement** depuis le réseau `edge` (`PRISME_TRUSTED_PROXY_CIDR=172.22.0.0/16`) ;
  risque résiduel accepté : un conteneur déjà sur `edge` peut choisir son en-tête.

### Faits de production à connaître

- Commande compose (toujours avec `sudo`, `secrets/` est fermé à `ubuntu`), depuis `/opt/projects/prisme` :
  `P="docker compose --env-file deploy/production/ovh-mantara.env -f deploy/production/compose.yml"`.
- Clone partiel (`git sparse-checkout` : `deploy cmd internal`) avec la clé de déploiement
  **lecture seule** `~/.ssh/deploy_prisme` (alias `github-prisme`, clé GitHub n° 166885280).
- Secrets : `/opt/projects/prisme/secrets` (`root:10001`, `0750`/`0640`) : `db-password`,
  `identity-secret`, `pki/` (CA « Prisme control plane CA » 5 ans, empreinte `629F3A2C…5F99` ;
  certificat API 397 jours pour `prisme-api`, `localhost`). Ne jamais les afficher.
- Caddy mutualisé : bloc `prisme.mantara.fr` ajouté le 19/10 à 13:17 (sauvegarde
  `/opt/projects/edge/Caddyfile.bak-20261019-131746`) ; CA de Prisme incluse en clair (public)
  via `tls_trust_pool inline`. DNS : A `prisme` → `51.255.163.21`, nuage gris (un générique
  `*.mantara.fr` pointe vers Vercel : tout nouveau sous-domaine a besoin de son propre enregistrement).
- Hôte : groupe `prisme-observation` (10002), compte `prisme-collector` (UID 997, groupes 10002 et
  `docker`), minuteurs `prisme-inventory.timer` (60 s) et `prisme-host-metrics.timer` (30 s),
  `/usr/local/lib/prisme/host_metrics.py`, exercice : `sudo touch /etc/prisme/drill` puis `sudo rm`.
- Mémoire réelle de Prisme ≈ 450 Mo (Alloy 256). RAM disponible du VPS ≈ 4,9 Go, swap ≈ 1 033 Mo.
- Un autre chantier travaille en parallèle sur le VPS (site `thielen-maquettes`, Caddyfile passé
  à 512 lignes) : relire l'état réel avant toute modification partagée.
- Redémarrage système du VPS en attente : **ne pas le faire**, c'est Alexy qui choisit.

## 4. Pièges déjà rencontrés (ne pas les refaire)

- **Fichiers montés seuls** (`config.alloy`, `observability.json`, Caddyfile) : `git pull` ou `mv`
  remplacent le fichier, le conteneur garde l'ancien. Après une mise à jour :
  `sudo $P up -d --force-recreate --no-deps --wait alloy api`. Caddyfile : écrire en place.
- **`docker compose run` lit l'entrée standard** : dans un script envoyé par `ssh 'bash -s'`, ajouter
  `< /dev/null` (sinon il avale la fin du script).
- **Git Bash** convertit les chemins `/…` : `export MSYS_NO_PATHCONV=1` pour `docker`.
- **Heredoc Python avec `\\` en fin de ligne** : la ligne fusionne ; vérifier le Dockerfile après.
- **Pas de `sleep` long** dans un appel : lancer en arrière-plan et attendre la notification.
- `date` du VPS est la version Rust (uutils) : préférer Python 3.14 pour les dates.
- Le moteur de règles ne lit qu'**une série brute** (horodatages d'origine) ; une donnée absente =
  « inconnu », jamais un incident. D'où `RuleMetric`, les transformations échantillon par
  échantillon et l'âge du fichier hôte. Le libellé `instance` doit rester fixe (`ovh-mantara`).
- VictoriaMetrics masque les 30 dernières secondes des requêtes de plage.
- Flutter sous Windows exige le Mode développeur (non activé, réglage système) : rejouer les
  contrôles de `apps/prisme_client` sous Linux dans Docker (voir le journal, étape 1).
- Ne jamais lancer de `rm -rf` sur un dossier de haut niveau ; un contrôle de sécurité le bloque.

## 5. Suite du travail

### A. Actions d'Alexy (à lui rappeler)

1. Première passkey : `ssh ovh-mantara 'sudo cat /opt/projects/prisme/secrets/identity-secret'`
   (il le lit lui-même), puis sur le portable, dans `apps/prisme_client` :
   `flutter run -d windows --dart-define=PRISME_IDENTITY=true --dart-define=PRISME_API_URL=https://prisme.mantara.fr`
   → bouton passkey → `/auth` → « Créer ou récupérer ». Codes de secours hors de l'appareil.
2. Jeton d'enrôlement (vue Serveurs, 10 min) collé par lui sur le VPS (LISEZMOI, « Agent ») ; puis
   commande 6 de l'étape 4 : `sudo $P --profile agent up -d --build --wait agent`, suppression du
   jeton, vérification de la vue Serveurs (critère de l'étape 4 encore ouvert).
3. Supprimer `D:\Prisme`.

### B. Étape 5 — menaces SSH (prochaine étape, préparation locale d'abord)

1. **Relevés en lecture seule** à montrer à Alexy avant de fixer quoi que ce soit : échecs SSH des
   7 derniers jours (journal `ssh.service`, par jour et par fenêtre de 10 min : maximum, médiane),
   bans fail2ban (`sudo fail2ban-client status sshd`), et **adresses des connexions réussies des
   30 derniers jours** (Alexy confirme lesquelles sont à lui → fichier d'IP connues).
2. Étendre la collecte hôte (fichier `textfile`) sans privilège inutile : lecture du journal SSH
   (groupe `systemd-journal` ou `adm` pour `prisme-collector`, à proposer) ; l'état fail2ban et ses
   bans demandent root → proposer un petit service séparé, minimal, qui n'écrit que des compteurs.
   Ne jamais modifier fail2ban ni `ufw` ; ne jamais provoquer de ban sur l'IP d'Alexy.
3. Métriques à ajouter à `RawMetric` (avec tests) et règles : > 50 échecs / 10 min = warning ;
   connexion réussie depuis une IP inconnue = critical ; fail2ban arrêté = critical. Les deux
   critiques seront `urgent: true` dans la configuration de notifications (étape 6). Le bruit normal
   (≈ 1 000 échecs/jour) ne doit rien déclencher.
4. Tests sur métriques synthétiques (comme `PRISME-DRILL`), puis observation 24 h sans faux positif.

### C. Étapes 6 à 8

Voir PROMPT_DEPLOIEMENT_VPS.md §6. Rappels : comptes de service Firebase **limités à l'envoi FCM**
(watchdog et API), créés avec Alexy, l'actuel compte complet à faire révoquer ; nouvel APK S24
(titre du watchdog) construit sur le portable ; tâche planifiée du watchdog sur la tour (redémarrage
automatique, ACL restreintes) et script + procédure pour le portable ; sauvegarde de la base et des
secrets non régénérables avec **restauration testée** ; README et `Aavkd/mantara-docs`.
