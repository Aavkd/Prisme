import 'dart:async';
import 'package:flutter/foundation.dart';
import '../api/connection.dart';
import '../api/generated.dart';

class FoundationController extends ChangeNotifier {
  FoundationController(this.api, {required this.setToken, required this.close});
  final PrismeApi api;
  final void Function(String?) setToken;
  final VoidCallback close;
  Identity? identity;
  AgentPage? page;
  bool busy = false;
  String? error;
  Timer? _expiry;
  bool _disposed = false;
  bool get connected => identity != null && page != null && error == null;

  Future<void> login(String secret, String deviceName) => _perform(() async {
    final session = await api.login(
      LoginRequest(secret: secret, device_name: deviceName),
    );
    if (_disposed) return;
    setToken(session.access_token);
    _expiry?.cancel();
    final remaining = DateTime.parse(
      session.expires_at,
    ).difference(DateTime.now());
    _expiry = Timer(remaining.isNegative ? Duration.zero : remaining, () {
      setToken(null);
      identity = null;
      page = null;
      error = 'session_expired';
      if (!_disposed) notifyListeners();
    });
    try {
      final result = await api.me();
      final agents = await api.agents();
      if (_disposed) return;
      identity = result;
      page = agents;
    } catch (_) {
      setToken(null);
      _expiry?.cancel();
      rethrow;
    }
  });
  Future<void> refresh({bool nextPage = false}) => _perform(() async {
    final result = await api.agents(
      cursor: nextPage ? page?.next_cursor ?? '' : '',
    );
    if (!_disposed) page = result;
  });
  Future<void> logout() => _perform(() async {
    await api.logout();
    setToken(null);
    _expiry?.cancel();
    identity = null;
    page = null;
  });
  Future<void> _perform(Future<void> Function() action) async {
    if (busy || _disposed) return;
    busy = true;
    error = null;
    notifyListeners();
    try {
      await action();
    } on ApiFailure catch (failure) {
      error = failure.status == 401 ? 'unauthorized' : failure.code;
      if (failure.status == 401) {
        setToken(null);
        _expiry?.cancel();
        identity = null;
        page = null;
      }
    } catch (_) {
      error = 'invalid_response';
    } finally {
      busy = false;
      if (!_disposed) notifyListeners();
    }
  }

  @override
  void dispose() {
    _disposed = true;
    _expiry?.cancel();
    close();
    super.dispose();
  }
}
