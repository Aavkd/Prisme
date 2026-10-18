import 'dart:async';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:prisme_ui/prisme_ui.dart';
import 'package:prisme_client/api/connection.dart';
import 'package:prisme_client/operations/controller.dart';
import 'package:prisme_client/operations/page.dart';

void main() {
  Map<String, dynamic> overview() => {
    'origin': 'normal',
    'observed_at': '2026-09-09T20:00:00Z',
    'vault': true,
    'targets': [
      {
        'id': 'local',
        'name': 'Fixture host',
        'user': 'operator',
        'environment': 'production',
        'roots': ['/srv/project'],
        'terminal': true,
        'files': true,
        'diagnostics': true,
        'containers': <String>[],
      },
    ],
    'sessions': <Map<String, dynamic>>[],
    'secrets': [
      {
        'id': 'a' * 64,
        'name': 'REGISTRY_TOKEN',
        'scope': 'pilot',
        'destination': 'local',
        'revision': 1,
        'expires_at': '2026-12-01T00:00:00Z',
      },
    ],
  };
  for (final width in [390.0, 1440.0]) {
    testWidgets('operations paths, masked secrets and denied proof at $width', (
      tester,
    ) async {
      tester.view.physicalSize = Size(width, 1100);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      final preferences = PrismePreferences(locale: const Locale('en'));
      var executions = 0;
      Future<Map<String, dynamic>> send(
        String method,
        String path, {
        Map<String, dynamic>? body,
        String? idempotencyKey,
      }) async {
        if (path.endsWith('/overview')) return overview();
        executions++;
        final intent = body!['intent'] as Map;
        if (intent['action'] == 'file_list') {
          return {
            'entries': [
              {
                'name': 'config.txt',
                'directory': false,
                'mode': '-rw-r--r--',
                'uid': 1000,
                'gid': 1000,
                'size': 32,
              },
            ],
          };
        }
        throw const ApiFailure('unexpected_mutation');
      }

      await tester.pumpWidget(
        PrismeApp(
          preferences: preferences,
          home: Scaffold(
            body: OperationsPage(
              send: send,
              prove: (_) async => throw const ApiFailure('operation_denied'),
            ),
          ),
        ),
      );
      await tester.pumpAndSettle();
      expect(find.textContaining('Fixture host'), findsOneWidget);
      await tester.tap(find.text('Files'));
      await tester.pumpAndSettle();
      await tester.tap(find.text('Browse'));
      await tester.pumpAndSettle();
      expect(find.text('config.txt'), findsOneWidget);
      expect(executions, 1);
      await tester.tap(find.text('config.txt'));
      await tester.pumpAndSettle();
      expect(find.textContaining('Access denied.'), findsOneWidget);
      expect(executions, 1);
      await tester.tap(find.text('Secrets').first);
      await tester.pumpAndSettle();
      expect(find.textContaining('REGISTRY_TOKEN'), findsOneWidget);
      await tester.ensureVisible(find.text('Reveal'));
      await tester.tap(find.text('Reveal'));
      await tester.pumpAndSettle();
      expect(executions, 1);
      expect(find.textContaining('Access denied.'), findsOneWidget);
      expect(tester.takeException(), isNull);
      await tester.pumpWidget(const SizedBox());
      preferences.dispose();
    });
  }
  testWidgets('recovery remains explicit and never displays central vault', (
    tester,
  ) async {
    final p = PrismePreferences(locale: const Locale('en'));
    await tester.pumpWidget(
      PrismeApp(
        preferences: p,
        home: Scaffold(
          body: OperationsPage(
            recovery: true,
            send: (method, path, {body, idempotencyKey}) async => {
              ...overview(),
              'vault': false,
              'secrets': <Map<String, dynamic>>[],
            },
            prove: (_) async => 'proof',
          ),
        ),
      ),
    );
    await tester.pumpAndSettle();
    expect(find.text('Recovery mode · independent identity'), findsOneWidget);
    expect(find.text('Secrets'), findsNothing);
    expect(tester.takeException(), isNull);
    await tester.pumpWidget(const SizedBox());
    p.dispose();
  });
  test('late responses cannot revive disposed operations', () async {
    final reply = Completer<Map<String, dynamic>>();
    final c = OperationsController(
      (method, path, {body, idempotencyKey}) => reply.future,
      (_) async => 'proof',
      null,
    );
    final pending = c.refresh();
    c.dispose();
    reply.complete(overview());
    await pending;
    expect(c.overview, isNull);
  });
  test('secret material is absent from multiline diffs', () {
    expect(
      redactDiff(
        'ordinary\n-----BEGIN PRIVATE KEY-----\nprivate-body\n-----END PRIVATE KEY-----',
      ),
      equals('[REDACTED]'),
    );
    expect(redactDiff('TOKEN=private-value'), equals('[REDACTED]'));
  });
}
