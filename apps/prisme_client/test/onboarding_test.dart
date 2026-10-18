import 'dart:async';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:prisme_ui/prisme_ui.dart';
import 'package:prisme_client/api/generated.dart';
import 'package:prisme_client/foundation/controller.dart';
import 'package:prisme_client/onboarding/page.dart';

const manifest = <String, dynamic>{
  'id': 'new-project-test',
  'repository_id': '11',
  'repository': 'fixture/new-project',
  'environment': 'test',
  'project': 'new-project',
  'server': 'local-fixture',
  'branch': 'main',
  'strategy': 'recreate',
  'backup_required': false,
  'services': [
    {'name': 'web', 'health_path': '/health', 'env_file': 'web.env'},
  ],
};
final overview = <String, dynamic>{
  'enabled': true,
  'targets': [
    {'manifest': manifest, 'blockers': []},
  ],
  'projects': [],
};

FoundationController controller(PrismeApi api) =>
    FoundationController(api, setToken: (_) {}, close: () {})
      ..identity = const Identity(
        user_id: 'fixture',
        device_name: 'fixture',
        auth_mode: 'development',
        expires_at: '2099-01-01T00:00:00Z',
      );

void main() {
  for (final width in [390.0, 1440.0]) {
    testWidgets('onboarding previews without execution at $width', (
      tester,
    ) async {
      tester.view.physicalSize = Size(width, 1100);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      final mutations = <String>[];
      final c = controller(
        PrismeApi((method, path, {body, idempotencyKey}) async {
          if (method == 'POST') {
            mutations.add(path);
            expect(body, {
              'target': 'new-project-test',
              'release': 'release-1',
            });
            expect(idempotencyKey, isNotEmpty);
            return {
              'job': {
                'id': 'job-1',
                'plan': {'blockers': []},
              },
              'files': {
                'compose.json': '{"services":{}}',
                'prisme-project.json': '{}',
              },
            };
          }
          if (path == '/v1/onboarding') return overview;
          return {
            'items': [
              {
                'id': 'release-1',
                'repository_id': '11',
                'artifact_id': '1',
                'manifest': {
                  'version': 1,
                  'repository': 'fixture/new-project',
                  'commit': 'a' * 40,
                  'run_id': '1',
                  'run_attempt': 1,
                  'environment': 'test',
                  'images': [],
                },
              },
            ],
            'has_more': false,
          };
        }),
      );
      final prefs = PrismePreferences(locale: const Locale('en'));
      await tester.pumpWidget(
        PrismeApp(
          preferences: prefs,
          home: Scaffold(body: OnboardingPage(controller: c)),
        ),
      );
      await tester.pumpAndSettle();
      expect(find.text('local-fixture'), findsOneWidget);
      for (
        var i = 0;
        i < 12 &&
            find.text('Prepare and preview').hitTestable().evaluate().isEmpty;
        i++
      ) {
        await tester.drag(find.byType(ListView).first, const Offset(0, -300));
        await tester.pumpAndSettle();
      }
      await tester.tap(find.text('Prepare and preview'));
      await tester.pumpAndSettle();
      expect(mutations, ['/v1/onboarding']);
      for (
        var i = 0;
        i < 8 && find.text('compose.json').evaluate().isEmpty;
        i++
      ) {
        await tester.drag(find.byType(ListView).first, const Offset(0, -300));
        await tester.pumpAndSettle();
      }
      expect(find.text('compose.json'), findsOneWidget);
      expect(tester.takeException(), isNull);
      c.lock();
      await tester.pumpAndSettle();
      expect(find.text('compose.json'), findsNothing);
      await tester.pumpWidget(const SizedBox.shrink());
      c.dispose();
      prefs.dispose();
    });
  }
  testWidgets('late onboarding response is discarded after lock', (
    tester,
  ) async {
    final response = Completer<Map<String, dynamic>>();
    final c = controller(
      PrismeApi((method, path, {body, idempotencyKey}) => response.future),
    );
    final prefs = PrismePreferences(locale: const Locale('en'));
    await tester.pumpWidget(
      PrismeApp(
        preferences: prefs,
        home: Scaffold(body: OnboardingPage(controller: c)),
      ),
    );
    await tester.pump();
    c.lock();
    response.complete(overview);
    await tester.pumpAndSettle();
    expect(find.text('local-fixture'), findsNothing);
    expect(tester.takeException(), isNull);
    await tester.pumpWidget(const SizedBox.shrink());
    c.dispose();
    prefs.dispose();
  });
  testWidgets('unconfigured wizard explains prerequisites', (tester) async {
    final c = controller(
      PrismeApi(
        (method, path, {body, idempotencyKey}) async => {
          'enabled': false,
          'targets': [],
          'projects': [],
        },
      ),
    );
    final prefs = PrismePreferences(locale: const Locale('en'));
    await tester.pumpWidget(
      PrismeApp(
        preferences: prefs,
        home: Scaffold(body: OnboardingPage(controller: c)),
      ),
    );
    await tester.pumpAndSettle();
    expect(find.text('Prepare a project target'), findsOneWidget);
    expect(find.text('Prepare and preview'), findsNothing);
    await tester.pumpWidget(const SizedBox.shrink());
    c.dispose();
    prefs.dispose();
  });
}
