import 'dart:convert';
import 'dart:io';

/// Run from packages/prisme_ui. --check verifies the committed generated file.
void main(List<String> args) {
  final json =
      jsonDecode(File('../../design/tokens.json').readAsStringSync())
          as Map<String, dynamic>;
  final colors = json['color'] as Map<String, dynamic>;
  const roles = [
    'background',
    'navigation',
    'surface',
    'surfaceRaised',
    'surfaceHover',
    'border',
    'textPrimary',
    'textSecondary',
    'accent',
    'accentSubtle',
    'healthy',
    'healthySubtle',
    'attention',
    'attentionSubtle',
    'critical',
    'criticalSubtle',
    'actionFill',
    'actionHover',
    'onAction',
  ];
  final out = StringBuffer(
    "// Generated from design/tokens.json. Do not edit.\npart of 'colors.dart';\n",
  );
  for (final mode in ['light', 'dark', 'matrix']) {
    final palette = colors[mode] as Map<String, dynamic>;
    out.writeln('\nconst _${mode}Colors = <PrismeColorToken, Color>{');
    for (final role in roles) {
      final hex = (palette[role] ?? colors[role]) as String;
      if (!RegExp(r'^#[0-9a-fA-F]{6}$').hasMatch(hex)) {
        throw FormatException('Invalid $mode/$role');
      }
      out.writeln(
        '  PrismeColorToken.$role: Color(0xFF${hex.substring(1).toUpperCase()}),',
      );
    }
    out.writeln('};');
  }
  final file = File('lib/src/foundation/colors.g.dart');
  if (args.contains('--check')) {
    if (!file.existsSync() || file.readAsStringSync() != out.toString()) {
      stderr.writeln('Palette drift: run dart run tool/sync_tokens.dart.');
      exitCode = 1;
    }
  } else {
    file.writeAsStringSync(out.toString());
  }
}
