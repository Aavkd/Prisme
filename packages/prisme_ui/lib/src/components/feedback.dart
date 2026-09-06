import 'package:flutter/material.dart';
import 'package:lucide_icons_flutter/lucide_icons.dart';
import '../foundation/colors.dart';
import '../foundation/tokens.dart';
import '../l10n/strings.dart';
import 'actions.dart';
import 'status.dart';

class PrismeBanner extends StatelessWidget {
  const PrismeBanner({
    super.key,
    required this.title,
    required this.message,
    this.tone = PrismeTone.attention,
    this.action,
    this.onDismiss,
    this.announce = false,
  });
  final String title, message;
  final PrismeTone tone;
  final Widget? action;
  final VoidCallback? onDismiss;
  final bool announce;
  @override
  Widget build(BuildContext context) {
    final color = tone.foreground(context.prismeColors);
    return Semantics(
      liveRegion: announce,
      child: Container(
        decoration: BoxDecoration(
          color: tone.background(context.prismeColors),
          border: Border.all(color: color.withValues(alpha: .65)),
          borderRadius: PrismeRadius.surface,
        ),
        padding: const EdgeInsets.all(16),
        child: Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Icon(
              tone == PrismeTone.critical
                  ? LucideIcons.circleAlert
                  : LucideIcons.info,
              size: 19,
              color: color,
            ),
            const SizedBox(width: 12),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    title,
                    style: Theme.of(
                      context,
                    ).textTheme.titleSmall!.copyWith(color: color),
                  ),
                  const SizedBox(height: 4),
                  Text(
                    message,
                    style: Theme.of(
                      context,
                    ).textTheme.bodyMedium!.copyWith(color: color),
                  ),
                  if (action != null) ...[const SizedBox(height: 12), action!],
                ],
              ),
            ),
            // Critical incidents cannot be dismissed by a presentation preference.
            if (onDismiss != null && tone != PrismeTone.critical)
              PrismeIconButton(
                icon: LucideIcons.x,
                label: context.prismeStrings.close,
                onPressed: onDismiss,
              ),
          ],
        ),
      ),
    );
  }
}

enum PrismeViewState { loading, empty, error, forbidden, offline }

class PrismeStateView extends StatelessWidget {
  const PrismeStateView({
    super.key,
    required this.state,
    this.title,
    this.description,
    this.actionLabel,
    this.onAction,
  });
  final PrismeViewState state;
  final String? title, description, actionLabel;
  final VoidCallback? onAction;
  @override
  Widget build(BuildContext context) {
    final s = context.prismeStrings;
    final (fallback, icon) = switch (state) {
      PrismeViewState.loading => (s.loading, LucideIcons.hourglass),
      PrismeViewState.empty => (s.noData, LucideIcons.inbox),
      PrismeViewState.error => (s.error, LucideIcons.circleAlert),
      PrismeViewState.forbidden => (s.forbidden, LucideIcons.lockKeyhole),
      PrismeViewState.offline => (s.offline, LucideIcons.wifiOff),
    };
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(24),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(icon, color: context.prismeColors.muted, size: 24),
            const SizedBox(height: 12),
            Semantics(
              liveRegion: true,
              child: Text(
                title ?? fallback,
                textAlign: TextAlign.center,
                style: Theme.of(context).textTheme.titleMedium,
              ),
            ),
            if (description != null) ...[
              const SizedBox(height: 8),
              Text(
                description!,
                textAlign: TextAlign.center,
                style: Theme.of(context).textTheme.bodySmall,
              ),
            ],
            if (onAction != null) ...[
              const SizedBox(height: 16),
              PrismeButton(label: actionLabel ?? s.retry, onPressed: onAction),
            ],
          ],
        ),
      ),
    );
  }
}

/// Static skeleton: it intentionally has no perpetual shimmer animation.
class PrismeSkeleton extends StatelessWidget {
  const PrismeSkeleton({super.key, this.lines = 3}) : assert(lines > 0);
  final int lines;
  @override
  Widget build(BuildContext context) => Semantics(
    label: context.prismeStrings.loading,
    excludeSemantics: true,
    child: Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        for (var i = 0; i < lines; i++)
          Padding(
            padding: const EdgeInsets.only(bottom: 10),
            child: FractionallySizedBox(
              widthFactor: i == lines - 1 ? .6 : 1,
              child: Container(
                height: 14,
                decoration: BoxDecoration(
                  color: context.prismeColors.raised,
                  borderRadius: PrismeRadius.badge,
                ),
              ),
            ),
          ),
      ],
    ),
  );
}

void showPrismeMessage(
  BuildContext context,
  String message, {
  SnackBarAction? action,
}) => ScaffoldMessenger.of(
  context,
).showSnackBar(SnackBar(content: Text(message), action: action));
