import 'dart:async';
import 'dart:convert';
import 'dart:math';
import 'package:flutter/material.dart';
import 'package:prisme_ui/prisme_ui.dart';
import '../api/generated.dart';
import '../foundation/controller.dart';

class DeploymentsPage extends StatefulWidget {
  const DeploymentsPage({super.key, required this.controller, this.initialJob});
  final FoundationController controller;
  final String? initialJob;
  @override
  State<DeploymentsPage> createState() => _DeploymentsPageState();
}

class _DeploymentsPageState extends State<DeploymentsPage> {
  FoundationController get c => widget.controller;
  DeploymentOverview? overview;
  DeploymentDetail? detail;
  GithubReleasePage? releases;
  String? target, release;
  bool busy = false, failed = false, rollback = false, dialogOpen = false;
  int generation = 0, page = 1;
  String? requestKey, requestSignature;
  Timer? timer;
  String t(String fr, String en) => context.prismeStrings.choose(fr, en);
  String label(String value) => switch (value) {
    'awaiting_approval' => t('À approuver', 'Awaiting approval'),
    'ready' => t('Prêt · attend le worker', 'Ready · waiting for worker'),
    'running' => t('En cours', 'Running'),
    'succeeded' => t('Release saine', 'Healthy release'),
    'failed' => t('Échec vérifié', 'Verified failure'),
    'uncertain' => t(
      'Résultat incertain · cible réservée',
      'Uncertain result · target reserved',
    ),
    'cancelled' => t('Annulé', 'Cancelled'),
    'preconditions' => t('Préconditions et disque', 'Preconditions and disk'),
    'configuration' => t('Validation Compose', 'Compose validation'),
    'pull' => t('Récupération des images', 'Pull images'),
    'start' => t('Démarrage', 'Start'),
    'health' => t('Tests de santé', 'Health checks'),
    'traffic' => t('Bascule du trafic', 'Traffic switch'),
    'observation' => t('Observation', 'Observation'),
    'promote' => t('Promotion saine', 'Healthy promotion'),
    'target_disabled' => t('Cible non activée', 'Target disabled'),
    'legacy_build_requires_immutable_images' => t(
      'Build historique : images immuables requises',
      'Legacy build: immutable images required',
    ),
    'schema_incompatible' => t(
      'Schéma incompatible avec un rollback',
      'Schema incompatible with rollback',
    ),
    'verified_backup_required' => t(
      'Sauvegarde vérifiée requise',
      'Verified backup required',
    ),
    'secret_references_unavailable' => t(
      'Références de secrets indisponibles',
      'Secret references unavailable',
    ),
    _ => value,
  };
  @override
  void initState() {
    super.initState();
    c.addListener(securityChanged);
    unawaited(
      refresh().then((_) async {
        if (mounted && widget.initialJob != null) {
          await open(widget.initialJob!);
        }
      }),
    );
    timer = Timer.periodic(const Duration(seconds: 10), (_) {
      if (!dialogOpen) unawaited(refresh());
    });
  }

  void securityChanged() {
    if (c.identity == null && mounted) {
      generation++;
      if (dialogOpen) Navigator.of(context).popUntil((r) => r.isFirst);
      setState(() {
        overview = null;
        detail = null;
        releases = null;
        busy = false;
        requestKey = null;
      });
    }
  }

  @override
  void dispose() {
    generation++;
    timer?.cancel();
    c.removeListener(securityChanged);
    super.dispose();
  }

  Map<String, dynamic> get manifest {
    final selected = overview?.targets
        .where((v) => (v['manifest'] as Map)['id'] == target)
        .firstOrNull;
    return selected == null
        ? {}
        : Map<String, dynamic>.from(selected['manifest'] as Map);
  }

  Future<void> refresh() async {
    if (!mounted || busy || c.busy || c.identity == null) return;
    final ticket = ++generation;
    setState(() => busy = true);
    final result = await c.securityAction(() async {
      final next = await c.api.deployments(page: '$page');
      final ids = next.targets.map(
        (v) => (v['manifest'] as Map)['id'] as String,
      );
      final id = ids.contains(target) ? target : ids.firstOrNull;
      final m = next.targets
          .where((v) => (v['manifest'] as Map)['id'] == id)
          .firstOrNull;
      GithubReleasePage? r;
      if (m != null) {
        final gh = await c.api.githubOverview();
        final repo = (m['manifest'] as Map)['repository_id'] as String;
        if (gh.enabled && gh.repositories.any((v) => v.id == repo)) {
          r = await c.api.githubReleases(repository: repo);
        }
      }
      final d = detail == null
          ? null
          : await c.api.deployment(job: detail!.job.id);
      if (!mounted || generation != ticket || c.identity == null) return false;
      overview = next;
      target = id;
      releases = r;
      detail = d;
      if (!choices.containsKey(release)) release = choices.keys.firstOrNull;
      return true;
    });
    if (!mounted || ticket != generation) return;
    setState(() {
      busy = false;
      failed = result != true;
    });
  }

