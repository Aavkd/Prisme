import 'package:flutter/material.dart';
import 'package:prisme_ui/prisme_ui.dart';

void main() => runApp(const PrismeCatalog());

/// Development-only catalogue. These are component specimens, not product pages.
class PrismeCatalog extends StatefulWidget {
  const PrismeCatalog({super.key});
  @override
  State<PrismeCatalog> createState() => _PrismeCatalogState();
}

class _PrismeCatalogState extends State<PrismeCatalog> {
  final preferences = PrismePreferences(
    appearance: PrismeAppearance.dark,
    locale: const Locale('fr'),
  );
  @override
  void dispose() {
    preferences.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => PrismeApp(
    preferences: preferences,
    title: 'Prisme · Catalogue UI',
    home: CatalogScreen(preferences: preferences),
  );
}

class CatalogScreen extends StatefulWidget {
  const CatalogScreen({super.key, required this.preferences});
  final PrismePreferences preferences;
  @override
  State<CatalogScreen> createState() => _CatalogScreenState();
}

class _CatalogScreenState extends State<CatalogScreen> {
  String section = 'foundations';
  final _name = TextEditingController();
  final _search = TextEditingController();
  final _secret = TextEditingController(text: 'demo-local-only');
  final _form = GlobalKey<FormState>();
  PrismeEnvironment _environment = PrismeEnvironment.production;
  PrismeFreshness _freshness = PrismeFreshness.recent;
  bool _selected = true;
  @override
  void dispose() {
    _name.dispose();
    _search.dispose();
    _secret.dispose();
    super.dispose();
  }

  String t(String fr, String en) => context.prismeStrings.choose(fr, en);
  List<PrismeDestination> get destinations => [
    PrismeDestination(
      id: 'foundations',
      label: t('Fondations', 'Foundations'),
      icon: LucideIcons.palette,
    ),
    PrismeDestination(
      id: 'actions',
      label: t('Actions', 'Actions'),
      icon: LucideIcons.mousePointer2,
    ),
    PrismeDestination(
      id: 'forms',
      label: t('Formulaires', 'Forms'),
      icon: LucideIcons.slidersHorizontal,
    ),
    PrismeDestination(
      id: 'states',
      label: t('États', 'States'),
      icon: LucideIcons.circleAlert,
    ),
    PrismeDestination(
      id: 'resources',
      label: t('Ressources', 'Resources'),
      icon: LucideIcons.server,
    ),
    PrismeDestination(
      id: 'data',
      label: t('Données', 'Data'),
      icon: LucideIcons.table2,
    ),
    PrismeDestination(
      id: 'overlays',
      label: t('Panneaux', 'Overlays'),
      icon: LucideIcons.panelRight,
    ),
  ];
  String appearanceLabel(PrismeAppearance a) => switch (a) {
    PrismeAppearance.system => t('Système', 'System'),
    PrismeAppearance.light => t('Clair', 'Light'),
    PrismeAppearance.dark => t('Sombre', 'Dark'),
    PrismeAppearance.matrix => 'Matrix',
  };
  PrismeObservation get observation => PrismeObservation(
    freshness: _freshness,
    observedAt: _freshness == PrismeFreshness.unknown
        ? null
        : DateTime.now().subtract(
            _freshness == PrismeFreshness.stale ||
                    _freshness == PrismeFreshness.offline
                ? const Duration(minutes: 14)
                : const Duration(seconds: 4),
          ),
  );
  void message(String value) => showPrismeMessage(context, value);

  Future<void> commands() async {
    final result = await showPrismeCommandPalette(
      context: context,
      commands: destinations
          .map(
            (d) => PrismeCommand(
              id: d.id,
              label: d.label,
              icon: d.icon,
              description: t(
                'Ouvrir les spécimens',
                'Open component specimens',
              ),
            ),
          )
          .toList(),
    );
    if (mounted && result != null) setState(() => section = result);
  }

