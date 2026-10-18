# Prompt agent — Mettre Prisme en service sur le VPS (supervision + alertes sécurité)

> Prompt à donner tel quel à un agent de code (Claude Code ou équivalent) ouvert dans
> `D:\Documents\MANTARA\Prisme`. Plan validé par Alexy le 18/10/2026.

---

## 1. Mission

Tu mets **Prisme** en service sur le VPS OVH d'Alexy, en version **supervision et alertes
uniquement**, accessible sur `https://prisme.mantara.fr`, avec :

- la collecte réelle des métriques/logs du VPS ;
- une détection des **menaces SSH** (échecs de connexion, bans fail2ban, connexions réussies
  inhabituelles) qui déclenche des incidents ;
- des **notifications sur le S24** d'Alexy (FCM) ;
- un **watchdog indépendant** tournant sur deux machines Windows (PC portable `Portable_Alexy`
  et la tour `desktop-ucq8srh`), en parallèle, qui prévient quand le VPS ou Prisme ne répond plus.

Alexy n'est pas développeur. Tu parles **français**, tu expliques simplement, tu annonces ce que
tu vas faire avant de le faire.

## 2. Décisions déjà prises (ne pas les remettre en cause)

| Sujet | Décision |
|---|---|
| Hébergement | VPS OVH (`ssh ovh-mantara`), pas la tour (NO-GO production, voir `DIAGNOSTIC_TOUR_WINDOWS.md`) |
| Accès | Passkeys seules, DNS seul : **A `prisme` → 51.255.163.21, nuage gris Cloudflare**. Pas de Cloudflare Access |
| Périmètre V1 | Supervision et alertes. **Désactivés** : terminal SSH/SFTP, déployeur, edge/Cloudflare, automatisation, runbooks, coffre pilote, agent de pilotage Codex/Claude |
| Nom | Prisme / `prisme.mantara.fr` (le nom reste provisoire, ne pas renommer le code) |
| Seuils sécurité | Normal (warning) : > 50 échecs SSH en 10 min. **Urgent** : connexion SSH réussie depuis une IP inconnue, ou fail2ban arrêté |
| Watchdog | Deux instances simultanées (portable + tour), pas de relais. Un service externe viendra s'ajouter plus tard sur la même URL |

## 3. État du projet à connaître

- Dépôt `Aavkd/Prisme` (privé), branche `main`, tout est poussé (commit `1966abb`). Phases 0 à 9
  écrites, `go build` et `go vet` passent. Lis d'abord `README.md`, `docs/PHASE_1.md` à
  `PHASE_9.md` (sections « limites/qualification »), `docs/THREAT_MODEL.md`, `deploy/`.
- **Tout a été éprouvé uniquement en local (Docker Desktop)** : scopes `docker-desktop` /
  `development` dans `deploy/observability/config.alloy` et `config.json`. Rien ne tourne sur le VPS.
- Les règles d'alerte (`internal/observability`) sont basées sur des **métriques**
  (VictoriaMetrics) et n'acceptent qu'une liste blanche (`RawMetric`). **À vérifier avant l'étape 5.**
- Le watchdog (`cmd/watchdog`, `internal/notifications/watchdog.go`) teste une URL toutes les 30 s,
  seuil de 3 échecs, `Urgent: false` écrit en dur, `IncidentID` aléatoire par instance.
- Le serveur API écoute en **TLS avec sa propre CA** sur 8443 : derrière Caddy, il faut un
  `reverse_proxy https://…` avec confiance sur cette CA (voir `deploy/compose.dev.yml`).
- Fichiers sensibles du dossier (ignorés par git, ne jamais les lire à voix haute, afficher,
  committer ou copier dans un prompt) : `.dev/`, `google-services.json`,
  `prisme-49202-firebase-adminsdk-*.json`.

## 4. Le VPS — faits relevés le 18/10/2026

- Ubuntu 26.04, 4 vCPU, RAM 7,6 Go (≈ 5 Go disponibles), swap 2 Go (1 Go utilisé), disque 72 Go
  (≈ 48 Go libres après purge du cache de build), utilisateur `ubuntu` (docker sans sudo, sudo sans mot de passe).
