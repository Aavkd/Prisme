# Prompt — Reprise de la phase 10 de Prisme sur le PC portable (état au 22/10/2026)

Tu reprends la **phase 10 de Prisme** (mise en service sur le VPS OVH, supervision et alertes) sur
le **PC portable d'Alexy** (`Portable_Alexy`), et non sur la tour `DESKTOP-UCQ8SRH` où les étapes 1
à 5 ont été menées. **Ta mission dans cette session : faire aboutir, avec Alexy, la création de tout
ce qui manque pour que les alertes arrivent sur son Samsung S24** : sa première passkey, l'agent
d'inventaire, les comptes Firebase limités à l'envoi, l'application sur le S24, l'activation des
notifications sur le VPS et le veilleur du portable, puis les essais. N'attends pas qu'il te le
demande : propose chaque partie dans l'ordre, donne-lui les instructions manuelles ci-dessous **au
moment voulu** (une partie à la fois), vérifie le résultat, consigne-le, puis passe à la suivante.

Alexy n'est pas développeur. Parle-lui **en français**, simplement, et **annonce ce que tu vas faire
avant de le faire**.

## 1. Démarrage (lecture seule)

1. Trouver le dépôt Prisme du portable (celui où Alexy a développé Prisme ; `git remote -v` doit
   montrer `Aavkd/Prisme`). Si le chemin n'est pas évident, **demande-le** ; ne clone pas ailleurs
   sans son accord. Puis `git pull --ff-only` sur `main`.
2. Lire, dans cet ordre :
   1. `docs/PROMPT_DEPLOIEMENT_VPS.md` : mission et règles non négociables (§5), toujours en vigueur ;
   2. `docs/PHASE_10_VPS.md` : journal complet (au minimum les étapes 5 et 6 et le « Point du
      22/10/2026 ») ; à tenir à jour ;
   3. `docs/ETAPE_6_PROCEDURE.md` : procédure de l'étape 6, parties A à H ;
   4. `deploy/production/LISEZMOI.md`, `deploy/production/ops/LISEZMOI.md`,
      `deploy/watchdog/windows/LISEZMOI.md`.
3. Relevés du portable, sans rien installer ni modifier : `git`, `gh auth status`, `go version`
   (1.26.x attendu), `flutter --version` (3.38.5) et `flutter doctor` (SDK Android, licences),
   `adb devices`, Docker (facultatif), identité Git (`git config user.name`, `user.email`).
   Présence **sans l'ouvrir** de `apps/prisme_client/android/app/google-services.json`.
4. Accès au VPS : `ssh -o BatchMode=yes ovh-mantara true`. Si l'alias diffère sur le portable, le
   retrouver dans `~/.ssh/config` et l'utiliser partout à la place de `ovh-mantara` (y compris
   `-SshHost` du script du veilleur). **Ne génère jamais de nouvelle clé SSH** : si l'accès manque,
   arrête-toi et demande.
5. État du VPS, depuis **Git Bash** (Windows PowerShell 5.1 ne connaît pas `<`) :
   `ssh -o BatchMode=yes ovh-mantara 'bash -s' < deploy/production/ops/etat.sh`.
   Le 22/10 à 16:47 UTC : pile saine, 67 règles saines, 0 incident ouvert, notifications non
   actives, **0 passkey, 0 appareil, 0 agent**, RAM disponible 4,9 Go, swap 91 Mo.
6. Montrer à Alexy ce relevé et le plan (**A → C bis → B → C → D → F → G**, puis **E** sur la tour
   et **H**) avant de commencer.

## 2. Règles (non négociables)

- Toute action qui **modifie** le VPS, le DNS, Firebase/Google Cloud ou une machine d'Alexy (S24 et
  tour compris) attend son **« oui » explicite**, après que tu as annoncé les **commandes exactes**
  et leur effet. Lecture seule : libre.
