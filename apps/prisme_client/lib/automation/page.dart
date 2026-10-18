import 'dart:async';
import 'dart:convert';
import 'dart:math';
import 'package:flutter/material.dart';
import 'package:prisme_ui/prisme_ui.dart';
import '../api/generated.dart';
import '../foundation/controller.dart';

class AutomationPage extends StatefulWidget {
  const AutomationPage({super.key, required this.controller});
  final FoundationController controller;
  @override
  State<AutomationPage> createState() => _AutomationPageState();
}

class _AutomationPageState extends State<AutomationPage> {
  FoundationController get c => widget.controller;
  Map<String, dynamic>? overview, detail;
  String? bookID, requestKey, requestSignature;
  final parameters = TextEditingController(text: '{}');
  final interval = TextEditingController(text: '60');
  bool busy = false, failed = false, dialogOpen = false;
  int generation = 0, page = 1;
  Timer? timer;
  String t(String fr, String en) => context.prismeStrings.choose(fr, en);
  String pretty(Object? value) =>
      const JsonEncoder.withIndent('  ').convert(value);
  List<Map<String, dynamic>> list(String key) =>
      (overview?[key] as List? ?? []).cast<Map<String, dynamic>>();
  List<Map<String, dynamic>> get books =>
      list('definitions').where((d) => d.containsKey('steps')).toList();
  Map<String, dynamic>? get book =>
      books.where((b) => b['id'] == bookID).firstOrNull;
  bool get ready =>
      !busy && !failed && overview?['enabled'] == true && c.identity != null;
  String state(String s) => switch (s) {
    'queued' => t('En attente', 'Queued'),
    'approval' => t('Approbation requise', 'Approval required'),
    'running' => t('En cours', 'Running'),
    'waiting' => t('Attente planifiée', 'Scheduled wait'),
    'uncertain' => t('Réconciliation requise', 'Reconciliation required'),
    'failed' => t('Échec', 'Failed'),
    'succeeded' => t('Vérifié', 'Verified'),
    'simulated' => t('Simulation terminée', 'Simulation complete'),
    'cancelled' => t('Annulé', 'Cancelled'),
    'observed' => t('Supervision seule', 'Observation only'),
    _ => s,
  };
  @override
  void initState() {
    super.initState();
    c.addListener(securityChanged);
    unawaited(refresh());
    timer = Timer.periodic(const Duration(seconds: 10), (_) {
      if (!dialogOpen) unawaited(refresh());
    });
  }

  void securityChanged() {
    if (c.identity != null || !mounted) return;
    generation++;
    if (dialogOpen) Navigator.of(context).popUntil((r) => r.isFirst);
    setState(() {
      overview = null;
      detail = null;
      busy = false;
      parameters.text = '{}';
      requestKey = null;
    });
  }

  @override
  void dispose() {
    generation++;
    timer?.cancel();
    c.removeListener(securityChanged);
    parameters.dispose();
    interval.dispose();
    super.dispose();
  }

  Future<void> refresh() async {
    if (!mounted || busy || c.busy || c.identity == null) return;
    final ticket = ++generation;
    setState(() => busy = true);
    final result = await c.securityAction(() async {
      final next = (await c.api.automation(page: '$page')).data;
      final d = detail == null
          ? null
          : (await c.api.automationRun(
              run: detail!['run']['id'] as String,
            )).data;
      if (!mounted || ticket != generation || c.identity == null) return false;
      overview = next;
      detail = d;
      if (!books.any((b) => b['id'] == bookID)) {
        bookID = books.firstOrNull?['id'] as String?;
      }
      return true;
    });
    if (!mounted || ticket != generation) return;
    setState(() {
      busy = false;
      failed = result != true;
    });
  }

  Future<void> act(Future<AutomationResult> Function() action) async {
    if (!ready) return;
    final ticket = ++generation;
    setState(() => busy = true);
    final result = await c.securityAction(action);
    if (!mounted || ticket != generation || c.identity == null) return;
    setState(() {
      busy = false;
      failed = result == null;
      if (result?.data['run'] != null) detail = result!.data;
    });
    if (result != null) await refresh();
  }

