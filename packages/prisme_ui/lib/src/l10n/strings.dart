import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:intl/intl.dart';

/// Only shared UI copy lives here; feature copy belongs to the consuming app.
class PrismeStrings {
  const PrismeStrings(this.locale);
  final Locale locale;
  bool get french => locale.languageCode == 'fr';
  String choose(String fr, String en) => french ? fr : en;
  static PrismeStrings of(BuildContext context) =>
      Localizations.of<PrismeStrings>(context, PrismeStrings)!;
  static const delegate = _PrismeStringsDelegate();
  static const supportedLocales = [Locale('fr'), Locale('en')];
  String get close => choose('Fermer', 'Close');
  String get cancel => choose('Annuler', 'Cancel');
  String get search => choose('Rechercher', 'Search');
  String get clear => choose('Effacer la recherche', 'Clear search');
  String get more => choose('Plus', 'More');
  String get noResults => choose('Aucun résultat', 'No results');
  String get loading => choose('Chargement', 'Loading');
  String get unavailable => choose('Indisponible', 'Unavailable');
  String get progressUnavailable =>
      choose('Progression indisponible', 'Progress unavailable');
  String get noData => choose('Aucune donnée', 'No data');
  String get retry => choose('Réessayer', 'Retry');
  String get error => choose('Chargement impossible', 'Unable to load');
  String get forbidden =>
      choose('Permission insuffisante', 'Insufficient permission');
  String get offline =>
      choose('Hors connexion · lecture seule', 'Offline · read only');
  String get stale => choose('Données anciennes', 'Stale data');
  String get partial => choose('Collecte partielle', 'Partial telemetry');
  String get unknown => choose('État inconnu', 'Unknown state');
  String get recent => choose('Données récentes', 'Recent data');
  String get live => choose('Flux connecté', 'Connected stream');
  String get healthy => choose('En ligne', 'Online');
  String get active => choose('En cours', 'Running');
  String get needsInput => choose('À vous', 'Needs you');
  String get warning => choose('À surveiller', 'Warning');
  String get degraded => choose('Dégradé', 'Degraded');
  String get critical => choose('Critique', 'Critical');
  String get idle => choose('Au repos', 'Idle');
  String get paused => choose('Suspendu', 'Paused');
  String get completed => choose('Terminé', 'Completed');
  String get failed => choose('Échoué', 'Failed');
  String get autonomous => choose('Autonome', 'Autonomous');
  String get production => 'Production';
  String get staging => choose('Préproduction', 'Staging');
  String get development => choose('Développement', 'Development');
  String get target => choose('Cible', 'Target');
  String get impact => choose('Impact attendu', 'Expected impact');
  String get verification => choose('Vérification', 'Verification');
  String get rollback => choose('Retour arrière', 'Rollback');
  String get showSecret => choose('Afficher la valeur', 'Show value');
  String get hideSecret => choose('Masquer la valeur', 'Hide value');
  String get navigation => choose('Navigation principale', 'Main navigation');
  String get collapseNavigation =>
      choose('Replier la navigation', 'Collapse navigation');
  String get expandNavigation =>
      choose('Déplier la navigation', 'Expand navigation');
  String get commands => choose('Rechercher une commande', 'Search commands');
  String get noSamples => choose('Aucune mesure', 'No samples');
  String get history => choose('Historique', 'History');
  String typeToConfirm(String value) => choose(
    'Saisissez « $value » pour continuer',
    'Type “$value” to continue',
  );
  String observedAt(DateTime value) =>
      choose('Observé le ', 'Observed ') +
      DateFormat.yMd(locale.languageCode).add_Hms().format(value.toLocal());
  String number(num value, {int decimals = 0}) =>
      NumberFormat.decimalPatternDigits(
        locale: locale.languageCode,
        decimalDigits: decimals,
      ).format(value);
  String percent(double value) =>
      NumberFormat.percentPattern(locale.languageCode).format(value);
}

class _PrismeStringsDelegate extends LocalizationsDelegate<PrismeStrings> {
  const _PrismeStringsDelegate();
  @override
  bool isSupported(Locale locale) => ['fr', 'en'].contains(locale.languageCode);
  @override
  Future<PrismeStrings> load(Locale locale) =>
      SynchronousFuture(PrismeStrings(locale));
  @override
  bool shouldReload(_PrismeStringsDelegate old) => false;
}

extension PrismeStringsContext on BuildContext {
  PrismeStrings get prismeStrings => PrismeStrings.of(this);
}
