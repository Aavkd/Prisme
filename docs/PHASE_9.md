# Phase 9 — Autonomie et runbooks

La livraison locale ajoute la migration 011, `internal/automation`, le worker
Linux `cmd/automation` et une page FR/EN Android/Windows réutilisant Prisme UI.
Le critère logiciel est vérifié sur une panne Docker Compose locale contrôlée.
Aucune politique des vrais pilotes n’est activée ; le NO-GO de la tour reste
inchangé.

## Livré

- Politiques versionnées supervisé/assisté/autonome, liées à un hôte, projet,
  environnement, service et signal d’observabilité précis.
- Restart d’un seul conteneur possédé ; rollback par le moteur de phase 4 vers
  une release immutable déjà saine, avec ses contrôles existants.
- Seuil d’échec, fenêtre, fraîcheur, dépendances/exclusions, nombre total de
  tentatives, délai exponentiel, observation, timeout, cooldown et circuit.
- Une seule exécution par incident et politique ; circuit persistant à réarmer
  explicitement, sans possibilité de rejouer le même incident.
- Runbooks immuables, paramètres typés, conditions, attentes durables, retries
  bornés des checks, simulation et approbation de chaque étape sensible.
- Planification persistante par intervalle UTC et version exacte, sans rattrapage
  en rafale ; maintenance globale/par définition et arrêt général.
- Journal corrélé à l’incident, audit et notification d’ouverture de circuit via
  le worker FCM existant ; historique et administration dans les clients.
- Arrêt propagé aux jobs de rollback et réconciliation explicite des résultats
  incertains avant toute nouvelle action sur la même cible.

Le [contrat automation v1](../contracts/automation-v1.md) précise les bornes,
les preuves, les états et les limites des adaptateurs de cette livraison.

## Configuration locale

1. Appliquer explicitement la migration avec `api -migrate`.
2. Adapter [config.example.json](../deploy/automation/config.example.json) aux
   identifiants exacts d’un manifeste de déploiement déjà qualifié et d’une règle
   d’observabilité existante. Le worker exige un service stateless et un garde
   d’hôte partagé avec les autres intervenants.
3. Fournir ce fichier à l’API par `PRISME_AUTOMATION_CONFIG`. L’API ne lance pas
   de commandes Docker. Fournir au worker le même fichier et la configuration
   de déploiement existante, avec ses références privées et droits Docker.
4. Utiliser une session passkey pour publier les
   [politiques](../deploy/automation/policy.example.json) et
   [runbooks](../deploy/automation/runbook.example.json) depuis la page Autonomie.
   La première version de chaque identifiant est 1, puis les versions augmentent
   strictement. L’éditeur montre la définition complète avant publication.
5. Vérifier d’abord le mode supervisé, puis les approbations assistées. Activer
   l’autonomie uniquement pour les actions réellement qualifiées. Les exemples
   sont supervisés et l’arrêt général SQL est actif initialement.

```sh
go build -o prisme-automation ./cmd/automation
./prisme-automation -config /etc/prisme/automation/config.json \
  -deployments /etc/prisme/deployment/config.json -validate
./prisme-automation -config /etc/prisme/automation/config.json \
  -deployments /etc/prisme/deployment/config.json
```

[prisme-automation.service](../deploy/systemd/prisme-automation.service) est un
modèle à adapter, pas un service installé. Conserver le même `coordination_root`,
la même identité serveur et le réseau loopback du worker de déploiement.
Fournir `PRISME_DATABASE_URL` par un fichier d’environnement privé ; le mode local
peut aussi utiliser `PRISME_DB_PASSWORD_FILE` et `PRISME_DB_HOST` selon le mécanisme
existant de `internal/config`. Ne pas placer de secrets dans les runbooks.

Un arrêt ne promet pas d’annuler une commande déjà reçue par Docker. Après un
résultat incertain, inspecter l’hôte et ses processus, réconcilier son garde avec
`host-guard -reconcile` et l’epoch observé, puis confirmer la réconciliation de
l’exécution dans le client. Le worker conserve l’incertitude tant que le garde
ou le verrou du déploiement est occupé. Il retire les jobs liés sans les rejouer.

## Vérifications

```powershell
./tool/test-automation.ps1 -Native
go vet ./...
go test ./...
go build ./...
dart run tool/generate_api.dart --check
cd apps/prisme_client
flutter analyze --no-pub
flutter test --no-pub
```

Le scénario natif utilise PostgreSQL isolé et deux images locales connues :
Caddy sain puis nginx sain. La suppression de la page d’accueil du conteneur
nginx appartenant au test provoque une vraie erreur HTTP. Le restart de ce
service conserve la panne ; le rollback recrée Caddy depuis son digest sain.
L’essai vérifie **deux tentatives et vingt événements**, le job de déploiement
lié et la santé réelle restaurée. Seul le projet `prisme-phase-nine-live-blue`
créé par ce test est supprimé à la fin.

Les tests PostgreSQL couvrent aussi l’échec persistant (restart, rollback puis
circuit, sans troisième action), la récupération évitant le rollback, les
modes, preuves et révisions, l’inhibition, les interruptions, la déduplication
des notifications et des planifications, les versions figées et l’arrêt en
pleine mutation. Les tests Flutter couvrent 390/1440 px, l’arrêt, la simulation,
l’exigence de preuve, les réponses tardives après verrouillage et le module
indisponible.

Résultats finaux du 13 septembre 2026 : suite Go complète avec PostgreSQL
(`go test -p 1 -count=1 ./...`), `go vet`, compilations Windows et Linux, validation
OpenAPI et contrôle de génération Dart réussis. Analyse Flutter sans avertissement
et **40 tests client réussis**. Le dernier scénario Docker réel termine en
24 secondes avec deux tentatives et vingt événements. Les vérifications de
formatage et de whitespace sont également propres.

## Qualification restante

Les politiques initiales de Sounding Lines/Arpent, leurs seuils et délais, les
fenêtres de maintenance récurrentes et la notification de circuit sur S24 réel
restent à qualifier. Cette page n’a pas de validation native physique propre à
la phase 9. Le worker ne pilote que les stacks immuables possédées par le moteur
de déploiement ; les stacks historiques importées en lecture ne deviennent pas
automatiquement gérées.

Cette première bibliothèque d’étapes n’orchestre pas encore les APIs de build,
sauvegarde, migration ou edge. Les calendriers utilisent des intervalles UTC,
la maintenance une date de fin absolue, et l’éditeur est une définition JSON
validée plutôt qu’un constructeur visuel. La rétention automatique des versions
et historiques reste un chantier de durcissement. La phase suivante est la
phase 10, avec qualification des pilotes, reprise et durcissement de production.
