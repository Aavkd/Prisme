import 'dart:async';
import 'dart:convert';
import 'dart:math';
import 'package:flutter/material.dart';
import 'package:prisme_ui/prisme_ui.dart';
import '../api/generated.dart';
import '../deployments/page.dart';
import '../foundation/controller.dart';

class OnboardingPage extends StatefulWidget {
  const OnboardingPage({super.key, required this.controller});
  final FoundationController controller;
  @override
  State<OnboardingPage> createState() => _OnboardingPageState();
}

class _OnboardingPageState extends State<OnboardingPage> {
  FoundationController get c => widget.controller;
  Map<String, dynamic>? overview, preview;
  GithubReleasePage? releases;
  String? target, release, requestKey, signature, openedJob;
  bool busy = false, failed = false;
  int generation = 0;
  String t(String fr, String en) => context.prismeStrings.choose(fr, en);
  List<Map<String, dynamic>> get targets => [
    for (final v in overview?['targets'] as List? ?? [])
      Map<String, dynamic>.from(v as Map),
  ];
  Map<String, dynamic> get manifest => Map<String, dynamic>.from(
    targets
                .where((v) => (v['manifest'] as Map)['id'] == target)
                .firstOrNull?['manifest']
            as Map? ??
        {},
  );

  @override
  void initState() {
    super.initState();
    c.addListener(securityChanged);
    unawaited(refresh());
  }

  void securityChanged() {
    if (mounted && c.identity == null) {
      generation++;
      setState(() {
        overview = preview = null;
        releases = null;
        target = release = requestKey = signature = openedJob = null;
        busy = false;
      });
    }
  }

  @override
  void dispose() {
    generation++;
    c.removeListener(securityChanged);
    super.dispose();
  }

  Future<void> refresh() async {
    if (busy || c.identity == null) return;
    final ticket = ++generation;
    setState(() => busy = true);
    final result = await c.securityAction(() async {
      final next = await c.api.send('GET', '/v1/onboarding');
      if (!mounted || ticket != generation || c.identity == null) return false;
      overview = next;
      if (!targets.any((v) => (v['manifest'] as Map)['id'] == target)) {
        target = targets.firstOrNull?['manifest']['id'] as String?;
        preview = null;
      }
      final repo = manifest['repository_id'] as String?;
      final nextReleases = repo == null
          ? null
          : await c.api.githubReleases(repository: repo);
      if (!mounted || ticket != generation || c.identity == null) return false;
      releases = nextReleases;
      if (!(releases?.items.any((v) => v.id == release) ?? false)) {
        release = releases?.items.firstOrNull?.id;
      }
      return true;
    });
    if (!mounted || ticket != generation) return;
    setState(() {
      busy = false;
      failed = result != true;
    });
  }

  Future<void> prepare() async {
    if (busy ||
        failed ||
        target == null ||
        release == null ||
        c.identity == null) {
      return;
    }
    final nextSignature = '$target/$release';
    if (signature != nextSignature || requestKey == null) {
      signature = nextSignature;
      requestKey = List.generate(
        24,
        (_) => Random.secure().nextInt(256).toRadixString(16).padLeft(2, '0'),
      ).join();
    }
    final ticket = ++generation;
    setState(() => busy = true);
    final result = await c.securityAction(
      () => c.api.send(
        'POST',
        '/v1/onboarding',
        body: {'target': target, 'release': release},
        idempotencyKey: requestKey,
      ),
    );
    if (!mounted || ticket != generation || c.identity == null) return;
    setState(() {
      busy = false;
      failed = result == null;
      preview = result;
    });
    // Keep the key after a lost response or successful preparation. Opening the
    // deployment page does not create another plan on return to this wizard.
  }

