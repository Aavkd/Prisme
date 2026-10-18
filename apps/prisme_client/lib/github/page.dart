import 'dart:async';
import 'package:flutter/material.dart';
import 'package:prisme_ui/prisme_ui.dart';
import '../api/generated.dart';
import '../foundation/controller.dart';

class GithubPage extends StatefulWidget {
  const GithubPage({super.key, required this.controller});
  final FoundationController controller;
  @override
  State<GithubPage> createState() => _GithubPageState();
}

class _GithubPageState extends State<GithubPage> {
  GithubOverview? overview;
  GithubBuildPage? builds;
  GithubReleasePage? releases;
  Map<String, dynamic>? data, detail;
  String? repository, run, log;
  String section = 'builds', filePath = '', reference = '';
  int page = 1, detailPage = 1, generation = 0;
  bool busy = false, failed = false;
  final search = TextEditingController(), refInput = TextEditingController();
  Timer? timer;
  FoundationController get c => widget.controller;
  String t(String fr, String en) => context.prismeStrings.choose(fr, en);
  GithubRepository? get repo =>
      overview?.repositories.where((r) => r.id == repository).firstOrNull;

  @override
  void initState() {
    super.initState();
    c.addListener(securityChanged);
    WidgetsBinding.instance.addPostFrameCallback((_) => refresh());
    timer = Timer.periodic(const Duration(seconds: 30), (_) {
      if (!busy && !c.busy && section == 'builds' && run == null) {
        unawaited(refresh());
      }
    });
  }

  void securityChanged() {
    if (c.identity == null && mounted) {
      generation++;
      setState(() {
        overview = null;
        builds = null;
        releases = null;
        data = null;
        detail = null;
        log = null;
        busy = false;
      });
    }
  }

  @override
  void dispose() {
    generation++;
    timer?.cancel();
    c.removeListener(securityChanged);
    search.dispose();
    refInput.dispose();
    super.dispose();
  }

  Future<void> refresh() async {
    if (!mounted || busy || c.busy || c.identity == null) return;
    final ticket = ++generation;
    setState(() => busy = true);
    final result = await c.securityAction(() async {
      final next = await c.api.githubOverview();
      final id = next.repositories.any((r) => r.id == repository)
          ? repository
          : next.repositories.firstOrNull?.id;
      GithubBuildPage? b;
      GithubReleasePage? r;
      Map<String, dynamic>? browser, buildDetail;
      if (next.enabled && id != null) {
        if (section == 'builds') {
          b = await c.api.githubBuilds(repository: id, page: '$page');
          if (run != null) {
            buildDetail = (await c.api.githubBuild(
              page: '$detailPage',
              repository: id,
              run: run!,
            )).data;
          }
        } else if (section == 'releases') {
          r = await c.api.githubReleases(repository: id, page: '$page');
        } else {
          browser = (await c.api.githubBrowse(
            repository: id,
            kind: section,
            ref: reference,
            path: filePath,
            page: '$page',
          )).data;
        }
      }
      if (!mounted || generation != ticket || c.identity == null) return false;
      overview = next;
      repository = id;
      builds = b;
      releases = r;
      data = browser;
      detail = buildDetail;
      return true;
    });
    if (!mounted || ticket != generation) return;
    setState(() {
      busy = false;
      failed = result != true;
    });
  }

  void selectSection(String value) {
    setState(() {
      section = value;
      page = 1;
      detailPage = 1;
      run = null;
      detail = null;
      log = null;
      builds = null;
      releases = null;
      data = null;
    });
    unawaited(refresh());
  }

  Future<void> openLogs(String job) async {
    if (busy || c.busy || repository == null || run == null) return;
    final ticket = ++generation;
    setState(() {
      busy = true;
      log = null;
    });
    final result = await c.securityAction(
      () => c.api.githubLogs(repository: repository!, run: run!, job: job),
    );
    if (!mounted || generation != ticket || c.identity == null) return;
    setState(() {
      busy = false;
      failed = result == null;
      log = result?.text;
    });
  }

