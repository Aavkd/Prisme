import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:prisme_ui/prisme_ui.dart';

void main() {
  test('Persisted preferences tolerate old or unsupported values', () {
    final prefs = PrismePreferences.fromJson({
      'appearance': 'removed-theme',
      'density': 42,
      'locale': 'de',
    });
    addTearDown(prefs.dispose);
    expect(prefs.appearance, PrismeAppearance.system);
    expect(prefs.density, PrismeDensity.comfortable);
    expect(prefs.locale, isNull);
    prefs.appearance = PrismeAppearance.matrix;
    prefs.density = PrismeDensity.compact;
    prefs.locale = const Locale('en');
    final restored = PrismePreferences.fromJson(prefs.toJson());
    addTearDown(restored.dispose);
    expect(restored.toJson(), prefs.toJson());
  });

  test('Historical or partial evidence cannot present a healthy resource', () {
    for (final freshness in [
      PrismeFreshness.stale,
      PrismeFreshness.offline,
      PrismeFreshness.partial,
      PrismeFreshness.unknown,
    ]) {
      final observation = PrismeObservation(freshness: freshness);
      expect(
        observation.displayStatus(PrismeStatus.healthy),
        isNot(PrismeStatus.healthy),
      );
    }
    expect(
      () => PrismeObservation(freshness: PrismeFreshness.live),
      throwsArgumentError,
    );
    expect(
      () => PrismeObservation(freshness: PrismeFreshness.recent),
      throwsArgumentError,
    );
    final recent = PrismeObservation(
      freshness: PrismeFreshness.recent,
      observedAt: DateTime.utc(2026),
    );
    expect(recent.displayStatus(PrismeStatus.critical), PrismeStatus.critical);
    expect(recent.displayStatus(PrismeStatus.healthy), PrismeStatus.healthy);
  });

  for (final entry in {
    'light': PrismeColors.light,
    'dark': PrismeColors.dark,
    'matrix': PrismeColors.matrix,
  }.entries) {
    test(
      '${entry.key}: semantic text and action pairs meet 4.5:1 contrast',
      () {
        final c = entry.value;
        final pairs = [
          (c.text, c.surface),
          (c.muted, c.surface),
          (c.muted, c.raised),
          (c.accent, c.accentSubtle),
          (c.healthy, c.healthySubtle),
          (c.attention, c.attentionSubtle),
          (c.critical, c.criticalSubtle),
          (c.onAction, c.action),
          (c.onAction, c.actionHover),
        ];
        for (final pair in pairs) {
          final a = pair.$1.computeLuminance(), b = pair.$2.computeLuminance();
          final ratio = a > b ? (a + .05) / (b + .05) : (b + .05) / (a + .05);
          expect(
            ratio,
            greaterThanOrEqualTo(4.5),
            reason: '${pair.$1} on ${pair.$2}',
          );
        }
      },
    );
  }

  testWidgets(
    'Theme, locale and density switch without replacing screen state',
    (tester) async {
      final prefs = PrismePreferences(locale: const Locale('fr'));
      final controller = TextEditingController();
      addTearDown(prefs.dispose);
      addTearDown(controller.dispose);
      await tester.pumpWidget(
        PrismeApp(
          preferences: prefs,
          home: Scaffold(
            body: Column(
              children: [
                PrismeTextField(label: 'Project', controller: controller),
                const PrismeStatusBadge(status: PrismeStatus.healthy),
              ],
            ),
          ),
        ),
      );
      await tester.enterText(find.byType(TextField), 'Sounding Lines');
      for (final appearance in [
        PrismeAppearance.light,
        PrismeAppearance.matrix,
        PrismeAppearance.dark,
      ]) {
        prefs.appearance = appearance;
        prefs.density = PrismeDensity.compact;
        prefs.locale = const Locale('en');
        await tester.pumpAndSettle();
        final context = tester.element(find.byType(PrismeTextField));
        expect(
          context.prismeStyle.matrix,
          appearance == PrismeAppearance.matrix,
        );
        expect(context.prismeStyle.density, PrismeDensity.compact);
        expect(
          Theme.of(context).brightness,
          appearance == PrismeAppearance.light
              ? Brightness.light
              : Brightness.dark,
        );
        expect(
          Theme.of(context).textTheme.bodyMedium!.fontFamily,
          appearance == PrismeAppearance.matrix
              ? PrismeFonts.mono
              : PrismeFonts.ui,
        );
        expect(controller.text, 'Sounding Lines');
        expect(find.text('Online'), findsOneWidget);
        expect(tester.takeException(), isNull);
      }
    },
  );
}