- **35 conteneurs** en production (sites clients, KE2C, Terrain, Arpent, Sounding Lines, Monsieur
  Vincent, mantara.fr…). **Ne rien arrêter, redémarrer ni modifier** parmi eux.
- Reverse proxy mutualisé : conteneur `edge-caddy-1`, fichier `/opt/projects/edge/Caddyfile`
  (monté en **bind de fichier unique** : écrire en place avec `>` ou `cat >>`, jamais `sed -i`/`mv`),
  réseau Docker externe `edge`. Chaque projet vit dans `/opt/projects/<nom>/`.
- Ajout d'un site (procédure éprouvée, voir mémoire `vps-maquettes-clients`) : déployer la pile →
  préparer le bloc → Alexy crée le DNS (gris) → vérifier `ssh ovh-mantara dig +short prisme.mantara.fr @1.1.1.1`
  **ET** `@8.8.8.8` = `51.255.163.21` → sauvegarder `Caddyfile.bak-AAAAMMJJ-HHMMSS` → ajouter le bloc →
  `docker exec edge-caddy-1 caddy validate` → `caddy reload`. **Jamais activer le bloc avant que le DNS résolve
  vers le VPS** (limite Let's Encrypt de 5 échecs/heure). Le DNS local du poste ne répond pas : toujours vérifier depuis le VPS.
- Déploiement par Git : clé de déploiement **lecture seule** `~/.ssh/deploy_prisme`, alias `github-prisme`,
  ajoutée avec `gh repo deploy-key add` (sous Windows, `cygpath -w` pour un fichier `/tmp`).
- Pare-feu `ufw` actif (SSH + 80/443). **fail2ban** installé le 18/10 : jail `sshd`, 5 échecs/10 min → ban 1 h
  (incrémental, max 1 semaine), backend `systemd`, config dans `/etc/fail2ban/jail.d/mantara-sshd.local`.
  Il y a ≈ 1 000 échecs SSH par jour : c'est le **bruit normal**.
- Sauvegardes : `mantara-backup.timer` (≈ 03:20 UTC, script `/usr/local/sbin/mantara-backup.sh`) pour Arpent,
  Vincent et mantara-social ; KE2C via `/etc/cron.d/ke2c-sauvegarde` vers `/var/sauvegardes/ke2c`.
- Un **redémarrage système est en attente** (noyau, libc) : ne pas le faire, c'est Alexy qui choisit le moment.

## 5. Règles non négociables

1. **Toute action qui modifie le VPS, le DNS, le pare-feu, Firebase ou une machine d'Alexy exige son accord
   explicite avant exécution** : tu annonces la commande et son effet, puis tu attends un « oui ».
   Lecture seule : libre.
2. Ne jamais afficher, journaliser, committer ou recopier un secret (clés, tokens, mots de passe, CA privée,
   compte de service Firebase, jetons FCM, clé du coffre). Fichiers à droits restreints (ACL Windows, `0600/0700` Linux).
3. Aucun port publié sur l'extérieur pour Prisme : seul Caddy `edge` l'expose, sur `prisme.mantara.fr`.
4. Limites mémoire/PIDs sur chaque conteneur (celles de `deploy/` conviennent : ≈ 1,8 Go au total). Surveille
   la RAM et le swap avant/après.
5. Ne touche pas aux autres piles, ni à `ufw`, ni à la config fail2ban existante (tu peux seulement la **lire**).
6. Pour tester la détection SSH, **ne jamais provoquer de ban sur l'IP d'Alexy**. Préfère des métriques
   synthétiques ; tout vrai test d'échecs SSH est annoncé, fait depuis une autre IP, et tu connais la commande
   de déblocage (`sudo fail2ban-client unban <ip>`).
7. Un push ou une action externe visible = accord d'Alexy. Commits par étape logique, message en anglais,
   terminés par `Co-Authored-By: Claude Sonnet 5.5 <noreply@anthropic.com>`.
8. Si un test échoue ou qu'une étape n'est pas vérifiée, **tu le dis tel quel** dans ton compte rendu.
9. Tiens à jour `docs/PHASE_10_VPS.md` (journal en français, une entrée par étape : fait, vérifié, reste).

## 6. Étapes (dans l'ordre, une passe de validation avec Alexy à la fin de chacune)

### Étape 1 — Préparation sur le poste
- `./tool/verify.ps1` (et `go test ./...` avec PostgreSQL local ; voir `docs/PHASE_7.md`). Rapporte le résultat réel.
- **Watchdog, deux modifications** (code + tests Go) :
  a. champ de configuration optionnel `urgent` (bool, défaut `false`) qui renseigne `Payload.Urgent` ;
  b. champ optionnel `instance` (court libellé de la machine qui surveille, ex. `portable`, `tour`) ajouté
     au titre de la notification. Vérifie d'abord les contraintes de `Payload`/Android (titre « fixe »,
     pas de hostname par conception) : n'ajoute que ce libellé court, validé.
  Garde `Threshold` et le reste inchangés. Mets à jour `deploy/notifications/watchdog.example.json` et `docs/PHASE_5.md`.
