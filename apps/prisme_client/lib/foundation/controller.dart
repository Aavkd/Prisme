import 'dart:async';
import 'dart:io';
import 'package:flutter/foundation.dart';
import 'package:flutter/services.dart';
import '../api/connection.dart';
import '../api/generated.dart';
import '../mobile/services.dart';

class FoundationController extends ChangeNotifier {
  FoundationController(
    this.api, {
    required this.setToken,
    required this.close,
    this.deviceId,
    this.development = true,
    this.openBrowser,
    this.mobile,
    this.cache,
    this.openTerminalSocket,
  });
  final PrismeApi api;
  final void Function(String?) setToken;
  final VoidCallback close;
  final String? deviceId;
  final bool development;
  final Future<bool> Function(Uri)? openBrowser;
  final MobileServices? mobile;
  final OfflineCache? cache;
  final Future<WebSocket> Function(String, String)? openTerminalSocket;

  Future<String> operationProof(Map<String, dynamic> intent) async {
    if (mobile == null || identity?.auth_mode != 'passkey') {
      throw const ApiFailure('passkey_session_required');
    }
    final generation = _generation;
    platformAuthentication = true;
    try {
      return await mobile!.proveOperation(
        api,
        intent,
        openBrowser,
        () => !_disposed && generation == _generation && identity != null,
        'Prisme · ${intent['action']}',
      );
    } finally {
      platformAuthentication = false;
    }
  }

  Map<String, dynamic>? offlineSnapshot;
  String? notificationRoute;
  bool platformAuthentication = false;
  void receiveNotification(String route) {
    if (!validNotificationRoute(route)) return;
    notificationRoute = route;
    if (!_disposed) notifyListeners();
  }

  Future<void> openCache() => _perform(() async {
    final generation = _generation;
    platformAuthentication = true;
    try {
      final data = await cache?.read();
      if (!_disposed && generation == _generation) offlineSnapshot = data;
    } finally {
      platformAuthentication = false;
    }
  });
  void closeCache() {
    offlineSnapshot = null;
    notifyListeners();
  }

  Future<String> actionProof(DeploymentJob job, String title) async {
    if (mobile == null ||
        identity == null ||
        identity!.auth_mode != 'passkey') {
      throw const ApiFailure('passkey_session_required');
    }
    final generation = _generation;
    platformAuthentication = true;
    try {
      return await mobile!.prove(
        api,
        job,
        openBrowser,
        () => !_disposed && generation == _generation && identity != null,
        title,
      );
    } finally {
      platformAuthentication = false;
    }
  }

  Identity? identity;
  AgentPage? page;
  ServerPage? servers;
  DevicePage? devices;
  AuditPage? audit;
  DevicePreferences preferences = const DevicePreferences(
    pinned_servers: [],
    pinned_workers: [],
  );
  bool busy = false;
  bool homeOnly = false;
  String _cursor = '';
  bool waitingForPasskey = false;
  String? error;
  Timer? _expiry, _refresh, _idle;
  bool _disposed = false;
  int _generation = 0;
  bool get connected => identity != null && servers != null && error == null;
  void touch() {
    _idle?.cancel();
    if (identity != null) _idle = Timer(const Duration(minutes: 5), lock);
  }

  void lock() {
    _generation++;
    setToken(null);
    identity = null;
    offlineSnapshot = null;
    page = null;
    servers = null;
    devices = null;
    audit = null;
    _cursor = '';
    _expiry?.cancel();
    _refresh?.cancel();
    _idle?.cancel();
    waitingForPasskey = false;
    if (!_disposed) notifyListeners();
  }

  Future<void> _accept(String token, String expiresAt) async {
    final generation = _generation;
    setToken(token);
    final remaining = DateTime.parse(expiresAt).difference(DateTime.now());
    _expiry?.cancel();
    _expiry = Timer(remaining.isNegative ? Duration.zero : remaining, () {
      lock();
      error = 'session_expired';
      if (!_disposed) notifyListeners();
    });
    try {
      final me = await api.me();
      if (_disposed || generation != _generation) return;
      identity = me;
      if (deviceId != null) {
        final result = await api.preferences();
        if (_disposed || generation != _generation) return;
        preferences = result;
      }
      await _readFleet();
      if (_disposed || generation != _generation) return;
      try {
        await mobile?.syncNotifications(
          api,
          () => !_disposed && generation == _generation && identity != null,
        );
      } catch (_) {
        /* Notification availability does not invalidate login. */
      }
      if (_disposed || generation != _generation) return;
      touch();
      _refresh?.cancel();
      _refresh = Timer.periodic(const Duration(seconds: 30), (_) {
        if (identity != null && !waitingForPasskey) unawaited(refresh());
      });
    } catch (_) {
      lock();
      rethrow;
    }
  }

