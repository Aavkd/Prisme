import 'dart:io';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:integration_test/integration_test.dart';
import 'package:prisme_client/api/connection.dart';
import 'package:prisme_client/foundation/controller.dart';
import 'package:prisme_client/main.dart';

void main() {
  IntegrationTestWidgetsFlutterBinding.ensureInitialized();
  testWidgets('native Flutter authenticates and reads the local Go backend', (
    tester,
  ) async {
    final dir = Platform.environment['PRISME_SMOKE_DIR'];
    if (dir == null) {
      throw StateError('Set PRISME_SMOKE_DIR to the absolute .dev directory');
    }
    final connection = ApiConnection(
      'https://localhost:8443',
      trustedCa: await File('$dir/pki/ca.pem').readAsBytes(),
    );
    final controller = FoundationController(
      connection.api,
      setToken: (v) => connection.token = v,
      close: connection.close,
    );
    await tester.pumpWidget(PrismeClientApp(controller: controller));
    await tester.enterText(
      find.byType(TextField).last,
      (await File('$dir/bootstrap-secret').readAsString()).trim(),
    );
    await tester.tap(find.text('Se connecter'));
    for (
      var attempt = 0;
      attempt < 100 && (controller.busy || controller.identity == null);
      attempt++
    ) {
      await tester.pump(const Duration(milliseconds: 100));
    }
    expect(controller.error, isNull);
    expect(controller.identity?.auth_mode, 'development');
    expect(controller.page, isNotNull);
    expect(find.text('Backend connecté'), findsOneWidget);
    await controller.logout();
    await tester.pumpAndSettle();
    expect(find.text('Se connecter'), findsOneWidget);
    await tester.pumpWidget(const SizedBox.shrink());
  });
}
