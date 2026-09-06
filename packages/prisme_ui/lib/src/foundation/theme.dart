import 'package:flutter/material.dart';
import 'colors.dart';
import 'tokens.dart';

abstract final class PrismeTheme {
  static ThemeData build({
    required Brightness brightness,
    bool matrix = false,
    PrismeDensity density = PrismeDensity.comfortable,
  }) {
    final c = matrix
        ? PrismeColors.matrix
        : brightness == Brightness.dark
        ? PrismeColors.dark
        : PrismeColors.light;
    final dark = matrix || brightness == Brightness.dark;
    final family = matrix ? PrismeFonts.mono : PrismeFonts.ui;
    final style = PrismeStyle(density: density, matrix: matrix);
    TextStyle text(
      double size, {
      FontWeight weight = FontWeight.w400,
      Color? color,
    }) => TextStyle(
      fontFamily: family,
      fontSize: size,
      height: 1.5,
      fontWeight: weight,
      color: color ?? c.text,
      letterSpacing: matrix ? -.4 : 0,
    );
    final scheme = ColorScheme(
      brightness: dark ? Brightness.dark : Brightness.light,
      primary: c.action,
      onPrimary: c.onAction,
      primaryContainer: c.accentSubtle,
      onPrimaryContainer: c.accent,
      secondary: c.accent,
      onSecondary: c.background,
      secondaryContainer: c.accentSubtle,
      onSecondaryContainer: c.accent,
      tertiary: c.healthy,
      onTertiary: c.background,
      error: c.critical,
      onError: c.background,
      errorContainer: c.criticalSubtle,
      onErrorContainer: c.critical,
      surface: c.surface,
      onSurface: c.text,
      onSurfaceVariant: c.muted,
      surfaceContainerLowest: c.background,
      surfaceContainerLow: c.navigation,
      surfaceContainer: c.surface,
      surfaceContainerHigh: c.raised,
      surfaceContainerHighest: c.hover,
      outline: c.muted,
      outlineVariant: c.border,
      inverseSurface: c.text,
      onInverseSurface: c.background,
      surfaceTint: Colors.transparent,
    );
    final border = OutlineInputBorder(
      borderRadius: PrismeRadius.control,
      borderSide: BorderSide(color: c.border),
    );
    final buttonStyle = ButtonStyle(
      minimumSize: const WidgetStatePropertyAll(Size(0, 40)),
      padding: const WidgetStatePropertyAll(
        EdgeInsets.symmetric(horizontal: 14, vertical: 10),
      ),
      shape: const WidgetStatePropertyAll(
        RoundedRectangleBorder(borderRadius: PrismeRadius.control),
      ),
      textStyle: WidgetStatePropertyAll(text(13, weight: FontWeight.w500)),
      animationDuration: PrismeMotion.interaction,
      overlayColor: WidgetStateProperty.resolveWith(
        (states) => states.contains(WidgetState.focused)
            ? c.accent.withValues(alpha: .18)
            : c.accent.withValues(alpha: .08),
      ),
    );
    return ThemeData(
      useMaterial3: true,
      brightness: scheme.brightness,
      colorScheme: scheme,
      fontFamily: family,
      scaffoldBackgroundColor: c.background,
      canvasColor: c.surface,
      disabledColor: c.muted.withValues(alpha: .5),
      dividerColor: c.border,
      focusColor: c.accent.withValues(alpha: .2),
      hoverColor: c.hover,
      splashFactory: NoSplash.splashFactory,
      extensions: [c, style],
      textTheme: TextTheme(
        displaySmall: text(
          28,
          weight: FontWeight.w600,
        ).copyWith(height: 1.25, letterSpacing: -1),
        headlineMedium: text(25, weight: FontWeight.w600),
        headlineSmall: text(20, weight: FontWeight.w600),
        titleLarge: text(18, weight: FontWeight.w600),
        titleMedium: text(15, weight: FontWeight.w600),
        titleSmall: text(14, weight: FontWeight.w600),
        bodyLarge: text(16),
        bodyMedium: text(14),
        bodySmall: text(12, color: c.muted),
        labelLarge: text(13, weight: FontWeight.w500),
        labelMedium: text(12, weight: FontWeight.w500),
        labelSmall: text(11, color: c.muted),
      ),
      appBarTheme: AppBarTheme(
        backgroundColor: c.background,
        foregroundColor: c.text,
        elevation: 0,
        scrolledUnderElevation: 0,
        surfaceTintColor: Colors.transparent,
        titleTextStyle: text(15, weight: FontWeight.w600),
      ),
      cardTheme: CardThemeData(
        color: c.surface,
        elevation: 0,
        margin: EdgeInsets.zero,
        shape: RoundedRectangleBorder(
          borderRadius: PrismeRadius.surface,
          side: BorderSide(color: c.border),
        ),
      ),
      filledButtonTheme: FilledButtonThemeData(
        style: buttonStyle.copyWith(
          backgroundColor: WidgetStateProperty.resolveWith(
            (s) => s.contains(WidgetState.disabled)
                ? c.raised
                : s.contains(WidgetState.hovered)
                ? c.actionHover
                : c.action,
          ),
          foregroundColor: WidgetStateProperty.resolveWith(
            (s) => s.contains(WidgetState.disabled) ? c.muted : c.onAction,
          ),
        ),
      ),
      outlinedButtonTheme: OutlinedButtonThemeData(
        style: buttonStyle.copyWith(
          foregroundColor: WidgetStateProperty.resolveWith(
            (s) => s.contains(WidgetState.disabled) ? c.muted : c.text,
          ),
          side: WidgetStateProperty.resolveWith(
            (s) => BorderSide(
              color: s.contains(WidgetState.focused) ? c.accent : c.border,
              width: s.contains(WidgetState.focused) ? 2 : 1,
            ),
          ),
        ),
      ),
      textButtonTheme: TextButtonThemeData(
        style: buttonStyle.copyWith(
          foregroundColor: WidgetStateProperty.resolveWith(
            (s) => s.contains(WidgetState.disabled) ? c.muted : c.accent,
          ),
        ),
      ),
      iconButtonTheme: IconButtonThemeData(
        style: buttonStyle.copyWith(
          padding: const WidgetStatePropertyAll(EdgeInsets.all(10)),
          foregroundColor: WidgetStatePropertyAll(c.muted),
        ),
      ),
      inputDecorationTheme: InputDecorationTheme(
        filled: true,
        fillColor: c.background,
        isDense: density == PrismeDensity.compact,
        contentPadding: const EdgeInsets.symmetric(
          horizontal: 12,
          vertical: 14,
        ),
        border: border,
        enabledBorder: border,
        focusedBorder: border.copyWith(
          borderSide: BorderSide(color: c.accent, width: 2),
        ),
        errorBorder: border.copyWith(borderSide: BorderSide(color: c.critical)),
        focusedErrorBorder: border.copyWith(
          borderSide: BorderSide(color: c.critical, width: 2),
        ),
        labelStyle: text(13, color: c.muted),
        hintStyle: text(14, color: c.muted),
        helperStyle: text(12, color: c.muted),
        errorStyle: text(12, color: c.critical),
        errorMaxLines: 4,
        helperMaxLines: 4,
      ),
      dialogTheme: DialogThemeData(
        backgroundColor: c.surface,
        surfaceTintColor: Colors.transparent,
        shape: RoundedRectangleBorder(
          borderRadius: PrismeRadius.overlay,
          side: BorderSide(color: c.border),
        ),
      ),
      bottomSheetTheme: BottomSheetThemeData(
        backgroundColor: c.surface,
        surfaceTintColor: Colors.transparent,
        showDragHandle: true,
        dragHandleColor: c.muted,
        shape: const RoundedRectangleBorder(
          borderRadius: BorderRadius.vertical(top: Radius.circular(12)),
        ),
      ),
      navigationBarTheme: NavigationBarThemeData(
        backgroundColor: c.navigation,
        indicatorColor: c.accentSubtle,
        elevation: 0,
        labelTextStyle: WidgetStatePropertyAll(text(11)),
        iconTheme: WidgetStateProperty.resolveWith(
          (s) => IconThemeData(
            size: 20,
            color: s.contains(WidgetState.selected) ? c.accent : c.muted,
          ),
        ),
      ),
      tabBarTheme: TabBarThemeData(
        labelColor: c.accent,
        unselectedLabelColor: c.muted,
        indicatorColor: c.accent,
        dividerColor: c.border,
        labelStyle: text(13, weight: FontWeight.w500),
      ),
      popupMenuTheme: PopupMenuThemeData(
        color: c.surface,
        surfaceTintColor: Colors.transparent,
        textStyle: text(13),
        shape: RoundedRectangleBorder(
          borderRadius: PrismeRadius.control,
          side: BorderSide(color: c.border),
        ),
      ),
      tooltipTheme: TooltipThemeData(
        decoration: BoxDecoration(
          color: c.raised,
          borderRadius: PrismeRadius.control,
          border: Border.all(color: c.border),
        ),
        textStyle: text(12),
      ),
      snackBarTheme: SnackBarThemeData(
        backgroundColor: c.raised,
        contentTextStyle: text(14),
        actionTextColor: c.accent,
        behavior: SnackBarBehavior.floating,
        shape: RoundedRectangleBorder(
          borderRadius: PrismeRadius.control,
          side: BorderSide(color: c.border),
        ),
      ),
      dataTableTheme: DataTableThemeData(
        headingTextStyle: text(12, color: c.muted),
        dataTextStyle: text(13),
        dividerThickness: 1,
        horizontalMargin: 16,
        columnSpacing: 24,
      ),
      progressIndicatorTheme: ProgressIndicatorThemeData(
        color: c.accent,
        linearTrackColor: c.raised,
      ),
      textSelectionTheme: TextSelectionThemeData(
        cursorColor: c.accent,
        selectionColor: c.accent.withValues(alpha: .25),
        selectionHandleColor: c.accent,
      ),
      dividerTheme: DividerThemeData(color: c.border, space: 1, thickness: 1),
      scrollbarTheme: ScrollbarThemeData(
        thumbColor: WidgetStatePropertyAll(c.muted.withValues(alpha: .5)),
        radius: const Radius.circular(3),
      ),
    );
  }

  static TextStyle metric(BuildContext context) => TextStyle(
    fontFamily: PrismeFonts.mono,
    fontFeatures: const [FontFeature.tabularFigures()],
    fontSize: 27,
    height: 1.2,
    letterSpacing: -1,
    color: context.prismeStyle.matrix
        ? context.prismeColors.accent
        : context.prismeColors.text,
    shadows: context.prismeStyle.matrix
        ? [
            Shadow(
              color: context.prismeColors.accent.withValues(alpha: .14),
              blurRadius: 16,
            ),
          ]
        : null,
  );

  static TextStyle mono(BuildContext context) =>
      Theme.of(context).textTheme.bodySmall!.copyWith(
        fontFamily: PrismeFonts.mono,
        fontFeatures: const [FontFeature.tabularFigures()],
      );
}
