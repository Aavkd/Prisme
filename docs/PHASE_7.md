# Phase 7 — Sauvegardes et restaurations

Implémentation locale du 11 septembre 2026. Schéma **9**, OpenAPI **0.8.0**.
Les dépôts, politiques et cibles restent désactivés tant qu'ils ne sont pas
configurés. Aucun accès à une base pilote de production, déploiement sur le VPS
ou publication de service n'a été effectué.

## Fonctionnement livré

- Worker Restic séparé : PostgreSQL logique, fichiers et arbres de volumes
  arrêtés/snapshots fournis explicitement par l'opérateur.
- Dépôts chiffrés locaux, montés ou S3 HTTPS, mot de passe extérieur à PostgreSQL,
  credentials privés sur le worker et aucune commande fournisseur dans l'API.
- Travaux durables, idempotence, planification par intervalle, audit, étapes,
  historiques paginés et incidents d'échec raccordables à FCM.
- Lecture d'intégrité complète avant publication d'une copie utilisable ;
  restauration avec vérification du contenu extrait.
- Rétention dernières/quotidiennes/hebdomadaires : simulation préalable,
  protection des dernières copies vérifiées, filtrage par politique et expiration
  des métadonnées. Une copie partielle non enregistrée ne remplace pas la dernière
  preuve de sauvegarde. Pas de suppression automatique des snapshots non suivis.
- Plan de restauration, phrase explicite et preuve passkey/Android. Nouvelles
  bases ou nouveaux répertoires de test uniquement ; restauration SQL atomique,
  rôle non-superuser et contrôle fonctionnel en lecture seule.
- Tests de restauration PostgreSQL planifiables. La réussite comprend le nettoyage
  de la base temporaire ; une base manuelle ou en échec reste inspectable.
- Coordination avec le garde de phase 6, exclusion entre workers et réconciliation
  après interruption. Aucun rejeu automatique d'une mutation incertaine.
- Écran Flutter FR/EN : politiques, destination, dernière preuve, rétention,
  copies disponibles, cible alternative, approbation, étapes, résultats et alerte
  lorsqu'une politique n'a pas de copie vérifiée sur une destination déclarée
  hors site. Les données disparaissent au verrouillage de session.
- Déploiements sensibles : lien vers une politique et une copie exacte,
  vérification du projet/environnement/serveur et de l'âge maximal à la préparation,
  à l'approbation et avant mutation. Une copie expirée ou un échec récent
  d'intégrité du dépôt maintient le blocage.

Voir le [contrat backup v1](../contracts/backup-v1.md) pour les routes et limites.

## Préparer un worker Linux

1. Installer Restic **0.18 ou supérieur**, les clients PostgreSQL de la version
   adaptée au serveur source, puis construire `go build -o prisme-backup ./cmd/backup`.
   Le binaire d'API ne nécessite aucun outil PostgreSQL/Restic ni secret fournisseur.
2. Copier [config.example.json](../deploy/backups/config.example.json) dans une
   configuration privée. Choisir les IDs définitifs, cibles, intervalles et état
   `enabled` **avant le premier enregistrement**. Toute révision ultérieure de la
   politique utilise un nouvel ID ; conserver les anciens IDs pour leurs copies.
3. Préparer `work_root`, les racines de restauration et `coordination_root` en
   0700 sous l'UID opérationnel partagé avec les autres workers/gardes du même
   hôte. Préparer un dépôt extérieur aux sources sauvegardées. Prévoir l'espace
   pour les archives temporaires, bases de test, rétention et croissance du dépôt.
4. Créer un mot de passe aléatoire privé en 0600, au moins 16 caractères, et sa
   copie hors ligne. Le fichier `location_file` contient uniquement le chemin
   absolu du dépôt ou `s3:https://endpoint/bucket/prefix`. `credentials_file`,
   privé et requis pour S3, est un objet JSON contenant uniquement les variables
   AWS autorisées (access key, secret key, session token, région).
5. Préparer [pg-service.example.conf](../deploy/backups/pg-service.example.conf),
   ses certificats et son fichier pgpass en 0600. Source : rôle de dump restreint.
   Cible : cluster de test, compte **non-superuser avec CREATEDB**, distinct du
   compte source. Les restaurations créent un nom de base généré, ne remplacent
   aucune base configurée et n'utilisent jamais `--clean`.
6. Remplacer [validate-pilot.example.sql](../deploy/backups/validate-pilot.example.sql)
   par un `SELECT` métier retournant exactement `t` : tables attendues, données
   cohérentes et invariants utiles. L'exemple retourne volontairement `false`.
   L'opérateur possède ce fichier ; le client ne fournit jamais de SQL.
7. Valider puis initialiser le dépôt explicitement :

   ```sh
   prisme-backup -config /etc/prisme/backup/config.json -validate
   prisme-backup -config /etc/prisme/backup/config.json -init-repository local
   ```

   Une erreur de mot de passe n'entraîne jamais une réinitialisation.
