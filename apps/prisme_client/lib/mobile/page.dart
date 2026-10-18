import 'dart:io';
import 'package:flutter/material.dart';
import 'package:prisme_ui/prisme_ui.dart';
import '../api/generated.dart';
import '../foundation/controller.dart';

class MobilePage extends StatefulWidget {
  const MobilePage({super.key, required this.controller, this.route});
  final FoundationController controller;
  final String? route;
  @override
  State<MobilePage> createState() => _MobilePageState();
}

class _MobilePageState extends State<MobilePage> {
  FoundationController get c => widget.controller;
  final timezone = TextEditingController(text: 'Europe/Paris');
  TimeOfDay start = const TimeOfDay(hour: 22, minute: 0),
      end = const TimeOfDay(hour: 7, minute: 0);
  bool enabled = false,
      configured = false,
      permission = false,
      registered = false;
  bool loading = true, failed = false;
  String? actionError;
  IncidentDetail? detail;
  String t(String fr, String en) => context.prismeStrings.choose(fr, en);
  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) => refresh());
  }

  @override
  void dispose() {
    timezone.dispose();
    super.dispose();
  }

  Future<void> refresh() async {
    final result = await c.securityAction(() async {
      final settings = await c.api.send('GET', '/v1/notifications');
      final status = await c.mobile?.status();
      IncidentDetail? incident;
      if (widget.route?.startsWith('/incidents/') == true) {
        final id = widget.route!.split('/').last;
        incident = IncidentDetail.fromJson(
          await c.api.send('GET', '/v1/notifications/incidents/$id'),
        );
      }
      return (settings, status, incident);
    });
    if (!mounted || c.identity == null) return;
    setState(() {
      loading = false;
      failed = result == null;
      if (result != null) {
        final p = result.$1['preferences'] as Map;
        enabled = p['enabled'] == true;
        configured = result.$1['configured'] == true;
        registered = result.$1['action_key_registered'] == true;
        timezone.text = p['timezone'] as String;
        final s = p['quiet_start'] as int, e = p['quiet_end'] as int;
        start = TimeOfDay(hour: s ~/ 60, minute: s % 60);
        end = TimeOfDay(hour: e ~/ 60, minute: e % 60);
        permission = result.$2?['permission'] == true;
        detail = result.$3;
      }
    });
  }

  Future<void> save(bool value) async {
    setState(() => loading = true);
    await c.securityAction(() async {
      String? token;
      if (value) {
        c.platformAuthentication = true;
        try {
          token = await c.mobile?.enableNotifications();
        } finally {
          c.platformAuthentication = false;
        }
      }
      await c.api.send(
        value ? 'PUT' : 'DELETE',
        '/v1/notifications',
        body: value
            ? {
                'token': token,
                'enabled': true,
                'timezone': timezone.text.trim(),
                'quiet_start': start.hour * 60 + start.minute,
                'quiet_end': end.hour * 60 + end.minute,
              }
            : null,
      );
      if (!value) await c.mobile?.disableNotifications();
    });
    actionError = c.error;
    if (mounted && c.identity != null) await refresh();
  }

  Future<void> action(String action) async {
    final incident = detail?.incident;
    if (incident == null || failed || loading) return;
    await c.securityAction(
      () => c.api.incidentAction(
        IncidentAction(action: action, note: '', revision: incident.revision),
        id: incident.id,
      ),
    );
    actionError = c.error;
    if (mounted && c.identity != null) await refresh();
  }

  @override
  Widget build(BuildContext context) => PrismePageFrame(
    header: PrismeSectionHeader(
      title: t(
        'Notifications et accès mobile',
        'Notifications and mobile access',
      ),
      actions: [
        PrismeButton(
          label: t('Actualiser', 'Refresh'),
          onPressed: loading || c.busy ? null : refresh,
        ),
      ],
    ),
    children: [
      if (failed || c.error != null || actionError != null)
        PrismeBanner(
          title: t(
            'Connexion ou action indisponible',
            'Connection or action unavailable',
          ),
          message:
              '${actionError ?? c.error ?? ''} · ${t('Actualisez avant toute action.', 'Refresh before taking action.')}',
        ),
      if (widget.route == '/watchdog')
        PrismeBanner(
          title: t(
            'Signal du watchdog indépendant',
            'Independent watchdog signal',
          ),
          message: t(
            'Ce signal concerne la joignabilité du centre de contrôle. L’état des sites reste inconnu. Le cache est en lecture seule ; le canal de secours est à configurer.',
            'This signal concerns control-plane reachability. Site health remains unknown. The cache is read only; the recovery channel needs configuration.',
          ),
        ),
      if (detail != null)
        PrismeSurface(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                detail!.incident.title,
                style: Theme.of(context).textTheme.titleLarge,
              ),
              Text('${detail!.incident.severity} · ${detail!.incident.status}'),
              Text('${t('Vérifié le', 'Checked at')} ${detail!.observed_at}'),
              const SizedBox(height: 12),
              if (!['closed', 'resolved'].contains(detail!.incident.status))
                Wrap(
                  spacing: 12,
                  runSpacing: 12,
                  children: [
                    PrismeButton(
                      label: t('Acquitter', 'Acknowledge'),
                      onPressed:
                          loading ||
                              failed ||
                              c.busy ||
                              detail!.incident.status == 'acknowledged'
                          ? null
                          : () => action('acknowledge'),
                    ),
                    PrismeButton(
                      label: t('Commencer le diagnostic', 'Start diagnosis'),
                      onPressed:
                          loading ||
                              failed ||
                              c.busy ||
                              detail!.incident.status == 'diagnosing'
                          ? null
                          : () => action('diagnose'),
                    ),
                  ],
                ),
              for (final event in detail!.events)
                Padding(
                  padding: const EdgeInsets.only(top: 12),
                  child: Text('${event.at} · ${event.message}'),
                ),
            ],
          ),
        ),
      PrismeSurface(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Text(
              t('Cet appareil', 'This device'),
              style: Theme.of(context).textTheme.titleLarge,
            ),
            Text(
              '${t('Transport serveur configuré', 'Server transport configured')}: ${configured ? t('oui', 'yes') : t('non', 'no')}',
            ),
            Text(
              '${t('Permission Android', 'Android permission')}: ${permission ? t('accordée', 'granted') : t('non accordée', 'not granted')}',
            ),
            SwitchListTile(
              contentPadding: EdgeInsets.zero,
              title: Text(t('Notifications Android', 'Android notifications')),
              value: enabled,
              onChanged:
                  loading || c.busy || c.mobile == null || !Platform.isAndroid
                  ? null
                  : save,
            ),
            PrismeTextField(
              label: t('Fuseau horaire IANA', 'IANA time zone'),
              controller: timezone,
              enabled: !loading && !c.busy,
            ),
            const SizedBox(height: 12),
            Wrap(
              spacing: 12,
              runSpacing: 12,
              children: [
                PrismeButton(
                  label:
                      '${t('Silence dès', 'Quiet from')} ${start.format(context)}',
                  onPressed: loading || c.busy
                      ? null
                      : () async {
                          final time = await showTimePicker(
                            context: context,
                            initialTime: start,
                          );
                          if (time != null && mounted) {
                            setState(() => start = time);
                          }
                        },
                ),
                PrismeButton(
                  label: '${t('Jusqu’à', 'Until')} ${end.format(context)}',
                  onPressed: loading || c.busy
                      ? null
                      : () async {
                          final time = await showTimePicker(
                            context: context,
                            initialTime: end,
                          );
                          if (time != null && mounted) {
                            setState(() => end = time);
                          }
                        },
                ),
                PrismeButton(
                  label: t('Enregistrer les horaires', 'Save quiet hours'),
                  onPressed: !enabled || loading || c.busy
                      ? null
                      : () => save(true),
                ),
              ],
            ),
            const SizedBox(height: 12),
            Text(
              t(
                'Seules les règles explicitement urgentes contournent ces horaires. Android conserve le contrôle des permissions et du mode Ne pas déranger.',
                'Only explicitly urgent rules bypass these hours. Android still controls permission and Do Not Disturb.',
              ),
            ),
          ],
        ),
      ),
      PrismeSurface(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Text(
              t('Confirmer les actions sensibles', 'Confirm sensitive actions'),
              style: Theme.of(context).textTheme.titleLarge,
            ),
            Text(
              Platform.isAndroid
                  ? t(
                      'Une clé Android exige une biométrie forte pour chaque approbation. Connectez-vous avec une passkey depuis moins de cinq minutes pour l’enregistrer.',
                      'An Android key requires strong biometrics for each approval. Sign in with a passkey within five minutes to enroll it.',
                    )
                  : t(
                      'Windows demande une nouvelle vérification passkey pour chaque approbation de production.',
                      'Windows requires fresh passkey verification for each production approval.',
                    ),
            ),
            if (Platform.isAndroid)
              PrismeButton(
                label: registered
                    ? t(
                        'Réenregistrer la clé biométrique',
                        'Re-enroll biometric key',
                      )
                    : t(
                        'Enregistrer la clé biométrique',
                        'Enroll biometric key',
                      ),
                onPressed:
                    loading ||
                        c.busy ||
                        c.identity?.auth_mode != 'passkey' ||
                        c.mobile == null
                    ? null
                    : () async {
                        await c.securityAction(
                          () => c.mobile!.enrollKey(c.api),
                        );
                        actionError = c.error;
                        if (mounted && c.identity != null) await refresh();
                      },
              ),
            SwitchListTile(
              contentPadding: EdgeInsets.zero,
              title: Text(
                t(
                  'Cache chiffré en lecture seule',
                  'Encrypted read-only cache',
                ),
              ),
              value: c.cache?.enabled == true,
              onChanged: c.cache == null || c.busy
                  ? null
                  : (value) async {
                      await c.securityAction(() async {
                        await c.cache!.setEnabled(value);
                        if (value) {
                          await c.api.servers();
                          await c.api.incidents();
                          await c.api.deployments();
                        }
                      });
                      if (mounted) setState(() {});
                    },
            ),
            Text(
              t(
                'Déverrouillage local requis. Les sessions, secrets et logs détaillés ne sont pas conservés. Déconnexion et révocation constatée effacent le cache.',
                'Local unlock required. Sessions, secrets and detailed logs are excluded. Sign-out and detected revocation clear the cache.',
              ),
            ),
            if (c.cache?.error != null)
              Text(
                t(
                  'Le cache n’a pas pu être enregistré.',
                  'The cache could not be saved.',
                ),
              ),
          ],
        ),
      ),
    ],
  );
}
