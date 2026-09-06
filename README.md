# Prisme

Plateforme de supervision et de pilotage d'infrastructure, prévue pour Windows et Android.

La direction graphique de l'accueil est validée. Le développement commence par le **socle UI Flutter partagé**, avant l'assemblage des pages métier.

- [Spécification produit](OPS_INFRASTRUCTURE_PLATFORM_SPEC.md)
- [Direction artistique et maquettes](design/ART_DIRECTION.md)
- [Bibliothèque de composants et intégration](packages/prisme_ui/README.md)
- [Contrat frontend et prochaines étapes](docs/FRONTEND_FOUNDATIONS.md)
- [Résultats des vérifications](docs/UI_VALIDATION.md)

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
