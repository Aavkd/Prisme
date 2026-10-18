# Conventions du monorepo

**État courant :** schéma 10, OpenAPI 0.9.0. La phase 8 ajoute `internal/edge`,
`cmd/edge` et les plans Caddy/Cloudflare ; voir [PHASE_8](PHASE_8.md). La phase 7 ajoute `internal/backup`,
`cmd/backup`, une file durable et des cibles de restauration isolées. Voir
[PHASE_7](PHASE_7.md) et [backup-v1](../contracts/backup-v1.md).
La phase 6 ajoute `internal/operations`,
`internal/coordination`, `internal/recovery`, `cmd/host-guard` et `cmd/recovery`.
Le worker Docker reste séparé de l'API ; voir `PHASE_6.md` pour les limites et
le garde commun. Les numéros des sections historiques désignent leurs livraisons.

- Nom de travail : **Prisme**, explicitement provisoire. Identifiants locaux :
  `prisme`, module `prisme.local/platform`, app `dev.mantara.prisme_client`.
  Aucun domaine, package de distribution ou dépôt distant n'est réservé.
- `apps/prisme_client` : application Flutter Android/Windows, transports et métier.
- `packages/prisme_ui` : présentation partagée ; API publique uniquement.
- `cmd` : exécutables Go ; `internal/api`, `store`, `pki`, `agent`, `config` : frontières
  du monolithe. Aucun accès fournisseur ou commande libre dans les handlers.
- `contracts` : contrats versionnés ; `deploy` : environnement local ; `docs/adr` : décisions.
- Go **1.26.8**, Flutter **3.38.5**, Dart **3.10.4** ; `go.sum` et lockfiles Dart versionnés.
  Le SDK Go peut être téléchargé automatiquement via `GOTOOLCHAIN=auto` ; le Go 1.22
  installé sur le poste n'est pas le compilateur retenu pour ce projet.
- UTF-8, LF, `gofmt`, `dart format`, analyse et tests avant livraison. Aucun secret
  dans les sources, arguments CLI, logs, images, captures ou fixtures versionnées.
- API `/v1`, JSON snake_case, UTC RFC3339, identifiants opaques ; aucun ID issu d'un
  nom de machine. Les chaînes `expires_at` et `observed_at` sont converties à la
  frontière métier ; `null` signifie observation indisponible.
- Évolution SQL par migration numérotée explicite. La migration 001 est atomique,
  protégée par verrou PostgreSQL ; le serveur refuse une version incompatible.
  Aucun down destructif automatique. Sauvegarde et migration corrective pour revenir.
- Les tests d'intégration utilisent un schéma temporaire à nom généré dans la base
  de test, puis suppriment **uniquement ce schéma**. Aucune base pilote utilisée.
- Branches `codex/…`, commits centrés sur un résultat. Les migrations, le contrat,
  le code généré et leurs tests évoluent ensemble.
- Le générateur Dart est volontairement limité aux objets à champs requis,
  scalaires, nullables, tableaux, références et dictionnaires de chaînes de phase 0.
  Il échoue sur une forme non prise en charge. Ne pas le présenter comme un
  générateur OpenAPI généraliste ; remplacer ou étendre avant une évolution complexe.
- Les chaînes métier restent FR/EN dans le client. La bibliothèque conserve les
  thèmes existants. Aucun secret ni préférence sensible n'est persisté par le client.

Vérifications : `tool/verify.ps1` pour Flutter ; `tool/verify-foundation.ps1 -Integration`
pour Go/PostgreSQL/TLS et la génération ; CI dans `.github/workflows/ci.yml`.

## Phase 1

Le schéma courant est **3** (migrations 001, 002 et 003). Le mode `identity` exige
une origine HTTPS explicite et refuse la session de développement. Le contrat
OpenAPI couvre les passkeys, appareils, préférences, flotte, audit et inventaire.
Le générateur accepte également les dictionnaires JSON, les objets nullables et
les paramètres de chemin ; il reste un générateur limité au contrat de ce dépôt.
Les unités systemd sont des modèles ; aucune installation distante n'est implicite.
Voir `PHASE_1.md` et l'ADR 0003 pour les preuves et qualifications restantes.

## Phase 2

Schéma courant **4**, OpenAPI **0.3.0**. Les incidents/événements utilisent des
identifiants séquentiels transmis comme chaînes et des révisions pour la concurrence ;
ils n'accordent aucun accès sans session. Les scopes/règles sont des identifiants
stables de configuration, distincts des identités opaques d'agents.
La migration 004 ajoute les états de règles et la chronologie sans logs bruts.
Les expressions des stores restent définies côté serveur ; paramètres URL encodés,
réponses, cardinalités et fenêtres bornés. Voir [PHASE_2](PHASE_2.md).

## Phase 3

Schéma courant **5**, OpenAPI **0.4.0**. `internal/github` contient le fournisseur
GitHub App et l'import des releases, `internal/api/github.go` les routes privées et
le webhook signé. Les IDs GitHub restent des chaînes aux frontières métier ;
l'identité d'une release est dérivée du dépôt/run/tentative. Les mappings de dépôt,
workflow de publication et environnement sont immuables sans migration explicite.
Voir [PHASE_3](PHASE_3.md) et le [contrat Release](../contracts/release-v1.md).

## Phase 5

Schéma courant **7**, OpenAPI **0.6.0**. Le transport FCM et le watchdog résident
dans `internal/notifications`, les preuves d'action dans `internal/api`. La
consommation de preuve reste dans la transaction d'approbation. Le client natif
Android signe via un canal restreint, sans secret serveur. Les fichiers Firebase
sont ignorés par Git et exclus du contexte d'image. Le cache sélectionne des
résumés bornés par origine/appareil ; sessions et logs détaillés restent exclus.
Configuration, preuves physiques et limites : [PHASE_5](PHASE_5.md).