  Map<String, String> get choices {
    final result = <String, String>{};
    if (!rollback) {
      for (final r in releases?.items ?? <GithubRelease>[]) {
        result[r.id] =
            '${r.manifest.commit.substring(0, 12)} · ${r.manifest.run_id}';
      }
    } else {
      for (final j in overview?.items ?? <DeploymentJob>[]) {
        if (j.target_id == target && j.state == 'succeeded') {
          final r = j.plan['release'] as Map;
          result[r['id'] as String] = '${(r['manifest'] as Map)['commit']}';
        }
      }
    }
    return result;
  }

  Future<void> prepare() async {
    if (busy || target == null || release == null || c.identity == null) return;
    final sig = '$target/$release/$rollback';
    if (requestSignature != sig || requestKey == null) {
      requestSignature = sig;
      requestKey = List.generate(
        24,
        (_) => Random.secure().nextInt(256).toRadixString(16).padLeft(2, '0'),
      ).join();
    }
    final ticket = ++generation;
    setState(() => busy = true);
    final result = await c.securityAction(
      () => c.api.prepareDeployment(
        DeploymentRequest(
          target: target!,
          release: release!,
          rollback: rollback,
        ),
        idempotencyKey: requestKey!,
      ),
    );
    if (!mounted || ticket != generation || c.identity == null) return;
    setState(() {
      busy = false;
      failed = result == null;
      if (result != null) {
        detail = DeploymentDetail(job: result.job, events: []);
        requestKey = null;
      }
    });
    if (result != null) await refresh();
  }

  Future<void> open(String id) async {
    if (busy || c.identity == null) return;
    final ticket = ++generation;
    setState(() => busy = true);
    final result = await c.securityAction(() => c.api.deployment(job: id));
    if (!mounted || ticket != generation || c.identity == null) return;
    setState(() {
      busy = false;
      failed = result == null;
      detail = result;
    });
  }

  Future<void> decide(String action) async {
    final job = detail?.job;
    if (job == null || busy || c.identity == null) return;
    final p = job.plan, m = p['manifest'] as Map;
    if (action == 'approve') {
      dialogOpen = true;
      final accepted = await showPrismeConfirmation(
        context: context,
        plan: PrismeActionPlan(
          title: t('Approuver ce déploiement', 'Approve this deployment'),
          actionLabel: t('Approuver', 'Approve'),
          target: '${job.target_id} · ${m['server']}',
          environment: m['environment'] == 'production'
              ? PrismeEnvironment.production
              : PrismeEnvironment.staging,
          impact: m['strategy'] == 'blue-green'
              ? t(
                  'Démarrer la version candidate puis basculer la route Caddy gérée.',
                  'Start the candidate, then switch the managed Caddy route.',
                )
              : t(
                  'Recréer les services Compose. Une interruption est attendue.',
                  'Recreate Compose services. An interruption is expected.',
                ),
          verification: t(
            'Images exactes, santé de chaque service et période d’observation.',
            'Exact images, every service health check and observation period.',
          ),
          rollback:
              '${t('Release saine précédente', 'Previous healthy release')}: ${p['previous_release'] == '' ? '—' : p['previous_release']}',
          requiredPhrase: job.target_id,
        ),
      );
      dialogOpen = false;
      if (!mounted || c.identity == null || !accepted) return;
    }
    final ticket = ++generation;
    setState(() => busy = true);
    final result = await c.securityAction(() async {
      String? proof;
      if (action == 'approve' && m['environment'] == 'production') {
        proof = await c.actionProof(
          job,
          t('Approuver ce déploiement', 'Approve this deployment'),
        );
      }
      return c.api.decideDeployment(
        DeploymentDecision(
          action: action,
          revision: job.revision,
          phrase: action == 'approve' ? job.target_id : '',
          proof: proof,
        ),
        job: job.id,
      );
    });
    if (!mounted || ticket != generation || c.identity == null) return;
    setState(() {
      busy = false;
      failed = result == null;
    });
    if (result != null) await refresh();
  }