- Compile les binaires **linux/amd64** (api, agent, probe… ceux que le compose de production utilise) et
  **windows/amd64** (`watchdog`).
- Lis comment l'API démarre en production (`PRISME_MODE`, RP ID/origine WebAuthn pour `prisme.mantara.fr`,
  rôle de la CA/TLS) et note ce qui manque dans le compose de dev pour la prod.
**Critère** : tests verts ou échecs expliqués ; binaires construits ; liste écrite des différences dev → prod.

### Étape 2 — Installation sur le VPS (après accord)
- Crée `/opt/projects/prisme/` (clone partiel du dépôt via la clé de déploiement en lecture seule, comme
  les autres projets) et un `docker-compose.yml` de production : Postgres 17, API (+ migration), VictoriaMetrics,
  Loki, Alloy. Secrets générés **sur le VPS** (ou transférés en `0600`) et jamais dans le dépôt.
- Réseau interne pour la base et les stores ; seul l'API rejoint `edge` (alias `prisme-api`).
**Critère** : conteneurs `healthy`, aucun port publié, RAM totale de Prisme sous ≈ 2 Go, autres conteneurs intacts.

### Étape 3 — Exposition `prisme.mantara.fr`
- Demande à Alexy de créer **A `prisme` → 51.255.163.21, nuage gris**. Vérifie `dig` sur 1.1.1.1 **et** 8.8.8.8 depuis le VPS.
- Prépare le bloc Caddy (en-têtes `base_headers`/`hsts` comme les autres, `reverse_proxy https://prisme-api:8443`
  avec confiance sur la CA de Prisme). Sauvegarde, valide, recharge (voir §4).
