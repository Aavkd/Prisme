# Prisme client — phases 0 à 9

**Autonomie** ajoute les politiques et runbooks versionnés, leurs paramètres,
la simulation, les approbations par étape, les circuits, la planification,
la maintenance et l’arrêt général. Les preuves restent liées à la version et à
l’action ; l’écran efface les réponses tardives après verrouillage. Voir
[PHASE_9](../../docs/PHASE_9.md) et le [contrat](../../contracts/automation-v1.md).

**Terminal et fichiers** assemble le PTY natif xterm, les fichiers SFTP et le
coffre sur Prisme UI. Les actions passent par des preuves liées à la requête.
L'accès de secours ouvre une connexion et une identité indépendantes, accessibles
avant la connexion normale ; aucune commande n'est mise en cache ni rejouée.
Voir [PHASE_6](../../docs/PHASE_6.md) et le
[contrat opérations](../../contracts/operations-v1.md).

La destination **Diagnostic** ajoute ressources, santé observée, métriques, logs
et incidents avec chronologie et actions auditées. Elle utilise le contrat 0.3.0
et les composants Prisme UI. Voir [PHASE_2](../../docs/PHASE_2.md) pour lancer
la collecte locale et son test de panne. Les tests mobiles couvrent 390 px ;
le test natif Windows lit la vraie API TLS.

Client Flutter Android/Windows, version 0.2.0+2, construit sur `packages/prisme_ui`.

Il fournit la connexion locale explicite ou les passkeys via navigateur système,
les vues Serveurs/Compose, les appareils et la récupération, l'audit et une sélection
d'accueil propre à l'installation. Les données viennent du backend ; aucune fixture
n'est utilisée dans l'application. Les sessions restent en mémoire. Les épingles
sont en base et l'apparence/locale sont conservées localement.

Depuis la racine du dépôt : `./tool/dev.ps1`, puis `./tool/run-client.ps1`.
Pour l'inventaire réel, les commandes et limites sont dans
[le guide phase 1](../../docs/PHASE_1.md). Prisme reste provisoire.

Depuis ce dossier :

```powershell
flutter analyze --no-pub
flutter test --no-pub
```

L'intégration native utilise `integration_test/local_backend_test.dart` contre
le vrai backend TLS. Le helper Android de la racine stage les entrées uniquement
dans le sandbox privé d'un émulateur. Aucune clé ou session n'est embarquée dans
l'application. La CA de développement, publique, peut être fournie explicitement.
Le mode identité requiert une origine HTTPS configurée ; le domaine final et
Windows Hello/S24 restent à qualifier avant exploitation.

La destination **GitHub** ajoute les builds, jobs/étapes/logs, artefacts, releases
avec images par digest, workflows, fichiers, branches, tags, commits et PR. Les
requêtes passent par le backend ; aucune clé GitHub n'est fournie au client.
L'intégration affiche son état désactivé tant que l'App n'est pas configurée.
Voir [phase 3](../../docs/PHASE_3.md) pour les instructions et limites réelles.

## Déploiements (phase 4)

La destination Déploiements utilise le contrat OpenAPI 0.5.0 : plans, approbation
avec phrase, historique, preuve par étape et rollback. L'API conserve l'autorité
sur la révision, la fraîcheur et les préconditions. Le client efface les données
au verrouillage. Voir le [guide phase 4](../../docs/PHASE_4.md) pour activer un import local.

## Mobile et notifications (phase 5)

La destination Notifications configure FCM, les horaires par appareil, la clé
biométrique Android et le cache chiffré opt-in. Les notifications ouvrent un
incident authentifié. Les approbations production demandent une signature
Android après empreinte ou une nouvelle vérification passkey Windows. Le cache
possède sa propre vue en lecture seule après déverrouillage local.
Voir [PHASE_5](../../docs/PHASE_5.md) pour la configuration et le test physique S24.

## Phase 8 — Caddy et Cloudflare

La destination Caddy · Cloudflare utilise les DTO OpenAPI 0.9.0 pour consulter
les observations datées, préparer et examiner un diff, approuver par preuve
fraîche et préparer un rollback. Les mutations sont exécutées par le worker.
Les états de chargement, lecture seule, données anciennes, échec et révocation
sont couverts aux largeurs 390 et 1440 px. Voir [PHASE_8](../../docs/PHASE_8.md).
