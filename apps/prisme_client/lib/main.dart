import 'dart:convert';
import 'package:flutter/material.dart';
import 'package:prisme_ui/prisme_ui.dart';
import 'api/connection.dart';
import 'foundation/controller.dart';
import 'foundation/page.dart';

void main() {
  WidgetsFlutterBinding.ensureInitialized();
  const development = bool.fromEnvironment('PRISME_DEVELOPMENT');
  if (!development) {
    runApp(
      const MaterialApp(
        home: Scaffold(
          body: Center(
            child: Text('Prisme phase 0: launch with tool/run-client.ps1.'),
          ),
        ),
      ),
    );
    return;
  }
  const ca = String.fromEnvironment('PRISME_CA_BASE64');
  final connection = ApiConnection(
    const String.fromEnvironment(
      'PRISME_API_URL',
      defaultValue: 'https://localhost:8443',
    ),
    trustedCa: ca.isEmpty ? null : base64Decode(ca),
  );
  runApp(
    PrismeClientApp(
      controller: FoundationController(
        connection.api,
        setToken: (v) => connection.token = v,
        close: connection.close,
      ),
    ),
  );
}

class PrismeClientApp extends StatefulWidget {
  const PrismeClientApp({super.key, required this.controller});
  final FoundationController controller;
  @override
  State<PrismeClientApp> createState() => _PrismeClientAppState();
}

class _PrismeClientAppState extends State<PrismeClientApp> {
  final preferences = PrismePreferences(
    appearance: PrismeAppearance.dark,
    locale: const Locale('fr'),
  );
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
