import 'dart:convert';
import 'dart:io';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:integration_test/integration_test.dart';
import 'package:prisme_ui/prisme_ui.dart';
import 'package:prisme_client/api/connection.dart';
import 'package:prisme_client/api/generated.dart';
import 'package:prisme_client/foundation/controller.dart';
import 'package:prisme_client/mobile/services.dart';
import 'package:prisme_client/mobile/page.dart';

void main() {
  IntegrationTestWidgetsFlutterBinding.ensureInitialized();
  testWidgets(
    'real FCM, incident route, native key signature and encrypted cache',
    (tester) async {
      expect(Platform.isAndroid, isTrue);
      const dir = '/data/data/dev.mantara.prisme_client/files/phase5';
      await Directory(dir).create(recursive: true);
      final input = File('$dir/input.json');
      if (await input.exists()) await input.delete();
      final completed = File('$dir/complete');
      if (await completed.exists()) await completed.delete();
      await File('$dir/ready').writeAsString('ready');
      await tester.pumpWidget(
        const MaterialApp(
          home: Scaffold(
            body: Center(child: Text('Prisme · préparation du test local')),
          ),
        ),
      );
      for (var i = 0; i < 300 && !await input.exists(); i++) {
        await tester.pump(const Duration(seconds: 1));
      }
      final config =
          jsonDecode(await input.readAsString()) as Map<String, dynamic>;
      await input.delete();
      final connection = ApiConnection(
        'https://localhost:18443',
        trustedCa: base64Decode(config['ca'] as String),
      );
      connection.token = config['token'] as String;
      final mobile = MobileServices(
        'https://localhost:18443',
        config['device'] as String,
      );
      final controller = FoundationController(
        connection.api,
        setToken: (v) => connection.token = v,
        close: connection.close,
        deviceId: config['device'] as String,
        mobile: mobile,
      );
      controller.identity = await connection.api.me();
      controller.servers = await connection.api.servers();
      final preferences = PrismePreferences(locale: const Locale('fr'));
      String? route;
      await mobile.listen((v) {
        route = v;
      });
      await tester.pumpWidget(
        PrismeApp(
          preferences: preferences,
          home: Scaffold(
            body: Center(
              child: Text(
                'Prisme · autorisez les notifications puis ouvrez l’alerte de test reçue.',
              ),
            ),
          ),
        ),
      );
      final token = await mobile.enableNotifications();
      await connection.api.saveNotificationSettings(
        NotificationSettingsRequest(
          token: token,
          enabled: true,
          timezone: 'Europe/Paris',
          quiet_start: 0,
          quiet_end: 0,
        ),
      );
      await mobile.enrollKey(connection.api);
      for (var i = 0; i < 300 && route == null; i++) {
        await tester.pump(const Duration(seconds: 1));
      }
      expect(route, matches(RegExp(r'^/incidents/[a-f0-9]{64}$')));
      await tester.pumpWidget(
        PrismeApp(
          preferences: preferences,
          home: Scaffold(
            body: MobilePage(controller: controller, route: route),
          ),
        ),
      );
      for (
        var i = 0;
        i < 50 &&
            find
                .text('S24 · panne HTTP locale de qualification')
                .evaluate()
                .isEmpty;
        i++
      ) {
        await tester.pump(const Duration(milliseconds: 200));
      }
      expect(
        find.text('S24 · panne HTTP locale de qualification'),
        findsOneWidget,
      );
      expect(find.text('Acquitter'), findsOneWidget);
      final cache = OfflineCache(
        '${mobile.scope}_qualification',
        unlock: () async => true,
      );
      await cache.initialize();
      await cache.setEnabled(true);
      await cache.record(
        '/v1/incidents',
        (await connection.api.incidents()).toJson(),
      );
      final restored = OfflineCache(
        '${mobile.scope}_qualification',
        unlock: () async => true,
      );
      await restored.initialize();
      expect((await restored.read())?['/v1/incidents'], isNotNull);
      await restored.clear();
      final job = DeploymentJob.fromJson(config['job'] as Map<String, dynamic>);
      final proof = await mobile.prove(
        connection.api,
        job,
        null,
        () => true,
        'Prisme · approuver le test local',
      );
      await connection.api.decideDeployment(
        DeploymentDecision(
          action: 'approve',
          revision: job.revision,
          phrase: job.target_id,
          proof: proof,
        ),
        job: job.id,
      );
      await connection.api.disableNotifications();
      await mobile.disableNotifications();
      await completed.writeAsString('verified');
      await tester.pumpWidget(const SizedBox.shrink());
      controller.dispose();
      preferences.dispose();
    },
    timeout: const Timeout(Duration(minutes: 10)),
  );
}
