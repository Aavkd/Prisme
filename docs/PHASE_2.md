# Phase 2 — Observabilité et incidents

État du 7 septembre 2026. L'observabilité locale prolonge les phases 0/1 sans
remplacer leurs identités, inventaires ou préférences. Prisme reste provisoire.
La tour conserve son NO-GO production ; aucun collecteur ni incident artificiel
n'a été installé ou déclenché sur OVH, Sounding Lines ou Arpent.

## Composants livrés

| Domaine | Fonctionnement |
|---|---|
| Alloy | Collecte Linux, cAdvisor, logs Docker sélectionnés, probes HTTP, module TCP, métriques propres au collecteur ; WAL métriques et positions de logs persistants |
| VictoriaMetrics | Single-node, séries temporelles, rétention détaillée 30 j, mémoire et coût des requêtes bornés |
| Loki | TSDB/filesystem, compactor, logs 30 j et erreurs 90 j ; limite de lignes, débit et nombre de streams |
| Diagnostic API | Catalogue de ressources configurées, métriques autorisées, recherche littérale et niveau des logs, fenêtre UTC commune ; session applicative requise |
| Alertes | Configuration explicite, seuils d'ouverture/récupération, hystérésis, durées minimales, nombre d'échantillons distincts, fraîcheur, maintenance et dépendances |
| Incidents | Détection, acquittement, diagnostic, surveillance, résolution automatique, clôture, réouverture sous 24 h, notes et chronologie paginée |
| Durabilité | Migration 004, transitions et audit atomiques, verrou d'évaluation, unicité d'incident actif et révisions contre les mises à jour concurrentes |
| Flutter | Destination Diagnostic, ressources, fenêtres 15 min/1 h/6 h/24 h, séries avec trous, logs corrélés, recherche, suivi toutes les 15 s et fiche incident |
| Prisme UI | Correction des cartes de tableaux mobiles : toucher la valeur ouvre la ligne ; la sélection reste disponible en lecture seule |

Les métriques hôte désignent **le système Linux de Docker Desktop**, pas Windows.
Les libellés serveur/projet/environnement/service servent à corréler les stores.
La ressource `local-pilot` est une fixture isolée et explicitement nommée. Ses logs
et métriques proviennent du vrai processus Go et de ses requêtes HTTP, pas de
données de démonstration intégrées dans le client.

## Lancer et vérifier localement

```powershell
./tool/dev.ps1 -Observability
python tool/test-observability.py
./tool/run-client.ps1
```

La commande Compose équivalente est :

```powershell
docker compose -f deploy/compose.dev.yml -f deploy/compose.observability.yml up --build --wait api alloy loki victoriametrics pilot
```

Les trois stores/collecteurs restent sur le réseau Docker `control` interne, sans
port publié. Le client utilise uniquement l'API TLS existante sur
`https://localhost:8443`. Ce profil de développement autorise HTTP **entre ces
conteneurs locaux**. Un collecteur distant nécessite un ingress HTTPS/mTLS
provisionné séparément, avec identité et révocation qualifiées. Le mode `identity`
refuse les URL de stores en HTTP. Le profil local n'est pas un déploiement distant.

Les images sont épinglées à Alloy 1.19.2, Loki 3.7.7 et VictoriaMetrics 1.151.0.
Sous Linux, définir `PRISME_DOCKER_SOCKET_GID` au GID de `/var/run/docker.sock`
avant Compose. La valeur par défaut 0 correspond au Docker Desktop testé.
Le collecteur tourne avec UID 473 et le groupe du socket ; toutes les capabilities
Linux sont retirées. Le cœur API et l'agent ne reçoivent aucun socket runtime.

Le socket Docker demeure un privilège administratif même monté `:ro`. Le montage
hôte de lecture permet aussi à cAdvisor de consulter le socket containerd requis
par le stockage `overlayfs` de Docker Desktop. Ne pas réutiliser ce profil sans
qualifier ces accès sur un serveur cible. Les labels collectés/persistés sont bornés ;
seuls les conteneurs `prisme.observation=true` du projet Compose `prisme-dev`
sont conservés par le pipeline. Le rapprochement cAdvisor/discovery utilise l'ID
du conteneur, puis retire cet ID des séries stockées.

Pour arrêter cette stack sans supprimer ses volumes :

```powershell
./tool/dev.ps1 -Observability -Stop
```

Un démarrage sans `-Observability` conserve le comportement phase 1. Arrêter avec
le même profil que celui utilisé au démarrage pour inclure tous ses services.