  Future<bool> confirm(String title, String target, String impact) async {
    final ticket = generation;
    dialogOpen = true;
    final result = await showPrismeConfirmation(
      context: context,
      plan: PrismeActionPlan(
        title: title,
        actionLabel: t('Confirmer', 'Confirm'),
        target: target,
        environment: PrismeEnvironment.production,
        impact: impact,
        verification: t(
          'Vérifier les étapes, les limites et la chronologie après exécution.',
          'Review steps, limits and the execution timeline.',
        ),
        rollback: t(
          'L’arrêt général bloque les prochaines actions. Une commande interrompue exige une réconciliation.',
          'Global stop blocks subsequent actions. An interrupted command requires reconciliation.',
        ),
        requiredPhrase: target,
      ),
    );
    dialogOpen = false;
    return mounted && ticket == generation && c.identity != null && result;
  }

  Future<void> control({required bool stopped, DateTime? maintenance}) async {
    if (!ready) return;
    final current = Map<String, dynamic>.from(overview!['control'] as Map);
    final value = {
      ...current,
      'stopped': stopped,
      'maintenance_until': maintenance?.toUtc().toIso8601String(),
    };
    if (!stopped &&
        !await confirm(
          t('Modifier l’automatisation', 'Change automation'),
          'global',
          t(
            'Les politiques autonomes approuvées pourront agir sur leurs cibles selon leurs limites.',
            'Approved autonomous policies can act on their targets within their limits.',
          ),
        )) {
      return;
    }
    await act(() async {
      final proof = stopped
          ? ''
          : await c.operationProof({
              'action': 'automation_control',
              'target': 'global',
              'payload': value,
            });
      return c.api.controlAutomation(
        AutomationControlRequest({'control': value, 'proof': proof}),
      );
    });
  }

  Future<void> prepare(bool simulation) async {
    if (!ready || book == null) return;
    Map<String, dynamic> values;
    try {
      values = Map<String, dynamic>.from(jsonDecode(parameters.text) as Map);
    } catch (_) {
      showPrismeMessage(
        context,
        t('Paramètres invalides', 'Invalid parameters'),
      );
      return;
    }
    final req = {
      'runbook': bookID,
      'version': book!['version'],
      'parameters': values,
      'simulation': simulation,
    };
    final signature = jsonEncode(req);
    if (signature != requestSignature || requestKey == null) {
      requestSignature = signature;
      requestKey = List.generate(
        24,
        (_) => Random.secure().nextInt(256).toRadixString(16).padLeft(2, '0'),
      ).join();
    }
    await act(
      () => c.api.prepareAutomation(
        AutomationRunRequest(req),
        idempotencyKey: requestKey!,
      ),
    );
    if (!failed) requestKey = null;
  }

  Future<void> open(String id) async => act(() => c.api.automationRun(run: id));
  Future<void> decide(String action) async {
    if (!ready || detail == null) return;
    final r = Map<String, dynamic>.from(detail!['run'] as Map);
    if (action != 'cancel' &&
        !await confirm(
          action == 'approve'
              ? t('Approuver cette étape', 'Approve this step')
              : t('Confirmer la réconciliation', 'Confirm reconciliation'),
          r['target'] as String,
          action == 'approve'
              ? t(
                  'Autoriser uniquement l’étape affichée de cette version.',
                  'Authorize only the displayed step of this version.',
                )
              : t(
                  'Confirmer que l’hôte et le job de déploiement lié ont été inspectés et réconciliés. L’exécution ne sera pas rejouée.',
                  'Confirm the host and linked deployment job have been inspected and reconciled. This run will not be replayed.',
                ),
        )) {
      return;
    }
    await act(() async {
      final proof = action == 'cancel'
          ? ''
          : await c.operationProof({
              'action': 'automation_$action',
              'target': r['id'],
              'payload': {'revision': r['revision']},
            });
      return c.api.decideAutomation(
        AutomationDecisionRequest({
          'action': action,
          'revision': r['revision'],
          'proof': proof,
        }),
        run: r['id'] as String,
      );
    });
  }

