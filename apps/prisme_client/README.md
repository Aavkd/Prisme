# Prisme client — nom provisoire

Client Flutter Android/Windows de la phase 0. Réutilise `packages/prisme_ui`,
les modèles/méthodes générés depuis OpenAPI et un transport TLS vérifié.

Depuis la racine : `./tool/dev.ps1`, puis `./tool/run-client.ps1`.
Voir le [guide de phase 0](../../docs/PHASE_0.md) pour la connexion locale,
les tests, l'agent, Android et les limites de validation.

Les tokens restent en mémoire. Aucun secret ne doit être ajouté aux dart-defines,
assets ou préférences. Le seul certificat embarqué par le script de lancement
est la CA **publique** de développement.

`lib/api/generated.dart` est généré ; le transport est dans `connection.dart` et
l'état de connexion dans `foundation/controller.dart`. L'application n'est pas
un catalogue de fixtures. Aucun runner web ni canal de publication n'est configuré.
