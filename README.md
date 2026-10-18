# Prisme

Plateforme de supervision et de pilotage d'infrastructure, prévue pour Windows et Android.

**Prisme est un nom provisoire.** La phase 9 ajoute les politiques bornées, les
runbooks versionnés, la planification, la maintenance et l’arrêt général. Le
restart puis rollback sont vérifiés sur une panne Docker locale ; voir
[phase 9](docs/PHASE_9.md). L’autonomie des vrais pilotes reste désactivée.

La phase 8 ajoute les plans Caddy/Cloudflare,
le worker séparé, les diffs et les restaurations vérifiées. Voir
[phase 8](docs/PHASE_8.md) pour la configuration et les qualifications restantes.

La phase 7 ajoute Restic, les sauvegardes
PostgreSQL/fichiers/volumes, la rétention, les restaurations de test approuvées
et les preuves de sauvegarde liées aux déploiements. Voir [phase 7](docs/PHASE_7.md)
pour la configuration, les essais réels locaux et la qualification pilote restante.

La phase 6 ajoute les terminaux PTY, SFTP,
l'éditeur avec sauvegarde atomique, le coffre chiffré, les fichiers `.env` et une
passerelle de secours indépendante avec garde d'hôte et audit signé. Les essais
locaux utilisent OpenSSH réel ; le VPS reste à déployer/qualifier. Voir
[phase 6](docs/PHASE_6.md) pour la configuration explicite et les limites.

La phase 5 ajoute FCM, les liens d’incident,
les horaires silencieux, le cache chiffré, les approbations biométriques et le
watchdog indépendant. Le parcours FCM/empreinte a été vérifié sur le S24 physique
avec une panne HTTP locale réelle. Voir [phase 5](docs/PHASE_5.md).
La phase 4 ajoute les plans approuvés, le worker
Compose durable, les checks, le rollback et un blue-green vérifié sur une stack
locale réelle. La qualification opérationnelle de Sounding Lines reste ouverte.
La phase 3 ajoute la consultation GitHub App,
les builds, logs, sources et releases liées aux images par digest, avec validation
locale sur fixtures. Le raccordement réel des deux pilotes reste à qualifier.
La phase 2 ajoute Alloy, VictoriaMetrics, Loki,
les diagnostics et le cycle de vie des incidents, vérifiés par une panne locale
contrôlée. La phase 1 apporte passkeys, appareils/récupération,
cycle de vie des certificats, capacités, inventaire Docker/Compose, audit et vues
Serveurs au client Flutter et backend Go/PostgreSQL existants. Le VPS est consulté
en lecture seule et ses observations sont importées localement. La tour reste en
NO-GO production selon son diagnostic existant.

- [Spécification produit](OPS_INFRASTRUCTURE_PLATFORM_SPEC.md)
- [Direction artistique et maquettes](design/ART_DIRECTION.md)
- [Bibliothèque de composants et intégration](packages/prisme_ui/README.md)
- [Contrat frontend et prochaines étapes](docs/FRONTEND_FOUNDATIONS.md)
- [Résultats des vérifications](docs/UI_VALIDATION.md)
- [Phase 0 : démarrage, preuves et limites](docs/PHASE_0.md)
- [Phase 1 : flotte, identité, vérifications et limites](docs/PHASE_1.md)
- [Phase 2 : observabilité, incidents et exercice local](docs/PHASE_2.md)
- [Phase 3 : GitHub, builds et releases](docs/PHASE_3.md)
- [Phase 4 : déploiements, approbation et rollback](docs/PHASE_4.md)
- [Phase 5 : mobile, FCM, biométrie et cache](docs/PHASE_5.md)
- [Phase 6 : terminal, SFTP, coffre et secours](docs/PHASE_6.md)
- [Phase 7 : Restic, sauvegardes et restaurations](docs/PHASE_7.md)
- [Phase 8 : Caddy, Cloudflare et plans de changement](docs/PHASE_8.md)
- [Phase 9 : autonomie, runbooks et planification](docs/PHASE_9.md)
- [Conventions du monorepo](docs/CONVENTIONS.md)
- [Contrat OpenAPI](contracts/openapi.json) et [contrat agent](contracts/agent-v1.md)
- [Modèle de menaces](docs/THREAT_MODEL.md)

## Démarrer la fondation locale

Go 1.26.8 (téléchargement automatique depuis Go si nécessaire), Flutter 3.38.5
et Docker Desktop avec moteur Linux sont requis.

```powershell
./tool/dev.ps1
go run ./cmd/inventory -ssh ovh-mantara -api https://localhost:8443 -dev-secret-file .dev/bootstrap-secret -name 'OVH Mantara'
./tool/run-client.ps1
```

Le script crée les secrets dans `.dev/` avec ACL privées, démarre uniquement le
projet Compose `prisme-dev` et publie l'API TLS sur `127.0.0.1:8443`. Le client
demande le secret local de `.dev/bootstrap-secret` dans un champ masqué. Ne pas
le copier dans une commande, une capture ou un commit. `./tool/dev.ps1 -Stop`
arrête la stack en conservant les données.

Les [instructions détaillées](docs/PHASE_0.md) couvrent l'agent conteneurisé,
Android via ADB, les tests natifs et les limites de cette authentification locale.

Pour l'observabilité locale, lancer `./tool/dev.ps1 -Observability`, puis
`python tool/test-observability.py`. Le [guide phase 2](docs/PHASE_2.md) décrit
les stores, le collecteur, le pilote de test et les qualifications de production.

## Essayer les composants

Flutter 3.38.5 / Dart 3.10.4 utilisés pour cette première version.

```powershell
cd packages/prisme_ui/example
flutter pub get
flutter run -d windows
```

Le catalogue possède sept rubriques, les trois thèmes, deux densités et les langues FR/EN. Toutes les données sont fictives. `Ctrl+K` ouvre la recherche de composants.

Pour une revue dans le navigateur local :

```powershell
flutter run -d chrome
```

Le navigateur sert uniquement à la revue des composants. Les clients produit restent Android et Windows.

## Vérifier les changements

Depuis la racine, `./tool/verify.ps1` installe les dépendances déclarées, vérifie les couleurs générées, le formatage, l'analyse statique et les tests de la bibliothèque et du catalogue. Ajouter `-BuildWindows` et/ou `-BuildWeb` pour compiler le catalogue.

Les maquettes HTML restent consultables séparément dans `design/`.