  Future<void> reset(Map<String, dynamic> policy) async {
    final id = policy['id'] as String;
    if (!await confirm(
      t('Réarmer le circuit', 'Reset circuit breaker'),
      id,
      t(
        'Le refroidissement doit être terminé. Le même incident ne sera pas rejoué.',
        'Cooldown must have elapsed. The same incident will not be replayed.',
      ),
    )) {
      return;
    }
    await act(() async {
      final proof = await c.operationProof({
        'action': 'automation_reset',
        'target': id,
        'payload': <String, dynamic>{},
      });
      return c.api.resetAutomation(
        AutomationResetRequest({'proof': proof}),
        policy: id,
      );
    });
  }

  Map<String, dynamic> template(String kind) {
    final target = list('targets').firstOrNull?['id'] ?? '';
    if (kind == 'policy') {
      return {
        'id': 'repair',
        'version': 1,
        'target': target,
        'mode': 'supervised',
        'failures': 3,
        'window_seconds': 60,
        'freshness_seconds': 30,
        'actions': ['restart'],
        'max_attempts': 1,
        'backoff_seconds': 10,
        'observation_seconds': 30,
        'timeout_seconds': 300,
        'cooldown_seconds': 600,
        'maintenance_until': null,
        'dependencies': [],
        'exclusions': [],
        'healthy_release': '',
        'migration': 'none',
        'notify': true,
      };
    }
    return {
      'id': 'inspect',
      'version': 1,
      'target': target,
      'parameters': <String, dynamic>{},
      'steps': [
        {
          'action': 'check',
          'when': 'always',
          'seconds': 5,
          'timeout_seconds': 30,
          'retries': 1,
        },
      ],
      'timeout_seconds': 300,
      'maintenance_until': null,
    };
  }

  Future<void> edit(String kind, [Map<String, dynamic>? previous]) async {
    if (!ready) return;
    final initial = previous == null
        ? template(kind)
        : {...previous, 'version': (previous['version'] as int) + 1};
    final editor = TextEditingController(text: pretty(initial));
    String? error;
    final ticket = generation;
    dialogOpen = true;
    final document = await showDialog<Map<String, dynamic>>(
      context: context,
      builder: (ctx) => StatefulBuilder(
        builder: (ctx, update) => AlertDialog(
          title: Text(
            kind == 'policy'
                ? t('Version de politique', 'Policy version')
                : t('Version de runbook', 'Runbook version'),
          ),
          content: SizedBox(
            width: 660,
            child: SingleChildScrollView(
              child: Column(
                mainAxisSize: MainAxisSize.min,
                children: [
                  Text(
                    t(
                      'Cible fixe, actions bornées : check, restart, rollback, wait, approval, notify. Conditions : always, failed, succeeded. Les paramètres sont typés et validés avant exécution.',
                      'Fixed target, bounded actions: check, restart, rollback, wait, approval, notify. Conditions: always, failed, succeeded. Parameters are typed and validated before execution.',
                    ),
                  ),
                  const SizedBox(height: 16),
                  TextField(
                    controller: editor,
                    minLines: 10,
                    maxLines: 24,
                    style: PrismeTheme.mono(context),
                    decoration: InputDecoration(
                      labelText: t(
                        'Définition versionnée',
                        'Versioned definition',
                      ),
                      errorText: error,
                    ),
                  ),
                ],
              ),
            ),
          ),
          actions: [
            TextButton(
              onPressed: () => Navigator.pop(ctx),
              child: Text(t('Annuler', 'Cancel')),
            ),
            TextButton(
              onPressed: () {
                try {
                  final value = Map<String, dynamic>.from(
                    jsonDecode(editor.text) as Map,
                  );
                  if (value['id'] is! String || value['version'] is! int) {
                    throw const FormatException();
                  }
                  Navigator.pop(ctx, value);
                } catch (_) {
                  update(
                    () =>
                        error = t('Définition invalide', 'Invalid definition'),
                  );
                }
              },
              child: Text(t('Vérifier et publier', 'Review and publish')),
            ),
          ],
        ),
      ),
    );
    dialogOpen = false;
    // Keep the controller alive until the dialog's closing transition completes.
    Future<void>.delayed(const Duration(seconds: 1), editor.dispose);
    if (!mounted ||
        ticket != generation ||
        document == null ||
        c.identity == null) {
      return;
    }
    if (!await confirm(
      t('Publier cette version', 'Publish this version'),
      document['id'] as String,
      '${document['target']} · ${document['mode'] ?? 'runbook'} · v${document['version']}\n${t('Les exécutions conservent leur version exacte. Toute modification de politique bloque ses anciennes exécutions.', 'Runs retain their exact version. Changing a policy blocks its older runs.')}',
    )) {
      return;
    }
    await act(() async {
      final proof = await c.operationProof({
        'action': 'automation_publish',
        'target': document['id'],
        'payload': {'kind': kind, 'document': document},
      });
      return c.api.publishAutomation(
        AutomationDefinitionRequest({
          'kind': kind,
          'document': document,
          'proof': proof,
        }),
      );
    });
  }

