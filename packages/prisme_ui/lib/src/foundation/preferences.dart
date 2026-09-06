import 'package:flutter/widgets.dart';
import 'tokens.dart';

/// In-memory presentation preferences. The host app owns device-scoped storage.
class PrismePreferences extends ChangeNotifier {
  PrismePreferences({
    PrismeAppearance appearance = PrismeAppearance.system,
    PrismeDensity density = PrismeDensity.comfortable,
    Locale? locale,
  }) : _appearance = appearance,
       _density = density,
       _locale = locale {
    _validateLocale(locale);
  }
  PrismeAppearance _appearance;
  PrismeDensity _density;
  Locale? _locale;
  PrismeAppearance get appearance => _appearance;
  PrismeDensity get density => _density;
  Locale? get locale => _locale;

  set appearance(PrismeAppearance value) {
    if (_appearance != value) {
      _appearance = value;
      notifyListeners();
    }
  }

  set density(PrismeDensity value) {
    if (_density != value) {
      _density = value;
      notifyListeners();
    }
  }

  set locale(Locale? value) {
    _validateLocale(value);
    if (_locale != value) {
      _locale = value;
      notifyListeners();
    }
  }

  static void _validateLocale(Locale? value) {
    if (value != null && !['fr', 'en'].contains(value.languageCode)) {
      throw ArgumentError.value(
        value,
        'locale',
        'Supported languages: fr, en.',
      );
    }
  }

  Map<String, Object?> toJson() => {
    'version': 1,
    'appearance': appearance.name,
    'density': density.name,
    'locale': locale?.languageCode,
  };

  factory PrismePreferences.fromJson(Map<String, Object?> json) {
    T read<T extends Enum>(List<T> values, Object? value, T fallback) =>
        values.where((v) => v.name == value).firstOrNull ?? fallback;
    final language = json['locale'];
    return PrismePreferences(
      appearance: read(
        PrismeAppearance.values,
        json['appearance'],
        PrismeAppearance.system,
      ),
      density: read(
        PrismeDensity.values,
        json['density'],
        PrismeDensity.comfortable,
      ),
      locale: language == 'fr' || language == 'en'
          ? Locale(language! as String)
          : null,
    );
  }
}
