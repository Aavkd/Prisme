import 'dart:async';
import 'dart:convert';
import 'dart:math';
import 'package:flutter/material.dart';
import 'package:prisme_ui/prisme_ui.dart';
import '../api/generated.dart';
import '../foundation/controller.dart';

class EdgePage extends StatefulWidget {
  const EdgePage({super.key, required this.controller});
  final FoundationController controller;
  @override
  State<EdgePage> createState() => _EdgePageState();
}

class _EdgePageState extends State<EdgePage> {
  FoundationController get c => widget.controller;
  EdgeOverview? overview;
  EdgeJobResponse? detail;
  final value = TextEditingController();
  String? target, upstream, requestKey, requestSignature;
  bool busy = false, failed = false, dialogOpen = false;
  int generation = 0, page = 1;
  Timer? timer;
  String t(String fr, String en) => context.prismeStrings.choose(fr, en);
  String state(String v) => switch (v) {
    'preparing' => t(
      'Validation en attente du worker',
      'Waiting for worker validation',
    ),
    'planned' => t('À approuver', 'Awaiting approval'),
    'queued' => t('En attente du worker', 'Waiting for worker'),
    'running' => t('En cours', 'Running'),
    'succeeded' => t('Vérifié', 'Verified'),
    'rolled_back' => t(
      'Configuration précédente restaurée',
      'Previous configuration restored',
    ),
    'uncertain' => t('Réconciliation requise', 'Reconciliation required'),
    'cancelled' => t('Annulé', 'Cancelled'),
    'failed' => t('Échec', 'Failed'),
    _ => v,
  };
  Map<String, dynamic> get selected =>
      overview?.targets.where((v) => v['id'] == target).firstOrNull ?? {};
  List<String> get choices =>
      (selected['choices'] as List? ?? []).cast<String>();
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
      value.clear();
      requestKey = null;
    });
  }

  @override
  void dispose() {
    generation++;
    timer?.cancel();
    c.removeListener(securityChanged);
    value.dispose();
    super.dispose();
  }

  Future<void> refresh() async {
    if (!mounted || busy || c.busy || c.identity == null) return;
    final ticket = ++generation;
    setState(() => busy = true);
    final result = await c.securityAction(() async {
      final next = await c.api.edge(page: '$page');
      final d = detail == null
          ? null
          : await c.api.edgeJob(job: detail!.job.id);
      if (!mounted || ticket != generation || c.identity == null) return false;
      overview = next;
      detail = d;
      if (!next.targets.any((v) => v['id'] == target)) {
        target = next.targets.firstOrNull?['id'] as String?;
      }
      if (!choices.contains(upstream)) upstream = choices.firstOrNull;
      return true;
    });
    if (!mounted || ticket != generation) return;
    setState(() {
      busy = false;
      failed = result != true;
    });
  }

  Future<void> prepare({EdgeJob? rollback}) async {
    if (busy || failed || c.identity == null || target == null) return;
    final req = EdgeRequest(
      target: rollback?.target ?? target!,
      value: rollback != null
          ? ''
          : selected['kind'] == 'caddy'
          ? upstream ?? ''
          : value.text.trim(),
      rollback: rollback?.id ?? '',
    );
    final signature = jsonEncode(req.toJson());
    if (requestSignature != signature || requestKey == null) {
      requestSignature = signature;
      requestKey = List.generate(
        24,
        (_) => Random.secure().nextInt(256).toRadixString(16).padLeft(2, '0'),
      ).join();
    }
    final ticket = ++generation;
    setState(() => busy = true);
    final result = await c.securityAction(
      () => c.api.prepareEdge(req, idempotencyKey: requestKey!),
    );
    if (!mounted || ticket != generation || c.identity == null) return;
    setState(() {
      busy = false;
      failed = result == null;
      if (result != null) {
        detail = result;
        requestKey = null;
      }
    });
  }

  Future<void> open(String id) async {
    if (busy || c.identity == null) return;
    final ticket = ++generation;
    setState(() => busy = true);
    final result = await c.securityAction(() => c.api.edgeJob(job: id));
    if (!mounted || ticket != generation || c.identity == null) return;
    setState(() {
      busy = false;
      failed = result == null;
      detail = result;
    });
  }

  Future<void> decide(String action) async {
    final d = detail;
    if (d == null || busy || failed || c.identity == null) return;
    final ticket = ++generation;
    if (action == 'approve') {
      final cfg = overview?.targets
          .where((v) => v['id'] == d.job.target)
          .firstOrNull;
      dialogOpen = true;
      final accepted = await showPrismeConfirmation(
        context: context,
        plan: PrismeActionPlan(
          title: t('Appliquer le plan', 'Apply plan'),
          actionLabel: t('Approuver', 'Approve'),
          target: d.job.target,
          environment: cfg?['environment'] == 'production'
              ? PrismeEnvironment.production
              : PrismeEnvironment.staging,
          impact: impact(d.job),
          verification: t(
            'Contrôler la configuration active et tous les tests HTTP/HTTPS affichés.',
            'Check active configuration and all listed HTTP/HTTPS tests.',
          ),
          rollback: rollbackText(d.job),
          requiredPhrase: d.confirmation,
        ),
      );
      dialogOpen = false;
      if (!mounted || ticket != generation || c.identity == null || !accepted) {
        return;
      }
    }
    setState(() => busy = true);
    final result = await c.securityAction(() async {
      final proof = action == 'approve'
          ? await c.operationProof({
              'action': 'edge_approve',
              'target': d.job.id,
              'payload': {'revision': d.job.revision},
            })
          : '';
      if (!mounted || ticket != generation || c.identity == null) return null;
      return c.api.decideEdge(
        EdgeDecision(
          action: action,
          revision: d.job.revision,
          phrase: action == 'approve' ? d.confirmation : '',
          proof: proof,
        ),
        job: d.job.id,
      );
    });
    if (!mounted || ticket != generation || c.identity == null) return;
    setState(() {
      busy = false;
      failed = result == null;
      if (result != null) detail = result;
    });
  }

  bool caddy(EdgeJob j) =>
      overview?.targets.any(
        (v) => v['id'] == j.target && v['kind'] == 'caddy',
      ) ??
      false;
  String impact(EdgeJob j) => caddy(j)
      ? t(
          'Recharger le Caddy mutualisé. Préserver les autres domaines et le fichier historique.',
          'Reload shared Caddy. Preserve other domains and the historical file.',
        )
      : t(
          'Modifier la destination DNS. Les caches peuvent conserver la valeur précédente jusqu’à expiration du TTL.',
          'Change the DNS destination. Caches may retain the previous value until TTL expiry.',
        );
  String rollbackText(EdgeJob j) => caddy(j)
      ? t(
          'Restauration automatique de la configuration saine si les tests échouent. Une dérive bloque le retour arrière.',
          'Automatically restore the healthy configuration if checks fail. Drift blocks rollback.',
        )
      : t(
          'Préparer et approuver un plan inverse. Une écriture incertaine exige une réconciliation.',
          'Prepare and approve a reverse plan. An uncertain write requires reconciliation.',
        );
  Widget kv(String label, Object? v) =>
      PrismeKeyValue(label: label, value: '${v ?? '—'}');
  @override
  Widget build(BuildContext context) {
    final d = detail;
    final enabled = !busy && !failed && c.identity != null;
    final canPlan = enabled && selected['write_enabled'] == true;
    final expired =
        d != null &&
        (DateTime.tryParse(d.job.expires_at)?.isBefore(DateTime.now()) ?? true);
    return PrismePageFrame(
      header: PrismeSectionHeader(
        title: 'Caddy · Cloudflare',
        subtitle: t(
          'Domaines, configuration et changements vérifiés.',
          'Domains, configuration and verified changes.',
        ),
        actions: [
          PrismeButton(
            label: t('Actualiser', 'Refresh'),
            loading: busy,
            onPressed: busy ? null : refresh,
          ),
        ],
      ),
      children: [
        if (failed)
          PrismeBanner(
            title: t('État à actualiser', 'Refresh required'),
            message: t(
              'La requête a échoué. Actualisez avant toute action.',
              'The request failed. Refresh before acting.',
            ),
          ),
        if (overview == null)
          PrismeStateView(
            state: failed ? PrismeViewState.error : PrismeViewState.loading,
            title: t('Chargement des domaines', 'Loading domains'),
          )
        else if (!overview!.enabled)
          PrismeStateView(
            state: PrismeViewState.empty,
            title: t(
              'Caddy et Cloudflare non configurés',
              'Caddy and Cloudflare not configured',
            ),
          )
        else ...[
          if (overview!.targets.isNotEmpty)
            PrismeSurface(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  PrismeSelect<String>(
                    label: t('Cible', 'Target'),
                    value: target,
                    options: overview!.targets
                        .map(
                          (v) => PrismeOption<String>(
                            v['id'] as String,
                            '${v['domain']} · ${v['environment']}',
                          ),
                        )
                        .toList(),
                    onChanged: busy
                        ? null
                        : (v) => setState(() {
                            target = v;
                            upstream = choices.firstOrNull;
                            value.clear();
                          }),
                  ),
                  const SizedBox(height: 12),
                  if (selected['kind'] == 'caddy')
                    PrismeSelect<String>(
                      label: t('Destination', 'Upstream'),
                      value: upstream,
                      options: choices
                          .map((v) => PrismeOption<String>(v, v))
                          .toList(),
                      onChanged: canPlan
                          ? (v) => setState(() => upstream = v)
                          : null,
                    )
                  else
                    PrismeTextField(
                      label: t(
                        'Nouvelle destination DNS',
                        'New DNS destination',
                      ),
                      controller: value,
                      enabled: canPlan,
                    ),
                  const SizedBox(height: 12),
                  if (selected['write_enabled'] != true)
                    Text(t('Lecture seule', 'Read only')),
                  PrismeButton(
                    label: t('Préparer le plan', 'Prepare plan'),
                    onPressed: canPlan ? () => prepare() : null,
                  ),
                ],
              ),
            ),
          if (d != null)
            PrismeSurface(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  PrismeSectionHeader(
                    title: t('Plan de changement', 'Change plan'),
                    subtitle: '${d.job.target} · ${state(d.job.state)}',
                  ),
                  kv(t('Révision', 'Revision'), d.job.revision),
                  kv(t('Expiration', 'Expires'), d.job.expires_at),
                  if (d.job.error_code.isNotEmpty)
                    PrismeBanner(
                      title: state(d.job.state),
                      message: t(
                        'Consultez l’audit et réconciliez la cible avant de préparer une nouvelle action.',
                        'Review the audit and reconcile the target before planning another action.',
                      ),
                    ),
                  if (d.job.plan.before.isNotEmpty ||
                      d.job.plan.after.isNotEmpty) ...[
                    Text(impact(d.job)),
                    const SizedBox(height: 8),
                    PrismeDiffView(
                      lines: [
                        for (final line in d.job.plan.before.split('\n'))
                          PrismeDiffLine(line, PrismeDiffKind.removed),
                        for (final line in d.job.plan.after.split('\n'))
                          PrismeDiffLine(line, PrismeDiffKind.added),
                      ],
                    ),
                    const SizedBox(height: 8),
                    Text(rollbackText(d.job)),
                    for (final check in d.job.plan.checks)
                      kv(
                        t('Test après changement', 'Post-change check'),
                        '${check['url']} · ${check['host']} → ${check['status']}',
                      ),
                  ],
                  if (d.job.result.isNotEmpty)
                    PrismeCodeBlock(
                      text: const JsonEncoder.withIndent(
                        '  ',
                      ).convert(d.job.result),
                    ),
                  const SizedBox(height: 12),
                  if (d.job.state == 'planned')
                    Wrap(
                      spacing: 8,
                      runSpacing: 8,
                      children: [
                        PrismeButton(
                          label: t('Approuver', 'Approve'),
                          onPressed: enabled && !expired
                              ? () => decide('approve')
                              : null,
                        ),
                        PrismeButton(
                          label: t('Annuler', 'Cancel'),
                          variant: PrismeButtonVariant.secondary,
                          onPressed: enabled && !expired
                              ? () => decide('cancel')
                              : null,
                        ),
                      ],
                    ),
                  if (d.job.state == 'succeeded' ||
                      (d.job.state == 'failed' &&
                          d.job.error_code == 'operator_reconciled'))
                    PrismeButton(
                      label: t('Préparer la restauration', 'Plan rollback'),
                      onPressed: enabled
                          ? () => prepare(rollback: d.job)
                          : null,
                    ),
                ],
              ),
            ),
          PrismeSectionHeader(title: t('Observations', 'Observations')),
          if (overview!.observations.isEmpty)
            Text(
              t(
                'En attente de la première collecte du worker.',
                'Waiting for the first worker observation.',
              ),
            ),
          for (final obs in overview!.observations)
            PrismeSurface(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  PrismeSectionHeader(
                    title: '${obs['id']}',
                    subtitle: '${obs['observed_at']}',
                  ),
                  if (obs['stale'] == true || obs['error_code'] != '')
                    PrismeBanner(
                      title: t(
                        'Observation indisponible ou ancienne',
                        'Unavailable or stale observation',
                      ),
                      message: t(
                        'La santé actuelle est inconnue.',
                        'Current health is unknown.',
                      ),
                    ),
                  if ((obs['data'] as Map?)?['drift'] == true)
                    PrismeBanner(
                      title: t('Dérive détectée', 'Drift detected'),
                      message: t(
                        'La configuration source diffère de la configuration active.',
                        'Source and active configurations differ.',
                      ),
                    ),
                  ExpansionTile(
                    title: Text(
                      t(
                        'Configuration et inventaire',
                        'Configuration and inventory',
                      ),
                    ),
                    children: [
                      PrismeCodeBlock(
                        text: const JsonEncoder.withIndent(
                          '  ',
                        ).convert(obs['data']),
                      ),
                    ],
                  ),
                ],
              ),
            ),
          PrismeSectionHeader(title: t('Historique des plans', 'Plan history')),
          for (final j in overview!.jobs)
            PrismeSurface(
              child: ListTile(
                title: Text(j.target),
                subtitle: Text('${state(j.state)} · ${j.created_at}'),
                onTap: enabled ? () => open(j.id) : null,
              ),
            ),
          Wrap(
            spacing: 8,
            children: [
              if (page > 1)
                PrismeButton(
                  label: t('Précédent', 'Previous'),
                  onPressed: enabled
                      ? () {
                          page--;
                          refresh();
                        }
                      : null,
                ),
              if (overview!.has_more)
                PrismeButton(
                  label: t('Suivant', 'Next'),
                  onPressed: enabled
                      ? () {
                          page++;
                          refresh();
                        }
                      : null,
                ),
            ],
          ),
        ],
      ],
    );
  }
}
