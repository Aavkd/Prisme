import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:prisme_ui/prisme_ui.dart';
import 'package:prisme_ui_catalog/main.dart';

void main() {
  testWidgets(
    'Catalogue boots with the shared theme and no feature integrations',
    (tester) async {
      await tester.pumpWidget(const PrismeCatalog());
      await tester.pumpAndSettle();
      expect(find.byType(PrismeAppShell), findsOneWidget);
      expect(find.text('PRISME / DESIGN SYSTEM'), findsOneWidget);
      expect(tester.takeException(), isNull);
    },
  );
  final configurations = [
    for (final appearance in [
      PrismeAppearance.light,
      PrismeAppearance.dark,
      PrismeAppearance.matrix,
    ])
      for (final width in [360.0, 1440.0])
        (appearance: appearance, width: width, scale: 1.0),
    (appearance: PrismeAppearance.matrix, width: 320.0, scale: 2.0),
    (appearance: PrismeAppearance.light, width: 1024.0, scale: 2.0),
  ];
  for (final config in configurations) {
    testWidgets(
      'Every specimen: ${config.appearance.name}, ${config.width}px, text ×${config.scale}',
      (tester) async {
        final report = FlutterError.onError!;
        FlutterError.onError = (details) {
          debugPrint(details.toString());
          report(details);
        };
        tester.view.physicalSize = Size(config.width, 900);
        tester.view.devicePixelRatio = 1;
        addTearDown(tester.view.resetPhysicalSize);
        addTearDown(tester.view.resetDevicePixelRatio);
        final preferences = PrismePreferences(
          appearance: config.appearance,
          locale: const Locale('fr'),
        );
        addTearDown(preferences.dispose);
        await tester.pumpWidget(
          PrismeApp(
            preferences: preferences,
            builder: (context, child) => MediaQuery(
              data: MediaQuery.of(
                context,
              ).copyWith(textScaler: TextScaler.linear(config.scale)),
              child: child!,
            ),
            home: CatalogScreen(preferences: preferences),
          ),
        );
        await tester.pumpAndSettle();
        expect(tester.takeException(), isNull);
        for (final id in [
          'actions',
          'forms',
          'states',
          'resources',
          'data',
          'overlays',
          'foundations',
        ]) {
          tester
              .widget<PrismeAppShell>(find.byType(PrismeAppShell))
              .onDestinationSelected(id);
          await tester.pump();
          await tester.pump(const Duration(milliseconds: 300));
          expect(tester.takeException(), isNull, reason: id);
          final page = find
              .descendant(
                of: find.byType(PrismePageFrame),
                matching: find.byType(Scrollable),
              )
              .first;
          final scroll = tester.state<ScrollableState>(page).position;
          for (
            var i = 0;
            i < 30 && scroll.pixels < scroll.maxScrollExtent;
            i++
          ) {
            await tester.dragFrom(
              tester.getRect(page).centerLeft + const Offset(8, 0),
              const Offset(0, -500),
            );
            await tester.pump(const Duration(milliseconds: 300));
            expect(
              tester.takeException(),
              isNull,
              reason: '$id, scroll ${scroll.pixels}',
            );
          }
          expect(
            scroll.extentAfter,
            lessThan(1),
            reason: '$id must remain fully reachable',
          );
        }
      },
    );
  }
}
