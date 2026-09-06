# Vérification du socle UI · 6 septembre 2026

Environnement : Flutter 3.38.5, Dart 3.10.4, Windows 11, Visual Studio 2022.

| Contrôle | Résultat |
|---|---|
| Synchronisation JSON → palettes Flutter | Aucun écart |
| Formatage Dart | Aucun changement requis |
| `flutter analyze --no-pub` | Aucune erreur ni recommandation |
| Tests de la bibliothèque | 13 réussis |
| Tests du catalogue | 9 réussis |
| Compilation Windows x64 release | Réussie |
| Compilation web release, ressources locales | Réussie |

Les tests couvrent les préférences et leur restauration, la séparation entre état observé et fraîcheur, les contrastes texte/fond, les mesures absentes, les boutons occupés, les cibles tactiles Android, la palette au clavier, les confirmations exactes, le masquage des secrets et le menu mobile.

Les sept familles du catalogue sont parcourues jusqu'en bas à 360 et 1 440 px dans les trois thèmes. Deux configurations supplémentaires vérifient une taille de texte de 200 % : Matrix à 320 px et clair à 1 024 px. Aucune erreur de mise en page n'est remontée dans ces scénarios.

Revue réelle du catalogue dans le navigateur local : fondations sombres sur desktop, bascule Matrix, cartes de ressources Matrix à 360 px, navigation Plus, inspecteur sous forme de feuille mobile, fermeture par Échap et fondations claires sur mobile. Les textes et icônes sont rendus ; aucun avertissement ou erreur n'a été relevé dans la console de l'aperçu final. Le viewport de test a été restauré après la revue.

Le contrôle de contraste concerne les paires sémantiques de texte et de boutons (minimum 4,5:1), et ne constitue pas un audit d'accessibilité complet.

## Vérifications restantes avant distribution

- Exécution et revue de l'application native sur les appareils cibles, dont le téléphone Android ; tests TalkBack et Narrateur.
- Compilation Android après installation des `cmdline-tools` et configuration des licences SDK. La toolchain locale ne permet pas encore de considérer cet emballage validé.
- Signature, identité finale, icônes de distribution et tests des intégrations lors de la création des clients produit.

Le rendu web est un catalogue local de développement. Aucun site ni service de production n'a été déployé.