  Future<void> login(String secret, String deviceName) => _perform(() async {
    final generation = _generation;
    final data = await api.send(
      'POST',
      '/v1/dev/sessions',
      body: {
        'secret': secret,
        'device_name': deviceName,
        if (deviceId != null) 'device_id': deviceId,
      },
    );
    if (!_disposed && generation == _generation) {
      await _accept(
        data['access_token'] as String,
        data['expires_at'] as String,
      );
    }
  });
  Future<void> passkeyLogin(String deviceName) => _perform(() async {
    if (deviceId == null || openBrowser == null) {
      throw const ApiFailure('passkeys_unconfigured');
    }
    final generation = ++_generation;
    final flow = await api.beginBrowser(
      BrowserLoginRequest(device_id: deviceId!, device_name: deviceName),
    );
    if (_disposed || generation != _generation) return;
    waitingForPasskey = true;
    notifyListeners();
    if (!await openBrowser!(Uri.parse(flow.browser_url))) {
      throw const ApiFailure('browser_unavailable');
    }
    final expires = DateTime.parse(flow.expires_at);
    while (!_disposed &&
        generation == _generation &&
        DateTime.now().isBefore(expires)) {
      await Future<void>.delayed(const Duration(seconds: 3));
      if (_disposed || generation != _generation) return;
      final result = await api.pollBrowser(
        PollRequest(flow_id: flow.flow_id, poll_token: flow.poll_token),
      );
      if (result.status == 'approved') {
        if (_disposed || generation != _generation) return;
        waitingForPasskey = false;
        await _accept(result.access_token, result.expires_at);
        return;
      }
    }
    if (generation == _generation && !_disposed) {
      throw const ApiFailure('session_expired');
    }
  });
  void cancelPasskey() {
    _generation++;
    waitingForPasskey = false;
    if (!_disposed) notifyListeners();
  }

  Future<void> _readFleet({String? cursor}) async {
    final generation = _generation;
    if (cursor != null) _cursor = cursor;
    final ServerPage result;
    if (homeOnly) {
      final records = <ServerRecord>[];
      for (final id in preferences.pinned_servers) {
        records.add(await api.serverDetail(id: id));
        if (_disposed || generation != _generation) return;
      }
      result = ServerPage(
        items: records,
        next_cursor: '',
        observed_at: DateTime.now().toUtc().toIso8601String(),
      );
    } else {
      result = await api.servers(cursor: _cursor);
    }
    if (_disposed || generation != _generation) return;
    servers = result;
    page = AgentPage(
      items: [],
      next_cursor: result.next_cursor,
      observed_at: result.observed_at,
    );
  }

  Future<void> refresh({bool nextPage = false}) => _perform(
    () => _readFleet(cursor: nextPage ? servers?.next_cursor ?? '' : _cursor),
  );
  Future<void> showFleet({required bool pinned}) {
    homeOnly = pinned;
    _cursor = '';
    return refresh();
  }

  Future<void> loadDevices() => _perform(() async {
    devices = await api.devices();
  });
  Future<void> loadAudit({bool nextPage = false}) => _perform(() async {
    audit = await api.audit(cursor: nextPage ? audit?.next_cursor ?? '' : '');
  });
  Future<void> pin(String id, bool selected) => _perform(() async {
    final ids = {...preferences.pinned_servers};
    if (selected) {
      ids.add(id);
    } else {
      ids.remove(id);
    }
    preferences = await api.savePreferences(
      DevicePreferences(
        pinned_servers: ids.toList(),
        pinned_workers: preferences.pinned_workers,
      ),
    );
  });
  Future<void> revokeServer(String id) => _perform(() async {
    await api.revokeServer(id: id);
    await _readFleet();
  });
  Future<void> capabilities(String id, List<String> caps) => _perform(() async {
    await api.setCapabilities(Capabilities(capabilities: caps), id: id);
    await _readFleet();
  });
  Future<void> revokeDevice(String id) => _perform(() async {
    await api.revokeDevice(id: id);
    if (id == deviceId) {
      lock();
    } else {
      devices = await api.devices();
    }
  });
  Future<void> logout() => _perform(() async {
    try {
      await api.logout();
    } finally {
      lock();
      await cache?.clear();
    }
  });
  Future<T?> securityAction<T>(Future<T> Function() action) async {
    final generation = _generation;
    T? result;
    await _perform(() async {
      result = await action();
    });
    return _disposed || generation != _generation ? null : result;
  }

  Future<void> _perform(Future<void> Function() action) async {
    if (busy || _disposed) return;
    busy = true;
    notifyListeners();
    try {
      await action();
      error = null;
    } on ApiFailure catch (failure) {
      error = failure.status == 401 ? 'unauthorized' : failure.code;
      if (failure.status == 401) {
        lock();
        try {
          await cache?.clear();
        } catch (_) {
          cache?.error = 'cache_unavailable';
        }
        try {
          await mobile?.disableNotifications();
        } catch (_) {
          /* Server revocation is authoritative. */
        }
      }
    } on PlatformException catch (failure) {
      error = failure.code;
    } catch (_) {
      error = 'invalid_response';
    } finally {
      busy = false;
      waitingForPasskey = false;
      if (!_disposed) notifyListeners();
    }
  }

  @override
  void dispose() {
    _disposed = true;
    lock();
    close();
    super.dispose();
  }
}
