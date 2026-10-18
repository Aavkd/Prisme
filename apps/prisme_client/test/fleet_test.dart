import 'dart:async';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:prisme_ui/prisme_ui.dart';
import 'package:prisme_client/api/connection.dart';
import 'package:prisme_client/api/generated.dart';
import 'package:prisme_client/foundation/controller.dart';
import 'package:prisme_client/main.dart';

Map<String, dynamic> fleetFixture() => {
  'id': 'a' * 64,
  'name': 'VPS fixture',
  'last_seen_at': DateTime.now().toUtc().toIso8601String(),
  'expires_at': DateTime.now()
      .add(const Duration(hours: 24))
      .toUtc()
      .toIso8601String(),
  'revoked': false,
  'capabilities': ['self:heartbeat', 'inventory:host', 'inventory:docker'],
  'connection_state': 'online',
  'inventory': {
    'protocol_version': '1',
    'observed_at': DateTime.now()
        .subtract(const Duration(minutes: 10))
        .toUtc()
        .toIso8601String(),
    'source': 'ssh-readonly',
    'host': {
      'hostname': 'fixture',
      'os': 'Linux',
      'architecture': 'amd64',
      'cpus': 4,
      'memory_bytes': null,
      'disk_total_bytes': null,
      'disk_free_bytes': null,
    },
    'docker_available': true,
    'containers': [
      for (final project in ['sounding-lines', 'arpent'])
        {
          'id': project,
          'name': '$project-web',
          'image': 'app:1',
          'image_id': 'sha256:fixture',
          'state': 'running',
          'health': 'not_configured',
          'project': project,
          'service': 'web',
          'config_files': '/opt/$project/compose.yml',
          'ports': '443/tcp',
          'networks': 'edge',
        },
    ],
    'networks': [],
    'volumes': [],
    'images': [],
    'warnings': ['memory_unavailable'],
  },
};
void main() {
  for (final width in [390.0, 1440.0]) {
    testWidgets('fleet detail, stale data, device pins and offline at $width', (
      tester,
    ) async {
      tester.view.physicalSize = Size(width, 1000);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      var offline = false;
      var pins = <String>[];
      var mutations = 0;
      final expires = DateTime.now()
          .add(const Duration(hours: 1))
          .toUtc()
          .toIso8601String();
      final api = PrismeApi((method, path, {body, idempotencyKey}) async {
        if (offline) throw const ApiFailure('offline');
        if (path == '/v1/dev/sessions') {
          return {
            'access_token': 'fixture-token',
            'expires_at': expires,
            'auth_mode': 'development',
          };
        }
        if (path == '/v1/me') {
          return {
            'user_id': 'development-user',
            'device_name': 'test device',
            'expires_at': expires,
            'auth_mode': 'development',
          };
        }
        if (path == '/v1/preferences') {
          if (method == 'PUT') {
            pins = List<String>.from(body!['pinned_servers'] as List);
            mutations++;
          }
          return {'pinned_servers': pins, 'pinned_workers': []};
        }
        return {
          'items': [fleetFixture()],
          'next_cursor': '',
          'observed_at': DateTime.now().toUtc().toIso8601String(),
        };
      });
      final c = FoundationController(
        api,
        setToken: (_) {},
        close: () {},
        deviceId: 'b' * 64,
      );
      await tester.pumpWidget(PrismeClientApp(controller: c));
      await c.login('fixture', 'device');
      await tester.pumpAndSettle();
      final card = find.byType(PrismeResourceCard);
      final frameScroll = find
          .descendant(
            of: find.byType(PrismePageFrame),
            matching: find.byType(Scrollable),
          )
          .first;
      await tester.scrollUntilVisible(card, 250, scrollable: frameScroll);
      expect(
        tester.widget<PrismeResourceCard>(card).observation.freshness,
        PrismeFreshness.stale,
      );
      await tester.tap(find.text('VPS fixture'));
      await tester.pumpAndSettle();
      await tester.scrollUntilVisible(
        find.byType(SwitchListTile),
        250,
        scrollable: frameScroll,
      );
      await tester.pumpAndSettle();
      await Scrollable.ensureVisible(
        tester.element(find.byType(SwitchListTile)),
        alignment: 0.2,
      );
      await tester.pumpAndSettle();
      await tester.tap(find.byType(SwitchListTile));
      await tester.pumpAndSettle();
      expect(pins, ['a' * 64]);
      expect(mutations, 1);
      await tester.scrollUntilVisible(
        find.text('Découverte Compose'),
        250,
        scrollable: frameScroll,
      );
      expect(find.text('Découverte Compose'), findsOneWidget);
      offline = true;
      await c.refresh();
      await tester.pumpAndSettle();
      expect(c.servers, isNotNull);
      expect(c.connected, isFalse);
      expect(tester.takeException(), isNull);
      await tester.pumpWidget(const SizedBox.shrink());
    });
  }
  test('a pending login cannot restore a locked session', () async {
    final pending = Completer<Map<String, dynamic>>();
    String? token;
    final c = FoundationController(
      PrismeApi((method, path, {body, idempotencyKey}) => pending.future),
      setToken: (value) => token = value,
      close: () {},
    );
    final future = c.login('fixture', 'device');
    c.lock();
    pending.complete({
      'access_token': 'late-token',
      'expires_at': DateTime.now()
          .add(const Duration(hours: 1))
          .toIso8601String(),
      'auth_mode': 'development',
    });
    await future;
    expect(c.identity, isNull);
    expect(token, isNull);
    c.dispose();
  });
}
