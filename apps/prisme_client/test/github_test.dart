import 'dart:async';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:prisme_ui/prisme_ui.dart';
import 'package:prisme_client/api/connection.dart';
import 'package:prisme_client/api/generated.dart';
import 'package:prisme_client/foundation/controller.dart';
import 'package:prisme_client/github/page.dart';

void main() {
  final at = DateTime.now().toUtc().toIso8601String();
  final build = {
    'id': '100',
    'repository_id': '11',
    'workflow_id': '70',
    'name': 'Publish pilot',
    'commit': 'a' * 40,
    'branch': 'main',
    'author': 'builder',
    'status': 'completed',
    'conclusion': 'success',
    'attempt': 1,
    'created_at': at,
    'started_at': at,
    'updated_at': at,
    'release_state': 'linked',
  };
  final overview = {
    'enabled': true,
    'repositories': [
      {
        'id': '11',
        'name': 'fixture/pilot',
        'environment': 'production',
        'workflow_id': '70',
        'error_code': '',
        'last_sync': at,
      },
    ],
  };
  Map<String, dynamic> response(String path) {
    final u = Uri.parse(path);
    if (u.path == '/v1/github') return overview;
    if (u.path.endsWith('/builds')) {
      return {
        'items': [build],
        'has_more': false,
      };
    }
    if (u.path.endsWith('/releases')) {
      return {
        'items': [
          {
            'id': 'r1',
            'repository_id': '11',
            'artifact_id': '55',
            'manifest': {
              'version': 1,
              'repository': 'fixture/pilot',
              'commit': 'a' * 40,
              'run_id': '100',
              'run_attempt': 1,
              'environment': 'production',
              'images': [
                {
                  'service': 'web',
                  'reference': 'ghcr.io/fixture/web@sha256:${'b' * 64}',
                  'tag': 'v1',
                  'provenance': '',
                },
              ],
            },
          },
        ],
        'has_more': false,
      };
    }
    if (u.path.endsWith('/builds/100')) {
      return {
        'data': {
          'build': build,
          'jobs': {
            'jobs': [
              {
                'id': 90,
                'name': 'Publish job',
                'conclusion': 'success',
                'runner_name': 'self-hosted',
                'steps': [
                  {'name': 'Push image', 'conclusion': 'success'},
                ],
              },
            ],
          },
          'artifacts': {
            'artifacts': [
              {
                'name': 'prisme-release-1',
                'size_in_bytes': 500,
                'expired': false,
                'digest': 'sha256:fixture',
              },
            ],
          },
        },
      };
    }
    if (u.path.endsWith('/logs')) {
      return {'text': 'Build completed password=[REDACTED]'};
    }
    if (u.path.endsWith('/browse')) {
      if (u.queryParameters['kind'] == 'files') {
        if (u.queryParameters['path'] == 'README.md') {
          return {
            'data': {
              'type': 'file',
              'name': 'README.md',
              'content': '# Pilot source',
              'encoding': 'utf-8',
            },
          };
        }
        return {
          'data': {
            'items': [
              {
                'name': 'README.md',
                'path': 'README.md',
                'type': 'file',
                'sha': 'c' * 40,
              },
            ],
          },
        };
      }
      if (u.queryParameters['kind'] == 'branches') {
        return {
          'data': {
            'items': [
              {
                'name': 'feature/audio',
                'commit': {'sha': 'c' * 40},
              },
            ],
          },
        };
      }
      if (u.queryParameters['kind'] == 'commits') {
        return {
          'data': {
            'items': [
              {
                'sha': 'c' * 40,
                'commit': {
                  'message': 'Improve audio',
                  'author': {'name': 'Builder'},
                },
              },
            ],
          },
        };
      }
      if (u.queryParameters['kind'] == 'pulls') {
        return {
          'data': {
            'items': [
              {
                'title': 'Audio improvements',
                'number': 12,
                'state': 'open',
                'head': {'ref': 'feature/audio'},
                'base': {'ref': 'main'},
              },
            ],
          },
        };
      }
      return {
        'data': {
          'workflows': [
            {
              'name': 'Publish workflow',
              'state': 'active',
              'path': '.github/workflows/build.yml',
            },
          ],
        },
      };
    }
    throw StateError('Unexpected route $path');
  }

  FoundationController controller(PrismeApi api) =>
      FoundationController(api, setToken: (_) {}, close: () {})
        ..identity = Identity(
          user_id: 'fixture',
          device_name: 'test',
          expires_at: DateTime.now()
              .add(const Duration(hours: 1))
              .toIso8601String(),
          auth_mode: 'development',
        );

  for (final width in [390.0, 1440.0]) {
    testWidgets('GitHub build provenance, logs and source browser at $width', (
      tester,
    ) async {
      tester.view.physicalSize = Size(width, 1000);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      var offline = false;
      final paths = <Uri>[];
      final c = controller(
        PrismeApi((method, path, {body, idempotencyKey}) async {
          expect(method, 'GET');
          paths.add(Uri.parse(path));
          if (offline) throw const ApiFailure('offline');
          return response(path);
        }),
      );
      final prefs = PrismePreferences(locale: const Locale('en'));
      await tester.pumpWidget(
        PrismeApp(
          preferences: prefs,
          home: Scaffold(body: GithubPage(controller: c)),
        ),
      );
      await tester.pumpAndSettle();
      expect(tester.takeException(), isNull);
      Future<void> tapText(String value) async {
        tester
            .state<ScrollableState>(find.byType(Scrollable).first)
            .position
            .jumpTo(0);
        await tester.pump();
        final f = find.text(value);
        for (var i = 0; i < 30 && f.evaluate().isEmpty; i++) {
          await tester.drag(
            find.byType(Scrollable).first,
            const Offset(0, -250),
          );
          await tester.pumpAndSettle();
        }
        await tester.pumpAndSettle();
        await tester.ensureVisible(f.first);
        await tester.pumpAndSettle();
        await tester.tap(f.first);
        await tester.pumpAndSettle();
        expect(tester.takeException(), isNull);
      }

      await tapText('Publish pilot #100 · 1');
      await tapText('Read logs');
      await tester.scrollUntilVisible(
        find.text('Build completed password=[REDACTED]'),
        250,
        scrollable: find.byType(Scrollable).first,
      );
      expect(find.text('Push image'), findsOneWidget);
      await tapText('Linked releases');
      expect(
        find.textContaining('ghcr.io/fixture/web@sha256:'),
        findsOneWidget,
      );
      await tapText('Files');
      await tapText('README.md');
      expect(find.text('# Pilot source'), findsOneWidget);
      await tapText('Branches');
      await tapText('feature/audio');
      expect(find.text('Improve audio'), findsOneWidget);
      expect(
        paths.any((p) => p.queryParameters['ref'] == 'feature/audio'),
        isTrue,
      );
      await tapText('Pull requests');
      expect(find.text('Audio improvements'), findsOneWidget);
      offline = true;
      await tapText('Refresh');
      expect(find.text('GitHub unavailable'), findsOneWidget);
      expect(find.text('Audio improvements'), findsOneWidget);
      c.lock();
      await tester.pumpAndSettle();
      expect(find.text('Audio improvements'), findsNothing);
      await tester.pumpWidget(const SizedBox());
      c.dispose();
      prefs.dispose();
    });
  }
  testWidgets(
    'GitHub disabled and expired sessions do not expose private data',
    (tester) async {
      var expired = false;
      final c = controller(
        PrismeApi((method, path, {body, idempotencyKey}) async {
          if (expired) throw const ApiFailure('unauthorized', 401);
          return {'enabled': false, 'repositories': []};
        }),
      );
      final prefs = PrismePreferences(locale: const Locale('en'));
      await tester.pumpWidget(
        PrismeApp(
          preferences: prefs,
          home: Scaffold(body: GithubPage(controller: c)),
        ),
      );
      await tester.pumpAndSettle();
      expect(find.textContaining('GitHub is not configured.'), findsOneWidget);
      expired = true;
      await tester.tap(find.text('Refresh'));
      await tester.pumpAndSettle();
      expect(c.identity, isNull);
      expect(find.textContaining('GitHub is not configured.'), findsNothing);
      await tester.pumpWidget(const SizedBox());
      c.dispose();
      prefs.dispose();
    },
  );
  testWidgets('Late responses cannot repopulate a locked GitHub screen', (
    tester,
  ) async {
    final pending = Completer<Map<String, dynamic>>();
    final c = controller(
      PrismeApi((method, path, {body, idempotencyKey}) => pending.future),
    );
    final prefs = PrismePreferences(locale: const Locale('en'));
    await tester.pumpWidget(
      PrismeApp(
        preferences: prefs,
        home: Scaffold(body: GithubPage(controller: c)),
      ),
    );
    await tester.pump();
    c.lock();
    pending.complete({'enabled': false, 'repositories': []});
    await tester.pumpAndSettle();
    expect(find.text('GitHub'), findsNothing);
    await tester.pumpWidget(const SizedBox());
    c.dispose();
    prefs.dispose();
  });
}
