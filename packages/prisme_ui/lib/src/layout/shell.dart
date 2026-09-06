import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:lucide_icons_flutter/lucide_icons.dart';
import '../components/actions.dart';
import '../components/surfaces.dart';
import '../foundation/colors.dart';
import '../foundation/tokens.dart';
import '../l10n/strings.dart';

@immutable
class PrismeDestination {
  const PrismeDestination({
    required this.id,
    required this.label,
    required this.icon,
    this.group,
  });
  final String id, label;
  final IconData icon;
  final String? group;
}

class _SearchIntent extends Intent {
  const _SearchIntent();
}

/// Presentational shell. Route ownership, authorization, connection and device
/// state stay with the host app. Every destination is reachable on mobile.
class PrismeAppShell extends StatefulWidget {
  const PrismeAppShell({
    super.key,
    required this.destinations,
    required this.selectedId,
    required this.onDestinationSelected,
    required this.body,
    this.toolbar = const [],
    this.workspaceLabel = 'Mantara',
    this.footer,
    this.onSearch,
  }) : assert(destinations.length > 0);
  final List<PrismeDestination> destinations;
  final String selectedId, workspaceLabel;
  final ValueChanged<String> onDestinationSelected;
  final Widget body;
  final List<Widget> toolbar;
  final Widget? footer;
  final VoidCallback? onSearch;
  @override
  State<PrismeAppShell> createState() => _PrismeAppShellState();
}

