import 'dart:async';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:prisme_ui/prisme_ui.dart';
import 'package:prisme_client/automation/page.dart';
import 'backups_test.dart' show controller;
import 'edge_test.dart' show scrollTo;

Map<String, dynamic> book() => {
  'id': 'inspect',
  'version': 2,
  'target': 'pilot',
  'parameters': <String, dynamic>{},
  'steps': [
    {'action': 'check', 'when': 'always', 'seconds': 1, 'timeout_seconds': 10},
  ],
  'timeout_seconds': 300,
};
Map<String, dynamic> run() => {
  'id': 'a' * 64,
  'revision': 3,
  'target': 'pilot',
  'state': 'approval',
  'book': book(),
  'step': 0,
  'attempts': 0,
  'origin': 'manual',
  'error_code': '',
};
Map<String, dynamic> overview() => {
  'enabled': true,
  'control': {'stopped': true, 'revision': 1, 'maintenance_until': null},
  'targets': [
    {'id': 'pilot', 'environment': 'test'},
  ],
  'definitions': [book()],
  'policies': [
    {'id': 'repair', 'breaker': true, 'cooldown_until': '2026-09-12T12:00:00Z'},
  ],
  'schedules': [],
  'runs': [],
  'has_more': false,
};
void main() {
  for (final width in [390.0, 1440.0]) {
    testWidgets('automation stop, simulation, and fresh approval at $width', (
      tester,
    ) async {
      tester.view.physicalSize = Size(width, 1000);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      var stops = 0, prepares = 0, decisions = 0;
      final c = controller((method, path, {body, idempotencyKey}) async {
        if (path == '/v1/automation/control') {
          stops++;
          expect(body?['proof'], '');
          expect(body?['control']['stopped'], true);
          return {'control': overview()['control']};
        }
        if (path == '/v1/automation/runs') {
          prepares++;
          expect(body?['version'], 2);
          expect(body?['simulation'], true);
          expect(idempotencyKey, isNotEmpty);
          return {'run': run()};
        }
        if (path.endsWith('/decision')) decisions++;
        if (path.contains('/runs/')) return {'run': run(), 'events': []};
        return overview();
      });
      final prefs = PrismePreferences(locale: const Locale('en'));
      await tester.pumpWidget(
        PrismeApp(
          preferences: prefs,
          home: Scaffold(body: AutomationPage(controller: c)),
        ),
      );
      await tester.pumpAndSettle();
      expect(find.text('Global stop active'), findsOneWidget);
      await tester.tap(find.text('Stop all'));
      await tester.pumpAndSettle();
      expect(stops, 1);
      await scrollTo(tester, 'Simulate');
      await tester.tap(find.text('Simulate'));
      await tester.pumpAndSettle();
      expect(prepares, 1);
      await scrollTo(tester, 'Approve step');
      await tester.tap(find.text('Approve step'));
      await tester.pumpAndSettle();
      expect(
        find.textContaining('Authorize only the displayed step'),
        findsOneWidget,
      );
      await tester.enterText(find.byType(TextField).last, 'pilot');
      await tester.pumpAndSettle();
      await tester.tap(find.text('Confirm').last);
      await tester.pumpAndSettle();
      expect(decisions, 0);
      expect(c.error, 'passkey_session_required');
      expect(tester.takeException(), isNull);
      await tester.pumpWidget(const SizedBox());
      c.dispose();
      prefs.dispose();
    });
  }
  testWidgets('automation ignores responses after signout', (tester) async {
    final pending = Completer<Map<String, dynamic>>();
    final c = controller(
      (method, path, {body, idempotencyKey}) => pending.future,
    );
    final prefs = PrismePreferences(locale: const Locale('en'));
    await tester.pumpWidget(
      PrismeApp(
        preferences: prefs,
        home: Scaffold(body: AutomationPage(controller: c)),
      ),
    );
    await tester.pump();
    c.identity = null;
    c.notifyListeners();
    pending.complete(overview());
    await tester.pumpAndSettle();
    expect(find.text('Stop all'), findsNothing);
    expect(tester.takeException(), isNull);
    await tester.pumpWidget(const SizedBox());
    c.dispose();
    prefs.dispose();
  });
  testWidgets('automation unavailable disables mutations', (tester) async {
    var calls = 0;
    final c = controller((method, path, {body, idempotencyKey}) async {
      calls++;
      return {'enabled': false};
    });
    final prefs = PrismePreferences(locale: const Locale('fr'));
    await tester.pumpWidget(
      PrismeApp(
        preferences: prefs,
        home: Scaffold(body: AutomationPage(controller: c)),
      ),
    );
    await tester.pumpAndSettle();
    expect(find.text('Worker non configuré'), findsOneWidget);
    expect(find.text('Arrêt général'), findsNothing);
    expect(calls, 1);
    await tester.pumpWidget(const SizedBox());
    c.dispose();
    prefs.dispose();
  });
}
