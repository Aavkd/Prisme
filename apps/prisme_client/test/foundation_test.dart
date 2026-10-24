import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:prisme_client/api/connection.dart';
import 'package:prisme_client/api/generated.dart';
import 'package:prisme_client/foundation/controller.dart';
import 'package:prisme_client/main.dart';

void main() {
  testWidgets('login, offline refresh and revoked session at mobile width', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(390, 844);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    String? token;
    var offline = false;
    var revoked = false;
    var loginSecret = '';
    final end = DateTime.now()
        .add(const Duration(hours: 1))
        .toUtc()
        .toIso8601String();
    final api = PrismeApi((method, path, {body, idempotencyKey}) async {
      if (path == '/v1/dev/sessions') {
        loginSecret = body!['secret'] as String;
        return {
          'access_token': 'test-session',
          'expires_at': end,
          'auth_mode': 'development',
        };
      }
      if (path == '/v1/me') {
        return {
          'user_id': 'development-user',
          'device_name': 'test',
          'expires_at': end,
          'auth_mode': 'development',
        };
      }
      if (revoked) throw const ApiFailure('request_rejected', 401);
      if (offline) throw const ApiFailure('offline');
      return {
        'items': [],
        'next_cursor': '',
        'observed_at': DateTime.now().toUtc().toIso8601String(),
      };
    });
    final controller = FoundationController(
      api,
      setToken: (v) => token = v,
      close: () {},
    );
    await tester.pumpWidget(PrismeClientApp(controller: controller));
    await tester.enterText(find.byType(TextField).last, 'local-secret');
    await tester.tap(find.text('Se connecter'));
    await tester.pumpAndSettle();
    expect(loginSecret, 'local-secret');
    expect(token, 'test-session');
    expect(find.text('Backend connecté'), findsOneWidget);
    offline = true;
    await tester.tap(find.text('Actualiser'));
    await tester.pumpAndSettle();
    expect(find.text('Backend connecté'), findsNothing);
    expect(find.text('État du backend inconnu'), findsOneWidget);
    offline = false;
    revoked = true;
    await controller.refresh();
    await tester.pumpAndSettle();
    expect(token, isNull);
    expect(find.text('Se connecter'), findsOneWidget);
    expect(tester.takeException(), isNull);
    await tester.pumpWidget(const SizedBox.shrink());
  });
  testWidgets('passkey login survives polls cut short by app suspension', (
    tester,
  ) async {
    String? token;
    var polls = 0;
    var rejectFlow = false;
    final end = DateTime.now()
        .add(const Duration(hours: 1))
        .toUtc()
        .toIso8601String();
    final api = PrismeApi((method, path, {body, idempotencyKey}) async {
      switch (path) {
        case '/v1/auth/browser':
          return {
            'flow_id': 'flow',
            'poll_token': 'poll',
            'browser_url': 'https://prisme.test/auth#flow',
            'expires_at': DateTime.now()
                .add(const Duration(minutes: 5))
                .toUtc()
                .toIso8601String(),
          };
        case '/v1/auth/poll':
          polls++;
          if (rejectFlow) throw const ApiFailure('invalid_flow', 401);
          if (polls == 1) throw const ApiFailure('timeout');
          if (polls == 2) throw const ApiFailure('offline');
          if (polls == 3) {
            return {
              'status': 'pending',
              'access_token': '',
              'expires_at': '',
              'auth_mode': '',
            };
          }
          return {
            'status': 'approved',
            'access_token': 'passkey-session',
            'expires_at': end,
            'auth_mode': 'passkey',
          };
        case '/v1/me':
          return {
            'user_id': 'administrator',
            'device_name': 'S24',
            'expires_at': end,
            'auth_mode': 'passkey',
          };
        case '/v1/preferences':
          return {'pinned_servers': [], 'pinned_workers': []};
      }
      return {
        'items': [],
        'next_cursor': '',
        'observed_at': DateTime.now().toUtc().toIso8601String(),
      };
    });
    Uri? opened;
    final controller = FoundationController(
      api,
      setToken: (v) => token = v,
      close: () {},
      deviceId: 'a' * 64,
      development: false,
      openBrowser: (uri) async {
        opened = uri;
        return true;
      },
    );
    final login = controller.passkeyLogin('S24');
    for (var i = 0; i < 4; i++) {
      await tester.pump(const Duration(seconds: 3));
    }
    await login;
    expect(opened.toString(), 'https://prisme.test/auth#flow');
    expect(polls, 4);
    expect(token, 'passkey-session');
    expect(controller.error, isNull);
    expect(controller.identity?.auth_mode, 'passkey');
    controller.lock();
    rejectFlow = true;
    final rejected = controller.passkeyLogin('S24');
    await tester.pump(const Duration(seconds: 3));
    await rejected;
    expect(controller.error, 'unauthorized');
    expect(controller.waitingForPasskey, isFalse);
    controller.dispose();
  });
  test('rejects insecure origins before credentials can be sent', () {
    for (final url in [
      'http://localhost:8443',
      'https://user@localhost',
      'https://localhost/path',
    ]) {
      expect(() => ApiConnection(url), throwsA(isA<ApiFailure>()));
    }
  });
}
