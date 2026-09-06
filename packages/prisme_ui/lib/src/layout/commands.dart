import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import '../components/forms.dart';
import '../components/overlays.dart';
import '../foundation/colors.dart';
import '../l10n/strings.dart';

@immutable
class PrismeCommand {
  const PrismeCommand({
    required this.id,
    required this.label,
    required this.icon,
    this.description,
    this.enabled = true,
  });
  final String id, label;
  final String? description;
  final IconData icon;
  final bool enabled;
}

/// Chooses a command ID. Execution and capability checking belong to the caller.
Future<String?> showPrismeCommandPalette({
  required BuildContext context,
  required List<PrismeCommand> commands,
}) => showPrismeDialog<String>(
  context: context,
  title: context.prismeStrings.commands,
  width: 560,
  builder: (_) => _Palette(commands: commands),
);

class _MoveIntent extends Intent {
  const _MoveIntent(this.delta);
  final int delta;
}

class _ChooseIntent extends Intent {
  const _ChooseIntent();
}

class _Palette extends StatefulWidget {
  const _Palette({required this.commands});
  final List<PrismeCommand> commands;
  @override
  State<_Palette> createState() => _PaletteState();
}

class _PaletteState extends State<_Palette> {
  final _query = TextEditingController();
  final _keys = <String, GlobalKey>{};
  int _selected = 0;
  List<PrismeCommand> get _matches => widget.commands
      .where(
        (c) => '${c.label} ${c.description ?? ''}'.toLowerCase().contains(
          _query.text.trim().toLowerCase(),
        ),
      )
      .toList();
  @override
  void dispose() {
    _query.dispose();
    super.dispose();
  }

  void _move(int delta) {
    final matches = _matches;
    if (!matches.any((c) => c.enabled)) return;
    var index = _selected;
    do {
      index = (index + delta) % matches.length;
    } while (!matches[index].enabled);
    setState(() => _selected = index);
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (!mounted) return;
      final item = _keys[matches[index].id]?.currentContext;
      if (item != null) Scrollable.ensureVisible(item, alignment: .5);
    });
  }

  void _choose() {
    final m = _matches;
    if (m.isNotEmpty && _selected < m.length && m[_selected].enabled) {
      Navigator.of(context).pop(m[_selected].id);
    }
  }

  @override
  Widget build(BuildContext context) {
    final matches = _matches;
    return Shortcuts(
      shortcuts: const {
        SingleActivator(LogicalKeyboardKey.arrowDown): _MoveIntent(1),
        SingleActivator(LogicalKeyboardKey.arrowUp): _MoveIntent(-1),
        SingleActivator(LogicalKeyboardKey.enter): _ChooseIntent(),
      },
      child: Actions(
        actions: {
          _MoveIntent: CallbackAction<_MoveIntent>(
            onInvoke: (i) {
              _move(i.delta);
              return null;
            },
          ),
          _ChooseIntent: CallbackAction<_ChooseIntent>(
            onInvoke: (_) {
              _choose();
              return null;
            },
          ),
        },
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            PrismeSearchField(
              controller: _query,
              autofocus: true,
              onChanged: (_) => setState(() {
                _selected = _matches.indexWhere((c) => c.enabled);
                if (_selected < 0) _selected = 0;
              }),
            ),
            const SizedBox(height: 12),
            if (matches.isEmpty)
              Padding(
                padding: const EdgeInsets.all(24),
                child: Text(context.prismeStrings.noResults),
              ),
            ConstrainedBox(
              constraints: const BoxConstraints(maxHeight: 320),
              child: SingleChildScrollView(
                child: Column(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    for (var i = 0; i < matches.length; i++)
                      ListTile(
                        key: _keys.putIfAbsent(matches[i].id, GlobalKey.new),
                        selected: i == _selected,
                        selectedTileColor: context.prismeColors.accentSubtle,
                        enabled: matches[i].enabled,
                        leading: Icon(matches[i].icon, size: 18),
                        title: Text(matches[i].label),
                        subtitle: matches[i].description == null
                            ? null
                            : Text(matches[i].description!),
                        onTap: matches[i].enabled
                            ? () => Navigator.of(context).pop(matches[i].id)
                            : null,
                      ),
                  ],
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}
