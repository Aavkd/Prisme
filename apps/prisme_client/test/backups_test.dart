import 'dart:async';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:prisme_ui/prisme_ui.dart';
import 'package:prisme_client/api/generated.dart';
import 'package:prisme_client/foundation/controller.dart';
import 'package:prisme_client/backups/page.dart';

Map<String, dynamic> overview({bool enabled = true}) => {
  'enabled': enabled,
  'policies': [
    {
      'id': 'pilot',
      'project': 'Pilot',
      'environment': 'test',
      'server': 'source',
      'kind': 'postgres',
      'consistency': 'logical',
      'repository': 'local',
      'storage_host': 'source',
      'offsite_verified': false,
      'enabled': true,
      'last_success_at': '2026-09-10T12:00:00Z',
      'interval_seconds': 86400,
      'restore_test_seconds': 604800,
      'retention': {'keep_last': 2, 'keep_daily': 7, 'keep_weekly': 4},
    },
  ],
  'targets': [
    {'id': 'test-target', 'policy': 'pilot', 'server': 'test-host'},
  ],
  'jobs': [],
  'artifacts': [
    {
      'id': 'a' * 64,
      'policy_id': 'pilot',
      'job_id': 'b' * 64,
      'snapshot_id': 'c' * 64,
      'bytes': 4096,
      'created_at': '2026-09-10T12:00:00Z',
      'verified_at': '2026-09-10T12:00:10Z',
      'expired_at': null,
    },
  ],
  'has_more': false,
};

FoundationController controller(ApiTransport transport) {
  final c = FoundationController(
    PrismeApi(transport),
    setToken: (_) {},
    close: () {},
  );
  c.identity = const Identity(
    user_id: 'fixture',
    auth_mode: 'development',
    device_name: 'Windows',
    expires_at: '2099-01-01T00:00:00Z',
  );
  return c;
}

void main() {
  for (final width in [390.0, 1440.0]) {
    testWidgets(
      'backup plan preserves source and requires fresh proof at $width',
      (tester) async {
        tester.view.physicalSize = Size(width, 1000);
        tester.view.devicePixelRatio = 1;
        addTearDown(tester.view.resetPhysicalSize);
        addTearDown(tester.view.resetDevicePixelRatio);
        var creates = 0, decisions = 0;
        final job = {
          'id': 'd' * 64,
          'policy_id': 'pilot',
          'kind': 'restore',
          'artifact_id': 'a' * 64,
          'target_id': 'test-target',
          'state': 'planned',
          'step': 'queued',
          'revision': 1,
          'scheduled': false,
          'created_at': '2026-09-10T12:00:00Z',
          'expires_at': DateTime.now()
              .add(const Duration(minutes: 15))
              .toUtc()
              .toIso8601String(),
          'started_at': null,
          'finished_at': null,
          'error_code': '',
          'evidence': <String, dynamic>{},
        };
        final c = controller((method, path, {body, idempotencyKey}) async {
          if (method == 'POST' && path == '/v1/backups/jobs') {
            creates++;
            expect(body?['target'], 'test-target');
            expect(body?['artifact'], 'a' * 64);
            expect(idempotencyKey, isNotEmpty);
            return {'job': job, 'confirmation': 'RESTORE test-target'};
          }
          if (method == 'POST') decisions++;
          if (path.startsWith('/v1/backups/jobs/')) {
            return {
              'job': job,
              'confirmation': 'RESTORE test-target',
              'events': [],
              'plan': <String, dynamic>{},
            };
          }
          return overview();
        });
        final prefs = PrismePreferences(locale: const Locale('en'));
        await tester.pumpWidget(
          PrismeApp(
            preferences: prefs,
            home: Scaffold(body: BackupsPage(controller: c)),
          ),
        );
        await tester.pumpAndSettle();
        expect(find.text('Off-site protection incomplete'), findsOneWidget);
        expect(tester.takeException(), isNull);
        for (
          var i = 0;
          i < 20 && find.text('Plan restore').hitTestable().evaluate().isEmpty;
          i++
        ) {
          await tester.drag(find.byType(ListView).first, const Offset(0, -300));
          await tester.pumpAndSettle();
        }
        await tester.tap(find.text('Plan restore'));
        await tester.pumpAndSettle();
        expect(creates, 1);
        expect(decisions, 0);
        for (
          var i = 0;
          i < 20 && find.text('Approve').hitTestable().evaluate().isEmpty;
          i++
        ) {
          await tester.drag(find.byType(ListView).first, const Offset(0, -300));
          await tester.pumpAndSettle();
        }
        await tester.tap(find.text('Approve'));
        await tester.pumpAndSettle();
        expect(find.textContaining('Create a new database'), findsOneWidget);
        await tester.enterText(
          find.byType(TextField).last,
          'RESTORE test-target',
        );
        await tester.pumpAndSettle();
        await tester.tap(find.text('Approve').last);
        await tester.pumpAndSettle();
        expect(
          decisions,
          0,
          reason: 'development session cannot produce a passkey proof',
        );
        expect(c.error, 'passkey_session_required');
        for (
          var i = 0;
          i < 20 && find.text('Refresh required').evaluate().isEmpty;
          i++
        ) {
          await tester.drag(find.byType(ListView).first, const Offset(0, 300));
          await tester.pumpAndSettle();
        }
        expect(find.text('Refresh required'), findsOneWidget);
        c.lock();
        await tester.pumpAndSettle();
        expect(find.text('RESTORE test-target'), findsNothing);
        expect(find.text('c' * 64), findsNothing);
        expect(tester.takeException(), isNull);
        await tester.pumpWidget(const SizedBox.shrink());
        c.dispose();
        prefs.dispose();
      },
    );
  }
  testWidgets('late response after lock does not repopulate backup state', (
    tester,
  ) async {
    final result = Completer<Map<String, dynamic>>();
    final c = controller(
      (method, path, {body, idempotencyKey}) => result.future,
    );
    final prefs = PrismePreferences(locale: const Locale('en'));
    await tester.pumpWidget(
      PrismeApp(
        preferences: prefs,
        home: Scaffold(body: BackupsPage(controller: c)),
      ),
    );
    c.lock();
    result.complete(overview());
    await tester.pump();
    expect(find.text('Off-site protection incomplete'), findsNothing);
    expect(find.text('Plan restore'), findsNothing);
    await tester.pumpWidget(const SizedBox.shrink());
    c.dispose();
    prefs.dispose();
  });
}
