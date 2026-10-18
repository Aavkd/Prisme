import 'dart:async';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:prisme_ui/prisme_ui.dart';
import 'package:prisme_client/api/connection.dart';
import 'package:prisme_client/api/generated.dart';
import 'package:prisme_client/foundation/controller.dart';
import 'package:prisme_client/observability/page.dart';

void main() {
  final now = DateTime.now().toUtc();
  final at = now.toIso8601String();
  final incident = {
    'id': '1',
    'rule_id': 'HTTP',
    'scope_id': 'pilot',
    'title': 'Pilot unavailable',
    'severity': 'critical',
    'status': 'detected',
    'opened_at': at,
    'updated_at': at,
    'resolved_at': null,
    'revision': 1,
  };
  Map<String, dynamic> fixture(String path) {
    final uri = Uri.parse(path);
    return switch (uri.path) {
      '/v1/observability' => {
        'enabled': true,
        'observed_at': at,
        'scopes': [
          {
            'id': 'pilot',
            'server': 'local',
            'project': 'phase2-pilot',
            'environment': 'development',
            'service': 'web',
          },
        ],
        'rules': [
          {
            'definition': {
              'scope_id': 'pilot',
              'title': 'HTTP check',
              'metric': 'probe_success',
              'for_seconds': 15,
              'recovery_seconds': 15,
            },
            'state': {
              'signal': 'unknown',
              'last_sample': at,
              'reason': 'missing_or_stale_samples',
            },
          },
        ],
      },
      '/v1/incidents' => {
        'items': [incident],
        'next_cursor': '',
        'observed_at': at,
      },
      '/v1/incidents/1' => {
        'incident': incident,
        'events': [
          {
            'id': '1',
            'at': at,
            'kind': 'detected',
            'actor': 'system',
            'message': 'Failure confirmed',
          },
        ],
        'next_cursor': '',
        'observed_at': at,
      },
      '/v1/observability/metrics' => {
        'series': [],
        'start': uri.queryParameters['start'],
        'end': uri.queryParameters['end'],
        'observed_at': at,
        'source': 'victoriametrics',
        'step_seconds': 5,
      },
      '/v1/observability/logs' => {
        'lines': [
          {'at': at, 'line': 'password=[REDACTED]', 'labels': {}},
        ],
        'truncated': false,
        'start': uri.queryParameters['start'],
        'end': uri.queryParameters['end'],
        'observed_at': at,
        'source': 'loki',
      },
      _ => throw StateError('Unexpected fixture route'),
    };
  }

  FoundationController controller(PrismeApi api) =>
      FoundationController(api, setToken: (_) {}, close: () {})
        ..identity = Identity(
          user_id: 'fixture',
          device_name: 'test',
          expires_at: now.add(const Duration(hours: 1)).toIso8601String(),
          auth_mode: 'development',
        );

  for (final width in [390.0, 1440.0]) {
    testWidgets('diagnostics preserve unknown and incidents at $width', (
      tester,
    ) async {
      tester.view.physicalSize = Size(width, 1000);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      var offline = false;
      final queries = <Uri>[];
      final c = controller(
        PrismeApi((method, path, {body, idempotencyKey}) async {
          if (offline) throw const ApiFailure('offline');
          queries.add(Uri.parse(path));
          return fixture(path);
        }),
      );
      final preferences = PrismePreferences(locale: const Locale('fr'));
      await tester.pumpWidget(
        PrismeApp(
          preferences: preferences,
          home: Scaffold(body: ObservabilityPage(controller: c)),
        ),
      );
      await tester.pumpAndSettle();
      expect(find.textContaining('Inconnu'), findsOneWidget);
      expect(tester.takeException(), isNull);
      final metricQuery = queries
          .firstWhere((u) => u.path.endsWith('/metrics'))
          .queryParameters;
      final logQuery = queries
          .firstWhere((u) => u.path.endsWith('/logs'))
          .queryParameters;
      expect(metricQuery['start'], logQuery['start']);
      expect(metricQuery['end'], logQuery['end']);
      await tester.scrollUntilVisible(
        find.text('Pilot unavailable'),
        250,
        scrollable: find.byType(Scrollable).first,
      );
      await tester.tap(find.text('Pilot unavailable'));
      await tester.pumpAndSettle();
      await tester.scrollUntilVisible(
        find.text('Acquitter'),
        200,
        scrollable: find.byType(Scrollable).first,
      );
      expect(find.text('Acquitter'), findsOneWidget);
      offline = true;
      await tester.scrollUntilVisible(
        find.text('Actualiser'),
        -200,
        scrollable: find.byType(Scrollable).first,
      );
      await Scrollable.ensureVisible(
        tester.element(find.text('Actualiser')),
        alignment: 0.5,
      );
      await tester.pumpAndSettle();
      await tester.tap(find.text('Actualiser'));
      await tester.pumpAndSettle();
      expect(find.text('Données non vérifiées'), findsOneWidget);
      await tester.scrollUntilVisible(
        find.text('Acquitter'),
        200,
        scrollable: find.byType(Scrollable).first,
      );
      final button = tester.widget<PrismeButton>(
        find.widgetWithText(PrismeButton, 'Acquitter'),
      );
      expect(button.onPressed, isNull);
      expect(find.text('Pilot unavailable'), findsOneWidget);
      await tester.pumpWidget(const SizedBox.shrink());
      c.dispose();
      preferences.dispose();
      expect(tester.takeException(), isNull);
    });
  }
  testWidgets(
    'late diagnostics response cannot update a disposed session view',
    (tester) async {
      final pending = Completer<Map<String, dynamic>>();
      final c = controller(
        PrismeApi((method, path, {body, idempotencyKey}) => pending.future),
      );
      final preferences = PrismePreferences();
      await tester.pumpWidget(
        PrismeApp(
          preferences: preferences,
          home: Scaffold(body: ObservabilityPage(controller: c)),
        ),
      );
      await tester.pump();
      c.lock();
      await tester.pumpWidget(const SizedBox.shrink());
      pending.complete(fixture('/v1/observability'));
      await tester.pump();
      expect(c.identity, isNull);
      expect(tester.takeException(), isNull);
      c.dispose();
      preferences.dispose();
    },
  );
  testWidgets('an old healthy observation is displayed as unknown', (
    tester,
  ) async {
    final c = controller(
      PrismeApi((method, path, {body, idempotencyKey}) async {
        final result = fixture(path);
        if (Uri.parse(path).path == '/v1/observability') {
          final rule = (result['rules'] as List).first as Map<String, dynamic>;
          rule['state'] = {
            'signal': 'healthy',
            'reason': '',
            'last_sample': DateTime.now()
                .toUtc()
                .subtract(const Duration(minutes: 5))
                .toIso8601String(),
          };
        }
        return result;
      }),
    );
    final preferences = PrismePreferences(locale: const Locale('fr'));
    await tester.pumpWidget(
      PrismeApp(
        preferences: preferences,
        home: Scaffold(body: ObservabilityPage(controller: c)),
      ),
    );
    await tester.pumpAndSettle();
    expect(find.text('Inconnu · probe_success'), findsOneWidget);
    expect(find.text('Sain · probe_success'), findsNothing);
    await tester.pumpWidget(const SizedBox.shrink());
    c.dispose();
    preferences.dispose();
  });
  test('chart leaves absent buckets and NaNs empty', () {
    final start = DateTime.utc(2026, 9, 7);
    final series = MetricSeries(
      labels: {},
      points: [
        MetricPoint(at: start.toIso8601String(), value: 1),
        MetricPoint(
          at: start.add(const Duration(seconds: 10)).toIso8601String(),
          value: null,
        ),
        MetricPoint(
          at: start.add(const Duration(seconds: 15)).toIso8601String(),
          value: 0,
        ),
      ],
    );
    final result = MetricResult(
      series: [series],
      start: start.toIso8601String(),
      end: start.add(const Duration(seconds: 15)).toIso8601String(),
      observed_at: at,
      source: 'fixture',
      step_seconds: 5,
    );
    expect(gapAware(series, result), [1.0, null, null, 0.0]);
  });
}