  String label(String value) => switch (value) {
    'builds' => 'Builds',
    'releases' => t('Releases liées', 'Linked releases'),
    'files' => t('Fichiers', 'Files'),
    'branches' => t('Branches', 'Branches'),
    'tags' => 'Tags',
    'commits' => 'Commits',
    'pulls' => 'Pull requests',
    'workflows' => 'Workflows',
    'queued' => t('En attente', 'Queued'),
    'in_progress' => t('En cours', 'In progress'),
    'completed' => t('Terminé', 'Completed'),
    'success' => t('Réussi', 'Succeeded'),
    'failure' => t('Échec', 'Failed'),
    'cancelled' => t('Annulé', 'Cancelled'),
    'linked' => t('Image liée', 'Image linked'),
    'unavailable' => t('Image non liée', 'Image not linked'),
    _ => value,
  };
  Widget keyValue(String name, String? value) => Padding(
    padding: const EdgeInsets.only(bottom: 8),
    child: PrismeKeyValue(
      label: name,
      value: value?.isNotEmpty == true ? value! : '—',
    ),
  );
  List<Map<String, dynamic>> entries(dynamic value) => value is List
      ? value.whereType<Map>().map((v) => Map<String, dynamic>.from(v)).toList()
      : [];
  String text(dynamic value) => value?.toString() ?? '';

