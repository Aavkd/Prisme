import 'dart:async';
import 'dart:convert';
import 'dart:io';
import 'package:crypto/crypto.dart';
import 'package:flutter/services.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:local_auth/local_auth.dart';
import '../api/connection.dart';
import '../api/generated.dart';

bool validNotificationRoute(String route) =>
    route == '/watchdog' ||
    RegExp(r'^/incidents/[a-f0-9]{64}$').hasMatch(route);

class MobileServices {
  MobileServices(String origin, String deviceId)
    : scope = sha256.convert(utf8.encode('$origin\n$deviceId')).toString();
  final String scope;
  static const channel = MethodChannel('prisme/mobile');
  Future<void> listen(void Function(String) onRoute) async {
    if (!Platform.isAndroid) return;
    channel.setMethodCallHandler((call) async {
      if (call.method == 'route' &&
          call.arguments is String &&
          validNotificationRoute(call.arguments as String)) {
        onRoute(call.arguments as String);
      }
    });
    final route = await channel.invokeMethod<String>('initialRoute');
    if (route != null && validNotificationRoute(route)) onRoute(route);
  }

  Future<String> enableNotifications() async {
    if (!Platform.isAndroid) throw const ApiFailure('android_required');
    return (await channel.invokeMethod<String>('enableNotifications'))!;
  }

  Future<void> disableNotifications() async {
    if (Platform.isAndroid) {
      await channel.invokeMethod<void>('disableNotifications');
    }
  }

  Future<Map<String, dynamic>> status() async => Platform.isAndroid
      ? Map<String, dynamic>.from(
          (await channel.invokeMapMethod<String, dynamic>(
            'notificationStatus',
          ))!,
        )
      : {'configured': false, 'permission': false};
  Future<void> enrollKey(PrismeApi api) async {
    if (!Platform.isAndroid) throw const ApiFailure('android_required');
    final key = await channel.invokeMethod<String>('actionPublicKey', {
      'scope': scope,
    });
    await api.send('POST', '/v1/action-key', body: {'public_key': key});
  }

  Future<void> syncNotifications(PrismeApi api, bool Function() valid) async {
    if (!Platform.isAndroid) return;
    final settings = await api.notificationSettings();
    if (!settings.preferences.enabled || !valid()) return;
    final platform = await status();
    if (platform['permission'] != true || !valid()) return;
    final token = await enableNotifications();
    if (!valid()) return;
    final p = settings.preferences;
    await api.saveNotificationSettings(
      NotificationSettingsRequest(
        token: token,
        enabled: true,
        timezone: p.timezone,
        quiet_start: p.quiet_start,
        quiet_end: p.quiet_end,
      ),
    );
  }

  Future<String> prove(
    PrismeApi api,
    DeploymentJob job,
    Future<bool> Function(Uri)? openBrowser,
    bool Function() valid,
    String title,
  ) async {
    final data = await api.send(
      'POST',
      '/v1/action-challenges',
      body: {
        'job_id': job.id,
        'revision': job.revision,
        'method': Platform.isAndroid ? 'android_key' : 'passkey',
      },
    );
    if (!valid()) throw const ApiFailure('session_expired');
    final id = data['id'] as String;
    if (Platform.isAndroid) {
      final signature = await channel.invokeMethod<String>('signAction', {
        'scope': scope,
        'challenge': data['challenge'],
        'title': title,
        'target':
            '${job.target_id} · ${(job.plan['manifest'] as Map)['environment']}',
      });
      if (!valid()) throw const ApiFailure('session_expired');
      await api.send(
        'POST',
        '/v1/action-challenges/$id/sign',
        body: {'signature': signature},
      );
    } else {
      if (openBrowser == null ||
          !await openBrowser(Uri.parse(data['browser_url'] as String))) {
        throw const ApiFailure('browser_unavailable');
      }
      final expires = DateTime.parse(data['expires_at'] as String);
      while (valid() && DateTime.now().isBefore(expires)) {
        await Future<void>.delayed(const Duration(seconds: 3));
        if (!valid()) break;
        final result = await api.send('GET', '/v1/action-challenges/$id');
        if (result['approved'] == true) return id;
      }
      throw const ApiFailure('session_expired');
    }
    if (!valid()) throw const ApiFailure('session_expired');
    return id;
  }

