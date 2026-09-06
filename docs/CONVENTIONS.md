# Conventions du monorepo

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