- **Aucun secret ne passe par toi** : secret d'initialisation, codes de secours, ticket d'ajout,
  jeton d'enrôlement, clés JSON de comptes de service, jetons FCM. Alexy les lit ou les colle
  **dans son propre terminal ou navigateur** ; tu ne lances jamais une commande qui les afficherait
  (`cat` du secret, contenu d'une clé). Ne les demande pas dans le chat, ne les écris dans aucun
  fichier du dépôt ni du journal.
- Ni adresse IP d'Alexy ni faiblesse de sécurité encore ouverte dans le dépôt (l'outil de sécurité
  de l'agent bloque d'ailleurs ces envois) : garder ces détails hors dépôt.
- Aucun port publié ; ne pas toucher aux autres piles, à `ufw` ni à fail2ban (corrigé le 19/10 ;
  toute nouvelle modification exige un « oui »). Ne jamais provoquer de ban d'une adresse d'Alexy.
- **S'arrêter** au moindre écart : RAM disponible du VPS sous ≈ 2 Go, swap qui grossit nettement,
  autre conteneur affecté, test qui échoue sans explication. Jamais de redémarrage du VPS.
- Commits par étape logique, message **en anglais**, terminés par la ligne `Co-Authored-By` de
  **ton** modèle ; si l'identité Git du portable est vide :
  `git -c user.name=Alexy -c user.email=traorealexy@gmail.com commit …`. **Pousser sur
  `origin/main` à chaque étape** et vérifier la CI : 11 jobs, contrôlés **un par un**
  (`gh run watch` s'arrête parfois trop tôt : interroger
  `gh api repos/Aavkd/Prisme/actions/runs/<id>/jobs` jusqu'à ce qu'aucun ne soit en cours).
- Tenir `docs/PHASE_10_VPS.md` à jour (une entrée par partie : fait, vérifié, reste) et dire tel
  quel ce qui n'est pas vérifié.

## 3. Où en est le chantier (22/10/2026)

| Étape | État |
|---|---|
| 1 à 4 | **Faites** : CI, pile `prisme` sur le VPS, `https://prisme.mantara.fr`, collecte réelle (28 sites, 42 conteneurs). Restes : première passkey (A) et agent d'inventaire (C bis). |
| 5 Menaces SSH | **Faite et close** : collecteur `ssh_metrics.py`, 5 règles, exercice réussi, fail2ban corrigé, 0 faux positif en 67 h. |
| 6 Alertes S24 + veilleurs | **Préparée, rien d'activé** : `deploy/production/notifications.json` (67 règles notifiées, **30 urgentes** : connexion SSH inconnue, fail2ban arrêté, les 28 `SITE-*-HTTP`), `compose.notifications.yml`, `deploy/watchdog/windows/install-watchdog.ps1` (essayé à blanc). |
| 7 Qualification + sauvegarde | À faire après 6. |
| 8 Documentation + mémoire | À faire. |

Faits utiles :

- VPS : `ssh ovh-mantara` (utilisateur `ubuntu`, sudo sans mot de passe). Prisme dans
  `/opt/projects/prisme` (clone partiel en lecture seule, à `e4ec993` : `git pull --ff-only`
  avant D). Toujours `sudo` (le dossier `secrets/` est fermé à `ubuntu`).
  `P="docker compose --env-file deploy/production/ovh-mantara.env -f deploy/production/compose.yml"` ;
  **après D**, toujours ajouter `-f deploy/production/compose.notifications.yml` (sans lui, un `up`
  recrée l'API sans notifications ; avec lui mais sans clé, l'API refuse de démarrer).
- Adresses SSH connues (`/etc/prisme/ssh-known-ips` sur le VPS) : la ligne de la tour et la ligne
  fixe du portable. Une fois les notifications actives, **toute connexion SSH depuis une autre
  adresse (mobile, déplacement, déploiement GitHub Actions) déclenche une alerte urgente** : c'est
  voulu ; prévenir Alexy si tu travailles depuis ailleurs.
- Firebase : projet `prisme-49202`, application Android `dev.mantara.prisme_client`. Le compte de
  service complet `firebase-adminsdk` est à révoquer en H.
- Scripts d'exploitation : `deploy/production/ops/` (`etat.sh`, `verif-*.sh`,
  `observation-ssh.sh`, `exercice-drill.sh`, `exercice-ssh.sh`…), lancés par
  `ssh -o BatchMode=yes ovh-mantara 'bash -s' < deploy/production/ops/<script>.sh`.

