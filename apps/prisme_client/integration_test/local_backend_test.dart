import 'dart:io';
import 'dart:ui' as ui;
import 'package:flutter/material.dart';
import 'package:flutter/rendering.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:prisme_ui/prisme_ui.dart';
import 'package:integration_test/integration_test.dart';
import 'package:prisme_client/api/connection.dart';
import 'package:prisme_client/foundation/controller.dart';
import 'package:prisme_client/main.dart';

void main() {
  IntegrationTestWidgetsFlutterBinding.ensureInitialized();
  testWidgets('native Flutter authenticates and reads the local Go backend', (
    tester,
  ) async {
    final dir = Platform.isAndroid
        ? '/data/data/dev.mantara.prisme_client/files/phase1'
        : Platform.environment['PRISME_SMOKE_DIR'];
    if (dir == null) {
      throw StateError('Set PRISME_SMOKE_DIR to the absolute .dev directory');
    }
    if (Platform.isAndroid) {
      await Directory('$dir/pki').create(recursive: true);
      await File('$dir/awaiting-input').writeAsString('ready');
      for (
        var attempt = 0;
        attempt < 90 && !await File('$dir/bootstrap-secret').exists();
        attempt++
      ) {
        await Future<void>.delayed(const Duration(seconds: 1));
      }
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
    final capture = GlobalKey();
    await tester.pumpWidget(
      RepaintBoundary(
        key: capture,
        child: PrismeClientApp(controller: controller),
      ),
    );
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
    expect(
      controller.servers!.items.any((s) => s.name == 'OVH Mantara'),
      isTrue,
    );
    await tester.scrollUntilVisible(
      find.text('OVH Mantara'),
      300,
      scrollable: find
          .descendant(
            of: find.byType(PrismePageFrame),
            matching: find.byType(Scrollable),
          )
          .first,
    );
    await tester.pumpAndSettle();
    await Scrollable.ensureVisible(
      tester.element(find.text('OVH Mantara')),
      alignment: 0.2,
    );
    await tester.pumpAndSettle();
    await tester.tap(find.text('OVH Mantara'));
    await tester.pumpAndSettle();
    await tester.scrollUntilVisible(
      find.text('Découverte Compose'),
      300,
      scrollable: find
          .descendant(
            of: find.byType(PrismePageFrame),
            matching: find.byType(Scrollable),
          )
          .first,
    );
    expect(find.text('Découverte Compose'), findsOneWidget);
    final target =
        capture.currentContext!.findRenderObject()! as RenderRepaintBoundary;
    final image = await target.toImage(pixelRatio: 1);
    final bytes = await image.toByteData(format: ui.ImageByteFormat.png);
    await File(
      '$dir/phase1-${Platform.operatingSystem}.png',
    ).writeAsBytes(bytes!.buffer.asUint8List());
    image.dispose();
    if (Platform.isAndroid) {
      await File('$dir/capture-ready').writeAsString('ready');
      for (
        var attempt = 0;
        attempt < 30 && !await File('$dir/capture-copied').exists();
        attempt++
      ) {
        await Future<void>.delayed(const Duration(seconds: 1));
      }
    }
    await controller.logout();
    await tester.pumpAndSettle();
    expect(find.text('Se connecter'), findsOneWidget);
    await tester.pumpWidget(const SizedBox.shrink());
  });
}
