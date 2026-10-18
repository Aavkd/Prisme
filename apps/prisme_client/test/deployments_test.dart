import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:prisme_ui/prisme_ui.dart';
import 'package:prisme_client/api/generated.dart';
import 'package:prisme_client/foundation/controller.dart';
import 'package:prisme_client/deployments/page.dart';

void main() {
  final manifest = <String, dynamic>{
    'id': 'pilot-test',
    'project': 'Pilot',
    'environment': 'test',
    'server': 'Local fixture',
    'repository_id': '11',
    'strategy': 'blue-green',
    'migration': 'none',
    'backup_required': false,
    'caddy_id': 'prisme-pilot',
    'observation_seconds': 3,
    'timeout_seconds': 180,
    'services': [
      {
        'name': 'web',
        'docker_health': false,
        'health_path': '/',
        'env_file': '',
      },
    ],
  };
  Map<String, dynamic> job(String state) => {
    'id': 'job-1',
    'target_id': 'pilot-test',
    'state': state,
    'revision': 1,
    'step': 0,
    'created_at': '2026-09-08T12:00:00Z',
    'expires_at': DateTime.now()
        .add(const Duration(minutes: 15))
        .toUtc()
        .toIso8601String(),
    'heartbeat_at': null,
    'approved_by': '',
    'error_code': '',
    'plan': {
      'manifest': manifest,
      'build': {'branch': 'main', 'author': 'fixture'},
      'release': {
        'id': 'release-1',
        'manifest': {'commit': 'a' * 40, 'run_id': '100'},
      },
      'previous_release': 'release-0',
      'previous_images': {'web': 'old@sha256:${'b' * 64}'},
      'images': {'web': 'new@sha256:${'c' * 64}'},
      'rollback': false,
      'blockers': [],
      'steps': [
        'preconditions',
        'configuration',
        'pull',
        'start',
        'health',
        'traffic',
        'observation',
        'promote',
      ],
    },
  };
  for (final width in [390.0, 1440.0]) {
    testWidgets('deployment approval and session lock at $width', (
      tester,
    ) async {
      tester.view.physicalSize = Size(width, 1000);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      var state = 'awaiting_approval';
      var mutations = 0;
      final api = PrismeApi((method, path, {body, idempotencyKey}) async {
        final uri = Uri.parse(path);
        if (method == 'POST') {
          mutations++;
          expect(body?['phrase'], 'pilot-test');
          expect(body?['revision'], 1);
          state = 'ready';
          return {'job': job(state)};
        }
        if (uri.path == '/v1/github') {
          return {'enabled': false, 'repositories': []};
        }
        if (uri.path == '/v1/deployments') {
          return {
            'enabled': true,
            'targets': [
              {'manifest': manifest, 'blockers': []},
            ],
            'items': [job(state)],
            'has_more': false,
          };
        }
        return {'job': job(state), 'events': []};
      });
      final controller = FoundationController(
        api,
        setToken: (_) {},
        close: () {},
      );
      controller.identity = const Identity(
        user_id: 'fixture',
        auth_mode: 'development',
        device_name: 'Windows',
        expires_at: '2099-01-01T00:00:00Z',
      );
      final prefs = PrismePreferences(locale: const Locale('en'));
      await tester.pumpWidget(
        PrismeApp(
          preferences: prefs,
          home: Scaffold(body: DeploymentsPage(controller: controller)),
        ),
      );
      await tester.pumpAndSettle();
      expect(tester.takeException(), isNull);
      await tester.ensureVisible(find.text('pilot-test · Awaiting approval'));
      await tester.tap(find.text('pilot-test · Awaiting approval'));
      await tester.pumpAndSettle();
      for (
        var i = 0;
        i < 15 && find.text('Approve').hitTestable().evaluate().isEmpty;
        i++
      ) {
        await tester.drag(find.byType(ListView).first, const Offset(0, -350));
        await tester.pumpAndSettle();
      }
      expect(find.text('Approve').hitTestable(), findsOneWidget);
      expect(mutations, 0);
      await tester.tap(find.text('Approve'));
      await tester.pumpAndSettle();
      expect(find.text('Approve this deployment'), findsOneWidget);
      expect(mutations, 0);
      await tester.enterText(find.byType(TextField).last, 'pilot-test');
      await tester.pumpAndSettle();
      await tester.ensureVisible(find.text('Approve').last);
      await tester.tap(find.text('Approve').last);
      await tester.pumpAndSettle();
      expect(mutations, 1);
      expect(find.text('Ready · waiting for worker'), findsOneWidget);
      expect(tester.takeException(), isNull);
      controller.lock();
      await tester.pumpAndSettle();
      expect(find.text('Local fixture'), findsNothing);
      await tester.pumpWidget(const SizedBox.shrink());
      controller.dispose();
      prefs.dispose();
    });
  }
  testWidgets('uncertain result has no approve or cancel action', (
    tester,
  ) async {
    final j = job('uncertain');
    final api = PrismeApi((method, path, {body, idempotencyKey}) async {
      if (path.startsWith('/v1/github')) {
        return {'enabled': false, 'repositories': []};
      }
      if (Uri.parse(path).path == '/v1/deployments') {
        return {
          'enabled': true,
          'targets': [
            {'manifest': manifest, 'blockers': []},
          ],
          'items': [j],
          'has_more': false,
        };
      }
      return {'job': j, 'events': []};
    });
    final controller = FoundationController(
      api,
      setToken: (_) {},
      close: () {},
    );
    controller.identity = const Identity(
      user_id: 'fixture',
      auth_mode: 'development',
      device_name: 'Windows',
      expires_at: '2099-01-01T00:00:00Z',
    );
    final prefs = PrismePreferences(locale: const Locale('en'));
    await tester.pumpWidget(
      PrismeApp(
        preferences: prefs,
        home: Scaffold(body: DeploymentsPage(controller: controller)),
      ),
    );
    await tester.pumpAndSettle();
    await tester.ensureVisible(
      find.text('pilot-test · Uncertain result · target reserved'),
    );
    await tester.tap(
      find.text('pilot-test · Uncertain result · target reserved'),
    );
    await tester.pumpAndSettle();
    expect(find.text('Approve'), findsNothing);
    expect(find.text('Cancel job'), findsNothing);
    await tester.pumpWidget(const SizedBox.shrink());
    controller.dispose();
    prefs.dispose();
  });
}