  @override
  Widget build(BuildContext context) {
    if (openedJob != null) {
      return Column(
        children: [
          PrismeButton(
            label: t('Retour aux projets', 'Back to projects'),
            onPressed: () => setState(() => openedJob = null),
          ),
          Expanded(
            child: DeploymentsPage(
              key: ValueKey(openedJob),
              controller: c,
              initialJob: openedJob,
            ),
          ),
        ],
      );
    }
    final m = manifest;
    final registered = (overview?['projects'] as List? ?? [])
        .where((v) => v['target'] == target)
        .firstOrNull;
    return PrismePageFrame(
      header: PrismeSectionHeader(
        title: t('Nouveau projet', 'New project'),
        subtitle: t(
          'GitHub → release → premier déploiement',
          'GitHub → release → first deployment',
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
            title: t('Connexion à vérifier', 'Check connection'),
            message: t(
              'Actualisez avant de continuer. Une demande perdue peut être retrouvée dans les projets.',
              'Refresh before continuing. A lost request can be recovered from the projects.',
            ),
          ),
        if (overview?['enabled'] == false ||
            (overview != null && targets.isEmpty))
          PrismeStateView(
            state: PrismeViewState.empty,
            title: t(
              'Préparer un emplacement de projet',
              'Prepare a project target',
            ),
            description: t(
              'Configurez le dépôt et le workflow GitHub autorisés, puis un manifeste de déploiement sur le worker. Les secrets, bases et routes doivent être provisionnés et vérifiés avant le premier déploiement.',
              'Configure the allowed GitHub repository and workflow, then a deployment manifest on the worker. Provision and verify secrets, databases and routes before the first deployment.',
            ),
          ),
        if (targets.isNotEmpty) ...[
          PrismeSurface(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                PrismeSectionHeader(
                  title: t(
                    '1 · Dépôt et destination',
                    '1 · Repository and destination',
                  ),
                ),
                PrismeSelect<String>(
                  label: t('Projet autorisé', 'Allowed project'),
                  value: target,
                  options: [
                    for (final v in targets)
                      PrismeOption(
                        v['manifest']['id'] as String,
                        '${v['manifest']['repository']} · ${v['manifest']['environment']}',
                      ),
                  ],
                  onChanged: busy
                      ? null
                      : (v) {
                          setState(() {
                            target = v;
                            release = null;
                            preview = null;
                            releases = null;
                            requestKey = null;
                          });
                          unawaited(refresh());
                        },
                ),
                PrismeKeyValue(
                  label: t('Serveur', 'Server'),
                  value: '${m['server']}',
                ),
                PrismeKeyValue(
                  label: t('Branche', 'Branch'),
                  value: '${m['branch']}',
                ),
                PrismeKeyValue(
                  label: t('Stratégie', 'Strategy'),
                  value: '${m['strategy']}',
                ),
                PrismeKeyValue(
                  label: t('Sauvegarde requise', 'Backup required'),
                  value: '${m['backup_required']}',
                ),
                for (final service in m['services'] as List? ?? [])
                  PrismeKeyValue(
                    label: '${service['name']}',
                    value: '${service['health_path']} · ${service['env_file']}',
                  ),
              ],
            ),
          ),
          if (registered != null)
            PrismeSurface(
              child: Column(
                children: [
                  Text(
                    t(
                      'Ce projet possède déjà un premier plan.',
                      'This project already has a first plan.',
                    ),
                  ),
                  PrismeButton(
                    label: t('Ouvrir le déploiement', 'Open deployment'),
                    onPressed: busy || failed
                        ? null
                        : () => setState(
                            () => openedJob = registered['job'] as String,
                          ),
                  ),
                ],
              ),
            )
          else
            PrismeSurface(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  PrismeSectionHeader(
                    title: t('2 · Release vérifiée', '2 · Verified release'),
                  ),
                  if (releases?.items.isEmpty ?? true)
                    Text(
                      t(
                        'Publiez le manifeste prisme-release-v1 depuis le workflow autorisé, puis actualisez. Une release doit référencer des images par digest.',
                        'Publish the prisme-release-v1 manifest from the allowed workflow, then refresh. A release must reference images by digest.',
                      ),
                    ),
                  if (releases?.items.isNotEmpty ?? false)
                    PrismeSelect<String>(
                      label: 'Release',
                      value: release,
                      options: [
                        for (final r in releases!.items)
                          PrismeOption(
                            r.id,
                            '${r.manifest.commit} · ${r.manifest.run_id}',
                          ),
                      ],
                      onChanged: busy
                          ? null
                          : (v) => setState(() {
                              release = v;
                              preview = null;
                            }),
                    ),
                  PrismeButton(
                    label: t(
                      'Préparer et prévisualiser',
                      'Prepare and preview',
                    ),
                    loading: busy,
                    onPressed: busy || failed || release == null
                        ? null
                        : prepare,
                  ),
                ],
              ),
            ),
        ],
        if (preview != null)
          PrismeSurface(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                PrismeSectionHeader(
                  title: t(
                    '3 · Fichiers et premier plan',
                    '3 · Files and first plan',
                  ),
                ),
                Text(
                  t(
                    'Les fichiers ci-dessous sont proposés en lecture seule. Le plan doit encore être approuvé avant toute exécution.',
                    'The files below are read-only previews. The plan still requires approval before execution.',
                  ),
                ),
                for (final e in (preview!['files'] as Map).entries)
                  ExpansionTile(
                    title: Text('${e.key}'),
                    children: [PrismeCodeBlock(text: '${e.value}')],
                  ),
                PrismeCodeBlock(
                  text: const JsonEncoder.withIndent(
                    '  ',
                  ).convert(preview!['job']['plan']['blockers']),
                ),
                PrismeButton(
                  label: t(
                    'Examiner et approuver le plan',
                    'Review and approve plan',
                  ),
                  onPressed: busy || failed
                      ? null
                      : () => setState(
                          () => openedJob = preview!['job']['id'] as String,
                        ),
                ),
              ],
            ),
          ),
      ],
    );
  }
}
