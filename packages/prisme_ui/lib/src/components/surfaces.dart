import 'package:flutter/material.dart';
import 'package:lucide_icons_flutter/lucide_icons.dart';
import '../foundation/colors.dart';
import '../foundation/tokens.dart';

class PrismeSurface extends StatelessWidget {
  const PrismeSurface({
    super.key,
    required this.child,
    this.padding,
    this.onTap,
    this.semanticLabel,
    this.raised = false,
  }) : assert(onTap == null || semanticLabel != null);
  final Widget child;
  final EdgeInsetsGeometry? padding;
  final VoidCallback? onTap;
  final String? semanticLabel;
  final bool raised;
  @override
  Widget build(BuildContext context) {
    final c = context.prismeColors;
    final content = Padding(
      padding: padding ?? EdgeInsets.all(context.prismeStyle.cardPadding),
      child: child,
    );
    return Material(
      color: raised ? c.raised : c.surface,
      shape: RoundedRectangleBorder(
        borderRadius: PrismeRadius.surface,
        side: BorderSide(color: c.border),
      ),
      clipBehavior: Clip.antiAlias,
      child: onTap == null
          ? content
          : Semantics(
              button: true,
              label: semanticLabel,
              child: InkWell(
                onTap: onTap,
                borderRadius: PrismeRadius.surface,
                focusColor: c.accent.withValues(alpha: .2),
                child: content,
              ),
            ),
    );
  }
}

class PrismeSectionHeader extends StatelessWidget {
  const PrismeSectionHeader({
    super.key,
    required this.title,
    this.subtitle,
    this.actions = const [],
  });
  final String title;
  final String? subtitle;
  final List<Widget> actions;
  @override
  Widget build(BuildContext context) => LayoutBuilder(
    builder: (context, constraints) {
      final heading = Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Semantics(
            header: true,
            child: Text(title, style: Theme.of(context).textTheme.titleMedium),
          ),
          if (subtitle != null) ...[
            const SizedBox(height: 4),
            Text(subtitle!, style: Theme.of(context).textTheme.bodySmall),
          ],
        ],
      );
      if (constraints.maxWidth < 440 ||
          MediaQuery.textScalerOf(context).scale(1) > 1.4) {
        return Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            heading,
            if (actions.isNotEmpty) ...[
              const SizedBox(height: 8),
              Wrap(spacing: 8, runSpacing: 8, children: actions),
            ],
          ],
        );
      }
      return Row(
        children: [
          Expanded(child: heading),
          const SizedBox(width: 12),
          ...actions,
        ],
      );
    },
  );
}

class PrismeBrand extends StatelessWidget {
  const PrismeBrand({super.key, this.compact = false});
  final bool compact;
  @override
  Widget build(BuildContext context) {
    final c = context.prismeColors;
    return Semantics(
      label: 'Prisme',
      excludeSemantics: true,
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Container(
            width: 28,
            height: 31,
            decoration: BoxDecoration(
              color: c.accentSubtle,
              borderRadius: PrismeRadius.control,
            ),
            child: Icon(LucideIcons.triangle, size: 20, color: c.accent),
          ),
          if (!compact) ...[
            const SizedBox(width: 9),
            Flexible(
              child: Text(
                'prisme',
                style: TextStyle(
                  fontFamily: context.prismeStyle.matrix
                      ? PrismeFonts.mono
                      : PrismeFonts.brand,
                  color: context.prismeStyle.matrix ? c.accent : c.text,
                  fontSize: 25,
                  letterSpacing: -1.5,
                  fontWeight: FontWeight.w600,
                ),
              ),
            ),
          ],
        ],
      ),
    );
  }
}

class PrismeKeyValue extends StatelessWidget {
  const PrismeKeyValue({
    super.key,
    required this.label,
    required this.value,
    this.selectable = true,
  });
  final String label;
  final String value;
  final bool selectable;
  @override
  Widget build(BuildContext context) => Column(
    crossAxisAlignment: CrossAxisAlignment.start,
    children: [
      Text(label, style: Theme.of(context).textTheme.bodySmall),
      const SizedBox(height: 4),
      if (selectable) SelectableText(value) else Text(value),
    ],
  );
}