8. Appliquer la migration avec `api -migrate`, puis configurer
   `PRISME_BACKUP_CONFIG` dans l'API. Configurer la connexion PostgreSQL du worker
   comme celle des autres exécutants via `PRISME_DATABASE_URL` dans un fichier
   d'environnement privé (ou `PRISME_DB_HOST` / `PRISME_DB_PASSWORD_FILE` pour le
   réseau local isolé de développement) et lancer le
   worker avec `-config`. Adapter [l'unité systemd](../deploy/systemd/prisme-backup.service),
   ses chemins inscriptibles et son fichier d'environnement.
9. Configurer `backup-<politique>-backup`, `-check`, `-retention` et `-restore`
   dans les règles de notifications pour recevoir leurs incidents. FCM suit les
   règles et horaires silencieux existants ; sans raccordement, l'incident reste
   visible dans Prisme mais aucun push n'est promis.

La limite CPU/mémoire et le poids I/O sont fournis dans le modèle systemd.
`upload_kib`, `download_kib` et `timeout_seconds` bornent transfert et durée.
Les quotas disque et limites de bande passante I/O du périphérique nécessitent
les montages/cgroups réels. Les octets et horodatages de l'historique permettent
d'évaluer une prochaine exécution ; aucune estimation préalable précise n'est
annoncée avant une première sauvegarde.

Pour fichiers/volumes : `kind: files|volume`, `path` absolu, pas de service PG.
`consistency: crash` est admis uniquement pour les fichiers ; `stopped` ou
`snapshot` convient aux arbres de volumes préparés par l'opérateur. Le worker ne
stoppe pas Docker et ne rend pas cohérente une base vivante copiée comme fichiers.
Les volumes Docker s'exposent au worker par un montage explicitement autorisé.

## Restaurer et vérifier

Dans **Sauvegardes**, choisir politique, copie retenue et cible alternative,
puis **Préparer la restauration**. Lire la source, la date de vérification, la
taille, la cible et les étapes. Approuver avec la phrase affichée et une preuve
fraîche. La session de développement permet les sauvegardes mais pas cette
approbation sensible.

La base de test générée reste disponible après une restauration manuelle. Le
résultat indique son nom et la validation SQL ; l'opérateur l'inspecte puis la
supprime explicitement avec le compte de test. Dix bases de test au maximum sont
admises pour ce rôle avant blocage de nouvelles créations. Un fichier de dump
temporaire n'est pas conservé après l'exécution normale. Un arrêt brutal peut
laisser des archives privées dans `work_root` : les inspecter/nettoyer après
réconciliation, sous contrôle du propriétaire du service.

Avant un déploiement sensible, exécuter une sauvegarde, puis préparer un nouveau
plan Compose avec `backup_required: true`, `backup_policy` et
`backup_max_age_seconds`. La copie doit concerner exactement le même projet,
environnement et serveur. Le worker de déploiement ne lance pas une sauvegarde
implicitement et ne change pas la copie liée à un plan déjà approuvé.

Après une interruption : inspecter processus Restic/PG, bases créées, fichiers et
verrous du dépôt. Réconcilier le garde avec son époque exacte, puis :

```sh
prisme-backup -config /etc/prisme/backup/config.json -acknowledge <job-id>
```

Ce geste marque le travail incertain en échec. Il ne rejoue rien et ne déverrouille
pas un processus Restic encore actif. Arrêter le worker et retirer sa configuration
de l'API pour désactiver le module ; conserver migration, données, dépôt et clés.

## Vérification locale et qualification restante

La suite `TestPhaseSevenLiveResticPostgresAndFiles` utilise une base et un rôle
PostgreSQL isolés, générés pour le test, ainsi qu'un dépôt Restic temporaire. Elle
effectue un vrai dump chiffré, une restauration de schéma/index/données avec
assertions métier, un rejet de validation incorrecte, une copie/restauration de
fichiers, une rétention entre politiques et un refus de mot de passe incorrect.
Les tests couvrent aussi l'idempotence, les preuves et rejeux, les incidents,
l'exclusion des workers, le garde de secours et les interruptions sans rejeu.

Vérifications acquises : suite Go complète avec PostgreSQL local ; scénario
Restic/PG réel incluant nettoyage du test planifié, corruption d'un bloc chiffré,
protection contre l'éviction par snapshots non enregistrés et blocage des
déploiements sur copie périmée ; `go vet`, contrat OpenAPI et génération Dart ;
analyse Flutter sans diagnostic, **32 tests Flutter**, builds debug Windows et
Android. Le backend a également été compilé pour Linux amd64.

```powershell
go test ./...
go vet ./...
dart run tool/generate_api.dart --check
./tool/test-backups.ps1
cd apps/prisme_client
flutter analyze --no-pub
flutter test --no-pub
```

Une image CI Linux isolée contient Restic et PostgreSQL, sans socket Docker,
volume utilisateur ni port publié. Docker Desktop restant bloqué sur son socket
d'inférence lors de cette tâche, le test live local a été exécuté avec des outils
portables **PostgreSQL 17.6 / Restic 0.18.0** sur Windows, sans installation système.
Cela valide les commandes réelles et l'intégration PostgreSQL ; cela ne qualifie
pas les limites cgroup, processus Linux, montages ou déploiement systemd.

Pour reproduire ce scénario sous Windows avec des binaires déjà disponibles :

```powershell
./tool/test-backups-native.ps1 -PostgresBin <repertoire-pg-bin> -ResticBin <repertoire-restic-exe>
```

Le script démarre une base temporaire sur loopback, exécute les tests, puis
l'arrête. Il conserve uniquement les données/logs de fixture dans `.dev/`.

Le critère opérationnel d'une copie d'une **vraie base pilote** restaurée sur sa
cible de test reste à qualifier. Les données synthétiques du scénario local ne
sont pas une sauvegarde de Sounding Lines ou d'Arpent. Restent aussi : destination
hors site réelle, conservation indépendante des clés, droits de dump/restore du
pilote, règles FCM réelles, capacités disque et comportement après panne d'hôte.
L'autorisation de production et le NO-GO de la tour restent inchangés.

WAL/PITR, sauvegarde physique, SFTP natif, arrêt automatique des applications,
restauration sur une base existante avec maintenance et réouverture du trafic,
et nettoyage global des anciennes sauvegardes SFTP de phase 6 restent hors de
cette livraison. Les répertoires de sauvegarde SFTP peuvent être inclus dans une
politique de fichiers explicite, mais le worker ne supprime pas leurs originaux.
