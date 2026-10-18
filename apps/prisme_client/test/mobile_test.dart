import 'dart:convert';
import 'package:flutter/material.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:prisme_ui/prisme_ui.dart';
import 'package:prisme_client/api/generated.dart';
import 'package:prisme_client/foundation/controller.dart';
import 'package:prisme_client/mobile/services.dart';
import 'package:prisme_client/mobile/offline_page.dart';
import 'package:prisme_client/mobile/page.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  test('notification routes cannot carry commands or external URLs', () {
    expect(validNotificationRoute('/incidents/${'a' * 64}'), isTrue);
    expect(validNotificationRoute('/watchdog'), isTrue);
    for (final route in [
      'https://evil.example',
      '/incidents/1',
      '/incidents/${'a' * 64}?approve=true',
      '/deployments/approve',
      '/incidents/../action',
    ]) {
      expect(validNotificationRoute(route), isFalse);
    }
  });
  test(
    'cache is allowlisted, scoped, unlock-gated and erased on opt-out',
    () async {
      FlutterSecureStorage.setMockInitialValues({});
      var allowed = false;
      final cache = OfflineCache('fixture-a', unlock: () async => allowed);
      await cache.initialize();
      expect(cache.enabled, isFalse);
      await cache.setEnabled(true);
      await cache.record('/v1/incidents', {
        'items': [
          {
            'id': '1',
            'title': 'incident',
            'severity': 'critical',
            'status': 'detected',
            'secret': 'must-never-persist',
            'logs': 'private-log',
          },
        ],
      });
      await cache.record('/v1/observation/logs', {
        'items': [
          {'line': 'must-never-persist'},
        ],
      });
      expect(await cache.read(), isNull);
      allowed = true;
      final data = await cache.read();
      expect(data, isNotNull);
      expect(jsonEncode(data), isNot(contains('must-never-persist')));
      expect(jsonEncode(data), isNot(contains('private-log')));
      final other = OfflineCache('fixture-b', unlock: () async => true);
      await other.initialize();
      expect(await other.read(), isNull);
      await cache.clear();
      expect(await cache.read(), isNull);
      expect(await const FlutterSecureStorage().read(key: cache.key), isNull);
      await cache.setEnabled(true);
      final lateWrite = cache.record('/v1/incidents', {
        'items': [
          {'id': 'late'},
        ],
      });
      final clearing = cache.clear();
      await Future.wait([lateWrite, clearing]);
      expect(await const FlutterSecureStorage().read(key: cache.key), isNull);
      expect(
        await const FlutterSecureStorage().read(key: '${cache.key}_enabled'),
        'false',
      );
    },
  );
  test(
    'locking removes cached display and defers notification navigation until login',
    () {
      final c = FoundationController(
        PrismeApi((_, _, {body, idempotencyKey}) async => {}),
        setToken: (_) {},
        close: () {},
      );
      c.offlineSnapshot = {'private': true};
      c.receiveNotification('/incidents/${'a' * 64}');
      expect(c.identity, isNull);
      expect(c.notificationRoute, isNotNull);
      c.lock();
      expect(c.offlineSnapshot, isNull);
      c.receiveNotification('/deployments/approve');
      expect(c.notificationRoute, '/incidents/${'a' * 64}');
      c.dispose();
    },
  );
  for (final width in [390.0, 1440.0]) {
    testWidgets('offline view has old timestamps and no actions at $width', (
      tester,
    ) async {
      tester.view.physicalSize = Size(width, 1000);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      final prefs = PrismePreferences(locale: const Locale('en'));
      await tester.pumpWidget(
        PrismeApp(
          preferences: prefs,
          home: OfflinePage(
            snapshot: {
              '/v1/incidents': {
                'captured_at': '2026-09-08T12:00:00Z',
                'items': [
                  {'title': 'Known critical incident', 'severity': 'critical'},
                ],
              },
            },
            close: () {},
          ),
        ),
      );
      await tester.pumpAndSettle();
      expect(find.text('Cache · read only'), findsOneWidget);
      expect(find.textContaining('Known critical incident'), findsOneWidget);
      expect(find.text('Approve'), findsNothing);
      expect(find.text('Acknowledge'), findsNothing);
      expect(tester.takeException(), isNull);
      await tester.pumpWidget(const SizedBox.shrink());
      prefs.dispose();
    });
    testWidgets('notification settings and authenticated incident at $width', (
      tester,
    ) async {
      tester.view.physicalSize = Size(width, 1100);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      final now = DateTime.now().toUtc().toIso8601String();
      final c = FoundationController(
        PrismeApi((method, path, {body, idempotencyKey}) async {
          if (path == '/v1/notifications') {
            return {
              'preferences': {
                'enabled': false,
                'timezone': 'Europe/Paris',
                'quiet_start': 1320,
                'quiet_end': 420,
              },
              'configured': false,
              'action_key_registered': false,
            };
          }
          return {
            'incident': {
              'id': '1',
              'rule_id': 'local',
              'scope_id': 'local',
              'title': 'Local incident',
              'severity': 'critical',
              'status': 'detected',
              'opened_at': now,
              'updated_at': now,
              'resolved_at': null,
              'revision': 1,
            },
            'events': [],
            'next_cursor': '',
            'observed_at': now,
          };
        }),
        setToken: (_) {},
        close: () {},
      );
      c.identity = Identity.fromJson({
        'user_id': 'fixture',
        'device_name': 'fixture',
        'device_id': 'a' * 64,
        'auth_mode': 'passkey',
        'expires_at': now,
      });
      final prefs = PrismePreferences(locale: const Locale('en'));
      await tester.pumpWidget(
        PrismeApp(
          preferences: prefs,
          home: Scaffold(
            body: MobilePage(controller: c, route: '/incidents/${'a' * 64}'),
          ),
        ),
      );
      await tester.pumpAndSettle();
      expect(find.text('Local incident'), findsOneWidget);
      expect(find.text('Acknowledge'), findsOneWidget);
      expect(tester.takeException(), isNull);
      await tester.pumpWidget(const SizedBox.shrink());
      c.dispose();
      prefs.dispose();
    });
  }
}
