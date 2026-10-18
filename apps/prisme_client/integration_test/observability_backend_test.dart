import 'dart:io';
import 'dart:ui' as ui;
import 'package:flutter/material.dart';
import 'package:flutter/rendering.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:integration_test/integration_test.dart';
import 'package:prisme_ui/prisme_ui.dart';
import 'package:prisme_client/api/connection.dart';
import 'package:prisme_client/foundation/controller.dart';
import 'package:prisme_client/main.dart';

void main() {
  IntegrationTestWidgetsFlutterBinding.ensureInitialized();
  testWidgets(
    'native diagnostic view reads actual telemetry and incident history',
    (tester) async {
      final dir = Platform.environment['PRISME_SMOKE_DIR'];
      if (dir == null) {
        throw StateError('Set PRISME_SMOKE_DIR to the absolute .dev directory');
      }
      final connection = ApiConnection(
        'https://localhost:8443',
        trustedCa: await File('$dir/pki/ca.pem').readAsBytes(),
      );
      final c = FoundationController(
        connection.api,
        setToken: (v) => connection.token = v,
        close: connection.close,
      );
      final capture = GlobalKey();
      await tester.pumpWidget(
        RepaintBoundary(
          key: capture,
          child: PrismeClientApp(controller: c),
        ),
      );
      await tester.enterText(
        find.byType(TextField).last,
        (await File('$dir/bootstrap-secret').readAsString()).trim(),
      );
      await tester.tap(find.text('Se connecter'));
      Future<void> settleNetwork() async {
        await tester.pump(const Duration(milliseconds: 100));
        for (var n = 0; n < 200 && c.busy; n++) {
          await tester.pump(const Duration(milliseconds: 100));
        }
        await tester.pumpAndSettle();
        expect(c.error, isNull);
      }

      await settleNetwork();
      expect(c.identity, isNotNull);
      await tester.tap(find.text('Diagnostic'));
      await settleNetwork();
      expect(find.text('Observabilité et incidents'), findsOneWidget);
      final target = find.text('Local pilot HTTP unavailable').last;
      await tester.scrollUntilVisible(
        target,
        250,
        scrollable: find
            .descendant(
              of: find.byType(PrismePageFrame),
              matching: find.byType(Scrollable),
            )
            .first,
      );
      await tester.tap(target);
      await settleNetwork();
      expect(find.text('Chronologie'), findsOneWidget);
      expect(find.textContaining('Résolu'), findsWidgets);
      final boundary =
          capture.currentContext!.findRenderObject()! as RenderRepaintBoundary;
      final image = await boundary.toImage(pixelRatio: 1);
      final bytes = await image.toByteData(format: ui.ImageByteFormat.png);
      await File(
        '$dir/phase2-windows.png',
      ).writeAsBytes(bytes!.buffer.asUint8List());
      image.dispose();
      await c.logout();
      await tester.pumpAndSettle();
      await tester.pumpWidget(const SizedBox.shrink());
    },
  );
}
