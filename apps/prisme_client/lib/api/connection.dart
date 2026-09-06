import 'dart:async';
import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';

import 'generated.dart';

class ApiFailure implements Exception {
  const ApiFailure(this.code, [this.status]);
  final String code;
  final int? status;
  @override
  String toString() => 'ApiFailure($code)';
}

/// Memory-only credentials. Trust is explicit; no certificate bypass or redirects.
class ApiConnection {
  ApiConnection(String origin, {Uint8List? trustedCa}) {
    final uri = Uri.parse(origin);
    if (uri.scheme != 'https' ||
        uri.host.isEmpty ||
        uri.userInfo.isNotEmpty ||
        uri.hasQuery ||
        uri.hasFragment ||
        (uri.path.isNotEmpty && uri.path != '/')) {
      throw const ApiFailure('invalid_origin');
    }
    _origin = uri.origin;
    final context = SecurityContext(withTrustedRoots: trustedCa == null);
    if (trustedCa != null) context.setTrustedCertificatesBytes(trustedCa);
    _http = HttpClient(context: context)
      ..connectionTimeout = const Duration(seconds: 5)
      ..idleTimeout = const Duration(seconds: 10);
  }
  late final String _origin;
  late final HttpClient _http;
  String? token;
  late final PrismeApi api = PrismeApi(send);

  Future<Map<String, dynamic>> send(
    String method,
    String path, {
    Map<String, dynamic>? body,
    String? idempotencyKey,
  }) async {
    HttpClientRequest? pending;
    try {
      return await (() async {
        final request = await _http.openUrl(method, Uri.parse('$_origin$path'));
        pending = request;
        request.followRedirects = false;
        request.headers.set(HttpHeaders.acceptHeader, 'application/json');
        if (token != null) {
          request.headers.set(HttpHeaders.authorizationHeader, 'Bearer $token');
        }
        if (idempotencyKey != null) {
          request.headers.set('Idempotency-Key', idempotencyKey);
        }
        if (body != null) {
          request.headers.contentType = ContentType.json;
          request.write(jsonEncode(body));
        }
        final response = await request.close();
        final bytes = <int>[];
        await for (final chunk in response) {
          bytes.addAll(chunk);
          if (bytes.length > 256 * 1024) {
            throw const ApiFailure('invalid_response');
          }
        }
        if (response.statusCode == 401) token = null;
        if (response.statusCode < 200 || response.statusCode >= 300) {
          throw ApiFailure('request_rejected', response.statusCode);
        }
        if (response.statusCode == 204) return <String, dynamic>{};
        return jsonDecode(utf8.decode(bytes)) as Map<String, dynamic>;
      })().timeout(const Duration(seconds: 8));
    } on TimeoutException {
      pending?.abort();
      throw const ApiFailure('timeout');
    } on ApiFailure {
      rethrow;
    } on HandshakeException {
      throw const ApiFailure('untrusted_server');
    } on SocketException {
      throw const ApiFailure('offline');
    } on HttpException {
      throw const ApiFailure('offline');
    } on FormatException {
      throw const ApiFailure('invalid_response');
    } on TypeError {
      throw const ApiFailure('invalid_response');
    }
  }

  void close() {
    token = null;
    _http.close(force: true);
  }
}