## Configurer des observations

`deploy/observability/config.alloy` définit les sources, leurs labels et probes.
`deploy/observability/config.json` définit les ressources interrogeables et règles.
Les deux doivent désigner les mêmes cibles. Une cible n'est jamais obtenue depuis
une URL fournie par le client : l'API n'offre ni proxy arbitraire ni PromQL/LogQL
libre. Il faut réviser la configuration puis redémarrer le service concerné.
L'API active l'évaluateur seulement si `PRISME_OBSERVABILITY_CONFIG` est défini.

Chaque scope contient un ID et les quatre labels. Ne pas réassigner un ID ayant
servi à un incident : le backend vérifie les associations persistées. Une nouvelle
cible doit recevoir un nouvel ID de scope et de règle. L'inventaire de phase 1
n'est pas silencieusement promu en ressource gérée.

Les règles utilisent un signal scalaire autorisé (`probe_success`, durée de probe,
expiration TLS, `up`, charge hôte, mémoire disponible, mémoire de conteneur, signal
de blocage de worker ou profondeur de file). Plusieurs séries pour la même règle
donnent un état inconnu : aucun choix arbitraire de conteneur ou de worker.
CPU, mémoire et disque en pourcentage sont des vues de diagnostic dérivées ; les
règles livrées n'acceptent pas d'expression libre. Les signaux worker restent
inconnus en l'absence d'un exporteur réellement raccordé.

Champs principaux d'une règle :

- `operator`: `below` ou `above` ; `open_threshold` et `resolve_threshold` ;
- `for_seconds`, `recovery_seconds`, `min_samples` (au moins deux) ;
- `freshness_seconds` entre 10 et 300 s ;
- `enabled`, `severity`, `scope_id`, identifiant et titre ;
- `maintenance_until` facultatif, UTC ; `inhibited_by` référence des règles
  déclarées plus tôt, ce qui interdit les cycles.

Le pilote local a des seuils de 15 s et trois échantillons pour rendre l'exercice
court. Ces délais ne sont pas une validation des seuils Sounding Lines/Arpent.
La règle de charge hôte est un exemple **désactivé**. Aucune notification, commande
de remédiation ou mutation Docker n'est exécutée par l'évaluateur.

## Sémantique des données manquantes et incidents

L'évaluateur travaille toutes les cinq secondes. Il lit les échantillons bruts
avec leurs dates d'origine, pas la date de la requête. Une même observation ne
compte qu'une fois. Une coupure, un échantillon ancien/futur/non numérique, une
réponse ambiguë ou une indisponibilité du store produit `unknown`.
Les durées de confirmation repartent après une coupure. Un incident existant
reste visible et ne se résout jamais sur absence de données.

Une maintenance ou une dépendance non saine bloque la confirmation des alertes
dérivées et conserve les incidents actifs. L'état de l'évaluateur devient lui-même
inconnu après 30 s sans actualisation. Le signal courant est distinct de l'état
historique de l'incident. Changer les paramètres d'une règle réinitialise les
preuves de confirmation sans supprimer son incident actif.

La résolution exige plusieurs nouvelles mesures de récupération sur la durée
configurée. Une récidive dans les 24 h rouvre le dernier incident résolu ; une
clôture explicite impose un nouvel incident. Les actions humaines requièrent la
révision courante ; un rejeu ou conflit renvoie 409 et n'ajoute pas de note en double.
La clôture n'est permise qu'après résolution vérifiée par la règle.

Les logs restent dans Loki. PostgreSQL conserve uniquement les métadonnées
d'incidents, états de règles, événements et audit. La fiche utilise le scope et la
fenêtre de l'incident pour interroger les deux stores ; au-delà de leur rétention,
la chronologie reste disponible et les données expirées ne sont pas fabriquées.

## Contrats et limites de lecture

Les graphiques limitent le report implicite du dernier point à 90 s ; les règles
emploient leur propre fraîcheur. Même sans suivi automatique, la vue passe un
état courant ancien en inconnu au lieu de conserver indéfiniment « sain ».

OpenAPI 0.3.0 documente les routes `/v1/observability`, `/metrics`, `/logs` sous ce
préfixe, `/v1/incidents`, le détail et les événements. La projection Dart accepte
désormais plusieurs paramètres de requête textuels correctement encodés.

