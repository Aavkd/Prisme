import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:prisme_ui/prisme_ui.dart';

Future<void> mount(
  WidgetTester tester,
  Widget child, {
  bool reducedMotion = false,
}) async {
  final prefs = PrismePreferences(locale: const Locale('en'));
  addTearDown(prefs.dispose);
  await tester.pumpWidget(
    PrismeApp(
      preferences: prefs,
      builder: (context, child) => MediaQuery(
        data: MediaQuery.of(context).copyWith(disableAnimations: reducedMotion),
        child: child!,
      ),
      home: Scaffold(
        body: Padding(padding: const EdgeInsets.all(16), child: child),
      ),
    ),
  );
  await tester.pump();
}

void main() {
  testWidgets('Unknown measurements and progress never become a zero', (
    tester,
  ) async {
    await mount(
      tester,
      const Column(
        children: [
          PrismeMetric(label: 'GPU', unit: '%'),
          PrismeProgress(),
          PrismeCapacityBar(label: 'RAM', unit: 'GiB'),
          PrismeSparkline(
            samples: [null, double.nan],
            label: 'CPU',
            unit: '%',
            startLabel: '-15m',
            endLabel: 'Now',
          ),
        ],
      ),
    );
    expect(find.byType(LinearProgressIndicator), findsNothing);
    expect(find.text('—'), findsOneWidget);
    expect(find.text('0'), findsNothing);
    expect(find.text('0%'), findsNothing);
  });

  testWidgets(
    'Loading action prevents duplicate activation and respects reduced motion',
    (tester) async {
      var calls = 0;
      await mount(
        tester,
        PrismeButton(label: 'Run', onPressed: () => calls++, loading: true),
        reducedMotion: true,
      );
      await tester.tap(find.text('Run'));
      await tester.pump();
      expect(calls, 0);
      expect(find.byType(CircularProgressIndicator), findsNothing);
      expect(find.byIcon(LucideIcons.hourglass), findsOneWidget);
    },
  );

  testWidgets('Android actions retain a 48dp hit target in compact density', (
    tester,
  ) async {
    final prefs = PrismePreferences(density: PrismeDensity.compact);
    addTearDown(prefs.dispose);
    await tester.pumpWidget(
      PrismeApp(
        preferences: prefs,
        home: Builder(
          builder: (context) => Theme(
            data: Theme.of(context).copyWith(platform: TargetPlatform.android),
            child: Scaffold(
              body: Column(
                children: [
                  PrismeButton(label: 'Run', onPressed: () {}),
                  PrismeIconButton(
                    icon: LucideIcons.plus,
                    label: 'Add',
                    onPressed: () {},
                  ),
                ],
              ),
            ),
          ),
        ),
      ),
    );
    expect(
      tester.getSize(find.byType(OutlinedButton)).height,
      greaterThanOrEqualTo(48),
    );
    expect(
      tester.getSize(find.byType(IconButton)).height,
      greaterThanOrEqualTo(48),
    );
  });

  testWidgets(
    'Palette skips disabled commands, filters and returns an ID with Enter',
    (tester) async {
      String? chosen;
      await mount(
        tester,
        Builder(
          builder: (context) => PrismeButton(
            label: 'Open',
            onPressed: () async {
              chosen = await showPrismeCommandPalette(
                context: context,
                commands: const [
                  PrismeCommand(
                    id: 'a',
                    label: 'Machines',
                    icon: LucideIcons.server,
                  ),
                  PrismeCommand(
                    id: 'b',
                    label: 'Forbidden',
                    icon: LucideIcons.lock,
                    enabled: false,
                  ),
                  PrismeCommand(
                    id: 'c',
                    label: 'Workers',
                    icon: LucideIcons.bot,
                  ),
                ],
              );
            },
          ),
        ),
      );
      await tester.tap(find.text('Open'));
      await tester.pumpAndSettle();
      await tester.sendKeyEvent(LogicalKeyboardKey.arrowDown);
      await tester.sendKeyEvent(LogicalKeyboardKey.enter);
      await tester.pumpAndSettle();
      expect(chosen, 'c');
      await tester.tap(find.text('Open'));
      await tester.pumpAndSettle();
      await tester.enterText(find.byType(TextField), 'mach');
      await tester.pump();
      expect(find.text('Workers'), findsNothing);
      await tester.sendKeyEvent(LogicalKeyboardKey.enter);
      await tester.pumpAndSettle();
      expect(chosen, 'a');
    },
  );

  testWidgets(
    'Confirmation requires the exact target phrase and cancellation returns false',
    (tester) async {
      bool? result;
      await mount(
        tester,
        Builder(
          builder: (context) => PrismeButton(
            label: 'Open',
            onPressed: () async {
              result = await showPrismeConfirmation(
                context: context,
                plan: const PrismeActionPlan(
                  title: 'Restart worker',
                  actionLabel: 'Confirm restart',
                  target: 'production / worker',
                  environment: PrismeEnvironment.production,
                  impact: 'Interrupted task',
                  verification: 'Observe queue',
                  rollback: 'Previous image',
                  requiredPhrase: 'worker',
                ),
              );
            },
          ),
        ),
      );
      await tester.tap(find.text('Open'));
      await tester.pumpAndSettle();
      Finder confirm = find.widgetWithText(FilledButton, 'Confirm restart');
      expect(tester.widget<FilledButton>(confirm).onPressed, isNull);
      await tester.enterText(find.byType(TextField), 'Worker');
      await tester.pump();
      expect(tester.widget<FilledButton>(confirm).onPressed, isNull);
      await tester.enterText(find.byType(TextField), 'worker');
      await tester.pump();
      await tester.ensureVisible(confirm);
      await tester.tap(confirm);
      await tester.pumpAndSettle();
      expect(result, isTrue);
      await tester.tap(find.text('Open'));
      await tester.pumpAndSettle();
      await tester.sendKeyEvent(LogicalKeyboardKey.escape);
      await tester.pumpAndSettle();
      expect(result, isFalse);
    },
  );

  testWidgets('Revealed secret is concealed on focus loss and app inactivity', (
    tester,
  ) async {
    final controller = TextEditingController(text: 'demo-value');
    addTearDown(controller.dispose);
    await mount(
      tester,
      Column(
        children: [
          PrismeSecretField(label: 'Secret', controller: controller),
          const PrismeTextField(label: 'Another field'),
        ],
      ),
    );
    final secret = find.descendant(
      of: find.byType(PrismeSecretField),
      matching: find.byType(TextField),
    );
    expect(tester.widget<TextField>(secret).obscureText, isTrue);
    await tester.tap(find.byIcon(LucideIcons.eye));
    await tester.pumpAndSettle();
    expect(tester.widget<TextField>(secret).obscureText, isFalse);
    await tester.tap(
      find.descendant(
        of: find.byType(PrismeTextField),
        matching: find.byType(TextField),
      ),
    );
    await tester.pumpAndSettle();
    expect(tester.widget<TextField>(secret).obscureText, isTrue);
    await tester.tap(find.byIcon(LucideIcons.eye));
    await tester.pumpAndSettle();
    tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.inactive);
    await tester.pump();
    expect(tester.widget<TextField>(secret).obscureText, isTrue);
    tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.resumed);
    await tester.pumpWidget(const SizedBox());
  });

  testWidgets('Mobile More menu exposes every destination', (tester) async {
    tester.view.physicalSize = const Size(360, 800);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    String selected = '0';
    await mount(
      tester,
      PrismeAppShell(
        destinations: [
          for (var i = 0; i < 7; i++)
            PrismeDestination(
              id: '$i',
              label: 'Section $i',
              icon: LucideIcons.box,
            ),
        ],
        selectedId: selected,
        onDestinationSelected: (id) => selected = id,
        body: const SizedBox(),
      ),
    );
    await tester.tap(find.text('More'));
    await tester.pumpAndSettle();
    expect(find.text('Section 4'), findsOneWidget);
    expect(find.text('Section 5'), findsOneWidget);
    await tester.tap(find.text('Section 6'));
    await tester.pumpAndSettle();
    expect(selected, '6');
    expect(find.byType(BottomSheet), findsNothing);
  });
}
