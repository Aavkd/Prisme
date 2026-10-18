import 'dart:convert';
import 'dart:math';
import 'package:flutter/material.dart';
import 'package:prisme_ui/prisme_ui.dart';
import 'api/connection.dart';
import 'foundation/controller.dart';
import 'foundation/page.dart';
import 'mobile/services.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:url_launcher/url_launcher.dart';

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  const development = bool.fromEnvironment('PRISME_DEVELOPMENT');
  const identity = bool.fromEnvironment('PRISME_IDENTITY');
  if (!development && !identity) {
    runApp(
      const MaterialApp(
        home: Scaffold(
          body: Center(
            child: Text(
              'Prisme: launch with tool/run-client.ps1 or configure PRISME_IDENTITY and PRISME_API_URL.',
            ),
          ),
        ),
      ),
    );
    return;
  }
  const ca = String.fromEnvironment('PRISME_CA_BASE64');
  final storage = await SharedPreferences.getInstance();
  var deviceId = storage.getString('device_id');
  if (deviceId == null) {
    final random = Random.secure();
    deviceId = List.generate(
      32,
      (_) => random.nextInt(256).toRadixString(16).padLeft(2, '0'),
    ).join();
    await storage.setString('device_id', deviceId);
  }
  const origin = String.fromEnvironment(
    'PRISME_API_URL',
    defaultValue: 'https://localhost:8443',
  );
  final mobile = MobileServices(origin, deviceId);
  final cache = OfflineCache(mobile.scope);
  try {
    await cache.initialize();
  } catch (_) {
    cache.error = 'cache_unavailable';
  }
  final connection = ApiConnection(
    origin,
    trustedCa: ca.isEmpty ? null : base64Decode(ca),
  );
  connection.onRead = cache.record;
  final controller = FoundationController(
    connection.api,
    setToken: (v) => connection.token = v,
    close: connection.close,
    deviceId: deviceId,
    development: development,
    openBrowser: (uri) => launchUrl(uri, mode: LaunchMode.externalApplication),
    mobile: mobile,
    cache: cache,
    openTerminalSocket: connection.openTerminalSocket,
  );
  try {
    await mobile.listen(controller.receiveNotification);
  } catch (_) {}
  runApp(PrismeClientApp(controller: controller, storage: storage));
}

class PrismeClientApp extends StatefulWidget {
  const PrismeClientApp({super.key, required this.controller, this.storage});
  final FoundationController controller;
  final SharedPreferences? storage;
  @override
  State<PrismeClientApp> createState() => _PrismeClientAppState();
}

class _PrismeClientAppState extends State<PrismeClientApp> {
  final preferences = PrismePreferences(
    appearance: PrismeAppearance.dark,
    locale: const Locale('fr'),
  );
  @override
  void initState() {
    super.initState();
    final raw = widget.storage?.getString('appearance');
    if (raw != null) {
      for (final value in PrismeAppearance.values) {
        if (value.name == raw) preferences.appearance = value;
      }
    }
    final locale = widget.storage?.getString('locale');
    if (locale != null) preferences.locale = Locale(locale);
    preferences.addListener(_save);
  }

  void _save() {
    widget.storage?.setString('appearance', preferences.appearance.name);
    widget.storage?.setString(
      'locale',
      preferences.locale?.languageCode ?? 'fr',
    );
  }

  @override
  void dispose() {
    widget.controller.dispose();
    preferences.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => PrismeApp(
    preferences: preferences,
    home: FoundationPage(
      controller: widget.controller,
      preferences: preferences,
    ),
  );
}