  @override
  Widget build(BuildContext context) {
    if (c.identity == null) return const SizedBox.shrink();
    final tabs = [
      'builds',
      'releases',
      'workflows',
      'files',
      'branches',
      'tags',
      'commits',
      'pulls',
    ];
    final filtered =
        overview?.repositories
            .where(
              (r) => r.name.toLowerCase().contains(search.text.toLowerCase()),
            )
            .toList() ??
        [];
    final observed = DateTime.tryParse(repo?.last_sync ?? '');
    final stale =
        observed == null ||
        DateTime.now().difference(observed) > const Duration(minutes: 5);
    return PrismePageFrame(
      header: PrismeSectionHeader(
        title: 'GitHub',
        subtitle: t(
          'Code, builds et images immuables',
          'Code, builds and immutable images',
        ),
        actions: [
          PrismeButton(
            label: t('Actualiser', 'Refresh'),
            loading: busy,
            onPressed: busy || c.busy ? null : refresh,
          ),
        ],
      ),
      children: [
        if (failed)
          PrismeBanner(
            title: t('GitHub indisponible', 'GitHub unavailable'),
            message: t(
              'Les données affichées peuvent être anciennes. Réessayez après vérification de la connexion.',
              'Displayed data may be old. Check the connection and retry.',
            ),
            tone: PrismeTone.attention,
          ),
        if (overview == null && busy) const PrismeSkeleton(),
        if (overview?.enabled == false)
          PrismeSurface(
            child: Text(
              t(
                'GitHub n’est pas configuré. Connectez la GitHub App et sélectionnez les dépôts côté serveur.',
                'GitHub is not configured. Connect the GitHub App and select repositories on the server.',
              ),
            ),
          ),
        if (overview?.enabled == true) ...[
          PrismeSearchField(
            controller: search,
            label: t('Rechercher un dépôt', 'Search repositories'),
            onChanged: (_) => setState(() {}),
          ),
          Wrap(
            spacing: 8,
            runSpacing: 8,
            children: [
              for (final r in filtered)
                ChoiceChip(
                  label: Text(r.name),
                  selected: repository == r.id,
                  onSelected: busy
                      ? null
                      : (_) {
                          setState(() {
                            repository = r.id;
                            reference = '';
                            refInput.clear();
                            filePath = '';
                          });
                          selectSection('builds');
                        },
                ),
            ],
          ),
          if (filtered.isEmpty)
            Text(t('Aucun dépôt correspondant.', 'No matching repositories.')),
          if (repo != null) ...[
            PrismeSurface(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  keyValue(t('Dépôt', 'Repository'), repo!.name),
                  keyValue(
                    t('Environnement cible', 'Target environment'),
                    repo!.environment,
                  ),
                  PrismeFreshnessBadge(
                    observation: PrismeObservation(
                      freshness: failed || repo!.error_code.isNotEmpty
                          ? PrismeFreshness.offline
                          : stale
                          ? PrismeFreshness.stale
                          : PrismeFreshness.recent,
                      observedAt: observed,
                    ),
                  ),
                  if (repo!.error_code.isNotEmpty)
                    Text(
                      t(
                        'Synchronisation GitHub interrompue.',
                        'GitHub synchronization interrupted.',
                      ),
                    ),
                ],
              ),
            ),
            Wrap(
              spacing: 8,
              runSpacing: 8,
              children: [
                for (final tab in tabs)
                  ChoiceChip(
                    label: Text(label(tab)),
                    selected: section == tab,
                    onSelected: busy ? null : (_) => selectSection(tab),
                  ),
              ],
            ),
            if (section == 'builds') ...buildWidgets(),
            if (section == 'releases') ...releaseWidgets(),
            if (section != 'builds' && section != 'releases')
              ...browseWidgets(),
            if (run == null &&
                !(section == 'files' && data?['content'] != null))
              Wrap(
                spacing: 12,
                crossAxisAlignment: WrapCrossAlignment.center,
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
                  Text('${t('Page', 'Page')} $page'),
                  PrismeButton(
                    label: t('Suivant', 'Next'),
                    onPressed: busy || !hasMore
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
      ],
    );
  }

  bool get hasMore => switch (section) {
    'builds' => builds?.has_more ?? false,
    'releases' => releases?.has_more ?? false,
    'files' => false,
    _ => entries(data?['items'] ?? data?['workflows']).length >= 50,
  };

  List<Widget> buildWidgets() => [
    if (run == null && builds != null)
      PrismeDataTable<GithubBuild>(
        rows: builds!.items,
        columns: [
          PrismeTableColumn(
            label: 'Build',
            value: (b) => '${b.name} #${b.id} · ${b.attempt}',
          ),
          PrismeTableColumn(
            label: t('État', 'State'),
            value: (b) => label(b.conclusion.isEmpty ? b.status : b.conclusion),
          ),
          PrismeTableColumn(
            label: 'Commit',
            value: (b) => b.commit.substring(0, 12),
          ),
          PrismeTableColumn(
            label: t('Branche', 'Branch'),
            value: (b) => b.branch,
          ),
          PrismeTableColumn(
            label: 'Release',
            value: (b) => label(b.release_state),
          ),
        ],
        onOpen: busy
            ? null
            : (b) {
                run = b.id;
                detailPage = 1;
                log = null;
                unawaited(refresh());
              },
      ),
    if (run != null)
      PrismeButton(
        label: t('Retour aux builds', 'Back to builds'),
        onPressed: busy
            ? null
            : () {
                setState(() {
                  run = null;
                  detail = null;
                  log = null;
                });
                unawaited(refresh());
              },
      ),
    if (detail != null) ...[
      PrismeSurface(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            keyValue('Build', text(detail!['build']?['name'])),
            keyValue(
              t('État', 'State'),
              label(
                text(detail!['build']?['conclusion']).isEmpty
                    ? text(detail!['build']?['status'])
                    : text(detail!['build']?['conclusion']),
              ),
            ),
            keyValue(
              'Release',
              label(text(detail!['build']?['release_state'])),
            ),
            if (detail!['build']?['release_state'] != 'linked')
              Text(
                t(
                  'Les métadonnées des images sont absentes, expirées ou incompatibles avec ce build.',
                  'Image metadata is missing, expired or inconsistent with this build.',
                ),
              ),
            for (final field in [
              'commit',
              'branch',
              'author',
              'attempt',
              'created_at',
              'started_at',
              'updated_at',
            ])
              keyValue(switch (field) {
                'branch' => t('Branche', 'Branch'),
                'author' => t('Auteur', 'Author'),
                'attempt' => t('Tentative', 'Attempt'),
                'created_at' => t('Créé', 'Created'),
                'started_at' => t('Démarré', 'Started'),
                'updated_at' => t('Actualisé', 'Updated'),
                _ => 'Commit',
              }, text(detail!['build']?[field])),
          ],
        ),
      ),
      PrismeSectionHeader(title: t('Jobs et étapes', 'Jobs and steps')),
      for (final job in entries(detail!['jobs']?['jobs']))
        PrismeSurface(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                text(job['name']),
                style: Theme.of(context).textTheme.titleMedium,
              ),
              keyValue(
                t('État', 'State'),
                label(text(job['conclusion'] ?? job['status'])),
              ),
              keyValue('Runner', text(job['runner_name'])),
              for (final step in entries(job['steps']))
                keyValue(
                  text(step['name']),
                  label(text(step['conclusion'] ?? step['status'])),
                ),
              PrismeButton(
                label: t('Lire les logs', 'Read logs'),
                onPressed: busy ? null : () => openLogs(text(job['id'])),
              ),
            ],
          ),
        ),
      PrismeSectionHeader(title: t('Artefacts', 'Artifacts')),
      if (entries(detail!['artifacts']?['artifacts']).isEmpty)
        Text(t('Aucun artefact disponible.', 'No artifacts available.')),
      for (final a in entries(detail!['artifacts']?['artifacts']))
        PrismeSurface(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              keyValue(t('Nom', 'Name'), text(a['name'])),
              keyValue(t('Octets', 'Bytes'), text(a['size_in_bytes'])),
              keyValue(
                t('Disponibilité', 'Availability'),
                a['expired'] == true
                    ? t('Expiré', 'Expired')
                    : t('Disponible', 'Available'),
              ),
              keyValue('Digest', text(a['digest'])),
            ],
          ),
        ),
      Wrap(
        spacing: 12,
        children: [
          PrismeButton(
            label: t('Jobs précédents', 'Previous jobs'),
            onPressed: busy || detailPage == 1
                ? null
                : () {
                    detailPage--;
                    log = null;
                    unawaited(refresh());
                  },
          ),
          PrismeButton(
            label: t('Jobs et artefacts suivants', 'Next jobs and artifacts'),
            onPressed:
                busy ||
                    (entries(detail!['jobs']?['jobs']).length < 50 &&
                        entries(detail!['artifacts']?['artifacts']).length < 50)
                ? null
                : () {
                    detailPage++;
                    log = null;
                    unawaited(refresh());
                  },
          ),
        ],
      ),
      if (log != null)
        PrismeCodeBlock(text: log!, label: t('Logs du job', 'Job logs')),
    ],
  ];

  List<Widget> releaseWidgets() => [
    if (releases?.items.isEmpty == true)
      PrismeSurface(
        child: Text(
          t(
            'Aucune release liée. Un build réussi doit publier le manifeste de ses images par digest.',
            'No linked releases. A successful build must publish its image digest manifest.',
          ),
        ),
      ),
    for (final release in releases?.items ?? <GithubRelease>[])
      PrismeSurface(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(
              'Build #${release.manifest.run_id} · ${release.manifest.run_attempt}',
              style: Theme.of(context).textTheme.titleMedium,
            ),
            keyValue('Commit', release.manifest.commit),
            keyValue(
              t('Environnement', 'Environment'),
              release.manifest.environment,
            ),
            keyValue(
              t('Artefact source', 'Source artifact'),
              release.artifact_id,
            ),
            for (final image in release.manifest.images) ...[
              keyValue(image.service, image.reference),
              if (image.tag.isNotEmpty) keyValue('Tag', image.tag),
              if (image.provenance.isNotEmpty)
                keyValue('Provenance', image.provenance),
            ],
          ],
        ),
      ),
  ];

  List<Widget> browseWidgets() {
    final items = entries(data?['items'] ?? data?['workflows']);
    return [
      if (section == 'files' || section == 'commits') ...[
        PrismeTextField(
          controller: refInput,
          label: t('Branche, tag ou commit', 'Branch, tag or commit'),
          enabled: !busy,
        ),
        PrismeButton(
          label: t('Ouvrir la référence', 'Open reference'),
          onPressed: busy
              ? null
              : () {
                  reference = refInput.text.trim();
                  filePath = '';
                  page = 1;
                  data = null;
                  unawaited(refresh());
                },
        ),
      ],
      if (section == 'files') ...[
        Text(filePath.isEmpty ? '/' : filePath),
        if (filePath.isNotEmpty)
          PrismeButton(
            label: t('Dossier parent', 'Parent directory'),
            onPressed: busy
                ? null
                : () {
                    final parts = filePath.split('/')..removeLast();
                    filePath = parts.join('/');
                    data = null;
                    unawaited(refresh());
                  },
          ),
      ],
      if (data?['content'] != null)
        PrismeCodeBlock(text: text(data!['content'])),
      if (data != null && items.isEmpty && data?['content'] == null)
        Text(t('Aucun résultat.', 'No results.')),
      for (final item in items)
        PrismeSurface(
          semanticLabel: text(item['name'] ?? item['title'] ?? item['sha']),
          onTap:
              busy ||
                  (section != 'files' &&
                      section != 'branches' &&
                      section != 'tags')
              ? null
              : () {
                  if (section == 'files') {
                    filePath = text(item['path']);
                    data = null;
                    unawaited(refresh());
                  } else {
                    reference = text(item['name']);
                    refInput.text = reference;
                    selectSection('commits');
                  }
                },
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                text(
                  item['name'] ??
                      item['title'] ??
                      item['commit']?['message'] ??
                      item['sha'],
                ),
                style: Theme.of(context).textTheme.titleMedium,
              ),
              if (item['sha'] != null || item['commit']?['sha'] != null)
                keyValue('Commit', text(item['sha'] ?? item['commit']?['sha'])),
              if (item['type'] != null) keyValue('Type', text(item['type'])),
              if (item['state'] != null)
                keyValue(t('État', 'State'), text(item['state'])),
              if (item['number'] != null) keyValue('PR', '#${item['number']}'),
              if (item['head'] != null)
                keyValue(t('Source', 'Source'), text(item['head']['ref'])),
              if (item['base'] != null)
                keyValue(t('Cible', 'Target'), text(item['base']['ref'])),
              if (item['path'] != null)
                keyValue(t('Chemin', 'Path'), text(item['path'])),
              if (item['commit']?['author'] != null)
                keyValue(
                  t('Auteur', 'Author'),
                  text(item['commit']['author']['name']),
                ),
            ],
          ),
        ),
    ];
  }
}