Les lectures sont limitées à 24 h par requête, 100 séries, environ 300 points par
série, 200 lignes de logs, 50 incidents et 200 événements par page. Les réponses
des stores sont limitées à 2 Mio avec timeout de 3 s. Une erreur de store est
sanitisée et reste distincte d'un résultat vide. Le filtre texte est littéral.
Les logs sont auditables et sélectionnables pour un export borné ; il n'existe
aucun lien public de partage ni export illimité.

Les motifs de credentials sont masqués avant Loki et de nouveau à la lecture,
ainsi que dans les notes. Les labels de niveau sont normalisés dans Alloy. Le
masquage par motifs ne remplace pas la prévention des secrets à la source, notamment
pour les valeurs sans nom reconnaissable ou les secrets répartis entre plusieurs
lignes. Les sorties des services restent des données non fiables.

## Preuves et qualifications restantes

Commandes reproductibles :

```powershell
./tool/verify-foundation.ps1 -Integration
./tool/verify.ps1
python tool/test-observability.py
$env:PRISME_SMOKE_DIR = (Resolve-Path .dev).Path
cd apps/prisme_client
flutter test integration_test/observability_backend_test.dart -d windows
```

Le 7 septembre, le test de panne a vérifié détection, acquittement, résolution et
réouverture de l'incident 1, avec sept lignes d'erreur corrélées lors du dernier
exercice à 19:33 UTC, et de vraies mesures
CPU/mémoire hôte et mémoire du conteneur. Résultat dans `.dev/phase2-drill.json`.
Le test natif Windows a lu la même API et sa fiche résolue ; capture dans
`.dev/phase2-windows.png`. Les tests Flutter couvrent 390/1 440 px, le mode inconnu,
la même fenêtre métriques/logs, l'indisponibilité des sources, les actions désactivées
hors connexion et une réponse tardive après fermeture. Les tests Go vérifient
durées, hystérésis, doublons, échantillons invalides, maintenance, dépendances,
reprise, mutations concurrentes, redaction et bornes HTTP/SQL.

La CI contient les validations de contrats/Alloy et l'exercice local Linux. Son
résultat distant n'est pas déduit des tests locaux. L'Android natif/S24 de cette
phase et les propriétés matérielles Windows restent à qualifier séparément.

Au contrôle final du 8 septembre, Docker Desktop répond « unable to start ».
L'analyse Flutter finale ne signale aucun problème et les dix tests du client
réussissent, dont le cas d'une observation anciennement saine devenue inconnue.
Les 13 tests du socle et neuf tests du catalogue ont également réussi ; Go vet,
les tests Go et la synchronisation du contrat Dart sont vérifiés.
L'arrêt explicite des services de télémétrie n'a donc pas pu être confirmé ;
la dernière preuve du pilote est sa récupération réussie dans le rapport ci-dessus.
Après rétablissement de Docker, utiliser la commande `-Observability -Stop` pour
arrêter ces services, ou `-Observability` pour reprendre leur validation.

Le **critère logiciel local** de panne corrélée puis résolution est vérifié. Le
critère opérationnel sur un vrai service Sounding Lines ou Arpent reste ouvert :
il nécessite collecte continue autorisée sur la cible, configuration des labels,
contrôle du stockage et exercice de panne ciblé. Les profils détaillés de la
section 7.23 restent en observation avant validation des délais.

Ne sont pas annoncés comme livrés : connecteurs Codex/Claude et leur télémétrie,
pilotage de workers, requêtes PostgreSQL de santé, scénarios métier audio, corrélation
build/déploiement des phases suivantes, notifications/remédiation, cache disque
chiffré, gateway distante mTLS, agrégats de métriques 12 mois ou quotas par projet.
Loki/VictoriaMetrics ont leurs rétentions et limites d'ingestion/mémoire ; celles-ci
ne constituent pas un quota disque global strict. VictoriaMetrics cesse d'écrire
sous 1 Gio d'espace libre. Le dimensionnement et les protections globales de la tour
restent liés au diagnostic, sans autoriser sa mise en production.

## Références de configuration

- [Alloy blackbox](https://grafana.com/docs/alloy/latest/reference/components/prometheus/prometheus.exporter.blackbox/)
- [Alloy cAdvisor et accès runtime](https://grafana.com/docs/alloy/latest/reference/components/prometheus/prometheus.exporter.cadvisor/)
- [Traitement Loki dans Alloy](https://grafana.com/docs/alloy/latest/reference/components/loki/loki.process/)
- [Rétention Loki](https://grafana.com/docs/loki/latest/operations/storage/retention/)
- [VictoriaMetrics single-node](https://docs.victoriametrics.com/victoriametrics/single-server-victoriametrics/)