## 4. Déroulé de la session et instructions manuelles pour Alexy

Pour chaque partie : annonce-la, donne à Alexy ses instructions, attends qu'il confirme,
**vérifie** (lecture seule, `etat.sh`), consigne dans le journal, committe et pousse si des
fichiers ont changé.

### A. Première passkey (portable)

Toi : vérifier que le client se lance (`flutter pub get` dans `apps/prisme_client` si besoin ;
sous Windows il faut le *Mode développeur* pour les liens des plugins : s'il manque, c'est un
réglage système, Alexy l'active lui-même). Tu peux lancer le client :
`flutter run -d windows --dart-define=PRISME_IDENTITY=true --dart-define=PRISME_API_URL=https://prisme.mantara.fr`

Instructions pour Alexy :

1. Ouvrir **ton propre** terminal (PowerShell ou Git Bash, pas celui de l'agent) et taper :
   `ssh ovh-mantara 'sudo cat /opt/projects/prisme/secrets/identity-secret'`
   Garde le secret affiché à l'écran ; ne le copie nulle part ailleurs (pas dans le chat).
2. Dans la fenêtre Prisme : bouton **passkey** → la page de connexion s'ouvre dans le navigateur
   → **« Créer ou récupérer »** → colle le secret d'initialisation → crée la passkey avec
   **Windows Hello** (empreinte, visage ou code).
3. Prisme affiche des **codes de secours** : recopie-les **hors du portable** (papier rangé, ou
   gestionnaire de mots de passe sur un autre appareil). Ils ne seront plus affichés.
4. Ferme ton terminal (le secret disparaît de l'écran).

Vérification (toi) : `etat.sh` → `passkeys : 1`, `appareils actifs : 1`.

### C bis. Agent d'inventaire (reste de l'étape 4)

Instructions pour Alexy :

1. Dans Prisme (portable), vue **Serveurs** : génère un **jeton d'enrôlement** (valable
   10 minutes). Ne le colle pas dans le chat.
2. Dans **ton propre terminal**, lance la commande ci-dessous, colle le jeton, puis **Entrée** et
   **Ctrl-D** :

   ```
   ssh -t ovh-mantara "cd /opt/projects/prisme && sudo install -d -m 0750 -o root -g 10001 secrets/agent && sudo sh -c 'umask 027; cat > secrets/agent/token' && sudo chgrp 10001 secrets/agent/token"
   ```

Toi, après son « oui » : depuis `/opt/projects/prisme`,
`sudo $P --profile agent up -d --build --wait agent < /dev/null` (avec relevé mémoire,
`ops/avec-memoire.sh`) ; vérifier avec Alexy la vue **Serveurs** (le VPS et ses conteneurs avec
des mesures fraîches : critère encore ouvert de l'étape 4) ; puis
`sudo rm /opt/projects/prisme/secrets/agent/token`.

### B. Comptes Firebase limités à l'envoi (navigateur du portable)

Instructions pour Alexy, sur <https://console.cloud.google.com/>, projet **prisme-49202**, avec le
compte propriétaire du projet :

1. **Rôle** : menu ☰ → *IAM et administration* → *Rôles* → **Créer un rôle**. Titre
   `Prisme - envoi FCM`, ID `prismeFcmSend`, étape *Disponibilité générale* → *Ajouter des
   autorisations* → cherche `cloudmessaging.messages.create`, coche-la (une seule) → *Créer*.
   Si cette autorisation n'est pas proposée, ne crée rien : tu utiliseras plus loin le rôle
   prédéfini **Firebase Cloud Messaging API Admin**.
2. **Compte de l'API** : *IAM et administration* → *Comptes de service* → **Créer un compte de
   service** : nom `prisme-api-fcm`, description `Prisme API, envoi FCM seul` → *Créer et
   continuer* → rôle `Prisme - envoi FCM` (ou *Firebase Cloud Messaging API Admin*) → *Continuer*
   → *OK* (personne d'autre à autoriser). Clique ensuite sur le compte → onglet **Clés** →
   *Ajouter une clé* → *Créer une clé* → **JSON** → *Créer* : le fichier se télécharge dans
   `Téléchargements`. Ne l'ouvre pas, ne l'envoie ni par mail ni par messagerie.
3. **Compte des veilleurs** : même chose avec le nom `prisme-watchdog-fcm`, description
   `Veilleurs Prisme, envoi FCM seul`, même rôle. Crée **une seule** clé JSON maintenant : celle
   du **portable**. Note l'**ID de la clé** affiché (ce n'est pas un secret) et « portable » à
   côté. La clé de la tour sera créée plus tard **depuis la tour** (partie E) : aucune clé ne
   voyage d'un PC à l'autre.
4. Si Google refuse la création de clé (« iam.disableServiceAccountKeyCreation ») : arrête-toi et
   dis-le à l'agent.
5. Donne à l'agent les **noms** des deux fichiers téléchargés (jamais leur contenu).

Toi : n'ouvre jamais ces fichiers ; seuls les contrôles par script prévus en D et F (type, projet,
nom du compte, sans la clé) sont permis.

### C. Application Prisme sur le S24

Toi, après son « oui » (compilation locale) : depuis `apps/prisme_client`,
`flutter build apk --release --dart-define=PRISME_IDENTITY=true --dart-define=PRISME_API_URL=https://prisme.mantara.fr`
(`google-services.json` requis ; l'APK est signé avec la clé de débogage du portable et contient le
titre des veilleurs). Installation : `adb install -r build\app\outputs\flutter-apk\app-release.apk`
si le S24 est branché en débogage USB (avec son « oui »), sinon Alexy copie l'APK sur le téléphone
et l'ouvre.

Instructions pour Alexy :

1. Sur le **portable**, dans Prisme : **Ajouter une passkey** → un **ticket d'ajout** s'affiche
   (valable **5 minutes**).
2. Sur le **S24**, dans Prisme : bouton **passkey** → **« Créer ou récupérer »** → saisis le ticket
   → crée la passkey (empreinte).
3. Sur le S24, écran **Notifications** :
   - autorise les **notifications Android** quand Android le demande ;
   - touche **Enregistrer la clé biométrique** (empreinte) ;
   - horaires : **Silence dès 22:00**, **Jusqu'à 07:00**, fuseau **Europe/Paris** →
     **Enregistrer les horaires**.
4. Vérifie que l'écran Serveurs et les incidents s'affichent sur le S24.

Toi : `etat.sh` → 2 passkeys, 2 appareils, `abonnés aux notifications : 1`. Rappelle-lui que la
nuit seules les alertes urgentes passent (connexion SSH inconnue, fail2ban arrêté, site
injoignable ou en erreur) ; les veilleurs aussi.

### D. Activer les notifications de l'API sur le VPS (toi, après « oui »)

Depuis **Git Bash** sur le portable, `<clé-api.json>` étant le fichier téléchargé en B.2 (jamais
affiché : il passe par l'entrée standard de `ssh`) :

```sh
ssh ovh-mantara "sudo sh -c 'umask 027; cat > /opt/projects/prisme/secrets/fcm-api.json && chgrp 10001 /opt/projects/prisme/secrets/fcm-api.json'" < <clé-api.json>
ssh ovh-mantara "sudo python3 -c \"import json; d = json.load(open('/opt/projects/prisme/secrets/fcm-api.json')); print(d['type'], d['project_id'], d['client_email'].split('@')[0])\"; sudo stat -c '%a %U:%G' /opt/projects/prisme/secrets/fcm-api.json"
```

Attendu : `service_account prisme-49202 prisme-api-fcm` et `640 root:10001` (si c'est
`firebase-adminsdk-…`, supprimer le fichier et s'arrêter). Puis, sur le VPS depuis
`/opt/projects/prisme` : `git pull --ff-only`, et
`sudo docker compose --env-file deploy/production/ovh-mantara.env -f deploy/production/compose.yml -f deploy/production/compose.notifications.yml up -d --force-recreate --no-deps --wait api`
(relevé mémoire avant/après). Vérifier : API `healthy`, `etat.sh` → `notifications actives dans
l'API : 1` et `clé FCM de l'API présente : oui`, journaux de l'API sans erreur. Alexy supprime
ensuite la clé de `Téléchargements` et vide la corbeille.

### F. Veilleur du portable (toi : compilation et essai ; Alexy : installation)

Toi : `go build -trimpath -ldflags="-s -w" -o <dossier temporaire>\watchdog.exe ./cmd/watchdog`, puis
un essai à blanc sans droits ni SSH (contrôle la clé sans l'afficher, refuse `firebase-adminsdk`) :
`deploy\watchdog\windows\install-watchdog.ps1 -Instance portable -Binary <…>\watchdog.exe -CredentialsFile <clé-portable.json> -DryRun`.

Instructions pour Alexy (la tâche planifiée exige les droits administrateur, que la session de
l'agent n'a probablement pas) :

1. Menu Démarrer → tape `PowerShell` → clic droit → **Exécuter en tant qu'administrateur** →
   accepte.
2. Tape (l'agent te donne les chemins exacts) :

   ```powershell
   Set-ExecutionPolicy -Scope Process Bypass
   cd <dépôt>\deploy\watchdog\windows
   .\install-watchdog.ps1 -Instance portable -Binary <…>\watchdog.exe -CredentialsFile <clé-portable.json>
   .\install-watchdog.ps1 -Status
   ```

   La première ligne ne vaut que pour cette fenêtre. Si l'alias SSH du portable n'est pas
   `ovh-mantara`, ajoute `-SshHost <alias>` à la troisième.
3. Attendu : « Veilleur 'portable' installé … 1 telephone(s) », puis `-Status` : tâche en cours,
   dernier contrôle il y a moins d'une minute, panne signalée : non. Si le script répond
   « Lecture des jetons impossible par SSH », taper `ssh -o BatchMode=yes ovh-mantara true` dans
   cette même fenêtre administrateur et montrer le message à l'agent (clé SSH protégée par une
   phrase de passe ou agent SSH non démarré).
4. Supprime `<clé-portable.json>` de `Téléchargements` et le `watchdog.exe` temporaire.

### G. Essais (critère de l'étape 6), quand Alexy a le S24 en main

Annoncer chaque essai et obtenir un « oui » avant toute modification du VPS.

1. **Chaîne de l'API** (de jour, hors horaires silencieux) : `ops/exercice-drill.sh` → notification
   `PRISME-DRILL` sur le S24 en ≈ 1 min ; en l'ouvrant, la bonne fiche s'affiche ; puis
   résolution. Vérifier dans `etat.sh` les notifications `sent`.
2. **Nuit simulée** : Alexy règle sur le S24 des horaires qui couvrent l'heure actuelle ; créer
   `/etc/prisme/drill-ssh-unknown-login` (urgent) **et** `/etc/prisme/drill` (non urgent) → seule
   l'alerte urgente arrive tout de suite. Retirer les deux drapeaux, remettre 22:00–07:00, et
   noter ce que devient la notification non urgente.
3. **Veilleurs** : `sudo $P stop api` (seul le conteneur API de Prisme, aucun site touché) pendant
   ≈ 3 min → « Prisme · portable » « injoignable » en ≈ 1 min 30 ; `sudo $P start api` →
   « rétabli ». Avec le veilleur de la tour installé, deux notifications : c'est normal.
4. Noter tel quel ce qui n'est pas vérifié (Doze, Ne pas déranger, portable endormi).

### E. Veilleur de la tour (Alexy, devant la tour)

Cette session n'a pas accès à la tour. Instructions pour Alexy, sur la tour :

1. Navigateur : console Google Cloud → *Comptes de service* → `prisme-watchdog-fcm` → *Clés* →
   *Ajouter une clé* → **JSON** (clé « tour » ; note son ID).
2. PowerShell **normal** :
   `cd D:\MANTARA\Prisme; git pull --ff-only; . D:\tools\prisme-env.ps1; go build -trimpath -ldflags="-s -w" -o .dev\bin\windows-amd64\watchdog.exe ./cmd/watchdog`
3. PowerShell **administrateur** :

   ```powershell
   Set-ExecutionPolicy -Scope Process Bypass
   cd D:\MANTARA\Prisme\deploy\watchdog\windows
   .\install-watchdog.ps1 -Instance tour -Root D:\PrismeWatchdog -Binary D:\MANTARA\Prisme\.dev\bin\windows-amd64\watchdog.exe -CredentialsFile <clé-tour.json>
   .\install-watchdog.ps1 -Root D:\PrismeWatchdog -Status
   ```

4. Supprimer `<clé-tour.json>`, puis refaire G.3 pour voir « Prisme · tour ». (Une session de code
   sur la tour peut l'accompagner avec `docs/PROMPT_REPRISE_PHASE_10.md`.)

### H. Révoquer l'ancienne clé complète (Alexy, une fois G réussi)

Console Google Cloud → *Comptes de service* → `firebase-adminsdk-…` → *Clés* → supprimer la clé
dont la date de création correspond au fichier `prisme-*-firebase-adminsdk-*.json` (dans
`D:\PrismeSecrets` sur la tour, et toute copie sur le portable, par exemple à la racine du dépôt
où Git l'ignore), puis supprimer ces fichiers. Garder `google-services.json` (configuration
publique, utile pour reconstruire l'APK). Refaire G.1 pour vérifier que tout passe encore.

## 5. Ensuite : étapes 7 et 8 (voir `docs/PROMPT_DEPLOIEMENT_VPS.md` §6)

- **7** : chaîne SSH de bout en bout (`ops/exercice-ssh.sh` → incident → FCM → S24 → acquittement
  par empreinte) ; sauvegarde de la base Prisme **et** des secrets non régénérables (proposer
  l'ajout à `mantara-backup.sh` ou Restic, voir `docs/PHASE_7.md`) avec **restauration testée** sur
  une base temporaire ; espace disque et RAM mesurés (VictoriaMetrics et Loki gardent 30 jours).
- **8** : `docs/PHASE_10_VPS.md` complet, `README.md` (état réel, limites), dépôt
  `Aavkd/mantara-docs` (nouveau sous-domaine, conteneurs, tâches planifiées, métriques SSH),
  compte rendu final.
- Reste pour Alexy : supprimer le clone fait par erreur sur la tour (`D:\Prisme`).

## 6. Pièges déjà rencontrés

- Fichiers montés un par un (`observability.json`, `notifications.json`, `config.alloy`,
  Caddyfile) : après `git pull`, recréer le conteneur (`up -d --force-recreate --no-deps --wait api`
  ou `alloy`) ; le Caddyfile s'écrit en place.
- `docker compose run`/`exec` dans un script envoyé par `ssh 'bash -s'` : ajouter `< /dev/null`
  (sinon il avale la fin du script).
- Git Bash convertit les chemins `/…` : `export MSYS_NO_PATHCONV=1` pour `docker`. Windows
  PowerShell 5.1 : pas de `<` ; une ligne sur stderr d'un programme natif devient une erreur avec
  `$ErrorActionPreference = 'Stop'` ; `Set-Content -Encoding UTF8` ajoute un BOM (Go refuse un JSON
  avec BOM) ; `ValidatePattern` ignore la casse ; scripts `.ps1` du dépôt en ASCII.
- Chemins Windows écrits par un heredoc Python ou Bash : `\t` et `\v` deviennent des caractères de
  contrôle ; écrire ces fichiers avec l'outil d'écriture de fichiers.
- `date` du VPS est la version Rust (uutils) : préférer Python 3 pour les dates. VictoriaMetrics
  masque les 30 dernières secondes des requêtes de plage.
- Pas de `sleep` long dans un appel : lancer en arrière-plan et attendre la notification.
- Le moteur de règles ne lit qu'une série brute ; une donnée absente donne « inconnu », jamais un
  incident.
- Une règle absente de `notifications.json` ne notifie personne ; un test Go l'impose (tout nouveau
  `SITE-*-HTTP` doit y être urgent).

## 7. Fin de session

Journal à jour, commits poussés, CI verte (11/11, vérifiée job par job), ce fichier
(`docs/PROMPT_REPRISE_PORTABLE.md`) mis à jour avec l'état réel et la suite, et un compte rendu
simple à Alexy : ce qui marche, ce qui n'est pas vérifié, ce qui reste.
