# Phase 8 — Caddy et Cloudflare

La livraison locale ajoute `internal/edge`, `cmd/edge`, la migration 010 et une
page FR/EN dans le client Android/Windows. L'historique, les diffs, les plans
inverses et les approbations réutilisent Prisme UI et les preuves de phase 6.
L'API conserve les plans ; le worker séparé possède seul les accès fournisseur.

## Périmètre livré

- Import en lecture de la configuration Caddy active et source, version,
  routes/matchers/handlers/upstreams via JSON actif et détection de dérive.
  Les champs sensibles connus sont masqués dans les vues publiques.
- Premier import approuvé d'un fichier géré dédié, puis modifications bornées à
  son domaine et aux upstreams explicitement autorisés. Le fichier historique
  reste identique hormis le point d'import ajouté à sa fin au premier changement.
- Diff persistant, validation avant approbation, snapshot, rechargement atomique
  conditionnel, checks HTTP/HTTPS, restauration automatique et plan inverse.
- Zones/comptes configurés, DNS paginés, état des tunnels et connecteurs.
  Modification de destination A/AAAA/CNAME avec snapshot et plan inverse.
- Lecture/écriture séparées, désactivation par défaut, expiration des plans,
  idempotence, preuve fraîche par révision, audit et garde d'hôte indépendant.
- États inconnus/anciens, résultats de checks et historique dans les clients.

## Configuration

Partir de [config.example.json](../deploy/edge/config.example.json). Les IDs,
domaines et chemins sont des exemples, aucune infrastructure n'est enrôlée par
ce fichier. Les drapeaux `write_enabled` restent faux initialement.

1. Appliquer explicitement la migration avec `api -migrate`.
2. Fournir à l'API `PRISME_EDGE_CONFIG` et au worker `-config` avec le même contenu.
   Les chemins privés doivent être identiques ; seuls le worker et ses services
   ont besoin des fichiers correspondants. L'API ne monte pas les tokens.
3. Préparer le répertoire de fichiers gérés et ses droits, le binaire Caddy et
   le chemin exact du Caddyfile. Le worker doit partager le réseau loopback et
   les chemins de Caddy. Le binaire et ses modules doivent correspondre au serveur.
   `snippets` permet de réutiliser les snippets existants de sécurité, logs ou
   restriction réseau dans le bloc géré. Leur existence et leur expansion sont
   validées par Caddy ; aucun nom ni argument n'est fourni par le client.
4. Fournir un jeton de lecture limité aux zones/comptes nécessaires : Zone Read,
   DNS Read et Cloudflare Tunnel Read ; un second jeton DNS Write limité aux
   seules zones mutables. Pour une configuration sans Cloudflare, utiliser `zones: []`.
5. Configurer les checks du domaine modifié **et de tous les domaines à préserver**.
   Au moins un check doit correspondre au domaine cible. HTTPS utilise la
   validation TLS normale, sans option de contournement ni redirection automatique.
6. Utiliser le même `coordination_root` et le même `server` que les autres
   intervenants sur cet hôte. Activer l'écriture uniquement après qualification.

```sh
go build -o prisme-edge ./cmd/edge
./prisme-edge -config /etc/prisme/edge/config.json -validate
./prisme-edge -config /etc/prisme/edge/config.json
```

[prisme-edge.service](../deploy/systemd/prisme-edge.service) est un modèle à
adapter aux chemins et droits exacts ; il n'est pas installé automatiquement.
L'API admin Caddy doit être sur une adresse IP littérale loopback. Elle ne doit
pas être publiée par Tunnel. Le mode `http_only` est réservé à `environment:test`.

Un bloc historique existant n'est jamais extrait ou réécrit automatiquement.
S'il utilise déjà le domaine voulu, la validation refuse une définition en
double. Sa migration vers un fichier possédé doit faire l'objet d'une revue
explicite avant activation. Les fichiers gérés ne permettent pas de directives
arbitraires ; tout contenu non reconnu bloque la mutation.

## Vérifications

```powershell
./tool/test-edge.ps1 -Native
go vet ./...
go test ./...
go build ./...
dart run tool/generate_api.dart --check
cd apps/prisme_client
flutter analyze --no-pub
flutter test --no-pub
```

Le test natif utilise PostgreSQL isolé et Caddy 2.11.4 dans un conteneur sans
socket Docker, avec backend bleu/vert et domaine voisin synthétiques. Il prépare
et approuve les changements via les preuves WebAuthn virtuelles du vrai backend.
Le scénario couvre l'import, la modification, le plan inverse, la restauration
automatique après HTTP 503 et le refus d'une configuration invalide. Les tests
vérifient aussi la conservation d'un en-tête issu d'un snippet partagé. Les tests
Cloudflare utilisent un serveur HTTP simulé, jamais un compte réel.

Résultats locaux du 12 septembre 2026 : tests Go complets avec PostgreSQL,
`go vet`, compilation Windows/Linux du backend, validation OpenAPI et génération
Dart réussis ; analyse Flutter sans avertissement et 36 tests client réussis,
dont quatre scénarios edge aux formats 390/1440 px. Le scénario Caddy réel
ci-dessus et la validation Linux du fichier de configuration exemple ont réussi.
Aucune qualification native S24/Windows spécifique à cette page n'est revendiquée.

## Qualifications et limites

La route pilote **locale synthétique** constitue la preuve logicielle. Sounding
Lines/Arpent, leurs certificats TLS publics, la propagation DNS réelle, les
permissions Cloudflare effectives et les passkeys physiques restent à qualifier.
Le Caddy mutualisé du VPS, ses ports, son réseau et ses restrictions ne sont pas
modifiés. Le NO-GO production de la tour reste inchangé.

Les clients affichent le JSON d'inventaire actif ; les journaux d'accès/ACME,
la surveillance continue des certificats et la corrélation automatique avec les
services Docker restent du travail d'intégration avec l'observabilité existante.
Les modifications de tunnels, routes privées, Access, TLS, certificats d'origine,
purges de cache et créations/suppressions DNS ne sont pas exposées par ce premier
contrat de mutation. L'inventaire des comptes est limité aux IDs configurés.

Le contrôle DNS vérifie l'état fournisseur et les checks configurés ; il ne
prétend pas prouver une propagation mondiale. Cloudflare ne fournit pas de
condition atomique dans ce contrat DNS : éviter les écrivains concurrents. Les
modifications de fichiers historiques hors du garde local présentent également
une fenêtre de concurrence ; leur accès en écriture doit être coordonné.

Les snapshots SQL contiennent des configurations privées. Protéger PostgreSQL
et ses sauvegardes ; éviter les secrets littéraux dans les Caddyfiles. Le worker
n'affiche jamais stderr ou les corps d'erreur des fournisseurs. La rétention des
plans/snapshots n'est pas encore automatisée.
