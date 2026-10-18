import 'dart:async';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:prisme_ui/prisme_ui.dart';
import 'package:prisme_client/edge/page.dart';
import 'backups_test.dart' show controller;

Map<String, dynamic> overview({bool enabled = true, bool write = true}) => {
  'enabled': enabled,
  'targets': [
    {
      'id': 'pilot',
      'kind': 'caddy',
      'environment': 'test',
      'domain': 'pilot.test',
      'write_enabled': write,
      'choices': ['app:8080'],
      'checks': [],
    },
  ],
  'observations': [
    {
      'id': 'pilot',
      'observed_at': '2026-09-12T10:00:00Z',
      'stale': true,
      'error_code': '',
      'data': {'drift': true},
    },
  ],
  'jobs': [],
  'has_more': false,
};
Map<String, dynamic> job() => {
  'id': 'd' * 64,
  'target': 'pilot',
  'request': {'target': 'pilot', 'value': 'app:8080', 'rollback': ''},
  'state': 'planned',
  'revision': 2,
  'created_at': '2026-09-12T10:00:00Z',
  'expires_at': DateTime.now()
      .add(const Duration(minutes: 15))
      .toIso8601String(),
  'plan': {
    'before': 'old destination',
    'after': 'app:8080',
    'impact': '',
    'verification': '',
    'rollback': '',
    'checks': [],
  },
  'result': <String, dynamic>{},
  'error_code': '',
};
Future<void> scrollTo(WidgetTester tester, String label) async {
  for (
    var i = 0;
    i < 25 && find.text(label).hitTestable().evaluate().isEmpty;
    i++
  ) {
    await tester.drag(find.byType(ListView).first, const Offset(0, -230));
    await tester.pumpAndSettle();
  }
}

void main() {
  for (final width in [390.0, 1440.0]) {
    testWidgets('edge review and fresh proof at $width', (tester) async {
      tester.view.physicalSize = Size(width, 1000);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      var prepares = 0, decisions = 0;
      final c = controller((method, path, {body, idempotencyKey}) async {
        if (method == 'POST' && path == '/v1/edge/jobs') {
          prepares++;
          expect(body?['value'], 'app:8080');
          expect(idempotencyKey, isNotEmpty);
          return {'job': job(), 'confirmation': 'APPLY pilot dddddddd'};
        }
        if (method == 'POST') decisions++;
        if (path.contains('/jobs/')) {
          return {'job': job(), 'confirmation': 'APPLY pilot dddddddd'};
        }
        return overview();
      });
      final prefs = PrismePreferences(locale: const Locale('en'));
      await tester.pumpWidget(
        PrismeApp(
          preferences: prefs,
          home: Scaffold(body: EdgePage(controller: c)),
        ),
      );
      await tester.pumpAndSettle();
      expect(find.text('Drift detected'), findsOneWidget);
      expect(find.text('Unavailable or stale observation'), findsOneWidget);
      await tester.tap(find.text('Prepare plan'));
      await tester.pumpAndSettle();
      expect(prepares, 1);
      expect(decisions, 0);
      await scrollTo(tester, 'Approve');
      await tester.tap(find.text('Approve'));
      await tester.pumpAndSettle();
      expect(find.textContaining('Reload shared Caddy'), findsWidgets);
      await tester.enterText(
        find.byType(TextField).last,
        'APPLY pilot dddddddd',
      );
      await tester.pumpAndSettle();
      await tester.tap(find.text('Approve').last);
      await tester.pumpAndSettle();
      expect(decisions, 0);
      expect(c.error, 'passkey_session_required');
      expect(tester.takeException(), isNull);
      await tester.pumpWidget(const SizedBox());
      c.dispose();
      prefs.dispose();
    });
  }
  testWidgets('edge readonly and late responses clear on signout', (
    tester,
  ) async {
    final pending = Completer<Map<String, dynamic>>();
    final c = controller(
      (method, path, {body, idempotencyKey}) => pending.future,
    );
    final prefs = PrismePreferences(locale: const Locale('en'));
    await tester.pumpWidget(
      PrismeApp(
        preferences: prefs,
        home: Scaffold(body: EdgePage(controller: c)),
      ),
    );
    await tester.pump();
    c.identity = null;
    c.notifyListeners();
    pending.complete(overview());
    await tester.pumpAndSettle();
    expect(find.text('pilot.test · test'), findsNothing);
    expect(tester.takeException(), isNull);
    await tester.pumpWidget(const SizedBox());
    c.dispose();
    prefs.dispose();
  });
  testWidgets('edge read only disables plan creation', (tester) async {
    final c = controller(
      (method, path, {body, idempotencyKey}) async => overview(write: false),
    );
    final prefs = PrismePreferences(locale: const Locale('en'));
    await tester.pumpWidget(
      PrismeApp(
        preferences: prefs,
        home: Scaffold(body: EdgePage(controller: c)),
      ),
    );
    await tester.pumpAndSettle();
    expect(find.text('Read only'), findsOneWidget);
    expect(
      tester
          .widget<PrismeButton>(
            find.widgetWithText(PrismeButton, 'Prepare plan'),
          )
          .onPressed,
      isNull,
    );
    await tester.pumpWidget(const SizedBox());
    c.dispose();
    prefs.dispose();
  });
}
