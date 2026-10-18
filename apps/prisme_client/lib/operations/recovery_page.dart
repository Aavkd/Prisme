import 'dart:async';
import 'dart:convert';
import 'dart:io';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:prisme_ui/prisme_ui.dart';
import '../api/connection.dart';
import '../api/generated.dart';
import '../mobile/services.dart';
import 'page.dart';

class RecoveryPage extends StatefulWidget {
  const RecoveryPage({
    super.key,
    required this.deviceId,
    required this.openBrowser,
    this.normalSend,
    this.platformActivity,
  });
  final String deviceId;
  final Future<bool> Function(Uri) openBrowser;
  final ApiTransport? normalSend;
  final void Function(bool)? platformActivity;
  @override
  State<RecoveryPage> createState() => _RecoveryPageState();
}

class _RecoveryPageState extends State<RecoveryPage>
    with WidgetsBindingObserver {
  final origin = TextEditingController(
    text: const String.fromEnvironment('PRISME_RECOVERY_URL'),
  );
  ApiConnection? connection;
  MobileServices? mobile;
  bool busy = false, authenticating = false;
  String? error;
  Timer? expiry;
  int generation = 0;
  String t(String fr, String en) => context.prismeStrings.choose(fr, en);
  void setAuthenticating(bool value) {
    authenticating = value;
    widget.platformActivity?.call(value);
  }

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
  }

  @override
  void dispose() {
    generation++;
    expiry?.cancel();
    connection?.close();
    origin.dispose();
    WidgetsBinding.instance.removeObserver(this);
    super.dispose();
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    if (state == AppLifecycleState.paused && !authenticating) {
      lock();
    }
  }

  void lock() {
    generation++;
    expiry?.cancel();
    connection?.close();
    connection = null;
    mobile = null;
    if (mounted) setState(() {});
  }

  Future<void> login() async {
    if (busy) return;
    setState(() {
      busy = true;
      error = null;
    });
    setAuthenticating(true);
    final ticket = ++generation;
    ApiConnection? candidate;
    try {
      const ca = String.fromEnvironment('PRISME_RECOVERY_CA_BASE64');
      candidate = ApiConnection(
        origin.text.trim(),
        trustedCa: ca.isEmpty ? null : base64Decode(ca),
      );
      final flow = await candidate.send(
        'POST',
        '/v1/auth/browser',
        body: {
          'device_id': widget.deviceId,
          'device_name': 'Prisme recovery ${Platform.operatingSystem}',
        },
      );
      if (!await widget.openBrowser(Uri.parse(flow['browser_url'] as String))) {
        throw const ApiFailure('browser_unavailable');
      }
      final end = DateTime.parse(flow['expires_at'] as String);
      while (mounted && ticket == generation && DateTime.now().isBefore(end)) {
        await Future<void>.delayed(const Duration(seconds: 2));
        if (!mounted || ticket != generation) break;
        final session = await candidate.send(
          'POST',
          '/v1/auth/poll',
          body: {'flow_id': flow['flow_id'], 'poll_token': flow['poll_token']},
        );
        if (session['status'] != 'approved') continue;
        candidate.token = session['access_token'] as String;
        connection = candidate;
        mobile = MobileServices(origin.text.trim(), widget.deviceId);
        expiry = Timer(
          DateTime.parse(
            session['expires_at'] as String,
          ).difference(DateTime.now()),
          lock,
        );
        return;
      }
      throw const ApiFailure('session_expired');
    } catch (_) {
      candidate?.close();
      if (mounted) {
        setState(
          () => error = t(
            'Secours indisponible ou authentification refusée. Vérifiez le canal indépendant et l’enrôlement de cet appareil.',
            'Recovery unavailable or authentication denied. Check the independent channel and this device’s enrollment.',
          ),
        );
      }
    } finally {
      setAuthenticating(false);
      if (mounted) setState(() => busy = false);
    }
  }

  Future<String> prove(Map<String, dynamic> intent) async {
    final current = connection;
    if (current == null || mobile == null) {
      throw const ApiFailure('session_expired');
    }
    final ticket = generation;
    setAuthenticating(true);
    try {
      return await mobile!.proveOperation(
        current.api,
        intent,
        widget.openBrowser,
        () => mounted && ticket == generation && connection?.token != null,
        'Prisme · Secours / Recovery',
        passkeyOnly: false,
      );
    } finally {
      setAuthenticating(false);
    }
  }

  Future<void> synchronize() async {
    final current = connection;
    if (current == null || widget.normalSend == null) return;
    setState(() => busy = true);
    try {
      var after = 0;
      while (mounted) {
        final batch = await current.send(
          'GET',
          '/v1/recovery/audit?after=$after',
        );
        final result = await widget.normalSend!(
          'POST',
          '/v1/recovery/import',
          body: batch,
        );
        final data = batch['batch'] as Map;
        after = result['cursor'] as int;
        if ((data['events'] as List).length < 200) break;
      }
      if (mounted) {
        setState(
          () => error = t(
            'Audit importé. Les automatismes restent suspendus jusqu’à la réconciliation sur le VPS.',
            'Audit imported. Automation remains suspended until reconciliation on the VPS.',
          ),
        );
      }
    } catch (_) {
      if (mounted) {
        setState(
          () => error = t(
            'Import impossible. Une session normale connectée à la tour est nécessaire. L’audit reste conservé sur le VPS.',
            'Import unavailable. A normal session connected to the control plane is required. Audit remains on the VPS.',
          ),
        );
      }
    } finally {
      if (mounted) setState(() => busy = false);
    }
  }

  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(
      title: Text(
        t('Accès indépendant de secours', 'Independent recovery access'),
      ),
      actions: [
        if (connection != null)
          IconButton(
            tooltip: t('Verrouiller', 'Lock'),
            onPressed: lock,
            icon: const Icon(LucideIcons.lock),
          ),
      ],
    ),
    body: SafeArea(
      child: connection == null
          ? PrismePageFrame(
              header: PrismeSectionHeader(
                title: t(
                  'Se connecter au VPS en secours',
                  'Connect to VPS recovery',
                ),
              ),
              children: [
                PrismeBanner(
                  tone: PrismeTone.attention,
                  title: t(
                    'Authentification indépendante',
                    'Independent authentication',
                  ),
                  message: t(
                    'Le canal et cet appareil doivent être préparés sur le VPS avant une panne de la tour. Une panne Cloudflare peut aussi empêcher cet accès.',
                    'The channel and this device must be prepared on the VPS before a control plane outage. A Cloudflare outage may also prevent access.',
                  ),
                ),
                PrismeTextField(
                  label: t('Origine HTTPS du secours', 'Recovery HTTPS origin'),
                  controller: origin,
                  enabled: !busy,
                ),
                PrismeKeyValue(
                  label: t(
                    'Identifiant d’appareil à enrôler',
                    'Device ID to enroll',
                  ),
                  value: widget.deviceId,
                ),
                PrismeButton(
                  label: t('Copier l’identifiant d’appareil', 'Copy device ID'),
                  onPressed: () =>
                      Clipboard.setData(ClipboardData(text: widget.deviceId)),
                ),
                if (error != null) Text(error!),
                PrismeButton(
                  label: t(
                    'Se connecter avec une passkey de secours',
                    'Sign in with a recovery passkey',
                  ),
                  loading: busy,
                  onPressed: busy ? null : login,
                ),
              ],
            )
          : Column(
              children: [
                if (Platform.isAndroid)
                  PrismeButton(
                    label: t(
                      'Enregistrer la clé biométrique de secours',
                      'Enroll recovery biometric key',
                    ),
                    onPressed: busy
                        ? null
                        : () async {
                            setState(() => busy = true);
                            setAuthenticating(true);
                            try {
                              await mobile!.enrollKey(connection!.api);
                            } catch (_) {
                              if (mounted) {
                                setState(
                                  () => error = t(
                                    'Reconnectez-vous avec la passkey de secours avant d’enregistrer cette clé.',
                                    'Sign in again with the recovery passkey before enrolling this key.',
                                  ),
                                );
                              }
                            } finally {
                              setAuthenticating(false);
                              if (mounted) setState(() => busy = false);
                            }
                          },
                  ),
                if (error != null)
                  Padding(
                    padding: const EdgeInsets.all(16),
                    child: Text(error!),
                  ),
                if (widget.normalSend != null)
                  PrismeButton(
                    label: t(
                      'Importer l’audit vers la tour',
                      'Import audit to control plane',
                    ),
                    onPressed: busy ? null : synchronize,
                  ),
                Expanded(
                  child: OperationsPage(
                    key: ValueKey(generation),
                    send: connection!.send,
                    prove: prove,
                    openSocket: connection!.openTerminalSocket,
                    recovery: true,
                    platformActivity: setAuthenticating,
                  ),
                ),
              ],
            ),
    ),
  );
}
