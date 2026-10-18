// Exercises the generated Dart API against real TLS/Go/PostgreSQL and the Go agent.
import 'dart:io';
import 'package:prisme_client/api/connection.dart';
import 'package:prisme_client/api/generated.dart';

Future<void> main(List<String> args) async {
  final root = File.fromUri(Platform.script).parent.parent.parent.parent.path;
  final dir = '$root/.dev';
  final connection = ApiConnection(
    'https://localhost:8443',
    trustedCa: await File('$dir/pki/ca.pem').readAsBytes(),
  );
  try {
    final session = await connection.api.login(
      LoginRequest(
        secret: (await File('$dir/bootstrap-secret').readAsString()).trim(),
        device_name: 'dart-smoke-client',
      ),
    );
    connection.token = session.access_token;
    final me = await connection.api.me();
    if (me.auth_mode != 'development') throw StateError('Unexpected identity');
    final key = 'smoke-${DateTime.now().microsecondsSinceEpoch}';
    final invitation = await connection.api.invite(
      const Capabilities(capabilities: ["self:heartbeat"]),
      idempotencyKey: key,
    );
    if (args.contains('--invite-only')) {
      await File(
        '$dir/enrollment-token',
      ).writeAsString(invitation.token, flush: true);
      stdout.writeln(
        'Ten-minute test enrollment token saved privately in .dev/enrollment-token.',
      );
    } else {
      final state = '$dir/$key';
      await Directory(state).create();
      await File('$state/token').writeAsString(invitation.token, flush: true);
      final result = await Process.run('go', [
        'run',
        './cmd/agent',
        '-once',
        '-ca',
        '$dir/pki/ca.pem',
        '-token-file',
        '$state/token',
        '-state-dir',
        state,
        '-name',
        key,
      ], workingDirectory: root);
      if (result.exitCode != 0) {
        throw StateError('Go test agent failed; credentials omitted');
      }
      final page = await connection.api.agents();
      if (!page.items.any((a) => a.name == key && a.last_seen_at != null)) {
        throw StateError('Agent heartbeat missing from API');
      }
      stdout.writeln(
        'PASS: generated Dart client authenticated; Go agent enrolled and sent an mTLS heartbeat.',
      );
    }
    await connection.api.logout();
    connection.token = session.access_token;
    try {
      await connection.api.me();
      throw StateError('Revoked session accepted');
    } on ApiFailure catch (failure) {
      if (failure.status != 401) rethrow;
    }
    stdout.writeln('PASS: logout revoked the session.');
  } finally {
    connection.close();
  }
}
