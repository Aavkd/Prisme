# Prisme UI · 0.1

Bibliothèque Flutter partagée pour les futurs écrans Windows et Android. Les composants reprennent la direction validée : graphite/cobalt, surfaces opaques, coins discrets, Inter et IBM Plex Mono. Le thème Matrix conserve les composants et leur disposition, avec sa palette phosphore, sa typographie monospace et un léger halo sur les mesures.

## Démarrer

Dans le `pubspec.yaml` du futur client, ajouter une dépendance locale vers ce dossier :

```yaml
dependencies:
  prisme_ui:
    path: ../../packages/prisme_ui # adapter au dossier du client
```

```dart
import 'package:flutter/material.dart';
import 'package:prisme_ui/prisme_ui.dart';

final preferences = PrismePreferences(
  appearance: PrismeAppearance.dark,
  locale: const Locale('fr'),
);

void main() => runApp(PrismeApp(
  preferences: preferences,
  home: Scaffold(
    body: PrismePageFrame(
      header: const PrismeSectionHeader(title: 'Mon écran'),
      children: const [
        PrismeSurface(child: PrismeMetric(label: 'GPU')),
      ],
    ),
  ),
));
```

L'application hôte crée et détruit `PrismePreferences`. Le contrôleur expose `appearance`, `density` et `locale`, notifie l'UI et sait convertir ses valeurs via `toJson` / `fromJson`. Le stockage par appareil appartient au client ; le catalogue réinitialise ses préférences au lancement. Aucune dépendance de persistance n'est imposée.

`PrismeApp` accepte **soit** `home`, **soit** `routerConfig`. Avec un routeur, enregistrer ses observateurs au niveau du routeur ; `navigatorObservers` sert uniquement au mode `home`. La navigation et les gardes d'autorisation restent dans le client.

## Ce qui est fourni

| Famille | Composants / API | Variantes et comportement |
|---|---|---|
| Fondations | `PrismeTheme`, `PrismeColors`, `PrismeStyle`, `PrismeSpace`, `PrismeRadius`, `PrismeFonts` | Système, clair, sombre, Matrix ; densités confortable/compacte |
| Cadre | `PrismeAppShell`, `PrismePageFrame`, `PrismeAdaptiveColumns` | Navigation latérale repliable, navigation basse, menu Plus ; colonnes empilables |
| Recherche | `showPrismeCommandPalette`, `PrismeCommand` | Filtrage, flèches haut/bas, Entrée, Échap ; renvoie un identifiant |
| Actions | `PrismeButton`, `PrismeIconButton` | Principal, secondaire, discret, danger ; désactivé, occupé, raison d'indisponibilité |
| Conteneurs | `PrismeSurface`, `PrismeSectionHeader`, `PrismeBrand`, `PrismeKeyValue` | Surfaces normales/rehaussées, titres, actions, données sélectionnables |
| Formulaires | `PrismeTextField`, `PrismeSelect`, `PrismeSearchField`, `PrismeSecretField` | Label, aide, erreur, lecture seule ; secret masqué à la perte de focus |
| États | `PrismeBadge`, `PrismeStatusBadge`, `PrismeFreshnessBadge`, `PrismeEnvironmentBadge` | Texte + icône + couleur ; fraîcheur séparée de la santé |
| Retours | `PrismeBanner`, `PrismeStateView`, `PrismeSkeleton`, `showPrismeMessage` | Chargement, vide, erreur, accès refusé, hors connexion ; alerte critique non masquable |
| Mesures | `PrismeMetric`, `PrismeCapacityBar`, `PrismeProgress`, `PrismeSparkline` | Valeurs nullable, unités explicites, trous dans les séries |
| Ressources | `PrismeResourceCard`, `PrismeWorkerCard` | Santé observée, fraîcheur, ressources, tâche et demande de décision |
| Données | `PrismeDataTable<T>`, `PrismeTimeline`, `PrismeCodeBlock`, `PrismeDiffView` | Table courte/cartes mobiles, journal, texte sélectionnable, diff avec signes +/− |
| Panneaux | `showPrismeDialog`, `showPrismeInspector`, `showPrismeConfirmation` | Dialogue, inspecteur latéral/feuille mobile, cible/impact/vérification/retour arrière |

Les primitives Material usuelles (case à cocher, switch, onglets, menus, infobulles, séparateurs) héritent aussi du thème partagé. Il n'est pas nécessaire de créer un wrapper sans comportement supplémentaire.

## Utiliser les fondations

```dart
final colors = context.prismeColors;
final style = context.prismeStyle;
final text = Theme.of(context).textTheme;
```

