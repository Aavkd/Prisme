import 'dart:io';
import 'package:prisme_client/api/connection.dart';

// Local-only TLS/WebSocket qualification. Argument is an existing development
// PKI directory; private keys are read locally and never printed or transmitted.
Future<void> main(List<String> args) async {
  if (args.length != 1) {
    throw ArgumentError('development PKI directory required');
  }
  final dir = args.single;
  final tls = SecurityContext()
    ..useCertificateChain('$dir/server.pem')
    ..usePrivateKey('$dir/server-key.pem');
  final server = await HttpServer.bindSecure(
    InternetAddress.loopbackIPv4,
    0,
    tls,
  );
  var redirect = false, unexpected = false;
  server.listen((request) async {
    if (request.uri.path == '/unexpected') {
      unexpected = true;
      request.response.statusCode = 403;
      await request.response.close();
      return;
    }
    if (redirect) {
      request.response.statusCode = 302;
      request.response.headers.set(
        'Location',
        'https://localhost:${server.port}/unexpected',
      );
      await request.response.close();
      return;
    }
    if (request.headers.value('Authorization') != 'Bearer local-fixture' ||
        request.headers.value('X-Prisme-Terminal-Ticket') !=
            'one-use-fixture') {
      request.response.statusCode = 403;
      await request.response.close();
      return;
    }
    final ws = await WebSocketTransformer.upgrade(request);
    ws.listen((value) {
      if (value == 'ping') ws.add('pong');
    });
  });
  final connection = ApiConnection(
    'https://localhost:${server.port}',
    trustedCa: await File('$dir/ca.pem').readAsBytes(),
  )..token = 'local-fixture';
  try {
    final socket = await connection.openTerminalSocket(
      'a' * 64,
      'one-use-fixture',
    );
    socket.add('ping');
    if (await socket.first != 'pong') {
      throw StateError('WebSocket transport did not preserve frames');
    }
    await socket.close();
    redirect = true;
    var rejected = false;
    try {
      final socket = await connection.openTerminalSocket(
        'a' * 64,
        'one-use-fixture',
      );
      await socket.close();
    } on ApiFailure {
      rejected = true;
    }
    if (!rejected || unexpected) {
      throw StateError('WebSocket followed a redirect');
    }
    final untrusted = ApiConnection('https://localhost:${server.port}')
      ..token = 'local-fixture';
    try {
      rejected = false;
      try {
        final socket = await untrusted.openTerminalSocket(
          'a' * 64,
          'one-use-fixture',
        );
        await socket.close();
      } on ApiFailure {
        rejected = true;
      }
      if (!rejected) throw StateError('Untrusted TLS accepted');
    } finally {
      untrusted.close();
    }
    stdout.writeln(
      'PASS: native Dart WSS frames, credential headers, redirect rejection and TLS trust.',
    );
  } finally {
    connection.close();
    await server.close(force: true);
  }
}