  Future<void> schedule({Map<String, dynamic>? existing}) async {
    if (!ready || (book == null && existing == null)) return;
    Map<String, dynamic> value;
    try {
      final minutes = int.parse(interval.text);
      value = existing == null
          ? {
              'id': 'schedule-$bookID',
              'revision': 1,
              'request': {
                'runbook': bookID,
                'version': book!['version'],
                'parameters': Map<String, dynamic>.from(
                  jsonDecode(parameters.text) as Map,
                ),
                'simulation': false,
              },
              'every_seconds': minutes * 60,
              'next_at': DateTime.now()
                  .toUtc()
                  .add(const Duration(minutes: 1))
                  .toIso8601String(),
              'enabled': true,
            }
          : {
              ...existing,
              'revision': (existing['revision'] as int) + 1,
              'enabled': !(existing['enabled'] as bool),
              'next_at': DateTime.now()
                  .toUtc()
                  .add(const Duration(minutes: 1))
                  .toIso8601String(),
            };
    } catch (_) {
      showPrismeMessage(
        context,
        t('Planification invalide', 'Invalid schedule'),
      );
      return;
    }
    if (!await confirm(
      t('Enregistrer la planification', 'Save schedule'),
      value['id'] as String,
      '${t('Intervalle en secondes', 'Interval in seconds')}: ${value['every_seconds']} · ${t('Version figée', 'Pinned version')}: ${value['request']['version']}\n${t('Chaque étape sensible demandera une approbation.', 'Each sensitive step will require approval.')}',
    )) {
      return;
    }
    await act(() async {
      final proof = await c.operationProof({
        'action': 'automation_schedule',
        'target': value['id'],
        'payload': value,
      });
      return c.api.scheduleAutomation(
        AutomationScheduleRequest({'schedule': value, 'proof': proof}),
      );
    });
  }