Employer les rôles `surface`, `text`, `muted`, `accent`, `healthy`, `attention`, `critical` plutôt que des couleurs en dur. Le fichier `design/tokens.json` génère les **couleurs** Flutter ; ne pas modifier `colors.g.dart` directement :

```powershell
dart run tool/sync_tokens.dart
dart run tool/sync_tokens.dart --check
```

L'espacement et les rayons sont dans `tokens.dart`, la typographie dans `theme.dart`. Les adaptations natives arrondissent les marges du prototype à 16/24 dp, utilisent une navigation desktop à partir de 1 024 dp disponibles, et empilent les colonnes/tableaux sous 720 dp disponibles. La taille de texte influence ces seuils : une police agrandie reçoit davantage d'espace. La constante 600 dp distingue les éléments très compacts de la barre d'outils. Les scanlines du prototype HTML ne sont pas peintes sur les composants Flutter ; la palette, la fonte, les signaux et le halo constituent le thème natif de cette version.

Inter, Space Grotesk et IBM Plex Mono sont embarquées et fonctionnent sans Google Fonts à l'exécution. [Sources, commits et empreintes](assets/fonts/SOURCES.json) et licences OFL se trouvent avec les fichiers. Les icônes utilisent Lucide, embarqué dans le bundle. Le nom et le signe Prisme restent une identité de travail.

## États et contrats à respecter

Une observation associe `PrismeFreshness` et `observedAt`. Une observation récente ou un flux connecté nécessite un horodatage. La politique de collecte classe la fraîcheur ; les widgets n'inventent pas de seuil temporel.

```dart
PrismeResourceCard(
  title: 'VPS',
  icon: LucideIcons.server,
  status: PrismeStatus.healthy,
  observation: PrismeObservation(
    freshness: PrismeFreshness.offline,
    observedAt: lastObservation,
  ),
  child: const PrismeMetric(label: 'GPU'),
)
```

Cette carte affiche des données anciennes, pas une santé actuelle verte. `null` pour une mesure ou une progression signifie indisponible, jamais zéro. Les trous des séries restent des trous. L'app doit conserver les incidents connus dans leur bandeau dédié, même si la collecte devient ancienne.

Un `PrismeActionPlan` précise la cible, l'environnement, l'impact, la vérification et le retour arrière ; une phrase exacte peut être demandée. `showPrismeConfirmation` renvoie une décision locale. L'appelant doit revalider cible, droits, politique, fraîcheur et éventuelle biométrie avant l'exécution. Les cartes de workers peuvent ouvrir la consultation d'une demande en cache ; cela n'autorise pas sa réponse ou une action sur la ressource.

## Interactions et accessibilité

- Actions natives focusables : Tab/Maj+Tab, Entrée/Espace ; palette via Ctrl/Cmd+K dans le shell.
- Dialogues avec parcours de focus, Échap et bouton Fermer ; Annuler/Fermer renvoient une absence de validation.
- Boutons tactiles d'au moins 48 dp sur Android, y compris en densité compacte. Le texte suit l'échelle système.
- Icônes seules avec un libellé obligatoire ; titres marqués comme en-têtes ; états portés par les mots et les symboles autant que par la couleur.
- Les boutons occupés bloquent leur callback. En réduction des animations, le témoin devient statique. Les squelettes ne scintillent pas.
- Les identifiants des destinations et des commandes doivent être uniques. Les callbacks et contrôleurs appartiennent à l'appelant.
- `PrismeTextField` participe à un `Form` et à son validateur ; `PrismeSelect` est contrôlé via `value` / `onChanged`, avec erreur externe.
- Les tableaux mobiles conservent toutes les colonnes sous forme de cartes. Les tris et la pagination appartiennent à la fonctionnalité. Limiter cette table aux petites collections.

Les contrôles automatisés ne remplacent pas une session TalkBack/Narrateur ni un essai sur le téléphone cible.

## Catalogue et vérifications

```powershell
flutter pub get
flutter analyze --no-pub
flutter test --no-pub
cd example
flutter pub get
flutter test --no-pub
flutter run -d windows
```

Le catalogue couvre les sept familles et leurs interactions avec des fixtures identifiables. Il est aussi exécutable dans Chrome pour la revue locale. Le script racine `tool/verify.ps1` réunit les contrôles et les compilations facultatives.

Cette version n'inclut pas les pages métier, le terminal interactif, SFTP, un éditeur de code complet, les grandes séries d'observabilité, les adaptateurs API, l'authentification, la biométrie ou le stockage de secrets. Les blocs de texte et de diff ne les simulent pas.