  @override
  Widget build(BuildContext context) {
    final prefs = widget.preferences;
    return PrismeAppShell(
      destinations: destinations,
      selectedId: section,
      onDestinationSelected: (id) => setState(() => section = id),
      workspaceLabel: t('Bibliothèque UI · 0.1', 'UI library · 0.1'),
      onSearch: commands,
      toolbar: [
        PopupMenuButton<PrismeAppearance>(
          tooltip: t('Changer de thème', 'Change theme'),
          initialValue: prefs.appearance,
          onSelected: (value) => prefs.appearance = value,
          itemBuilder: (_) => PrismeAppearance.values
              .map(
                (a) => PopupMenuItem(value: a, child: Text(appearanceLabel(a))),
              )
              .toList(),
          child: Padding(
            padding: const EdgeInsets.all(10),
            child: Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                const Icon(LucideIcons.palette, size: 16),
                const SizedBox(width: 8),
                Flexible(child: Text(appearanceLabel(prefs.appearance))),
                const Icon(LucideIcons.chevronDown, size: 14),
              ],
            ),
          ),
        ),
        PrismeIconButton(
          icon: prefs.density == PrismeDensity.compact
              ? LucideIcons.rows3
              : LucideIcons.rows2,
          label: t('Changer la densité', 'Change density'),
          onPressed: () =>
              prefs.density = prefs.density == PrismeDensity.compact
              ? PrismeDensity.comfortable
              : PrismeDensity.compact,
        ),
        PopupMenuButton<String>(
          tooltip: t('Langue', 'Language'),
          onSelected: (value) => prefs.locale = Locale(value),
          itemBuilder: (_) => const [
            PopupMenuItem(value: 'fr', child: Text('Français')),
            PopupMenuItem(value: 'en', child: Text('English')),
          ],
          child: Padding(
            padding: const EdgeInsets.all(10),
            child: Text(prefs.locale?.languageCode.toUpperCase() ?? 'FR'),
          ),
        ),
      ],
      footer: Text(
        t(
          'Catalogue de développement · Données fictives · Aucune connexion à la production',
          'Development catalogue · Sample data · No production connection',
        ),
        style: Theme.of(context).textTheme.labelSmall,
      ),
      body: PrismePageFrame(
        key: ValueKey(section),
        header: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(
              'PRISME / DESIGN SYSTEM',
              style: Theme.of(context).textTheme.labelSmall,
            ),
            const SizedBox(height: 8),
            Text(
              destinations.firstWhere((d) => d.id == section).label,
              style: Theme.of(context).textTheme.displaySmall,
            ),
            const SizedBox(height: 8),
            Text(
              t(
                'Les mêmes briques, sur chaque écran.',
                'The same building blocks on every screen.',
              ),
              style: Theme.of(context).textTheme.bodySmall,
            ),
          ],
        ),
        children: switch (section) {
          'actions' => _actions(),
          'forms' => _forms(),
          'states' => _states(),
          'resources' => _resources(),
          'data' => _data(),
          'overlays' => _overlays(),
          _ => _foundations(),
        },
      ),
    );
  }

  List<Widget> _foundations() {
    final c = context.prismeColors;
    return [
      PrismeSurface(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            const PrismeBrand(),
            const SizedBox(height: 24),
            Text(
              t('Précision calme', 'Calm precision'),
              style: Theme.of(context).textTheme.headlineMedium,
            ),
            const SizedBox(height: 8),
            Text(
              t(
                'Graphite, cobalt et signaux lisibles. Matrix apporte le vert phosphore et le monospace.',
                'Graphite, cobalt and clear signals. Matrix adds phosphor green and monospace typography.',
              ),
            ),
          ],
        ),
      ),
      PrismeSectionHeader(title: t('Palette sémantique', 'Semantic palette')),
      Wrap(
        spacing: 12,
        runSpacing: 12,
        children: [
          for (final entry in <String, Color>{
            t('Fond', 'Background'): c.background,
            t('Surface', 'Surface'): c.surface,
            t('Action', 'Action'): c.accent,
            t('Sain', 'Healthy'): c.healthy,
            t('Attention', 'Attention'): c.attention,
            t('Critique', 'Critical'): c.critical,
          }.entries)
            SizedBox(
              width: 110,
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Container(
                    height: 64,
                    decoration: BoxDecoration(
                      color: entry.value,
                      borderRadius: PrismeRadius.control,
                      border: Border.all(color: c.border),
                    ),
                  ),
                  const SizedBox(height: 8),
                  Text(entry.key, style: Theme.of(context).textTheme.bodySmall),
                ],
              ),
            ),
        ],
      ),
      PrismeAdaptiveColumns(
        primary: PrismeSurface(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                t('Typographie', 'Typography'),
                style: Theme.of(context).textTheme.titleMedium,
              ),
              const SizedBox(height: 16),
              Text(
                t('Vue d’ensemble', 'Overview'),
                style: Theme.of(context).textTheme.displaySmall,
              ),
              const SizedBox(height: 12),
              Text(
                t('Mes machines', 'My machines'),
                style: Theme.of(context).textTheme.titleMedium,
              ),
              const SizedBox(height: 8),
              Text(
                t('Texte d’interface et actions', 'Interface text and actions'),
              ),
              Text(
                t(
                  'Métadonnées · observé il y a 4 s',
                  'Metadata · observed 4 s ago',
                ),
                style: Theme.of(context).textTheme.bodySmall,
              ),
            ],
          ),
        ),
        secondary: const PrismeSurface(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              PrismeMetric(label: 'CPU', value: '28', unit: '%'),
              SizedBox(height: 18),
              PrismeMetric(label: 'p95', value: '142', unit: 'ms'),
            ],
          ),
        ),
      ),
      PrismeSectionHeader(title: t('États partagés', 'Shared states')),
      Wrap(
        spacing: 8,
        runSpacing: 8,
        children: [
          for (final status in PrismeStatus.values)
            PrismeStatusBadge(status: status),
        ],
      ),
    ];
  }

  List<Widget> _actions() => [
    PrismeSurface(
      child: Wrap(
        spacing: 12,
        runSpacing: 12,
        children: [
          PrismeButton(
            label: t('Action principale', 'Primary action'),
            icon: LucideIcons.arrowUpRight,
            variant: PrismeButtonVariant.primary,
            onPressed: () =>
                message(t('Action locale reçue', 'Local action received')),
          ),
          PrismeButton(
            label: t('Personnaliser', 'Customize'),
            icon: LucideIcons.slidersHorizontal,
            onPressed: () {},
          ),
          PrismeButton(
            label: t('Voir les détails', 'View details'),
            variant: PrismeButtonVariant.ghost,
            onPressed: () {},
          ),
          PrismeButton(
            label: t('Interrompre', 'Interrupt'),
            variant: PrismeButtonVariant.danger,
            onPressed: () =>
                message(t('Spécimen de bouton', 'Button specimen')),
          ),
          PrismeButton(
            label: t('Indisponible', 'Unavailable'),
            disabledReason: t('Données anciennes', 'Stale data'),
          ),
          PrismeButton(label: t('Vérification', 'Checking'), loading: true),
          PrismeIconButton(
            icon: LucideIcons.settings2,
            label: t('Paramètres', 'Settings'),
            onPressed: () {},
          ),
        ],
      ),
    ),
    PrismeSurface(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Text(
            t('Clavier et retours', 'Keyboard and feedback'),
            style: Theme.of(context).textTheme.titleMedium,
          ),
          const SizedBox(height: 12),
          Text(
            t(
              'Tab pour parcourir les actions. Entrée ou Espace pour activer. Ctrl+K pour la palette.',
              'Tab through actions. Enter or Space to activate. Ctrl+K opens the command palette.',
            ),
          ),
          const SizedBox(height: 16),
          PrismeButton(
            label: t('Ouvrir la palette', 'Open command palette'),
            onPressed: commands,
          ),
        ],
      ),
    ),
  ];
  List<Widget> _forms() => [
    PrismeSurface(
      child: Form(
        key: _form,
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            PrismeTextField(
              label: t('Nom du projet', 'Project name'),
              controller: _name,
              hint: 'Sounding Lines',
              validator: (value) => value == null || value.trim().isEmpty
                  ? t('Saisissez un nom.', 'Enter a name.')
                  : null,
            ),
            const SizedBox(height: 20),
            PrismeSelect<PrismeEnvironment>(
              label: t('Environnement', 'Environment'),
              value: _environment,
              options: [
                PrismeOption(
                  PrismeEnvironment.production,
                  context.prismeStrings.production,
                ),
                PrismeOption(
                  PrismeEnvironment.staging,
                  context.prismeStrings.staging,
                ),
                PrismeOption(
                  PrismeEnvironment.development,
                  context.prismeStrings.development,
                ),
              ],
              onChanged: (value) {
                if (value != null) setState(() => _environment = value);
              },
            ),
            const SizedBox(height: 20),
            PrismeSecretField(
              label: t(
                'Valeur confidentielle de démonstration',
                'Example confidential value',
              ),
              controller: _secret,
            ),
            const SizedBox(height: 20),
            PrismeSearchField(controller: _search, onChanged: (_) {}),
            const SizedBox(height: 16),
            CheckboxListTile(
              contentPadding: EdgeInsets.zero,
              title: Text(t('Épingler sur cet appareil', 'Pin on this device')),
              value: _selected,
              onChanged: (v) => setState(() => _selected = v!),
            ),
            SwitchListTile(
              contentPadding: EdgeInsets.zero,
              title: Text(t('Préférence locale', 'Local preference')),
              value: _selected,
              onChanged: (v) => setState(() => _selected = v),
            ),
            const SizedBox(height: 16),
            PrismeButton(
              label: t('Vérifier le formulaire', 'Validate form'),
              variant: PrismeButtonVariant.primary,
              onPressed: () {
                _form.currentState!.validate();
              },
            ),
          ],
        ),
      ),
    ),
    PrismeTextField(
      label: t('Chemin distant', 'Remote path'),
      error: t(
        'Sélectionnez un dossier accessible.',
        'Select an accessible folder.',
      ),
      readOnly: true,
    ),
  ];
  List<Widget> _states() => [
    PrismeBanner(
      title: t('Centre de contrôle injoignable', 'Control centre unavailable'),
      message: t(
        'Données en cache. L’état actuel du VPS reste inconnu.',
        'Cached data. The current VPS state remains unknown.',
      ),
    ),
    PrismeBanner(
      tone: PrismeTone.critical,
      title: t('Traitement audio bloqué', 'Audio processing blocked'),
      message: t(
        'Aucun progrès depuis 8 min. Quatre pistes en attente.',
        'No progress for 8 min. Four tracks waiting.',
      ),
      action: PrismeButton(
        label: t('Voir l’incident', 'View incident'),
        onPressed: () {},
      ),
    ),
    const PrismeSurface(child: PrismeSkeleton()),
    for (final state in PrismeViewState.values)
      PrismeSurface(
        child: PrismeStateView(
          state: state,
          description: state == PrismeViewState.error
              ? t(
                  'Vérifiez la connexion puis réessayez.',
                  'Check the connection, then retry.',
                )
              : null,
          onAction: state == PrismeViewState.error ? () {} : null,
        ),
      ),
  ];
  List<Widget> _resources() => [
    PrismeSelect<PrismeFreshness>(
      label: t('Fraîcheur de la source', 'Source freshness'),
      value: _freshness,
      options: [
        for (final f in PrismeFreshness.values) PrismeOption(f, f.name),
      ],
      onChanged: (value) {
        if (value != null) setState(() => _freshness = value);
      },
    ),
    PrismeAdaptiveColumns(
      primary: PrismeResourceCard(
        title: 'VPS OVH',
        subtitle: 'Ubuntu · Production',
        icon: LucideIcons.server,
        status: PrismeStatus.healthy,
        observation: observation,
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            const PrismeMetric(label: 'CPU', value: '28', unit: '%'),
            const SizedBox(height: 14),
            PrismeSparkline(
              samples: const [22, 24, 23, 28, 34, 30, null, 27, 29, 28],
              label: 'CPU',
              unit: '%',
              startLabel: t('−15 min', '−15 min'),
              endLabel: t('Dernière mesure', 'Last sample'),
            ),
            const SizedBox(height: 14),
            const PrismeCapacityBar(
              label: 'RAM',
              unit: 'Gio',
              used: 3.4,
              total: 7.6,
            ),
            const SizedBox(height: 14),
            const PrismeCapacityBar(
              label: 'SSD',
              unit: 'Gio',
              used: 40,
              total: 72,
            ),
          ],
        ),
      ),
      secondary: PrismeWorkerCard(
        name: 'Claude Code',
        task: t(
          'Améliorer la navigation du catalogue',
          'Improve catalogue navigation',
        ),
        icon: LucideIcons.asterisk,
        contextLabel: 'Sounding Lines · Atelier',
        observation: observation,
        status: PrismeStatus.needsInput,
        resources: 'CPU 0 % · RAM 412 Mio',
        request: t(
          'Une décision sur la pagination',
          'A decision about pagination',
        ),
        reviewLabel: t('Examiner', 'Review'),
        onReview: () => showPrismeInspector<void>(
          context: context,
          title: t('Demande du worker', 'Worker request'),
          builder: (_) => Text(
            t(
              'Pagination par pages ou chargement progressif ?',
              'Page-based pagination or progressive loading?',
            ),
          ),
        ),
      ),
    ),
    const PrismeAdaptiveColumns(
      primary: PrismeSurface(child: PrismeMetric(label: 'GPU')),
      secondary: PrismeSurface(child: PrismeProgress()),
    ),
  ];
  List<Widget> _data() => [
    PrismeDataTable<String>(
      onOpen: (project) =>
          message(t('Exemple ouvert : $project', 'Opened example: $project')),
      rows: const ['Sounding Lines', 'Arpent'],
      columns: [
        PrismeTableColumn(label: t('Projet', 'Project'), value: (v) => v),
        PrismeTableColumn(
          label: t('Version', 'Version'),
          value: (v) => v == 'Arpent' ? '1.2.1' : '0.8.4',
        ),
        PrismeTableColumn(
          label: t('Services sains', 'Healthy services'),
          value: (_) => '4 / 4',
          numeric: true,
        ),
      ],
    ),
    PrismeTimeline(
      events: [
        PrismeTimelineEvent(
          title: t('Build Arpent en cours', 'Arpent build running'),
          time: DateTime(2026, 9, 5, 14, 29),
          icon: LucideIcons.gitPullRequest,
          tone: PrismeTone.accent,
          description: 'main · GitHub Actions',
        ),
        PrismeTimelineEvent(
          title: t('Sauvegarde vérifiée', 'Backup verified'),
          time: DateTime(2026, 9, 5, 13, 55),
          icon: LucideIcons.archive,
          tone: PrismeTone.healthy,
        ),
      ],
    ),
    PrismeCodeBlock(
      label: t('Journal en lecture seule', 'Read-only log'),
      text:
          '14:29:02 INFO  health check passed\n14:29:03 INFO  worker idle\n14:29:04 WARN  GPU telemetry unavailable',
    ),
    const PrismeDiffView(
      height: 150,
      lines: [
        PrismeDiffLine('services:', PrismeDiffKind.context),
        PrismeDiffLine('  image: worker:0.8.3', PrismeDiffKind.removed),
        PrismeDiffLine('  image: worker:0.8.4', PrismeDiffKind.added),
      ],
    ),
  ];
  List<Widget> _overlays() => [
    PrismeSurface(
      child: Wrap(
        spacing: 12,
        runSpacing: 12,
        children: [
          PrismeButton(
            label: t('Ouvrir le détail', 'Open detail'),
            onPressed: () => showPrismeInspector<void>(
              context: context,
              title: 'VPS OVH',
              builder: (_) => Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  const PrismeEnvironmentBadge(
                    environment: PrismeEnvironment.production,
                  ),
                  const SizedBox(height: 16),
                  const PrismeKeyValue(label: 'CPU', value: '28 %'),
                  const SizedBox(height: 16),
                  Text(
                    t(
                      'Panneau latéral sur desktop, feuille inférieure sur mobile.',
                      'Side panel on desktop, bottom sheet on mobile.',
                    ),
                  ),
                ],
              ),
            ),
          ),
          PrismeButton(
            label: t('Prévisualiser une confirmation', 'Preview confirmation'),
            variant: PrismeButtonVariant.danger,
            onPressed: () async {
              final result = await showPrismeConfirmation(
                context: context,
                plan: PrismeActionPlan(
                  title: t(
                    'Redémarrer le worker audio',
                    'Restart the audio worker',
                  ),
                  actionLabel: t('Redémarrer le worker', 'Restart worker'),
                  target: 'VPS OVH / Sounding Lines / worker',
                  environment: PrismeEnvironment.production,
                  impact: t(
                    'La piste en cours peut être retraitée.',
                    'The current track may be processed again.',
                  ),
                  verification: t(
                    'Vérifier que la file avance.',
                    'Verify the queue is advancing.',
                  ),
                  rollback: t(
                    'Revenir à la dernière image saine.',
                    'Return to the last healthy image.',
                  ),
                  requiredPhrase: 'worker',
                ),
              );
              if (mounted) {
                message(
                  result
                      ? t(
                          'Confirmation locale reçue · aucune action exécutée',
                          'Local confirmation received · no action executed',
                        )
                      : t('Annulé', 'Cancelled'),
                );
              }
            },
          ),
          PrismeButton(
            label: t('Afficher un message', 'Show a message'),
            onPressed: () => message(
              t(
                'Préférence mise à jour dans le catalogue.',
                'Preference updated in the catalogue.',
              ),
            ),
          ),
        ],
      ),
    ),
    Text(
      t(
        'Les composants collectent une décision. L’application devra ensuite vérifier les capacités, la fraîcheur, l’autorisation et la biométrie éventuelle.',
        'Components collect a decision. The app must then verify capability, freshness, authorization and any required biometrics.',
      ),
    ),
  ];
}
