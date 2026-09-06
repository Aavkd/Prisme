import 'package:flutter/material.dart';
import 'package:lucide_icons_flutter/lucide_icons.dart';
import '../foundation/colors.dart';
import '../foundation/tokens.dart';
import '../l10n/strings.dart';

enum PrismeButtonVariant { primary, secondary, ghost, danger }

class PrismeButton extends StatelessWidget {
  const PrismeButton({
    super.key,
    required this.label,
    this.onPressed,
    this.icon,
    this.variant = PrismeButtonVariant.secondary,
    this.loading = false,
    this.disabledReason,
    this.expand = false,
  });
  final String label;
  final VoidCallback? onPressed;
  final IconData? icon;
  final PrismeButtonVariant variant;
  final bool loading;
  final String? disabledReason;
  final bool expand;

  @override
  Widget build(BuildContext context) {
    final c = context.prismeColors;
    final callback = loading ? null : onPressed;
    final content = Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        if (loading) ...[
          SizedBox.square(
            dimension: 16,
            child: MediaQuery.disableAnimationsOf(context)
                ? const Icon(LucideIcons.hourglass, size: 16)
                : const CircularProgressIndicator(strokeWidth: 2),
          ),
          const SizedBox(width: 8),
        ] else if (icon != null) ...[
          Icon(icon, size: 16),
          const SizedBox(width: 8),
        ],
        Flexible(child: Text(label, textAlign: TextAlign.center)),
      ],
    );
    final style = ButtonStyle(
      minimumSize: WidgetStatePropertyAll(Size(0, context.prismeControlHeight)),
      animationDuration: PrismeMotion.duration(context),
      side: WidgetStateProperty.resolveWith(
        (s) => s.contains(WidgetState.focused)
            ? BorderSide(color: c.accent, width: 2)
            : variant == PrismeButtonVariant.secondary
            ? BorderSide(color: c.border)
            : BorderSide.none,
      ),
    );
    final Widget button = switch (variant) {
      PrismeButtonVariant.primary => FilledButton(
        onPressed: callback,
        style: style,
        child: content,
      ),
      PrismeButtonVariant.secondary => OutlinedButton(
        onPressed: callback,
        style: style,
        child: content,
      ),
      PrismeButtonVariant.ghost => TextButton(
        onPressed: callback,
        style: style,
        child: content,
      ),
      PrismeButtonVariant.danger => FilledButton(
        onPressed: callback,
        style: style.copyWith(
          backgroundColor: WidgetStatePropertyAll(c.criticalSubtle),
          foregroundColor: WidgetStatePropertyAll(
            callback == null ? c.muted : c.critical,
          ),
        ),
        child: content,
      ),
    };
    Widget result = Semantics(
      liveRegion: loading,
      value: loading ? context.prismeStrings.loading : null,
      child: expand ? SizedBox(width: double.infinity, child: button) : button,
    );
    if (disabledReason != null && callback == null) {
      result = Tooltip(message: disabledReason!, child: result);
    }
    return result;
  }
}

class PrismeIconButton extends StatelessWidget {
  const PrismeIconButton({
    super.key,
    required this.icon,
    required this.label,
    this.onPressed,
  });
  final IconData icon;
  final String label;
  final VoidCallback? onPressed;
  @override
  Widget build(BuildContext context) => IconButton(
    tooltip: label,
    onPressed: onPressed,
    icon: Icon(icon, size: 18),
    constraints: BoxConstraints(
      minWidth: context.prismeTouch ? 48 : 40,
      minHeight: context.prismeTouch ? 48 : 40,
    ),
  );
}