  Widget value(String key, Object? data) => Padding(
    padding: const EdgeInsets.only(bottom: 10),
    child: PrismeKeyValue(
      label: key,
      value: data?.toString().isNotEmpty == true ? '$data' : '—',
    ),
  );
  @override
  Widget build(BuildContext context) => PrismePageFrame(
    header: PrismeSectionHeader(
      title: t('Déploiements', 'Deployments'),
      subtitle: t(
        'Planifier, approuver, vérifier et restaurer.',
        'Plan, approve, verify and restore.',
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
          title: t(
            'Action ou actualisation impossible',
            'Action or refresh failed',
          ),
          message: c.error == 'biometric_key_invalidated'
              ? t(
                  'Reconnectez-vous avec une passkey puis réenregistrez la clé biométrique dans Notifications.',
                  'Sign in with a passkey, then re-enroll the biometric key in Notifications.',
                )
              : t(
                  'Les données affichées peuvent être anciennes. Actualisez avant d’agir. Une cible occupée, un plan expiré ou une révision modifiée impose un nouveau plan.',
                  'Displayed data may be stale. Refresh before acting. A busy target, expired plan or changed revision requires a new plan.',
                ),
        ),
      if (overview == null)
        PrismeStateView(
          state: PrismeViewState.loading,
          title: t('Chargement', 'Loading'),
        )
      else if (!overview!.enabled)
        PrismeStateView(
          state: PrismeViewState.empty,
          title: t('Déploiements non configurés', 'Deployments not configured'),
          description: t(
            'Importez un manifeste projet pour préparer une release.',
            'Import a project manifest to prepare a release.',
          ),
        )
      else ...[
        PrismeSurface(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              PrismeSelect<String>(
                label: t('Projet et environnement', 'Project and environment'),
                value: target,
                options: [
                  for (final v in overview!.targets)
                    PrismeOption(
                      (v['manifest'] as Map)['id'] as String,
                      '${(v['manifest'] as Map)['project']} · ${(v['manifest'] as Map)['environment']}',
                    ),
                ],
                onChanged: busy
                    ? null
                    : (id) {
                        setState(() {
                          target = id;
                          release = null;
                          releases = null;
                        });
                        unawaited(refresh());
                      },
              ),
              const SizedBox(height: 12),
              for (final code
                  in ((overview!.targets
                              .where(
                                (v) => (v['manifest'] as Map)['id'] == target,
                              )
                              .firstOrNull?['blockers']
                          as List?) ??
                      []))
                Text(label('$code')),
              SwitchListTile(
                contentPadding: EdgeInsets.zero,
                title: Text(
                  t('Restaurer une release saine', 'Restore a healthy release'),
                ),
                value: rollback,
                onChanged: busy
                    ? null
                    : (v) => setState(() {
                        rollback = v;
                        release = choices.keys.firstOrNull;
                      }),
              ),
              if (choices.isNotEmpty)
                PrismeSelect<String>(
                  label: 'Release',
                  value: release,
                  options: [
                    for (final e in choices.entries)
                      PrismeOption(e.key, e.value),
                  ],
                  onChanged: busy ? null : (v) => setState(() => release = v),
                )
              else
                Text(
                  t(
                    'Aucune release éligible dans cette page. Consultez GitHub ou l’historique.',
                    'No eligible release on this page. Check GitHub or history.',
                  ),
                ),
              const SizedBox(height: 12),
              PrismeButton(
                label: t('Préparer le plan', 'Prepare plan'),
                onPressed: busy || failed || release == null ? null : prepare,
              ),
            ],
          ),
        ),
        if (detail != null) planView(detail!),
        PrismeSectionHeader(title: t('Historique', 'History')),
        for (final job in overview!.items)
          PrismeSurface(
            child: ListTile(
              contentPadding: EdgeInsets.zero,
              title: Text('${job.target_id} · ${label(job.state)}'),
              subtitle: Text(
                '${job.created_at}\n${job.plan['rollback'] == true ? t('Restauration', 'Restore') : 'Release'} ${(job.plan['release'] as Map)['id']}',
              ),
              onTap: busy ? null : () => open(job.id),
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
                      setState(() => page--);
                      unawaited(refresh());
                    },
            ),
            Text('$page'),
            PrismeButton(
              label: t('Suivant', 'Next'),
              onPressed: busy || !overview!.has_more
                  ? null
                  : () {
                      setState(() => page++);
                      unawaited(refresh());
                    },
            ),
          ],
        ),
      ],
    ],
  );
  Widget planView(DeploymentDetail d) {
    final j = d.job,
        p = j.plan,
        m = p['manifest'] as Map,
        r = (p['release'] as Map)['manifest'] as Map,
        b = p['build'] as Map;
    final blockers = p['blockers'] as List;
    final expired =
        DateTime.tryParse(j.expires_at)?.isBefore(DateTime.now()) ?? true;
    final production = m['environment'] == 'production';
    return PrismeSurface(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          PrismeSectionHeader(title: label(j.state), subtitle: j.target_id),
          value(t('Serveur', 'Server'), m['server']),
          value(t('Stratégie', 'Strategy'), m['strategy']),
          value('Commit / build', '${r['commit']} / ${r['run_id']}'),
          value(
            t('Branche / auteur', 'Branch / author'),
            '${b['branch']} / ${b['author']}',
          ),
          value(
            t('Release saine précédente', 'Previous healthy release'),
            p['previous_release'],
          ),
          value(
            t('Dernier signal du worker', 'Last worker heartbeat'),
            j.heartbeat_at,
          ),
          value(t('Expiration du plan', 'Plan expiry'), j.expires_at),
          value(t('Migration déclarée', 'Declared migration'), m['migration']),
          value(
            t('Sauvegarde requise', 'Backup required'),
            m['backup_required'],
          ),
          if (m['backup_required'] == true) ...[
            value(
              t('Politique de sauvegarde', 'Backup policy'),
              m['backup_policy'],
            ),
            value(
              t(
                'Copie vérifiée liée au plan',
                'Verified copy bound to the plan',
              ),
              p['backup_artifact'],
            ),
            value(
              t('Âge maximal (secondes)', 'Maximum age (seconds)'),
              m['backup_max_age_seconds'],
            ),
          ],
          value(t('Route Caddy', 'Caddy route'), m['caddy_id']),
          value('Cloudflare', t('Aucun changement', 'No changes')),
          value(
            t('Observation', 'Observation'),
            '${m['observation_seconds']} s',
          ),
          value(
            t('Limite de durée', 'Time limit'),
            '${m['timeout_seconds']} s',
          ),
          value(t('Résultat', 'Result'), j.error_code),
          Text(
            t(
              'Diff des images Compose (avant → après)',
              'Compose image diff (before → after)',
            ),
          ),
          for (final e in (p['images'] as Map).entries)
            value(
              '${e.key}',
              '${(p['previous_images'] as Map)[e.key] ?? '—'}\n→ ${e.value}',
            ),
          for (final s in m['services'] as List) ...[
            value(
              '${(s as Map)['name']} · ${t('santé', 'health')}',
              '${s['docker_health'] == true ? 'Docker + ' : ''}${s['health_path']}',
            ),
            if (s['env_file'] != '')
              value(
                t(
                  'Référence de secret (valeur masquée)',
                  'Secret reference (value hidden)',
                ),
                s['env_file'],
              ),
          ],
          for (final step in p['steps'] as List)
            Text(
              '${(p['steps'] as List).indexOf(step) + 1}. ${label('$step')}',
            ),
          for (final blocker in blockers) Text(label('$blocker')),
          if (production)
            Text(
              t(
                'Production : biométrie Android ou vérification passkey Windows pour cette action précise.',
                'Production: Android biometrics or Windows passkey verification for this exact action.',
              ),
            ),
          if (expired && j.state == 'awaiting_approval')
            Text(
              t(
                'Plan expiré : préparez un nouveau plan.',
                'Plan expired: prepare a new plan.',
              ),
            ),
          const SizedBox(height: 12),
          Wrap(
            spacing: 12,
            runSpacing: 12,
            children: [
              if (j.state == 'awaiting_approval')
                PrismeButton(
                  label: t('Approuver', 'Approve'),
                  onPressed:
                      busy ||
                          failed ||
                          blockers.isNotEmpty ||
                          expired ||
                          (production &&
                              (c.mobile == null ||
                                  c.identity?.auth_mode != 'passkey'))
                      ? null
                      : () => decide('approve'),
                ),
              if (j.state == 'awaiting_approval' || j.state == 'ready')
                PrismeButton(
                  label: t('Annuler le travail', 'Cancel job'),
                  onPressed: busy || failed ? null : () => decide('cancel'),
                ),
            ],
          ),
          if (d.events.isNotEmpty) ...[
            const SizedBox(height: 20),
            Text(t('Preuves d’exécution', 'Execution evidence')),
            for (final event in d.events)
              Padding(
                padding: const EdgeInsets.symmetric(vertical: 8),
                child: SelectableText(
                  '${event['created_at']} · ${label('${event['step']}')} · ${label('${event['state']}')}\n${jsonEncode(event['evidence'])}',
                ),
              ),
          ],
        ],
      ),
    );
  }
}
