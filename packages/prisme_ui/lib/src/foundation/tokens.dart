import 'package:flutter/material.dart';

enum PrismeAppearance { system, light, dark, matrix }

enum PrismeDensity { comfortable, compact }

abstract final class PrismeSpace {
  static const double xs = 4;
  static const double sm = 8;
  static const double md = 12;
  static const double lg = 16;
  static const double xl = 24;
  static const double xxl = 32;
}

abstract final class PrismeRadius {
  static const badge = BorderRadius.all(Radius.circular(4));
  static const control = BorderRadius.all(Radius.circular(6));
  static const surface = BorderRadius.all(Radius.circular(10));
  static const overlay = BorderRadius.all(Radius.circular(12));
}

abstract final class PrismeBreakpoints {
  static const double compact = 600;
  static const double desktop = 1024;
  static const double sidebar = 168;
  static const double inspector = 380;
}

abstract final class PrismeFonts {
  static const ui = 'packages/prisme_ui/Inter';
  static const brand = 'packages/prisme_ui/SpaceGrotesk';
  static const mono = 'packages/prisme_ui/IBMPlexMono';
}

abstract final class PrismeMotion {
  static const interaction = Duration(milliseconds: 120);
  static Duration duration(BuildContext context) =>
      MediaQuery.disableAnimationsOf(context) ? Duration.zero : interaction;
}

@immutable
class PrismeStyle extends ThemeExtension<PrismeStyle> {
  const PrismeStyle({
    this.density = PrismeDensity.comfortable,
    this.matrix = false,
  });
  final PrismeDensity density;
  final bool matrix;
  double get cardPadding => density == PrismeDensity.compact ? 13 : 18;
  double get controlHeight => density == PrismeDensity.compact ? 36 : 40;

  @override
  PrismeStyle copyWith({PrismeDensity? density, bool? matrix}) => PrismeStyle(
    density: density ?? this.density,
    matrix: matrix ?? this.matrix,
  );

  @override
  PrismeStyle lerp(covariant PrismeStyle? other, double t) =>
      other == null || t < .5 ? this : other;
}

extension PrismeStyleContext on BuildContext {
  PrismeStyle get prismeStyle => Theme.of(this).extension<PrismeStyle>()!;
  bool get prismeTouch => switch (Theme.of(this).platform) {
    TargetPlatform.android ||
    TargetPlatform.iOS ||
    TargetPlatform.fuchsia => true,
    _ => false,
  };
  double get prismeControlHeight =>
      prismeTouch ? 48 : prismeStyle.controlHeight;
}
