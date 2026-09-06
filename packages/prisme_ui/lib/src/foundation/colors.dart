import 'package:flutter/material.dart';

part 'colors.g.dart';

enum PrismeColorToken {
  background,
  navigation,
  surface,
  surfaceRaised,
  surfaceHover,
  border,
  textPrimary,
  textSecondary,
  accent,
  accentSubtle,
  healthy,
  healthySubtle,
  attention,
  attentionSubtle,
  critical,
  criticalSubtle,
  actionFill,
  actionHover,
  onAction,
}

@immutable
class PrismeColors extends ThemeExtension<PrismeColors> {
  PrismeColors(Map<PrismeColorToken, Color> values)
    : _values = Map.unmodifiable(values) {
    if (PrismeColorToken.values.any((key) => !_values.containsKey(key))) {
      throw ArgumentError('A Prisme palette must define every semantic color.');
    }
  }
  final Map<PrismeColorToken, Color> _values;
  static final light = PrismeColors(_lightColors);
  static final dark = PrismeColors(_darkColors);
  static final matrix = PrismeColors(_matrixColors);

  Color operator [](PrismeColorToken token) => _values[token]!;
  Color get background => this[PrismeColorToken.background];
  Color get navigation => this[PrismeColorToken.navigation];
  Color get surface => this[PrismeColorToken.surface];
  Color get raised => this[PrismeColorToken.surfaceRaised];
  Color get hover => this[PrismeColorToken.surfaceHover];
  Color get border => this[PrismeColorToken.border];
  Color get text => this[PrismeColorToken.textPrimary];
  Color get muted => this[PrismeColorToken.textSecondary];
  Color get accent => this[PrismeColorToken.accent];
  Color get accentSubtle => this[PrismeColorToken.accentSubtle];
  Color get healthy => this[PrismeColorToken.healthy];
  Color get healthySubtle => this[PrismeColorToken.healthySubtle];
  Color get attention => this[PrismeColorToken.attention];
  Color get attentionSubtle => this[PrismeColorToken.attentionSubtle];
  Color get critical => this[PrismeColorToken.critical];
  Color get criticalSubtle => this[PrismeColorToken.criticalSubtle];
  Color get action => this[PrismeColorToken.actionFill];
  Color get actionHover => this[PrismeColorToken.actionHover];
  Color get onAction => this[PrismeColorToken.onAction];

  @override
  PrismeColors copyWith({Map<PrismeColorToken, Color>? values}) =>
      PrismeColors({..._values, ...?values});

  @override
  PrismeColors lerp(covariant PrismeColors? other, double t) => other == null
      ? this
      : PrismeColors({
          for (final token in PrismeColorToken.values)
            token: Color.lerp(this[token], other[token], t)!,
        });
}

extension PrismeColorsContext on BuildContext {
  PrismeColors get prismeColors => Theme.of(this).extension<PrismeColors>()!;
}
