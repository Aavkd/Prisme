import 'dart:async';
import 'package:flutter/material.dart';
import 'package:prisme_ui/prisme_ui.dart';
import '../api/connection.dart';
import '../api/generated.dart';
import '../foundation/controller.dart';
import '../operations/page.dart';

class ObservabilityPage extends StatefulWidget {
  const ObservabilityPage({super.key, required this.controller});
  final FoundationController controller;
  @override
  State<ObservabilityPage> createState() => _ObservabilityPageState();
}

class _ObservabilityPageState extends State<ObservabilityPage> {
  ObservationOverview? overview;
  IncidentPage? incidents;
  IncidentDetail? detail;
  MetricResult? metrics;
  LogResult? logs;
  String? scopeId, incidentId;
  String metric = 'probe_success', level = '';
  int minutes = 15, generation = 0;
  bool busy = false, live = false;
  final search = TextEditingController(), note = TextEditingController();
  final warnings = <String>{};
  Timer? timer;
  FoundationController get c => widget.controller;
  String t(String fr, String en) => context.prismeStrings.choose(fr, en);

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) => refresh());
    timer = Timer.periodic(const Duration(seconds: 15), (_) {
      if (live && !busy && c.identity != null) unawaited(refresh());
      if (mounted) setState(() {});
    });
  }

  @override
  void dispose() {
    generation++;
    timer?.cancel();
    search.dispose();
    note.dispose();
    super.dispose();
  }

  Future<T?> source<T>(String name, Future<T> Function() fetch) async {
    try {
      return await fetch();
    } on ApiFailure catch (e) {
      if (e.status == 401) rethrow;
      warnings.add(name);
      return null;
    }
  }

  Future<void> refresh({
    bool nextIncidents = false,
    bool moreEvents = false,
  }) async {
    if (busy || c.busy || !mounted || c.identity == null) return;
    final current = ++generation;
    setState(() => busy = true);
    final result = await c.securityAction(() async {
      final freshOverview = await c.api.observationOverview();
      if (!mounted || current != generation) return false;
      overview = freshOverview;
      scopeId ??= overview!.scopes.firstOrNull?.id;
      warnings.clear();
      final page = await c.api.incidents(
        cursor: nextIncidents ? incidents?.next_cursor ?? '' : '',
      );
      IncidentDetail? incident;
      if (incidentId != null) {
        incident = await c.api.incidentDetail(
          id: incidentId!,
          cursor: moreEvents ? detail?.next_cursor ?? '' : '',
        );
      }
      final end = incident == null
          ? DateTime.now().toUtc()
          : DateTime.parse(
              incident.incident.resolved_at ?? incident.incident.updated_at,
            ).add(const Duration(minutes: 2));
      final boundedEnd = end.isAfter(DateTime.now().toUtc())
          ? DateTime.now().toUtc()
          : end;
      final start = incident == null
          ? boundedEnd.subtract(Duration(minutes: minutes))
          : DateTime.parse(
              incident.incident.opened_at,
            ).subtract(const Duration(minutes: 2));
      final boundedStart =
          boundedEnd.difference(start) > const Duration(hours: 24)
          ? boundedEnd.subtract(const Duration(hours: 24))
          : start;
      final scope = incident?.incident.scope_id ?? scopeId;
      MetricResult? nextMetrics;
      LogResult? nextLogs;
      if (freshOverview.enabled && scope != null) {
        nextMetrics = await source(
          'metrics',
          () => c.api.observationMetrics(
            scope: scope,
            metric: metric,
            start: boundedStart.toIso8601String(),
            end: boundedEnd.toIso8601String(),
          ),
        );
        nextLogs = await source(
          'logs',
          () => c.api.observationLogs(
            scope: scope,
            start: boundedStart.toIso8601String(),
            end: boundedEnd.toIso8601String(),
            search: search.text.trim(),
            level: level,
          ),
        );
      }
      if (!mounted || current != generation || c.identity == null) return false;
      incidents = page;
      detail = incident;
      metrics = nextMetrics;
      logs = nextLogs;
      return true;
    });
    if (!mounted || current != generation) return;
    if (result == null) warnings.add('connection');
    setState(() => busy = false);
  }

  Future<void> action(String action) async {
    final incident = detail?.incident;
    if (busy || incident == null || c.error != null || warnings.isNotEmpty) {
      return;
    }
    final result = await c.securityAction(
      () => c.api.incidentAction(
        IncidentAction(
          action: action,
          note: note.text.trim(),
          revision: incident.revision,
        ),
        id: incident.id,
      ),
    );
    if (!mounted || c.identity == null) return;
    if (result != null) note.clear();
    if (result != null) await refresh();
    if (mounted) setState(() {});
  }

  String stateLabel(String value) => switch (value) {
    'acknowledge' => t('Acquittement', 'Acknowledgement'),
    'diagnose' => t('Diagnostic', 'Diagnosis'),
    'monitor' => t('Surveillance', 'Monitoring'),
    'close' => t('Clôture', 'Closure'),
    'note' => t('Note', 'Note'),
    'signal_changed' => t(
      'État de collecte modifié',
      'Collection state changed',
    ),
    'detected' => t('Détecté', 'Detected'),
    'acknowledged' => t('Acquitté', 'Acknowledged'),
    'diagnosing' => t('En diagnostic', 'Diagnosing'),
    'monitoring' => t('Surveillé', 'Monitoring'),
    'resolved' => t('Résolu', 'Resolved'),
    'closed' => t('Clôturé', 'Closed'),
    'reopened' => t('Rouvert', 'Reopened'),
    'healthy' => t('Sain', 'Healthy'),
    'failing' => t('En échec', 'Failing'),
    'inhibited' => t('Inhibé', 'Inhibited'),
    'unstable' => t('Instable', 'Unstable'),
    _ => t('Inconnu', 'Unknown'),
  };
  String displayedSignal(ObservationRule rule) {
    final signal = rule.state['signal'] as String? ?? 'unknown';
    final observed = DateTime.tryParse(overview?.observed_at ?? '');
    if (observed == null ||
        DateTime.now().difference(observed).inSeconds > 30) {
      return 'unknown';
    }
    final sample = DateTime.tryParse(
      rule.state['last_sample'] as String? ?? '',
    );
    if (['healthy', 'failing', 'unstable'].contains(signal) &&
        (sample == null ||
            DateTime.now().difference(sample).inSeconds >
                (rule.definition['freshness_seconds'] as num? ?? 90))) {
      return 'unknown';
    }
    return signal;
  }

  String severityLabel(String value) => switch (value) {
    'critical' => t('Critique', 'Critical'),
    'degraded' => t('Dégradé', 'Degraded'),
    'warning' => t('Avertissement', 'Warning'),
    _ => t('Information', 'Information'),
  };
  String reasonLabel(String value) => switch (value) {
    'missing_or_stale_samples' => t(
      'Échantillons absents ou anciens',
      'Samples missing or stale',
    ),
    'evaluator_stale' => t('Évaluation non actualisée', 'Evaluation is stale'),
    'not_evaluated' => t(
      'En attente de la première évaluation',
      'Waiting for the first evaluation',
    ),
    'disabled' => t('Règle désactivée', 'Rule disabled'),
    'maintenance' => t('Maintenance en cours', 'Maintenance in progress'),
    _ =>
      value.startsWith('dependency:')
          ? t(
              'Dépendance non saine : ${value.substring(11)}',
              'Unhealthy dependency: ${value.substring(11)}',
            )
          : '',
  };

  @override
  Widget build(BuildContext context) {
    final incident = detail?.incident;
    final disabled = busy || c.busy;
    return PrismePageFrame(
      key: ValueKey(incident?.id ?? 'overview'),
      header: PrismeSectionHeader(
        title:
            incident?.title ??
            t('Observabilité et incidents', 'Observability and incidents'),
        subtitle: incident == null
            ? t(
                'Santé, métriques et logs dans la même fenêtre.',
                'Health, metrics and logs in the same time window.',
              )
            : '${incident.scope_id} · ${stateLabel(incident.status)}',
        actions: [
          if (incidentId != null)
            PrismeButton(
              label: t('Retour', 'Back'),
              onPressed: disabled
                  ? null
                  : () {
                      setState(() {
                        incidentId = null;
                        detail = null;
                        metrics = null;
                        logs = null;
                      });
                      refresh();
                    },
            ),
          PrismeButton(
            label: t('Actualiser', 'Refresh'),
            loading: busy,
            onPressed: disabled ? null : refresh,
          ),
        ],
      ),
      children: [
        if (overview == null && busy) const LinearProgressIndicator(),
        if (c.error != null || warnings.isNotEmpty)
          PrismeBanner(
            tone: PrismeTone.attention,
            title: t('Données non vérifiées', 'Data not verified'),
            message: t(
              'Collecte interrompue ou source indisponible. Les incidents connus restent visibles. Actualisez avant toute action.',
              'Collection interrupted or source unavailable. Known incidents remain visible. Refresh before taking action.',
            ),
          ),
        if (overview?.enabled == false)
          PrismeBanner(
            title: t('Collecte non configurée', 'Collection not configured'),
            message: t(
              'Aucune source de télémétrie n’est activée sur ce backend.',
              'No telemetry source is enabled on this backend.',
            ),
          ),
        if (overview?.enabled == true) ...[
          if (incident == null)
            PrismeSurface(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  DropdownButtonFormField<String>(
                    initialValue: scopeId,
                    isExpanded: true,
                    decoration: InputDecoration(
                      labelText: t('Ressource', 'Resource'),
                    ),
                    items: [
                      for (final scope in overview!.scopes)
                        DropdownMenuItem(
                          value: scope.id,
                          child: Text(
                            '${scope.project} / ${scope.service}',
                            overflow: TextOverflow.ellipsis,
                          ),
                        ),
                    ],
                    onChanged: disabled
                        ? null
                        : (value) {
                            setState(() {
                              scopeId = value;
                              metrics = null;
                              logs = null;
                            });
                            refresh();
                          },
                  ),
                  const SizedBox(height: 12),
                  Wrap(
                    spacing: 12,
                    runSpacing: 8,
                    crossAxisAlignment: WrapCrossAlignment.center,
                    children: [
                      for (final duration in [15, 60, 360, 1440])
                        ChoiceChip(
                          label: Text(
                            duration < 60
                                ? '$duration min'
                                : '${duration ~/ 60} h',
                          ),
                          selected: minutes == duration,
                          onSelected: disabled
                              ? null
                              : (_) {
                                  setState(() => minutes = duration);
                                  refresh();
                                },
                        ),
                      FilterChip(
                        label: Text(t('Suivi · 15 s', 'Live · 15 s')),
                        selected: live,
                        onSelected: (value) => setState(() => live = value),
                      ),
                    ],
                  ),
                ],
              ),
            ),
          for (final rule in overview!.rules.where(
            (rule) =>
                rule.definition['scope_id'] == (incident?.scope_id ?? scopeId),
          ))
            PrismeSurface(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    rule.definition['title'] as String,
                    style: Theme.of(context).textTheme.titleSmall,
                  ),
                  Text(
                    '${stateLabel(displayedSignal(rule))} · ${rule.definition['metric']}',
                  ),
                  if (rule.state['last_sample'] != null)
                    Text(
                      '${t('Dernier échantillon', 'Last sample')}: ${DateTime.parse(rule.state['last_sample'] as String).toLocal()}',
                    ),
                  if (rule.state['reason'] != '')
                    Text(reasonLabel(rule.state['reason'] as String? ?? '')),
                  Text(
                    '${t('Échec / récupération', 'Failure / recovery')}: ${rule.definition['for_seconds']} s / ${rule.definition['recovery_seconds']} s',
                  ),
                ],
              ),
            ),
        ],
        if (incident == null) ...[
          PrismeSectionHeader(
            title: t('Incidents', 'Incidents'),
            subtitle: t(
              'Les incidents résolus conservent leur chronologie.',
              'Resolved incidents retain their timeline.',
            ),
          ),
          PrismeDataTable<IncidentRecord>(
            rows: incidents?.items ?? [],
            columns: [
              PrismeTableColumn(
                label: t('Incident', 'Incident'),
                value: (i) => i.title,
              ),
              PrismeTableColumn(
                label: t('Ressource', 'Resource'),
                value: (i) => i.scope_id,
              ),
              PrismeTableColumn(
                label: t('État', 'Status'),
                value: (i) => stateLabel(i.status),
              ),
              PrismeTableColumn(
                label: t('Sévérité', 'Severity'),
                value: (i) => severityLabel(i.severity),
              ),
            ],
            onOpen: disabled
                ? null
                : (i) {
                    setState(() {
                      incidentId = i.id;
                      metric = 'probe_success';
                      metrics = null;
                      logs = null;
                    });
                    refresh();
                  },
          ),
          if (incidents?.next_cursor.isNotEmpty ?? false)
            PrismeButton(
              label: t('Page suivante', 'Next page'),
              onPressed: disabled ? null : () => refresh(nextIncidents: true),
            ),
        ] else ...[
          PrismeSurface(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                PrismeKeyValue(
                  label: t('Règle', 'Rule'),
                  value: incident.rule_id,
                ),
                PrismeKeyValue(
                  label: t('Ouverture', 'Opened'),
                  value: '${DateTime.parse(incident.opened_at).toLocal()}',
                ),
                PrismeTextField(
                  label: t('Note ou conclusion', 'Note or conclusion'),
                  controller: note,
                  enabled: !disabled,
                ),
                const SizedBox(height: 12),
                Wrap(
                  spacing: 12,
                  runSpacing: 8,
                  children: [
                    PrismeButton(
                      label: t(
                        'Terminal lié à cet incident',
                        'Terminal for this incident',
                      ),
                      onPressed: disabled
                          ? null
                          : () => Navigator.of(context).push(
                              MaterialPageRoute<void>(
                                builder: (_) => Scaffold(
                                  appBar: AppBar(
                                    title: Text('Incident ${incident.id}'),
                                  ),
                                  body: OperationsPage(
                                    send: c.api.send,
                                    prove: c.operationProof,
                                    openSocket: c.openTerminalSocket,
                                    incident: incident.id,
                                    platformActivity: (v) =>
                                        c.platformAuthentication = v,
                                  ),
                                ),
                              ),
                            ),
                    ),
                    for (final entry in <String, String>{
                      if (![
                        'resolved',
                        'closed',
                        'acknowledged',
                      ].contains(incident.status))
                        'acknowledge': t('Acquitter', 'Acknowledge'),
                      if (![
                        'resolved',
                        'closed',
                        'diagnosing',
                      ].contains(incident.status))
                        'diagnose': t('Diagnostiquer', 'Diagnose'),
                      if (![
                        'resolved',
                        'closed',
                        'monitoring',
                      ].contains(incident.status))
                        'monitor': t('Surveiller', 'Monitor'),
                      if (incident.status == 'resolved')
                        'close': t('Clôturer', 'Close'),
                      'note': t('Ajouter une note', 'Add note'),
                    }.entries)
                      PrismeButton(
                        label: entry.value,
                        onPressed:
                            disabled || c.error != null || warnings.isNotEmpty
                            ? null
                            : () => action(entry.key),
                      ),
                  ],
                ),
              ],
            ),
          ),
          PrismeSectionHeader(title: t('Chronologie', 'Timeline')),
          PrismeTimeline(
            events: [
              for (final event in detail!.events)
                PrismeTimelineEvent(
                  icon: LucideIcons.activity,
                  time: DateTime.parse(event.at),
                  title: stateLabel(event.kind),
                  description: '${event.actor} · ${event.message}',
                ),
            ],
          ),
          if (detail!.next_cursor.isNotEmpty)
            PrismeButton(
              label: t('Événements suivants', 'Next events'),
              onPressed: disabled ? null : () => refresh(moreEvents: true),
            ),
        ],
        if (overview?.enabled == true) ...[
          PrismeSurface(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                DropdownButtonFormField<String>(
                  key: ValueKey(metric),
                  initialValue: metric,
                  isExpanded: true,
                  decoration: InputDecoration(
                    labelText: t('Métrique', 'Metric'),
                  ),
                  items: [
                    for (final entry in <String, String>{
                      'probe_success': t(
                        'Disponibilité HTTP',
                        'HTTP availability',
                      ),
                      'probe_duration_seconds': t(
                        'Latence HTTP (s)',
                        'HTTP latency (s)',
                      ),
                      'host_cpu_percent': t('CPU hôte (%)', 'Host CPU (%)'),
                      'host_memory_percent': t(
                        'Mémoire hôte (%)',
                        'Host memory (%)',
                      ),
                      'host_disk_percent': t(
                        'Disque hôte (%)',
                        'Host disk (%)',
                      ),
                      'container_cpu_percent': t(
                        'CPU conteneur (%)',
                        'Container CPU (%)',
                      ),
                      'container_memory_usage_bytes': t(
                        'Mémoire conteneur (octets)',
                        'Container memory (bytes)',
                      ),
                      'up': t('Collecteur joignable', 'Collector reachable'),
                    }.entries)
                      DropdownMenuItem(
                        value: entry.key,
                        child: Text(entry.value),
                      ),
                  ],
                  onChanged: disabled
                      ? null
                      : (value) {
                          if (value != null) {
                            setState(() {
                              metric = value;
                              metrics = null;
                            });
                            refresh();
                          }
                        },
                ),
                const SizedBox(height: 16),
                if (metrics?.series.isEmpty ?? true)
                  Text(
                    t(
                      'Échantillons indisponibles. Aucun zéro n’est déduit.',
                      'Samples unavailable. No zero is inferred.',
                    ),
                  ),
                for (final series in metrics?.series ?? <MetricSeries>[])
                  PrismeSparkline(
                    samples: gapAware(series, metrics!),
                    label: '$metric ${series.labels['name'] ?? ''}',
                    unit: '',
                    startLabel: '${DateTime.parse(metrics!.start).toLocal()}',
                    endLabel: '${DateTime.parse(metrics!.end).toLocal()}',
                  ),
              ],
            ),
          ),
          PrismeSurface(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                PrismeSectionHeader(
                  title: t('Logs corrélés', 'Correlated logs'),
                  subtitle: t(
                    'Même fenêtre temporelle · texte masqué · 200 lignes maximum',
                    'Same time window · redacted text · up to 200 lines',
                  ),
                ),
                PrismeTextField(
                  label: t('Rechercher du texte', 'Search text'),
                  controller: search,
                  enabled: !disabled,
                ),
                const SizedBox(height: 12),
                Wrap(
                  spacing: 8,
                  runSpacing: 8,
                  children: [
                    for (final value in [
                      '',
                      'info',
                      'warning',
                      'error',
                      'debug',
                    ])
                      ChoiceChip(
                        label: Text(
                          value.isEmpty
                              ? t('Tous niveaux', 'All levels')
                              : value,
                        ),
                        selected: level == value,
                        onSelected: disabled
                            ? null
                            : (_) {
                                setState(() => level = value);
                                refresh();
                              },
                      ),
                    PrismeButton(
                      label: t('Rechercher', 'Search'),
                      onPressed: disabled ? null : refresh,
                    ),
                  ],
                ),
                const SizedBox(height: 12),
                if (logs?.truncated ?? false)
                  Text(
                    t(
                      'Résultat limité. Réduisez la fenêtre ou affinez le filtre.',
                      'Result limited. Narrow the window or refine the filter.',
                    ),
                  ),
                if (logs?.lines.isEmpty ?? true)
                  Text(
                    t(
                      'Aucune ligne disponible dans cette fenêtre. Cela ne prouve pas la continuité de collecte.',
                      'No lines available in this window. This does not prove collection continuity.',
                    ),
                  ),
                if (logs?.lines.isNotEmpty ?? false)
                  PrismeCodeBlock(
                    text: logs!.lines
                        .map(
                          (line) =>
                              '${DateTime.parse(line.at).toLocal()}  ${line.line}',
                        )
                        .join('\n'),
                    maxHeight: 400,
                  ),
              ],
            ),
          ),
        ],
      ],
    );
  }
}

/// Preserve missing time buckets instead of joining points across collection gaps.
List<double?> gapAware(MetricSeries series, MetricResult result) {
  final start = DateTime.parse(result.start);
  final count =
      DateTime.parse(result.end).difference(start).inSeconds ~/
          result.step_seconds +
      1;
  final values = List<double?>.filled(count.clamp(1, 1001), null);
  for (final point in series.points) {
    final index =
        DateTime.parse(point.at).difference(start).inSeconds ~/
        result.step_seconds;
    if (index >= 0 && index < values.length) {
      values[index] = point.value?.toDouble();
    }
  }
  return values;
}
