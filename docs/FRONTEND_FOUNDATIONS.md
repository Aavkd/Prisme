# Contrat frontend · première base Flutter

La bibliothèque `packages/prisme_ui` est la couche de présentation partagée. Le catalogue dans son dossier `example` permet de composer, examiner et tester ses composants avant la création des pages.

## Frontières

| Couche | Responsabilité |
|---|---|
| `prisme_ui` | Tokens, thèmes, textes communs FR/EN, widgets, présentation de décisions et d'observations |
| Futur client Flutter | Routes, écrans, état métier, préférences par appareil, liaison des composants aux modèles |
| Services/adaptateurs | Transport API, SSE/WebSocket, reprise, cache horodaté, mapping des réponses |
| Backend/agent | Autorisation, politiques, exécution, idempotence, audit, collecte et intégrations |

La bibliothèque ne dépend d'aucun SDK cloud, mécanisme SSH ou contrôleur d'infrastructure. Le futur client peut choisir sa gestion d'état et son routeur sans réécrire les composants.

## Assembler un prochain écran

1. Définir son modèle de présentation : données observées et horodatées, droits disponibles, états vide/chargement/erreur/hors connexion.
2. Composer les widgets existants dans `PrismePageFrame` et placer le shell au niveau du routeur.
3. Garder les traductions métier dans le client ; les libellés génériques proviennent de `PrismeStrings`.
4. Relier une intention utilisateur à un contrôleur métier. Une confirmation UI n'est jamais une permission serveur.
5. Ajouter une fixture représentative et tester le comportement de la fonctionnalité, y compris source ancienne et autorisation refusée.

Pour une observation ancienne, conserver sa valeur et sa date mais retirer toute assurance sur l'état actuel. En hors connexion, les capacités de mutation doivent être désactivées par le client. Aucun composant de cette bibliothèque ne met une commande en file d'attente ni ne contourne cette règle. Les urgences connues restent visibles à travers un bandeau critique indépendant.

## Choix de cette version

- Thèmes clair, sombre et Matrix issus des couleurs validées ; le mode Système suit le système d'exploitation.
- Densité distincte du thème. Préférences sérialisables, stockage laissé au client.
- Polices et icônes locales ; FR/EN et formats de date/nombre adaptés.
- Widgets natifs Material thémés pour les interactions standard.
- Tables adaptées aux synthèses courtes ; séries simples et diff en lecture seule. Une future vue de logs à grand volume nécessitera une source paginée et une virtualisation spécialisée.
- Panneaux modaux adaptatifs ; l'inspecteur d'une page desktop pourra plus tard devenir non modal selon le besoin métier, sans changer les composants de contenu.

## Suite proposée

Les écrans pourront maintenant être assemblés progressivement avec ce socle. Commencer par l'accueil validé et la fiche worker permettra de tester les données observées, les demandes d'intervention et les états hors connexion. Le raccordement aux contrats backend et aux mécanismes de sécurité se fera dans la phase correspondante de la spécification.

Avant distribution Android : compléter la toolchain Android locale, valider les licences SDK, tester sur le téléphone et contrôler TalkBack. Les identifiants, signatures et icônes des runners du catalogue sont ceux d'une application de développement, pas ceux d'une release produit.