- Alexy enregistre sa passkey (bootstrap) ; vérifie qu'un anonyme ne voit que `/readyz` (et pas de données).
**Critère** : HTTPS valide (Let's Encrypt), `/readyz` 200, connexion passkey OK depuis le portable et le S24.

### Étape 4 — Collecte réelle du VPS
- Installe l'agent d'inventaire/lecture seule du VPS (unités `deploy/systemd/prisme-agent.service`,
  `prisme-inventory.*` adaptées) ; Alloy configuré pour l'hôte (CPU, RAM, disque, réseau, cAdvisor sur les 35 conteneurs).
- Remplace `docker-desktop`/`development` par `ovh-mantara`/`production` ; supprime les cibles « pilot » de test.
- Checks HTTP des 22 domaines servis par `edge` (liste : `grep` des blocs de `/opt/projects/edge/Caddyfile`).
- Règles : disque > 85 %, RAM disponible faible, swap anormal, conteneur arrêté/redémarrant, certificat expirant < 14 j,
  site en erreur 5xx/injoignable (avec hystérésis de `internal/observability`).
**Critère** : l'écran Serveurs de Prisme montre le VPS et ses conteneurs avec des mesures fraîches ; une règle de test
se déclenche puis se résout.

### Étape 5 — Détection des menaces SSH
- **Vérifie d'abord la liste blanche `RawMetric`** (`internal/observability/config.go`) ; si besoin, ajoute les noms de
  métriques nécessaires avec tests.
- Script + timer systemd (lecture seule, sans privilèges inutiles) qui écrit un fichier *textfile* pour Alloy :
  échecs SSH sur 10 min (journal `ssh.service`), bans fail2ban actifs, état du service fail2ban, connexions SSH **réussies**
  récentes avec leur IP source.
- IP « connues » : liste dans un fichier de configuration **validée par Alexy** (propose-lui les IP des connexions
  réussies des 30 derniers jours, il confirme lesquelles sont à lui). Toute autre IP en connexion réussie = urgent.
- Règles : > 50 échecs / 10 min = warning ; connexion réussie depuis IP inconnue = **critique + `urgent: true`** ;
  fail2ban inactif = critique + urgent. Le bruit habituel (≈ 1 000/jour réparti) ne doit **pas** déclencher d'alerte : montre les chiffres réels des 7 derniers jours avant de figer les seuils.
**Critère** : incidents créés et résolus sur métriques synthétiques ; aucun faux positif sur 24 h d'observation.

### Étape 6 — Alertes S24 et watchdog
- FCM de production : réutilise le projet Firebase existant avec un **compte de service limité à l'envoi** pour le
  watchdog ; la clé reste hors dépôt. Enrôle le S24 (passkey + biométrie) et active les notifications du control plane.
- Horaires silencieux d'Alexy (propose 22 h–7 h Europe/Paris) ; les règles `urgent` les traversent.
- Watchdog : `url = https://prisme.mantara.fr/readyz`, `urgent: true`, `instance` = `portable` / `tour`, seuil 3.
  Installe sur chaque machine Windows une **tâche planifiée au démarrage** (redémarrage automatique en cas d'échec,
  droits minimaux, fichiers de configuration et clé en ACL restreintes, aucune écoute réseau). Pas de service Windows à écrire.
- Explique à Alexy les limites : le portable dort ; la tour a eu 6 arrêts non propres en 90 jours ; deux notifications
  sont normales si les deux machines voient la panne.
**Critère** : coupure contrôlée de l'URL (ex. arrêt du seul conteneur API de Prisme, jamais d'un autre service) → chaque
instance active notifie le S24 (avec son libellé), puis notifie le rétablissement ; ouverture de la bonne fiche ; une alerte
`urgent` passe bien la nuit simulée.

### Étape 7 — Qualification et sauvegarde de Prisme
- Test de bout en bout de la chaîne SSH : métrique synthétique → incident → FCM → S24 → acquittement (empreinte).
- Sauvegarde de la base Prisme **et** de ses secrets non régénérables : ajoute-la à la routine existante
  (`mantara-backup.sh`, en proposant la modification à Alexy) ou à Restic selon `docs/PHASE_7.md` ; **teste une restauration**
  sur une base temporaire.
- Rétention : durée des logs/métriques (VictoriaMetrics 30 j) cohérente avec les 48 Go libres ; note l'espace consommé.
**Critère** : restauration prouvée ; consommation disque/RAM mesurée et notée.

### Étape 8 — Documentation et mémoire
- `docs/PHASE_10_VPS.md` complet ; mise à jour de `README.md` du dépôt (état réel, limites restantes).
- Dépôt `Aavkd/mantara-docs` à jour (contexte structurel : nouveau sous-domaine, nouveaux conteneurs, tâche planifiée,
  métriques SSH) ; mémoire projet `app-ops-pilotage`/nouvelle fiche Prisme si l'environnement le permet.
- Compte rendu final à Alexy : ce qui marche, ce qui n'est pas vérifié, ce qui reste (service externe de supervision,
  redémarrage du VPS, sauvegarde Terrain, qualification passkeys/Doze).

## 7. Hors périmètre (ne pas le faire sans nouvelle demande)

Redémarrage du VPS, mises à jour système, modification de fail2ban/ufw, capacités d'action de Prisme (terminal, déploiements,
edge/Cloudflare, runbooks autonomes), phase 10 du spec (audit de sécurité complet, tests de charge), service de surveillance externe.

## 8. Quand t'arrêter et demander

- Un test échoue de façon inexpliquée ou touche un service en production ;
- la RAM disponible du VPS tombe sous ≈ 2 Go ou le swap grossit nettement ;
- une étape exige une action sur le DNS, Firebase ou une machine Windows ;
- une décision ci-dessus ne tient plus (par ex. Prisme exige Cloudflare Access pour la prod) ;
- tu hésites sur la sécurité d'une action : tu proposes, Alexy décide.