  Widget section(String title, List<Widget> children) => PrismeSurface(
    child: Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Text(title, style: Theme.of(context).textTheme.titleMedium),
        const SizedBox(height: 12),
        ...children,
      ],
    ),
  );
  @override
  Widget build(BuildContext context) {
    final stopped = overview?['control']?['stopped'] != false;
    final r = detail?['run'] as Map<String, dynamic>?;
    return PrismePageFrame(
      header: PrismeSectionHeader(
        title: t('Autonomie et runbooks', 'Automation and runbooks'),
        actions: [
          PrismeButton(
            label: t('Actualiser', 'Refresh'),
            onPressed: busy ? null : refresh,
          ),
        ],
      ),
      children: [
        if (failed)
          PrismeBanner(
            title: t('État indisponible', 'State unavailable'),
            message: t(
              'Actualisez avant toute action.',
              'Refresh before taking action.',
            ),
            tone: PrismeTone.critical,
          ),
        if (overview == null && !failed && c.identity != null)
          const LinearProgressIndicator(),
        if (overview?['enabled'] == false)
          Text(t('Worker non configuré', 'Worker not configured')),
        if (overview?['enabled'] == true) ...[
          section(
            stopped
                ? t('Arrêt général actif', 'Global stop active')
                : t('Automatisation activée', 'Automation enabled'),
            [
              Text(
                t(
                  'Les politiques autonomes sont bornées. Les runbooks manuels et planifiés demandent une preuve fraîche à chaque étape sensible.',
                  'Autonomous policies are bounded. Manual and scheduled runbooks require a fresh proof for every sensitive step.',
                ),
              ),
              if (overview?['control']?['maintenance_until'] != null)
                Text(
                  '${t('Maintenance jusqu’au', 'Maintenance until')} ${overview!['control']['maintenance_until']}',
                ),
              const SizedBox(height: 12),
              Wrap(
                spacing: 8,
                runSpacing: 8,
                children: [
                  PrismeButton(
                    label: t('Arrêt général', 'Stop all'),
                    variant: PrismeButtonVariant.danger,
                    onPressed: ready ? () => control(stopped: true) : null,
                  ),
                  PrismeButton(
                    label: t('Activer / reprendre', 'Enable / resume'),
                    onPressed: ready ? () => control(stopped: false) : null,
                  ),
                  PrismeButton(
                    label: t('Maintenance 1 h', 'Maintenance 1 h'),
                    onPressed: ready
                        ? () => control(
                            stopped: stopped,
                            maintenance: DateTime.now().add(
                              const Duration(hours: 1),
                            ),
                          )
                        : null,
                  ),
                ],
              ),
            ],
          ),
          section(t('Politiques et procédures', 'Policies and procedures'), [
            Wrap(
              spacing: 8,
              runSpacing: 8,
              children: [
                PrismeButton(
                  label: t('Nouvelle politique', 'New policy'),
                  onPressed: ready ? () => edit('policy') : null,
                ),
                PrismeButton(
                  label: t('Nouveau runbook', 'New runbook'),
                  onPressed: ready ? () => edit('runbook') : null,
                ),
              ],
            ),
            for (final d in list('definitions'))
              ExpansionTile(
                title: Text('${d['id']} · v${d['version']}'),
                subtitle: Text('${d['target']} · ${d['mode'] ?? 'runbook'}'),
                children: [
                  PrismeCodeBlock(text: pretty(d)),
                  PrismeButton(
                    label: t('Nouvelle version', 'New version'),
                    onPressed: ready
                        ? () => edit(
                            d.containsKey('steps') ? 'runbook' : 'policy',
                            d,
                          )
                        : null,
                  ),
                ],
              ),
            for (final p in list('policies').where((p) => p['breaker'] == true))
              Padding(
                padding: const EdgeInsets.only(top: 12),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      '${t('Circuit ouvert', 'Circuit open')}: ${p['id']} · ${p['cooldown_until']}',
                    ),
                    PrismeButton(
                      label: t(
                        'Réarmer après refroidissement',
                        'Reset after cooldown',
                      ),
                      onPressed: ready ? () => reset(p) : null,
                    ),
                  ],
                ),
              ),
          ]),
          section(t('Exécuter ou planifier', 'Run or schedule'), [
            if (books.isEmpty)
              Text(
                t(
                  'Créez un runbook pour commencer.',
                  'Create a runbook to begin.',
                ),
              ),
            if (books.isNotEmpty) ...[
              PrismeSelect<String>(
                label: 'Runbook',
                value: bookID,
                options: books
                    .map(
                      (b) => PrismeOption<String>(
                        b['id'] as String,
                        '${b['id']} · v${b['version']}',
                      ),
                    )
                    .toList(),
                onChanged: ready ? (v) => setState(() => bookID = v) : null,
              ),
              const SizedBox(height: 12),
              PrismeCodeBlock(text: pretty(book?['steps'])),
              PrismeTextField(
                label: t('Paramètres typés (JSON)', 'Typed parameters (JSON)'),
                controller: parameters,
              ),
              const SizedBox(height: 12),
              Wrap(
                spacing: 8,
                runSpacing: 8,
                children: [
                  PrismeButton(
                    label: t('Simuler', 'Simulate'),
                    onPressed: ready ? () => prepare(true) : null,
                  ),
                  PrismeButton(
                    label: t('Préparer l’exécution', 'Prepare run'),
                    onPressed: ready ? () => prepare(false) : null,
                  ),
                ],
              ),
              const SizedBox(height: 12),
              PrismeTextField(
                label: t('Intervalle (minutes)', 'Interval (minutes)'),
                controller: interval,
              ),
              PrismeButton(
                label: t('Planifier cette version', 'Schedule this version'),
                onPressed: ready ? schedule : null,
              ),
            ],
            for (final s in list('schedules'))
              ListTile(
                title: Text(
                  '${s['id']} · ${s['enabled'] == true ? t('Actif', 'Enabled') : t('Suspendu', 'Paused')}',
                ),
                subtitle: Text('${s['next_at']} · ${s['every_seconds']} s'),
                trailing: IconButton(
                  tooltip: t('Activer / suspendre', 'Enable / pause'),
                  icon: const Icon(Icons.schedule),
                  onPressed: ready ? () => schedule(existing: s) : null,
                ),
              ),
          ]),
          if (r != null)
            section('${t('Exécution', 'Run')} · ${state(r['state'] as String)}', [
              Text(
                '${r['target']} · ${r['book']['id']} v${r['book']['version']}',
              ),
              Text(
                '${t('Étape', 'Step')} ${r['step']} · ${t('Tentatives', 'Attempts')} ${r['attempts']} · ${r['error_code'] ?? ''}',
              ),
              PrismeCodeBlock(text: pretty(r['book']['steps'])),
              Wrap(
                spacing: 8,
                runSpacing: 8,
                children: [
                  if (r['state'] == 'approval')
                    PrismeButton(
                      label: t('Approuver l’étape', 'Approve step'),
                      onPressed: ready ? () => decide('approve') : null,
                    ),
                  if (['queued', 'waiting', 'approval'].contains(r['state']))
                    PrismeButton(
                      label: t('Annuler l’exécution', 'Cancel run'),
                      onPressed: ready ? () => decide('cancel') : null,
                    ),
                  if (r['state'] == 'uncertain')
                    PrismeButton(
                      label: t(
                        'Confirmer la réconciliation',
                        'Confirm reconciliation',
                      ),
                      onPressed: ready ? () => decide('reconcile') : null,
                    ),
                ],
              ),
              for (final event in detail?['events'] as List? ?? [])
                Padding(
                  padding: const EdgeInsets.only(top: 8),
                  child: Text(
                    '${event['at']} · ${event['kind']}\n${pretty(event['detail'])}',
                  ),
                ),
            ]),
          section(t('Historique', 'History'), [
            for (final run in list('runs'))
              ListTile(
                title: Text(
                  '${run['book']['id']} v${run['book']['version']} · ${state(run['state'] as String)}',
                ),
                subtitle: Text(
                  '${run['target']} · ${run['origin']} · ${run['id']}',
                ),
                onTap: ready ? () => open(run['id'] as String) : null,
              ),
            Wrap(
              spacing: 8,
              children: [
                PrismeButton(
                  label: t('Précédent', 'Previous'),
                  onPressed: ready && page > 1
                      ? () {
                          page--;
                          refresh();
                        }
                      : null,
                ),
                PrismeButton(
                  label: t('Suivant', 'Next'),
                  onPressed: ready && overview?['has_more'] == true
                      ? () {
                          page++;
                          refresh();
                        }
                      : null,
                ),
              ],
            ),
          ]),
        ],
      ],
    );
  }
}
