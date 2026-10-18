import 'dart:async';
import 'dart:math';
import 'package:flutter/material.dart';
import 'package:prisme_ui/prisme_ui.dart';
import '../api/generated.dart';
import '../foundation/controller.dart';

class BackupsPage extends StatefulWidget {
  const BackupsPage({super.key, required this.controller});
  final FoundationController controller;
  @override
  State<BackupsPage> createState() => _BackupsPageState();
}

class _BackupsPageState extends State<BackupsPage> {
  FoundationController get c => widget.controller;
  BackupOverview? overview;
  BackupDetail? detail;
  String? policy, artifact, target, requestKey, requestSignature;
  bool busy = false, failed = false, dialogOpen = false;
  int generation = 0, page = 1;
  Timer? timer;
  String t(String fr, String en) => context.prismeStrings.choose(fr, en);
  String label(String v) => switch (v) {
    'backup' => t('Sauvegarde', 'Backup'),
    'check' => t('Vérification', 'Integrity check'),
    'retention' => t('Rétention', 'Retention'),
    'restore' => t('Restauration de test', 'Test restore'),
    'planned' => t('À approuver', 'Awaiting approval'),
    'queued' => t('En attente du worker', 'Waiting for worker'),
    'running' => t('En cours', 'Running'),
    'succeeded' => t('Vérifié', 'Verified'),
    'failed' => t('Échec', 'Failed'),
    'uncertain' => t(
      'À réconcilier sur l’hôte',
      'Host reconciliation required',
    ),
    'cancelled' => t('Annulé', 'Cancelled'),
    'snapshot' => t('Capture', 'Snapshot'),
    'integrity' => t(
      'Lecture complète et intégrité',
      'Full read and integrity',
    ),
    'restore_files' => t('Extraction vérifiée', 'Verified extraction'),
    'restore_database' => t('Nouvelle base de test', 'New test database'),
    'validate_database' => t('Validation de la base', 'Database validation'),
    'cleanup' => t(
      'Nettoyage de la base temporaire',
      'Temporary database cleanup',
    ),
    _ => v,
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
      requestKey = null;
    });
  }

  @override
  void dispose() {
    generation++;
    timer?.cancel();
    c.removeListener(securityChanged);
    super.dispose();
  }

  List<BackupArtifact> get artifacts =>
      overview?.artifacts
          .where((a) => a.policy_id == policy && a.expired_at == null)
          .toList() ??
      [];
  List<Map<String, dynamic>> get targets =>
      overview?.targets.where((v) => v['policy'] == policy).toList() ?? [];
  Map<String, dynamic> get selected =>
      overview?.policies.where((v) => v['id'] == policy).firstOrNull ?? {};

  void choose() {
    if (!(overview?.policies.any((v) => v['id'] == policy) ?? false)) {
      policy = overview?.policies.firstOrNull?['id'] as String?;
    }
    if (!artifacts.any((a) => a.id == artifact)) {
      artifact = artifacts.firstOrNull?.id;
    }
    if (!targets.any((v) => v['id'] == target)) {
      target = targets.firstOrNull?['id'] as String?;
    }
  }

  Future<void> refresh() async {
    if (!mounted || busy || c.busy || c.identity == null) return;
    final ticket = ++generation;
    setState(() => busy = true);
    final result = await c.securityAction(() async {
      final next = await c.api.backups(page: '$page');
      final d = detail == null
          ? null
          : await c.api.backupJob(job: detail!.job.id);
      if (!mounted || ticket != generation || c.identity == null) return false;
      overview = next;
      detail = d;
      choose();
      return true;
    });
    if (!mounted || ticket != generation) return;
    setState(() {
      busy = false;
      failed = result != true;
    });
  }

  Future<void> prepare(String kind) async {
    if (busy || failed || c.identity == null || policy == null) return;
    final req = BackupRequest(
      policy: policy!,
      kind: kind,
      artifact: kind == 'restore' ? artifact! : '',
      target: kind == 'restore' ? target! : '',
    );
    final signature = '${req.policy}/${req.kind}/${req.artifact}/${req.target}';
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
      () => c.api.prepareBackup(req, idempotencyKey: requestKey!),
    );
    if (!mounted || ticket != generation || c.identity == null) return;
    setState(() {
      busy = false;
      failed = result == null;
      if (result != null) {
        requestKey = null;
        detail = BackupDetail(
          job: result.job,
          confirmation: result.confirmation,
          plan: {},
          events: [],
        );
      }
    });
    if (result != null) await refresh();
  }

  Future<void> open(String id) async {
    if (busy || c.identity == null) return;
    final ticket = ++generation;
    setState(() => busy = true);
    final result = await c.securityAction(() => c.api.backupJob(job: id));
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
      dialogOpen = true;
      final accepted = await showPrismeConfirmation(
        context: context,
        plan: PrismeActionPlan(
          title: label(d.job.kind),
          actionLabel: t('Approuver', 'Approve'),
          target: d.job.kind == 'restore' ? d.job.target_id : d.job.policy_id,
          environment: PrismeEnvironment.staging,
          impact: d.job.kind == 'restore'
              ? t(
                  'Créer une nouvelle base ou un nouveau répertoire sur la cible de test.',
                  'Create a new database or directory on the test target.',
                )
              : t(
                  'Supprimer les anciennes copies selon la politique affichée.',
                  'Delete older snapshots according to the displayed policy.',
                ),
          verification: t(
            'Lecture complète du dépôt, extraction vérifiée et contrôle SQL pour les bases.',
            'Full repository read, verified extraction and SQL checks for databases.',
          ),
          rollback: d.job.kind == 'restore'
              ? t(
                  'La source reste disponible. La copie de test est conservée pour inspection.',
                  'The source remains available. The test copy is retained for inspection.',
                )
              : t(
                  'Les copies supprimées ne peuvent pas être récupérées.',
                  'Deleted snapshots cannot be recovered.',
                ),
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
              'action': 'backup_approve',
              'target': d.job.id,
              'payload': {'revision': d.job.revision},
            })
          : '';
      if (!mounted || ticket != generation || c.identity == null) return null;
      return c.api.decideBackup(
        BackupDecision(
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
    });
    if (result != null) await refresh();
  }

  Widget value(String label, Object? v) => Padding(
    padding: const EdgeInsets.only(bottom: 8),
    child: PrismeKeyValue(
      label: label,
      value: v == null || '$v'.isEmpty ? '—' : '$v',
    ),
  );

  @override
  Widget build(BuildContext context) {
    final a = artifacts.where((v) => v.id == artifact).firstOrNull;
    final usable =
        !busy && !failed && selected['enabled'] == true && c.identity != null;
    return PrismePageFrame(
      header: PrismeSectionHeader(
        title: t('Sauvegardes', 'Backups'),
        subtitle: t(
          'Copies chiffrées et restaurations vérifiées.',
          'Encrypted copies and verified restores.',
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
              'La requête a échoué. Actualisez avant toute action. Une approbation exige une passkey ou la biométrie.',
              'The request failed. Refresh before acting. Approval requires a passkey or biometrics.',
            ),
          ),
        if (overview == null)
          PrismeStateView(
            state: failed ? PrismeViewState.error : PrismeViewState.loading,
            title: t('Chargement des sauvegardes', 'Loading backups'),
          )
        else if (!overview!.enabled)
          PrismeStateView(
            state: PrismeViewState.empty,
            title: t('Sauvegardes non configurées', 'Backups not configured'),
            description: t(
              'Ajoutez une politique et une destination dans la configuration du serveur.',
              'Add a policy and destination in the server configuration.',
            ),
          )
        else ...[
          if (overview!.policies.any((p) => p['offsite_verified'] != true))
            PrismeBanner(
              title: t(
                'Protection hors site incomplète',
                'Off-site protection incomplete',
              ),
              message: t(
                'Au moins une politique ne dispose pas de sauvegarde vérifiée sur une destination déclarée hors site. Une copie sur le serveur source ne suffit pas.',
                'At least one policy has no verified backup at a destination declared off-site. A copy on the source server is insufficient.',
              ),
            ),
          PrismeSurface(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                PrismeSelect<String>(
                  label: t('Politique', 'Policy'),
                  value: policy,
                  options: [
                    for (final p in overview!.policies)
                      PrismeOption(
                        p['id'] as String,
                        '${p['project']} · ${p['environment']} · ${p['kind']}',
                      ),
                  ],
                  onChanged: busy
                      ? null
                      : (v) => setState(() {
                          policy = v;
                          choose();
                        }),
                ),
                const SizedBox(height: 12),
                value(
                  t('Source', 'Source'),
                  '${selected['server']} · ${selected['consistency']}',
                ),
                value(
                  t('Destination', 'Destination'),
                  '${selected['repository']} · ${selected['storage_host']}',
                ),
                value(
                  t('Dernière copie vérifiée', 'Last verified copy'),
                  selected['last_success_at'],
                ),
                value(
                  t(
                    'Période de sauvegarde (secondes, 0 = manuel)',
                    'Backup interval (seconds, 0 = manual)',
                  ),
                  selected['interval_seconds'],
                ),
                value(
                  t(
                    'Période du test de restauration (secondes)',
                    'Restore test interval (seconds)',
                  ),
                  selected['restore_test_seconds'],
                ),
                value(
                  t(
                    'Rétention : dernières / quotidiennes / hebdomadaires',
                    'Retention: last / daily / weekly',
                  ),
                  '${(selected['retention'] as Map?)?['keep_last']} / ${(selected['retention'] as Map?)?['keep_daily']} / ${(selected['retention'] as Map?)?['keep_weekly']}',
                ),
                Wrap(
                  spacing: 8,
                  runSpacing: 8,
                  children: [
                    PrismeButton(
                      label: t('Sauvegarder', 'Back up now'),
                      onPressed: usable ? () => prepare('backup') : null,
                    ),
                    PrismeButton(
                      label: t('Vérifier le dépôt', 'Check repository'),
                      onPressed: usable ? () => prepare('check') : null,
                    ),
                    PrismeButton(
                      label: t('Préparer la rétention', 'Plan retention'),
                      onPressed: usable ? () => prepare('retention') : null,
                    ),
                  ],
                ),
              ],
            ),
          ),
          PrismeSurface(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                PrismeSectionHeader(
                  title: t(
                    'Restaurer une copie de test',
                    'Restore a test copy',
                  ),
                ),
                if (artifacts.isEmpty)
                  Text(
                    t(
                      'Aucune copie vérifiée disponible sur cette page.',
                      'No verified copy available on this page.',
                    ),
                  )
                else ...[
                  PrismeSelect<String>(
                    label: t('Copie vérifiée', 'Verified snapshot'),
                    value: artifact,
                    options: [
                      for (final v in artifacts)
                        PrismeOption(v.id, '${v.created_at} · ${v.bytes} B'),
                    ],
                    onChanged: busy
                        ? null
                        : (v) => setState(() => artifact = v),
                  ),
                  const SizedBox(height: 12),
                  if (a != null) ...[
                    value(
                      t('Identité de la copie', 'Snapshot identity'),
                      a.snapshot_id,
                    ),
                    value(
                      t('Chiffrement / intégrité', 'Encryption / integrity'),
                      'Restic · ${a.verified_at}',
                    ),
                  ],
                  if (targets.isNotEmpty)
                    PrismeSelect<String>(
                      label: t('Cible alternative', 'Alternative target'),
                      value: target,
                      options: [
                        for (final v in targets)
                          PrismeOption(
                            v['id'] as String,
                            '${v['id']} · ${v['server']}',
                          ),
                      ],
                      onChanged: busy
                          ? null
                          : (v) => setState(() => target = v),
                    )
                  else
                    Text(
                      t(
                        'Aucune cible de test configurée.',
                        'No test target configured.',
                      ),
                    ),
                  const SizedBox(height: 12),
                  PrismeButton(
                    label: t('Préparer la restauration', 'Plan restore'),
                    onPressed: usable && artifact != null && target != null
                        ? () => prepare('restore')
                        : null,
                  ),
                ],
              ],
            ),
          ),
          if (detail != null) planView(detail!),
          PrismeSectionHeader(title: t('Historique', 'History')),
          for (final j in overview!.jobs)
            PrismeSurface(
              child: ListTile(
                contentPadding: EdgeInsets.zero,
                title: Text(
                  '${j.policy_id} · ${label(j.kind)} · ${label(j.state)}',
                ),
                subtitle: Text('${j.created_at}\n${label(j.step)}'),
                onTap: busy ? null : () => open(j.id),
              ),
            ),
          Wrap(
            spacing: 12,
            children: [
              PrismeButton(
                label: t('Précédent', 'Previous'),
                onPressed: busy || page == 1
                    ? null
                    : () {
                        page--;
                        unawaited(refresh());
                      },
              ),
              Text('$page'),
              PrismeButton(
                label: t('Suivant', 'Next'),
                onPressed: busy || !overview!.has_more
                    ? null
                    : () {
                        page++;
                        unawaited(refresh());
                      },
              ),
            ],
          ),
        ],
      ],
    );
  }

  Widget planView(BackupDetail d) {
    final j = d.job;
    final boundPolicy = d.plan['policy'] as Map?;
    final boundArtifact = d.plan['artifact'] as Map?;
    final boundTarget = d.plan['target'] as Map?;
    final expired =
        DateTime.tryParse(j.expires_at)?.isBefore(DateTime.now()) ?? true;
    return PrismeSurface(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          PrismeSectionHeader(
            title: '${label(j.kind)} · ${label(j.state)}',
            subtitle: j.id,
          ),
          value(
            t('Politique / cible', 'Policy / target'),
            '${j.policy_id} / ${j.target_id}',
          ),
          value(t('Copie source', 'Source artifact'), j.artifact_id),
          if (boundPolicy != null) ...[
            value(
              t('Origine liée au plan', 'Origin bound to this plan'),
              '${boundPolicy['project']} · ${boundPolicy['environment']} · ${boundPolicy['server']}',
            ),
            value(
              t('Rétention liée au plan', 'Retention bound to this plan'),
              '${(boundPolicy['retention'] as Map?)?['keep_last']} / ${(boundPolicy['retention'] as Map?)?['keep_daily']} / ${(boundPolicy['retention'] as Map?)?['keep_weekly']}',
            ),
          ],
          if (boundArtifact != null) ...[
            value(
              t('Date de la copie liée', 'Bound snapshot date'),
              boundArtifact['created_at'],
            ),
            value(t('Taille (octets)', 'Size (bytes)'), boundArtifact['bytes']),
            value(
              t(
                'Chiffrement / dernière vérification',
                'Encryption / last verification',
              ),
              'Restic · ${boundArtifact['verified_at']}',
            ),
          ],
          if (boundTarget != null)
            value(
              t('Serveur de restauration', 'Restore server'),
              boundTarget['server'],
            ),
          value(t('Étape', 'Step'), label(j.step)),
          value(
            t('Expiration de l’approbation', 'Approval expiry'),
            j.expires_at,
          ),
          value(t('Résultat', 'Result'), j.error_code),
          for (final v in j.evidence.entries) value(v.key, v.value),
          for (final v in d.events) value(label('${v['step']}'), v['at']),
          if (j.state == 'planned')
            PrismeButton(
              label: t('Approuver', 'Approve'),
              onPressed: busy || failed || expired
                  ? null
                  : () => decide('approve'),
            ),
          if (j.state == 'planned' || j.state == 'queued')
            PrismeButton(
              label: t('Annuler', 'Cancel'),
              onPressed: busy || failed ? null : () => decide('cancel'),
            ),
        ],
      ),
    );
  }
}
