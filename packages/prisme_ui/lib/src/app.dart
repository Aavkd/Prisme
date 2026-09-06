import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'foundation/preferences.dart';
import 'foundation/theme.dart';
import 'foundation/tokens.dart';
import 'l10n/strings.dart';

/// Entry point for either a normal Navigator app or a future Router integration.
/// [preferences] remains owned/disposed by the caller. No feature routes are added.
class PrismeApp extends StatelessWidget {
  const PrismeApp({
    super.key,
    required this.preferences,
    this.home,
    this.routerConfig,
    this.title = 'Prisme',
    this.builder,
    this.navigatorObservers = const [],
  }) : assert(
         (home == null) != (routerConfig == null),
         'Provide home or routerConfig, exclusively.',
       );
  final PrismePreferences preferences;
  final Widget? home;
  final RouterConfig<Object>? routerConfig;
  final String title;
  final TransitionBuilder? builder;
  final List<NavigatorObserver> navigatorObservers;

  @override
  Widget build(BuildContext context) => ListenableBuilder(
    listenable: preferences,
    builder: (context, _) {
      final theme = PrismeTheme.build(
        brightness: Brightness.light,
        matrix: preferences.appearance == PrismeAppearance.matrix,
        density: preferences.density,
      );
      final darkTheme = PrismeTheme.build(
        brightness: Brightness.dark,
        matrix: preferences.appearance == PrismeAppearance.matrix,
        density: preferences.density,
      );
      final mode = switch (preferences.appearance) {
        PrismeAppearance.system => ThemeMode.system,
        PrismeAppearance.light => ThemeMode.light,
        _ => ThemeMode.dark,
      };
      const delegates = <LocalizationsDelegate<dynamic>>[
        PrismeStrings.delegate,
        GlobalMaterialLocalizations.delegate,
        GlobalWidgetsLocalizations.delegate,
        GlobalCupertinoLocalizations.delegate,
      ];
      // Zero-duration theme changes avoid incompatible font metrics during transitions.
      if (routerConfig != null) {
        return MaterialApp.router(
          title: title,
          routerConfig: routerConfig,
          theme: theme,
          darkTheme: darkTheme,
          themeMode: mode,
          themeAnimationDuration: Duration.zero,
          locale: preferences.locale,
          supportedLocales: PrismeStrings.supportedLocales,
          localizationsDelegates: delegates,
          builder: builder,
          debugShowCheckedModeBanner: false,
        );
      }
      return MaterialApp(
        title: title,
        home: home,
        navigatorObservers: navigatorObservers,
        theme: theme,
        darkTheme: darkTheme,
        themeMode: mode,
        themeAnimationDuration: Duration.zero,
        locale: preferences.locale,
        supportedLocales: PrismeStrings.supportedLocales,
        localizationsDelegates: delegates,
        builder: builder,
        debugShowCheckedModeBanner: false,
      );
    },
  );
}
