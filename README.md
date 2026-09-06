# Prisme

Plateforme de supervision et de pilotage d'infrastructure, prévue pour Windows et Android.

**Prisme est un nom provisoire.** La phase 0 fournit un client Flutter authentifié,
un backend Go/PostgreSQL local et un agent de test avec certificat et heartbeat.
Le client réutilise le socle UI partagé. La tour reste en NO-GO production selon
son diagnostic existant.

- [Spécification produit](OPS_INFRASTRUCTURE_PLATFORM_SPEC.md)
- [Direction artistique et maquettes](design/ART_DIRECTION.md)
- [Bibliothèque de composants et intégration](packages/prisme_ui/README.md)
- [Contrat frontend et prochaines étapes](docs/FRONTEND_FOUNDATIONS.md)
- [Résultats des vérifications](docs/UI_VALIDATION.md)
- [Phase 0 : démarrage, preuves et limites](docs/PHASE_0.md)
- [Conventions du monorepo](docs/CONVENTIONS.md)
- [Contrat OpenAPI](contracts/openapi.json) et [contrat agent](contracts/agent-v1.md)
- [Modèle de menaces](docs/THREAT_MODEL.md)

## Démarrer la fondation locale

Go 1.26.8 (téléchargement automatique depuis Go si nécessaire), Flutter 3.38.5
et Docker Desktop avec moteur Linux sont requis.

```powershell
./tool/dev.ps1
dart run apps/prisme_client/tool/smoke.dart
./tool/run-client.ps1
```

Le script crée les secrets dans `.dev/` avec ACL privées, démarre uniquement le
projet Compose `prisme-dev` et publie l'API TLS sur `127.0.0.1:8443`. Le client
demande le secret local de `.dev/bootstrap-secret` dans un champ masqué. Ne pas
le copier dans une commande, une capture ou un commit. `./tool/dev.ps1 -Stop`
arrête la stack en conservant les données.

Les [instructions détaillées](docs/PHASE_0.md) couvrent l'agent conteneurisé,
Android via ADB, les tests natifs et les limites de cette authentification locale.

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