  Future<String> proveOperation(
    PrismeApi api,
    Map<String, dynamic> intent,
    Future<bool> Function(Uri)? openBrowser,
    bool Function() valid,
    String title, {
    bool passkeyOnly = false,
  }) async {
    final android = Platform.isAndroid && !passkeyOnly;
    final data = await api.send(
      'POST',
      '/v1/operations/challenges',
      body: {'intent': intent, 'method': android ? 'android_key' : 'passkey'},
    );
    if (!valid()) throw const ApiFailure('session_expired');
    final id = data['id'] as String;
    if (android) {
      final signature = await channel.invokeMethod<String>('signAction', {
        'scope': scope,
        'challenge': data['challenge'],
        'title': title,
        'target': intent['target'],
      });
      if (!valid()) throw const ApiFailure('session_expired');
      await api.send(
        'POST',
        '/v1/operations/challenges/$id/sign',
        body: {'signature': signature},
      );
    } else {
      if (openBrowser == null ||
          !await openBrowser(Uri.parse(data['browser_url'] as String))) {
        throw const ApiFailure('browser_unavailable');
      }
      final expires = DateTime.parse(data['expires_at'] as String);
      while (valid() && DateTime.now().isBefore(expires)) {
        await Future<void>.delayed(const Duration(seconds: 2));
        if (!valid()) break;
        final status = await api.send('GET', '/v1/operations/challenges/$id');
        if (status['approved'] == true) return id;
      }
      throw const ApiFailure('session_expired');
    }
    return id;
  }
}

/// Only allowlisted summaries are persisted. The encrypted store is scoped to the
/// server and device; neither bearer sessions nor detailed logs enter this cache.
class OfflineCache {
  OfflineCache(
    this.scope, {
    FlutterSecureStorage? storage,
    Future<bool> Function()? unlock,
  }) : storage = storage ?? const FlutterSecureStorage(),
       unlock =
           unlock ??
           (() => LocalAuthentication().authenticate(
             localizedReason:
                 'Prisme · Déverrouiller le cache / Unlock cached data',
             biometricOnly: Platform.isAndroid,
           ));
  final String scope;
  final FlutterSecureStorage storage;
  final Future<bool> Function() unlock;
  bool enabled = false;
  String? error;
  int _generation = 0;
  Future<void> _writes = Future.value();
  String get key => 'prisme_snapshot_$scope';
  Future<void> initialize() async {
    enabled = await storage.read(key: '${key}_enabled') == 'true';
  }

  Future<void> setEnabled(bool value) async {
    final ticket = ++_generation;
    enabled = value;
    final work = _writes.then((_) async {
      if (ticket != _generation) return;
      await storage.write(key: '${key}_enabled', value: '$value');
      if (!value) await storage.delete(key: key);
    });
    _writes = work.catchError((Object _) {
      error = 'cache_unavailable';
    });
    await work;
  }

  Future<void> clear() => setEnabled(false);
  Future<void> record(String path, Map<String, dynamic> body) async {
    if (!enabled) return;
    final domain = Uri.parse(path).path;
    final fields = switch (domain) {
      '/v1/servers' => ['id', 'name', 'connection_state', 'last_seen_at'],
      '/v1/incidents' => [
        'id',
        'title',
        'severity',
        'status',
        'updated_at',
        'scope_id',
      ],
      '/v1/deployments' => ['id', 'target_id', 'state', 'created_at'],
      _ => <String>[],
    };
    if (fields.isEmpty) return;
    final ticket = _generation;
    final list = body['items'];
    if (list is! List) return;
    final items = list
        .take(50)
        .whereType<Map>()
        .map(
          (item) => {
            for (final field in fields)
              if (item[field] is String)
                field: (item[field] as String).substring(
                  0,
                  (item[field] as String).length.clamp(0, 128),
                ),
          },
        )
        .toList();
    _writes = _writes
        .then((_) async {
          if (ticket != _generation || !enabled) return;
          final raw = await storage.read(key: key);
          final value = raw == null
              ? <String, dynamic>{}
              : jsonDecode(raw) as Map<String, dynamic>;
          value[domain] = {
            'captured_at': DateTime.now().toUtc().toIso8601String(),
            'items': items,
          };
          value['version'] = 1;
          value['scope'] = scope;
          if (ticket == _generation && enabled) {
            await storage.write(key: key, value: jsonEncode(value));
          }
          error = null;
        })
        .catchError((Object _) {
          error = 'cache_unavailable';
        });
    await _writes;
  }

  Future<Map<String, dynamic>?> read() async {
    final ticket = _generation;
    if (!enabled || !await unlock()) return null;
    await _writes;
    final raw = await storage.read(key: key);
    if (ticket != _generation ||
        !enabled ||
        raw == null ||
        raw.length > 131072) {
      return null;
    }
    try {
      final data = jsonDecode(raw) as Map<String, dynamic>;
      if (data['scope'] != scope || data['version'] != 1) return null;
      return data;
    } catch (_) {
      return null;
    }
  }
}
