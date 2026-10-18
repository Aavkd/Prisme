import 'dart:async';
import 'dart:math';
import 'package:flutter/material.dart';
import 'package:prisme_ui/prisme_ui.dart';
import '../api/generated.dart';
import 'controller.dart';
import '../observability/page.dart';
import '../github/page.dart';
import '../deployments/page.dart';
import '../backups/page.dart';
import '../edge/page.dart';
import '../automation/page.dart';
import '../onboarding/page.dart';
import '../mobile/page.dart';
import '../mobile/offline_page.dart';
import '../operations/page.dart';
import '../operations/recovery_page.dart';

class FoundationPage extends StatefulWidget {
  const FoundationPage({
    super.key,
    required this.controller,
    required this.preferences,
  });
  final FoundationController controller;
  final PrismePreferences preferences;
  @override
  State<FoundationPage> createState() => _FoundationPageState();
}

class _FoundationPageState extends State<FoundationPage>
    with WidgetsBindingObserver {
  final _secret = TextEditingController();
  final _search = TextEditingController();
  final _device = TextEditingController(text: 'Prisme ${DateTime.now().year}');
  String destination = 'servers';
  String query = '';
  String? selectedId;
  String? notificationRoute;
  Timer? _clock;
  bool _sensitiveDialogOpen = false;
  FoundationController get c => widget.controller;
  String t(String fr, String en) => context.prismeStrings.choose(fr, en);
  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    c.addListener(_securityStateChanged);
    _clock = Timer.periodic(const Duration(seconds: 10), (_) {
      if (mounted) setState(() {});
    });
  }

  void _securityStateChanged() {
    if (mounted && c.identity != null && c.notificationRoute != null) {
      setState(() {
        notificationRoute = c.notificationRoute;
        c.notificationRoute = null;
        destination = 'mobile';
      });
    }
    if (mounted && c.identity == null && _sensitiveDialogOpen) {
      Navigator.of(context).popUntil((route) => route.isFirst);
    }
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    if (state == AppLifecycleState.paused &&
        !c.waitingForPasskey &&
        !c.platformAuthentication) {
      c.lock();
    }
    _secret.clear();
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    c.removeListener(_securityStateChanged);
    _clock?.cancel();
    _secret.dispose();
    _search.dispose();
    _device.dispose();
    super.dispose();
  }

  List<Widget> get settings => [
    if (c.deviceId != null && c.openBrowser != null)
      PrismeIconButton(
        icon: LucideIcons.lifeBuoy,
        label: t('Accès de secours', 'Recovery access'),
        onPressed: () => Navigator.of(context).push(
          MaterialPageRoute<void>(
            builder: (_) => RecoveryPage(
              deviceId: c.deviceId!,
              openBrowser: c.openBrowser!,
              normalSend: c.api.send,
              platformActivity: (v) => c.platformAuthentication = v,
            ),
          ),
        ),
      ),
    PrismeIconButton(
      icon: LucideIcons.languages,
      label: t('Passer en anglais', 'Switch to French'),
      onPressed: () => widget.preferences.locale = Locale(t('en', 'fr')),
    ),
    PopupMenuButton<PrismeAppearance>(
      tooltip: t('Thème', 'Theme'),
      icon: const Icon(LucideIcons.palette),
      onSelected: (a) => widget.preferences.appearance = a,
      itemBuilder: (_) => [
        for (final a in PrismeAppearance.values)
          PopupMenuItem(
            value: a,
            child: Text(switch (a) {
              PrismeAppearance.system => t('Système', 'System'),
              PrismeAppearance.light => t('Clair', 'Light'),
              PrismeAppearance.dark => t('Sombre', 'Dark'),
              PrismeAppearance.matrix => 'Matrix',
            }),
          ),
      ],
    ),
    if (c.identity != null)
      PrismeIconButton(
        icon: LucideIcons.logOut,
        label: t('Se déconnecter', 'Sign out'),
        onPressed: c.busy ? null : c.logout,
      ),
  ];
  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: c,
    builder: (context, _) => Listener(
      onPointerDown: (_) => c.touch(),
      child: c.offlineSnapshot != null
          ? OfflinePage(snapshot: c.offlineSnapshot!, close: c.closeCache)
          : c.identity == null
          ? Scaffold(
              body: SafeArea(
                child: PrismePageFrame(
                  header: PrismeSectionHeader(
                    title: 'Prisme',
                    subtitle: t(
                      'Votre infrastructure, en vue.',
                      'Your infrastructure, in view.',
                    ),
                    actions: settings,
                  ),
                  children: [
                    if (c.development)
                      PrismeBanner(
                        title: t(
                          'Environnement de développement',
                          'Development environment',
                        ),
                        message: t(
                          'Session locale explicite. Les services de production restent en lecture seule.',
                          'Explicit local session. Production services remain read-only.',
                        ),
                      ),
                    if (c.error != null) errorBanner(),
                    if (c.deviceId != null && c.openBrowser != null)
                      PrismeButton(
                        label: t(
                          'Accéder au VPS en secours',
                          'Open VPS recovery',
                        ),
                        onPressed: () => Navigator.of(context).push(
                          MaterialPageRoute<void>(
                            builder: (_) => RecoveryPage(
                              deviceId: c.deviceId!,
                              openBrowser: c.openBrowser!,
                              normalSend: c.api.send,
                              platformActivity: (v) =>
                                  c.platformAuthentication = v,
                            ),
                          ),
                        ),
                      ),
                    if (c.cache?.enabled == true)
                      PrismeButton(
                        label: t(
                          'Consulter le cache chiffré',
                          'Open encrypted cache',
                        ),
                        onPressed: c.busy ? null : c.openCache,
                      ),
                    PrismeSurface(
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.stretch,
                        children: [
                          PrismeTextField(
                            label: t('Nom de cet appareil', 'Device name'),
                            controller: _device,
                            enabled: !c.busy,
                          ),
                          const SizedBox(height: 20),
                          if (c.openBrowser != null) ...[
                            PrismeButton(
                              label: t(
                                'Se connecter avec une passkey',
                                'Sign in with a passkey',
                              ),
                              loading: c.busy,
                              onPressed: c.busy
                                  ? null
                                  : () => c.passkeyLogin(_device.text.trim()),
                            ),
                            if (c.waitingForPasskey) ...[
                              const SizedBox(height: 12),
                              Text(
                                t(
                                  'Confirmez dans le navigateur, puis revenez ici.',
                                  'Confirm in your browser, then return here.',
                                ),
                              ),
                              PrismeButton(
                                label: t('Annuler', 'Cancel'),
                                onPressed: c.cancelPasskey,
                              ),
                            ],
                            const SizedBox(height: 20),
                          ],
                          if (c.development) ...[
                            PrismeSecretField(
                              label: t(
                                'Secret de développement',
                                'Development secret',
                              ),
                              controller: _secret,
                              enabled: !c.busy,
                            ),
                            const SizedBox(height: 20),
                            PrismeButton(
                              label: t('Se connecter', 'Sign in'),
                              loading: c.busy,
                              variant: PrismeButtonVariant.primary,
                              onPressed: c.busy
                                  ? null
                                  : () async {
                                      final secret = _secret.text;
                                      _secret.clear();
                                      await c.login(
                                        secret,
                                        _device.text.trim(),
                                      );
                                    },
                            ),
                          ],
                        ],
                      ),
                    ),
                  ],
                ),
              ),
            )
          : PrismeAppShell(
              destinations: [
                PrismeDestination(
                  id: 'operations',
                  label: t('Terminal et fichiers', 'Terminal and files'),
                  icon: LucideIcons.terminal,
                ),
                PrismeDestination(
                  id: 'home',
                  label: t('Mes machines', 'My machines'),
                  icon: LucideIcons.house,
                ),
                PrismeDestination(
                  id: 'servers',
                  label: t('Serveurs', 'Servers'),
                  icon: LucideIcons.server,
                ),
                PrismeDestination(
                  id: 'observability',
                  label: t('Diagnostic', 'Diagnostics'),
                  icon: LucideIcons.activity,
                ),
                PrismeDestination(
                  id: 'devices',
                  label: t('Appareils', 'Devices'),
                  icon: LucideIcons.shieldCheck,
                ),
                PrismeDestination(
                  id: 'github',
                  label: 'GitHub',
                  icon: LucideIcons.gitBranch,
                ),
                PrismeDestination(
                  id: 'deployments',
                  label: t('Déploiements', 'Deployments'),
                  icon: LucideIcons.rocket,
                ),
                PrismeDestination(
                  id: 'onboarding',
                  label: t('Nouveau projet', 'New project'),
                  icon: LucideIcons.folderPlus,
                ),
                PrismeDestination(
                  id: 'audit',
                  label: t('Audit', 'Audit'),
                  icon: LucideIcons.list,
                ),
                PrismeDestination(
                  id: 'edge',
                  label: 'Caddy · Cloudflare',
                  icon: LucideIcons.globe,
                ),
                PrismeDestination(
                  id: 'automation',
                  label: t('Autonomie', 'Automation'),
                  icon: LucideIcons.workflow,
                ),
                PrismeDestination(
                  id: 'backups',
                  label: t('Sauvegardes', 'Backups'),
                  icon: LucideIcons.database,
                ),
                PrismeDestination(
                  id: 'mobile',
                  label: t('Notifications', 'Notifications'),
                  icon: LucideIcons.bell,
                ),
              ],
              selectedId: destination,
              onDestinationSelected: (id) {
                setState(() {
                  destination = id;
                  selectedId = null;
                });
                if (id == 'devices') c.loadDevices();
                if (id == 'audit') c.loadAudit();
                if (id == 'home' || id == 'servers') {
                  c.showFleet(pinned: id == 'home');
                }
              },
              toolbar: settings,
              workspaceLabel: 'Mantara',
              body: destination == 'mobile'
                  ? MobilePage(
                      key: ValueKey(notificationRoute),
                      controller: c,
                      route: notificationRoute,
                    )
                  : destination == 'operations'
                  ? OperationsPage(
                      send: c.api.send,
                      prove: c.operationProof,
                      openSocket: c.openTerminalSocket,
                      platformActivity: (v) => c.platformAuthentication = v,
                    )
                  : destination == 'observability'
                  ? ObservabilityPage(controller: c)
                  : destination == 'github'
                  ? GithubPage(controller: c)
                  : destination == 'deployments'
                  ? DeploymentsPage(controller: c)
                  : destination == 'onboarding'
                  ? OnboardingPage(controller: c)
                  : destination == 'edge'
                  ? EdgePage(controller: c)
                  : destination == 'automation'
                  ? AutomationPage(controller: c)
                  : destination == 'backups'
                  ? BackupsPage(controller: c)
                  : PrismePageFrame(
                      header: PrismeSectionHeader(
                        title: title(),
                        subtitle: c.connected
                            ? t('Backend connecté', 'Backend connected')
                            : t(
                                'État du backend inconnu',
                                'Backend state unknown',
                              ),
                        actions: [
                          PrismeButton(
                            label: t('Actualiser', 'Refresh'),
                            loading: c.busy,
                            onPressed: c.busy
                                ? null
                                : () {
                                    if (destination == 'devices') {
                                      c.loadDevices();
                                    } else if (destination == 'audit') {
                                      c.loadAudit();
                                    } else {
                                      c.refresh();
                                    }
                                  },
                          ),
                        ],
                      ),
                      children: [
                        if (c.error != null) errorBanner(),
                        if (destination == 'devices')
                          ...deviceWidgets()
                        else if (destination == 'audit')
                          ...auditWidgets()
                        else
                          ...fleetWidgets(),
                      ],
                    ),
            ),
    ),
  );
  String title() => switch (destination) {
    'home' => t('Mes machines', 'My machines'),
    'devices' => t('Appareils et récupération', 'Devices and recovery'),
    'audit' => t('Journal d’audit', 'Audit log'),
    _ => t('Serveurs', 'Servers'),
  };
  Widget errorBanner() => PrismeBanner(
    tone: PrismeTone.attention,
    title: t('Connexion non vérifiée', 'Connection not verified'),
    message: switch (c.error) {
      'biometric_key_invalidated' => t(
        'Reconnectez-vous avec une passkey puis réenregistrez la clé biométrique dans Notifications.',
        'Sign in with a passkey, then re-enroll the biometric key in Notifications.',
      ),
      'reauthentication_required' => t(
        'Reconnectez-vous avec une passkey pour cette action sensible.',
        'Sign in again with a passkey for this sensitive action.',
      ),
      'passkeys_unconfigured' => t(
        'Le domaine de connexion passkey doit être configuré sur le serveur.',
        'The passkey sign-in domain must be configured on the server.',
      ),
      'untrusted_server' => t(
        'Le certificat du serveur n’est pas approuvé.',
        'The server certificate is not trusted.',
      ),
      'unauthorized' || 'session_expired' => t(
        'Session expirée ou révoquée. Reconnectez-vous.',
        'Session expired or revoked. Sign in again.',
      ),
      _ => t(
        'Données non actualisées. Vérifiez la connexion puis réessayez.',
        'Data could not be refreshed. Check your connection and retry.',
      ),
    },
  );
  List<Widget> fleetWidgets() {
    final items = c.servers?.items ?? [];
    final selected = items.where((s) => s.id == selectedId).firstOrNull;
    if (selected != null) return detailWidgets(selected);
    final filtered = items
        .where(
          (s) =>
              (destination != 'home' ||
                  c.preferences.pinned_servers.contains(s.id)) &&
              s.name.toLowerCase().contains(query.toLowerCase()),
        )
        .toList();
    return [
      if (destination == 'servers')
        PrismeSearchField(
          controller: _search,
          onChanged: (v) => setState(() => query = v),
        ),
      if (destination == 'servers')
        Wrap(
          spacing: 12,
          runSpacing: 12,
          children: [
            PrismeButton(
              label: t('Enrôler un serveur', 'Enroll a server'),
              onPressed: c.busy || !c.connected ? null : enroll,
            ),
          ],
        ),
      if (c.busy && items.isEmpty) const LinearProgressIndicator(),
      if (filtered.isEmpty && !c.busy)
        PrismeSurface(
          child: Text(
            destination == 'home'
                ? t(
                    'Épinglez des serveurs depuis leur fiche. Cette sélection appartient à cet appareil.',
                    'Pin servers from their detail view. This selection belongs to this device.',
                  )
                : t(
                    'Aucun serveur trouvé. Enrôlez un agent ou importez un inventaire SSH en lecture seule.',
                    'No servers found. Enroll an agent or import a read-only SSH inventory.',
                  ),
          ),
        ),
      for (final server in filtered) serverCard(server),
      if (c.servers?.next_cursor.isNotEmpty ?? false)
        PrismeButton(
          label: t('Page suivante', 'Next page'),
          onPressed: c.busy ? null : () => c.refresh(nextPage: true),
        ),
    ];
  }

  InventorySnapshot? snapshot(ServerRecord server) => server.inventory == null
      ? null
      : InventorySnapshot.fromJson(server.inventory!);
  PrismeObservation observation(ServerRecord server) {
    final inventory = snapshot(server);
    final at = inventory == null
        ? null
        : DateTime.tryParse(inventory.observed_at);
    return PrismeObservation(
      freshness: at == null
          ? PrismeFreshness.unknown
          : !c.connected || server.connection_state != 'online'
          ? PrismeFreshness.offline
          : DateTime.now().difference(at) > const Duration(seconds: 90)
          ? PrismeFreshness.stale
          : PrismeFreshness.recent,
      observedAt: at,
    );
  }

  Widget serverCard(ServerRecord server) {
    final inventory = snapshot(server);
    final projects =
        inventory?.containers
            .map((v) => v.project)
            .where((v) => v.isNotEmpty)
            .toSet() ??
        {};
    return PrismeResourceCard(
      title: server.name,
      icon: LucideIcons.server,
      observation: observation(server),
      status: server.revoked
          ? PrismeStatus.failed
          : inventory == null
          ? PrismeStatus.unknown
          : inventory.warnings.isNotEmpty
          ? PrismeStatus.degraded
          : PrismeStatus.active,
      subtitle:
          inventory?.host.os ??
          t('En attente d’inventaire', 'Awaiting inventory'),
      onTap: () => setState(() => selectedId = server.id),
      child: Padding(
        padding: const EdgeInsets.only(top: 16),
        child: Wrap(
          spacing: 32,
          runSpacing: 16,
          children: [
            PrismeMetric(
              label: t('Projets Compose', 'Compose projects'),
              value: inventory == null ? null : '${projects.length}',
            ),
            PrismeMetric(
              label: t('Conteneurs', 'Containers'),
              value: inventory == null
                  ? null
                  : '${inventory.containers.length}',
            ),
            PrismeMetric(
              label: 'CPU',
              value: inventory?.host.cpus.toString(),
              unit: t('cœurs', 'cores'),
            ),
            PrismeMetric(
              label: 'RAM',
              value: gib(inventory?.host.memory_bytes),
              unit: 'GiB',
            ),
          ],
        ),
      ),
    );
  }

  String? gib(int? value) =>
      value == null ? null : (value / 1073741824).toStringAsFixed(1);
  List<Widget> detailWidgets(ServerRecord server) {
    final inventory = snapshot(server);
    final groups = <String, List<InventoryContainer>>{};
    for (final container in inventory?.containers ?? <InventoryContainer>[]) {
      groups
          .putIfAbsent(
            container.project.isEmpty
                ? t('Sans projet Compose', 'No Compose project')
                : container.project,
            () => [],
          )
          .add(container);
    }
    return [
      PrismeButton(
        label: t('Retour aux serveurs', 'Back to servers'),
        onPressed: () => setState(() => selectedId = null),
      ),
      serverCard(server),
      PrismeSurface(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            PrismeKeyValue(label: t('Identité', 'Identity'), value: server.id),
            const SizedBox(height: 12),
            PrismeKeyValue(
              label: t('Source', 'Source'),
              value: inventory?.source ?? 'agent',
            ),
            const SizedBox(height: 12),
            PrismeKeyValue(
              label: t('Dernier signal', 'Last heartbeat'),
              value: server.last_seen_at ?? '—',
            ),
            const SizedBox(height: 12),
            PrismeKeyValue(
              label: t('Capacités autorisées', 'Granted capabilities'),
              value: server.capabilities.join(', '),
            ),
            if (inventory?.source != 'ssh-readonly') ...[
              const SizedBox(height: 12),
              PrismeKeyValue(
                label: t('Expiration du certificat', 'Certificate expires'),
                value: server.expires_at,
              ),
            ],
            const SizedBox(height: 16),
            if (c.deviceId != null)
              SwitchListTile(
                contentPadding: EdgeInsets.zero,
                title: Text(t('Afficher sur mon accueil', 'Show on my home')),
                value: c.preferences.pinned_servers.contains(server.id),
                onChanged: c.busy || !c.connected
                    ? null
                    : (v) => c.pin(server.id, v),
              ),
            Wrap(
              spacing: 12,
              runSpacing: 12,
              children: [
                if (inventory?.source != 'ssh-readonly')
                  PrismeButton(
                    label: server.capabilities.contains('inventory:docker')
                        ? t('Limiter au signal', 'Restrict to heartbeat')
                        : t('Inventaire Docker', 'Docker inventory'),
                    onPressed: c.busy || !c.connected || server.revoked
                        ? null
                        : () async {
                            if (await confirm(
                              server.name,
                              server.capabilities.contains('inventory:docker')
                                  ? t(
                                      'Arrêter les nouvelles observations d’inventaire pour cette identité.',
                                      'Stop new inventory observations from this identity.',
                                    )
                                  : t(
                                      'Autoriser la collecte hôte et Docker en lecture seule.',
                                      'Allow read-only host and Docker collection.',
                                    ),
                              t(
                                'Modifier les capacités à nouveau.',
                                'Change capabilities again.',
                              ),
                            )) {
                              await c.capabilities(
                                server.id,
                                server.capabilities.contains('inventory:docker')
                                    ? ['self:heartbeat']
                                    : [
                                        'self:heartbeat',
                                        'inventory:host',
                                        'inventory:docker',
                                      ],
                              );
                            }
                          },
                  ),
                PrismeButton(
                  label: t('Révoquer', 'Revoke'),
                  variant: PrismeButtonVariant.danger,
                  onPressed: c.busy || !c.connected || server.revoked
                      ? null
                      : () async {
                          if (await confirm(
                            server.name,
                            t(
                              'Refuser immédiatement les observations et certificats de cette identité.',
                              'Immediately reject observations and certificates from this identity.',
                            ),
                            t(
                              'Un nouvel enrôlement est nécessaire.',
                              'A new enrollment is required.',
                            ),
                          )) {
                            await c.revokeServer(server.id);
                          }
                        },
                ),
              ],
            ),
          ],
        ),
      ),
      if (inventory != null) ...[
        if (inventory.warnings.isNotEmpty)
          PrismeBanner(
            tone: PrismeTone.attention,
            title: t('Inventaire partiel', 'Partial inventory'),
            message: inventory.warnings.map(warning).join(' · '),
          ),
        PrismeSurface(
          child: Wrap(
            spacing: 32,
            runSpacing: 16,
            children: [
              PrismeMetric(
                label: t('Disque total', 'Total disk'),
                value: gib(inventory.host.disk_total_bytes),
                unit: 'GiB',
              ),
              PrismeMetric(
                label: t('Disque libre', 'Free disk'),
                value: gib(inventory.host.disk_free_bytes),
                unit: 'GiB',
              ),
              PrismeMetric(
                label: t('Réseaux', 'Networks'),
                value: '${inventory.networks.length}',
              ),
              PrismeMetric(
                label: t('Volumes', 'Volumes'),
                value: '${inventory.volumes.length}',
              ),
            ],
          ),
        ),
        PrismeSectionHeader(
          title: t('Découverte Compose', 'Compose discovery'),
          subtitle: t(
            'Regroupement par labels Compose ; ressources découvertes, gestion non activée.',
            'Grouped by Compose labels; discovered resources, management not enabled.',
          ),
        ),
        for (final group in groups.entries)
          PrismeSurface(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                PrismeSectionHeader(
                  title: group.key,
                  subtitle:
                      '${group.value.length} ${t('conteneurs', 'containers')}',
                ),
                for (final container in group.value)
                  ExpansionTile(
                    tilePadding: EdgeInsets.zero,
                    title: Text(
                      container.service.isEmpty
                          ? container.name
                          : container.service,
                    ),
                    subtitle: Text(
                      '${containerState(container.state)} · ${containerHealth(container.health)}',
                    ),
                    children: [
                      for (final pair in <String, String>{
                        t('Conteneur', 'Container'): container.name,
                        'ID': container.id,
                        'Image': container.image,
                        t('Image immuable', 'Immutable image'):
                            container.image_id,
                        t('Ports publiés', 'Published ports'): container.ports,
                        t('Réseaux', 'Networks'): container.networks,
                        t('Fichiers Compose (label)', 'Compose files (label)'):
                            container.config_files,
                      }.entries)
                        Padding(
                          padding: const EdgeInsets.symmetric(vertical: 8),
                          child: PrismeKeyValue(
                            label: pair.key,
                            value: pair.value.isEmpty ? '—' : pair.value,
                          ),
                        ),
                    ],
                  ),
              ],
            ),
          ),
        if (inventory.images.isNotEmpty)
          PrismeSurface(
            child: ExpansionTile(
              title: Text(t('Images et digests', 'Images and digests')),
              children: [
                for (final image in inventory.images)
                  Padding(
                    padding: const EdgeInsets.all(8),
                    child: PrismeKeyValue(
                      label: image.tags.join(', '),
                      value: image.digests.isEmpty
                          ? image.id
                          : image.digests.join(', '),
                    ),
                  ),
              ],
            ),
          ),
        for (final group in {
          t('Réseaux', 'Networks'): inventory.networks,
          t('Volumes', 'Volumes'): inventory.volumes,
        }.entries)
          if (group.value.isNotEmpty)
            PrismeSurface(
              child: ExpansionTile(
                title: Text(group.key),
                children: [
                  for (final resource in group.value)
                    ListTile(
                      title: Text(resource.name),
                      subtitle: Text(
                        '${resource.driver} · ${resource.project}',
                      ),
                    ),
                ],
              ),
            ),
      ],
    ];
  }

  String containerState(String value) => switch (value) {
    'running' => t('En cours', 'Running'),
    'exited' => t('Arrêté', 'Stopped'),
    'paused' => t('En pause', 'Paused'),
    'restarting' => t('Redémarrage', 'Restarting'),
    'created' => t('Créé', 'Created'),
    'dead' => t('Hors service', 'Dead'),
    _ => t('État inconnu', 'Unknown state'),
  };
  String containerHealth(String value) => switch (value) {
    'healthy' => t('Sain', 'Healthy'),
    'unhealthy' => t('Sonde en échec', 'Unhealthy'),
    'starting' => t('Sonde en démarrage', 'Health check starting'),
    'not_configured' => t('Sans sonde', 'No health check'),
    _ => t('Santé inconnue', 'Health unknown'),
  };
  String warning(String code) => switch (code) {
    'docker_unavailable' => t(
      'Docker indisponible ou accès refusé',
      'Docker unavailable or access denied',
    ),
    'memory_unavailable' => t('Mémoire indisponible', 'Memory unavailable'),
    'container_details_partial' => t(
      'Détails conteneurs partiels',
      'Partial container details',
    ),
    _ => t(
      'Une partie de la collecte est indisponible',
      'Part of the collection is unavailable',
    ),
  };
  Future<bool> confirm(String target, String impact, String rollback) =>
      showPrismeConfirmation(
        context: context,
        plan: PrismeActionPlan(
          title: t('Confirmer l’action', 'Confirm action'),
          actionLabel: t('Confirmer', 'Confirm'),
          target: target,
          environment: c.development
              ? PrismeEnvironment.development
              : PrismeEnvironment.production,
          impact: impact,
          verification: t(
            'Relire l’état depuis le serveur.',
            'Read the resulting state from the server.',
          ),
          rollback: rollback,
        ),
      );
  Future<void> enroll() async {
    if (!await confirm(
      t('Nouvelle identité serveur', 'New server identity'),
      t(
        'Créer un jeton valable dix minutes, limité au heartbeat et à l’inventaire.',
        'Create a ten-minute token restricted to heartbeat and inventory.',
      ),
      t(
        'Révoquer l’identité après enrôlement.',
        'Revoke the identity after enrollment.',
      ),
    )) {
      return;
    }
    final random = Random.secure();
    final key = List.generate(
      20,
      (_) => random.nextInt(16).toRadixString(16),
    ).join();
    final invitation = await c.securityAction(
      () => c.api.invite(
        const Capabilities(
          capabilities: [
            'self:heartbeat',
            'inventory:host',
            'inventory:docker',
          ],
        ),
        idempotencyKey: key,
      ),
    );
    if (invitation != null && mounted) {
      await secretDialog(
        t('Jeton d’enrôlement', 'Enrollment token'),
        invitation.token,
        t(
          'Enregistrez ce jeton dans un fichier privé sur le serveur. Lancez l’agent avec -token-file, -ca et -url. Aucun port entrant requis.',
          'Save this token in a private file on the server. Start the agent with -token-file, -ca and -url. No inbound port is required.',
        ),
      );
    }
  }

  Future<void> secretDialog(
    String title,
    String value,
    String explanation,
  ) async {
    final controllers = value
        .split('\n')
        .map((v) => TextEditingController(text: v))
        .toList();
    _sensitiveDialogOpen = true;
    try {
      await showPrismeDialog<void>(
        context: context,
        title: title,
        builder: (_) => Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Text(explanation),
            const SizedBox(height: 16),
            for (var i = 0; i < controllers.length; i++)
              Padding(
                padding: const EdgeInsets.only(bottom: 12),
                child: PrismeSecretField(
                  label: controllers.length == 1 ? title : '$title ${i + 1}',
                  controller: controllers[i],
                ),
              ),
          ],
        ),
      );
    } finally {
      _sensitiveDialogOpen = false;
      for (final controller in controllers) {
        controller.clear();
        controller.dispose();
      }
    }
  }

  List<Widget> deviceWidgets() => [
    PrismeBanner(
      title: t('Sécurité de la session', 'Session security'),
      message: t(
        'Les actions sensibles exigent une connexion passkey de moins de cinq minutes. La session se verrouille après cinq minutes sans interaction.',
        'Sensitive actions require a passkey sign-in within five minutes. The session locks after five minutes without interaction.',
      ),
    ),
    Wrap(
      spacing: 12,
      runSpacing: 12,
      children: [
        PrismeButton(
          label: t('Codes de secours', 'Recovery codes'),
          onPressed: c.busy || !c.connected
              ? null
              : () async {
                  if (!await confirm(
                    t('Récupération du compte', 'Account recovery'),
                    t(
                      'Remplacer tous les anciens codes de secours.',
                      'Replace all existing recovery codes.',
                    ),
                    t('Générer de nouveaux codes.', 'Generate new codes.'),
                  )) {
                    return;
                  }
                  final codes = await c.securityAction(c.api.recoveryCodes);
                  if (codes != null && mounted) {
                    await secretDialog(
                      t('Codes à usage unique', 'Single-use codes'),
                      codes.codes.join('\n'),
                      t(
                        'Conservez ces codes hors de cet appareil. Ils ne seront plus affichés.',
                        'Store these codes outside this device. They will not be shown again.',
                      ),
                    );
                  }
                },
        ),
        PrismeButton(
          label: t('Ajouter une passkey', 'Add a passkey'),
          onPressed: c.busy || !c.connected
              ? null
              : () async {
                  final ticket = await c.securityAction(
                    c.api.registrationTicket,
                  );
                  if (ticket != null && mounted) {
                    await secretDialog(
                      t('Ticket d’ajout', 'Registration ticket'),
                      ticket.ticket,
                      t(
                        'Copiez ce ticket, déconnectez-vous puis ouvrez « Créer ou récupérer » dans la connexion passkey. Valable cinq minutes.',
                        'Copy this ticket, sign out, then open “Set up or recover” during passkey sign-in. Valid for five minutes.',
                      ),
                    );
                  }
                },
        ),
      ],
    ),
    if (c.devices == null && c.busy) const LinearProgressIndicator(),
    for (final device in c.devices?.items ?? <DeviceRecord>[])
      PrismeSurface(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            PrismeSectionHeader(
              title: device.name,
              subtitle: device.current
                  ? t('Cet appareil', 'This device')
                  : device.id,
            ),
            Text(
              device.revoked
                  ? t('Révoqué', 'Revoked')
                  : t('Autorisé', 'Authorized'),
            ),
            PrismeButton(
              label: t('Révoquer', 'Revoke'),
              onPressed: device.revoked || c.busy || !c.connected
                  ? null
                  : () async {
                      if (await confirm(
                        device.name,
                        t(
                          'Fermer toutes les sessions de cet appareil.',
                          'End every session on this device.',
                        ),
                        t(
                          'Une nouvelle identité d’appareil sera nécessaire.',
                          'A new device identity will be required.',
                        ),
                      )) {
                        await c.revokeDevice(device.id);
                      }
                    },
            ),
          ],
        ),
      ),
  ];
  List<Widget> auditWidgets() => [
    if (c.audit == null && c.busy) const LinearProgressIndicator(),
    for (final event in c.audit?.items ?? <AuditEvent>[])
      PrismeSurface(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            PrismeSectionHeader(
              title: event.action,
              subtitle: event.occurred_at,
            ),
            PrismeKeyValue(label: t('Acteur', 'Actor'), value: event.actor),
            const SizedBox(height: 8),
            PrismeKeyValue(label: t('Cible', 'Target'), value: event.target),
          ],
        ),
      ),
    if (c.audit?.next_cursor.isNotEmpty ?? false)
      PrismeButton(
        label: t('Page suivante', 'Next page'),
        onPressed: c.busy ? null : () => c.loadAudit(nextPage: true),
      ),
  ];
}