class _PrismeAppShellState extends State<PrismeAppShell> {
  bool _collapsed = false;
  void _more(BuildContext context) => showModalBottomSheet<void>(
    context: context,
    useSafeArea: true,
    isScrollControlled: true,
    builder: (sheetContext) => ConstrainedBox(
      constraints: BoxConstraints(
        maxHeight: MediaQuery.sizeOf(sheetContext).height * .8,
      ),
      child: SingleChildScrollView(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            for (final d in widget.destinations.skip(4))
              ListTile(
                leading: Icon(d.icon, size: 20),
                title: Text(d.label),
                selected: d.id == widget.selectedId,
                onTap: () {
                  Navigator.of(sheetContext).pop();
                  widget.onDestinationSelected(d.id);
                },
              ),
            const SizedBox(height: 16),
          ],
        ),
      ),
    ),
  );

  @override
  Widget build(BuildContext context) {
    final c = context.prismeColors, s = context.prismeStrings;
    final index = widget.destinations.indexWhere(
      (d) => d.id == widget.selectedId,
    );
    assert(index >= 0, 'selectedId must exist in destinations.');
    assert(
      widget.destinations.map((d) => d.id).toSet().length ==
          widget.destinations.length,
      'Destination IDs must be unique.',
    );
    return LayoutBuilder(
      builder: (context, box) {
        final desktop =
            box.maxWidth / MediaQuery.textScalerOf(context).scale(1) >=
            PrismeBreakpoints.desktop;
        final visible = widget.destinations.length > 5
            ? widget.destinations.take(4).toList()
            : widget.destinations;
        final more = widget.destinations.length > 5;
        return Shortcuts(
          shortcuts: const {
            SingleActivator(LogicalKeyboardKey.keyK, control: true):
                _SearchIntent(),
            SingleActivator(LogicalKeyboardKey.keyK, meta: true):
                _SearchIntent(),
          },
          child: Actions(
            actions: {
              _SearchIntent: CallbackAction<_SearchIntent>(
                onInvoke: (_) {
                  widget.onSearch?.call();
                  return null;
                },
              ),
            },
            child: Scaffold(
              backgroundColor: c.background,
              body: SafeArea(
                child: Column(
                  children: [
                    Expanded(
                      child: Row(
                        crossAxisAlignment: CrossAxisAlignment.stretch,
                        children: [
                          if (desktop)
                            Container(
                              width: _collapsed
                                  ? 64
                                  : PrismeBreakpoints.sidebar,
                              decoration: BoxDecoration(
                                color: c.navigation,
                                border: Border(
                                  right: BorderSide(color: c.border),
                                ),
                              ),
                              child: Column(
                                children: [
                                  Padding(
                                    padding: EdgeInsets.symmetric(
                                      horizontal: _collapsed ? 12 : 16,
                                      vertical: 24,
                                    ),
                                    child: PrismeBrand(compact: _collapsed),
                                  ),
                                  if (!_collapsed)
                                    Padding(
                                      padding: const EdgeInsets.fromLTRB(
                                        16,
                                        0,
                                        16,
                                        20,
                                      ),
                                      child: Text(
                                        widget.workspaceLabel,
                                        style: Theme.of(
                                          context,
                                        ).textTheme.bodySmall,
                                      ),
                                    ),
                                  Expanded(
                                    child: Semantics(
                                      label: s.navigation,
                                      child: ListView(
                                        padding: const EdgeInsets.symmetric(
                                          horizontal: 8,
                                        ),
                                        children: [
                                          for (
                                            var i = 0;
                                            i < widget.destinations.length;
                                            i++
                                          ) ...[
                                            if (!_collapsed &&
                                                widget.destinations[i].group !=
                                                    null &&
                                                (i == 0 ||
                                                    widget
                                                            .destinations[i]
                                                            .group !=
                                                        widget
                                                            .destinations[i - 1]
                                                            .group))
                                              Padding(
                                                padding:
                                                    const EdgeInsets.fromLTRB(
                                                      8,
                                                      18,
                                                      0,
                                                      8,
                                                    ),
                                                child: Text(
                                                  widget.destinations[i].group!,
                                                  style: Theme.of(
                                                    context,
                                                  ).textTheme.labelSmall,
                                                ),
                                              ),
                                            _NavigationItem(
                                              destination:
                                                  widget.destinations[i],
                                              selected:
                                                  widget.destinations[i].id ==
                                                  widget.selectedId,
                                              collapsed: _collapsed,
                                              onTap: () =>
                                                  widget.onDestinationSelected(
                                                    widget.destinations[i].id,
                                                  ),
                                            ),
                                          ],
                                        ],
                                      ),
                                    ),
                                  ),
                                ],
                              ),
                            ),
                          Expanded(
                            child: Column(
                              children: [
                                Container(
                                  decoration: BoxDecoration(
                                    border: Border(
                                      bottom: BorderSide(color: c.border),
                                    ),
                                  ),
                                  padding: const EdgeInsets.symmetric(
                                    horizontal: 16,
                                    vertical: 10,
                                  ),
                                  child: Row(
                                    children: [
                                      if (desktop)
                                        PrismeIconButton(
                                          icon: _collapsed
                                              ? LucideIcons.panelLeftOpen
                                              : LucideIcons.panelLeftClose,
                                          label: _collapsed
                                              ? s.expandNavigation
                                              : s.collapseNavigation,
                                          onPressed: () => setState(
                                            () => _collapsed = !_collapsed,
                                          ),
                                        )
                                      else
                                        PrismeBrand(
                                          compact:
                                              widget.toolbar.isNotEmpty &&
                                              box.maxWidth /
                                                      MediaQuery.textScalerOf(
                                                        context,
                                                      ).scale(1) <
                                                  PrismeBreakpoints.compact,
                                        ),
                                      const SizedBox(width: 12),
                                      Expanded(
                                        child: Wrap(
                                          alignment: WrapAlignment.end,
                                          crossAxisAlignment:
                                              WrapCrossAlignment.center,
                                          spacing: 8,
                                          runSpacing: 8,
                                          children: widget.toolbar,
                                        ),
                                      ),
                                      if (widget.onSearch != null)
                                        PrismeIconButton(
                                          icon: LucideIcons.search,
                                          label: '${s.search} · Ctrl+K',
                                          onPressed: widget.onSearch,
                                        ),
                                    ],
                                  ),
                                ),
                                Expanded(child: widget.body),
                              ],
                            ),
                          ),
                        ],
                      ),
                    ),
                    if (!desktop && widget.destinations.length > 1)
                      NavigationBar(
                        selectedIndex: more && index >= 4
                            ? 4
                            : index.clamp(0, visible.length - 1),
                        labelBehavior:
                            MediaQuery.textScalerOf(context).scale(1) > 1.4
                            ? NavigationDestinationLabelBehavior
                                  .onlyShowSelected
                            : NavigationDestinationLabelBehavior.alwaysShow,
                        onDestinationSelected: (i) => more && i == 4
                            ? _more(context)
                            : widget.onDestinationSelected(visible[i].id),
                        destinations: [
                          for (final d in visible)
                            NavigationDestination(
                              icon: Icon(d.icon),
                              label: d.label,
                            ),
                          if (more)
                            NavigationDestination(
                              icon: const Icon(LucideIcons.ellipsis),
                              label: s.more,
                            ),
                        ],
                      ),
                    if (widget.footer != null)
                      Container(
                        width: double.infinity,
                        decoration: BoxDecoration(
                          color: c.navigation,
                          border: Border(top: BorderSide(color: c.border)),
                        ),
                        padding: const EdgeInsets.symmetric(
                          horizontal: 16,
                          vertical: 8,
                        ),
                        child: widget.footer!,
                      ),
                  ],
                ),
              ),
            ),
          ),
        );
      },
    );
  }
}

class _NavigationItem extends StatelessWidget {
  const _NavigationItem({
    required this.destination,
    required this.selected,
    required this.collapsed,
    required this.onTap,
  });
  final PrismeDestination destination;
  final bool selected, collapsed;
  final VoidCallback onTap;
  @override
  Widget build(BuildContext context) {
    final c = context.prismeColors;
    final item = Semantics(
      selected: selected,
      button: true,
      label: destination.label,
      excludeSemantics: true,
      child: Padding(
        padding: const EdgeInsets.only(bottom: 4),
        child: Material(
          color: selected ? c.accentSubtle : Colors.transparent,
          borderRadius: PrismeRadius.control,
          child: InkWell(
            onTap: onTap,
            borderRadius: PrismeRadius.control,
            child: Padding(
              padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 10),
              child: Row(
                children: [
                  Icon(
                    destination.icon,
                    size: 17,
                    color: selected ? c.accent : c.muted,
                  ),
                  if (!collapsed) ...[
                    const SizedBox(width: 9),
                    Expanded(
                      child: Text(
                        destination.label,
                        style: Theme.of(context).textTheme.labelLarge!.copyWith(
                          color: selected ? c.accent : c.muted,
                        ),
                      ),
                    ),
                  ],
                ],
              ),
            ),
          ),
        ),
      ),
    );
    return collapsed ? Tooltip(message: destination.label, child: item) : item;
  }
}
