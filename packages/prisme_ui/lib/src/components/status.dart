import 'package:flutter/material.dart';
import 'package:lucide_icons_flutter/lucide_icons.dart';
import '../foundation/colors.dart';
import '../foundation/tokens.dart';
import '../l10n/strings.dart';

enum PrismeTone { neutral, accent, healthy, attention, critical }

enum PrismeStatus {
  healthy,
  active,
  needsInput,
  warning,
  degraded,
  critical,
  idle,
  paused,
  completed,
  failed,
  unknown,
  stale,
  autonomous,
}

enum PrismeFreshness { live, recent, stale, offline, partial, unknown }

enum PrismeEnvironment { production, staging, development }

extension PrismeToneColors on PrismeTone {
  Color foreground(PrismeColors c) => switch (this) {
    PrismeTone.neutral => c.muted,
    PrismeTone.accent => c.accent,
    PrismeTone.healthy => c.healthy,
    PrismeTone.attention => c.attention,
    PrismeTone.critical => c.critical,
  };
  Color background(PrismeColors c) => switch (this) {
    PrismeTone.neutral => c.raised,
    PrismeTone.accent => c.accentSubtle,
    PrismeTone.healthy => c.healthySubtle,
    PrismeTone.attention => c.attentionSubtle,
    PrismeTone.critical => c.criticalSubtle,
  };
}

extension PrismeStatusPresentation on PrismeStatus {
  PrismeTone get tone => switch (this) {
    PrismeStatus.healthy || PrismeStatus.completed => PrismeTone.healthy,
    PrismeStatus.active || PrismeStatus.autonomous => PrismeTone.accent,
    PrismeStatus.needsInput ||
    PrismeStatus.warning ||
    PrismeStatus.degraded => PrismeTone.attention,
    PrismeStatus.critical || PrismeStatus.failed => PrismeTone.critical,
    _ => PrismeTone.neutral,
  };
  IconData get icon => switch (this) {
    PrismeStatus.healthy || PrismeStatus.completed => LucideIcons.check,
    PrismeStatus.active => LucideIcons.activity,
    PrismeStatus.needsInput => LucideIcons.messageCircle,
    PrismeStatus.warning || PrismeStatus.degraded => LucideIcons.triangleAlert,
    PrismeStatus.critical || PrismeStatus.failed => LucideIcons.circleAlert,
    PrismeStatus.idle => LucideIcons.moon,
    PrismeStatus.paused => LucideIcons.pause,
    PrismeStatus.stale => LucideIcons.clock,
    PrismeStatus.autonomous => LucideIcons.bot,
    PrismeStatus.unknown => LucideIcons.circleHelp,
  };
  String label(PrismeStrings s) => switch (this) {
    PrismeStatus.healthy => s.healthy,
    PrismeStatus.active => s.active,
    PrismeStatus.needsInput => s.needsInput,
    PrismeStatus.warning => s.warning,
    PrismeStatus.degraded => s.degraded,
    PrismeStatus.critical => s.critical,
    PrismeStatus.idle => s.idle,
    PrismeStatus.paused => s.paused,
    PrismeStatus.completed => s.completed,
    PrismeStatus.failed => s.failed,
    PrismeStatus.unknown => s.unknown,
    PrismeStatus.stale => s.stale,
    PrismeStatus.autonomous => s.autonomous,
  };
}

class PrismeBadge extends StatelessWidget {
  const PrismeBadge({
    super.key,
    required this.label,
    this.tone = PrismeTone.neutral,
    this.icon,
  });
  final String label;
  final PrismeTone tone;
  final IconData? icon;
  @override
  Widget build(BuildContext context) {
    final color = tone.foreground(context.prismeColors);
    return Semantics(
      label: label,
      excludeSemantics: true,
      child: DecoratedBox(
        decoration: BoxDecoration(
          color: tone.background(context.prismeColors),
          borderRadius: PrismeRadius.badge,
        ),
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 7, vertical: 4),
          child: Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              if (icon != null) ...[
                Icon(icon, size: 13, color: color),
                const SizedBox(width: 5),
              ],
              Flexible(
                child: Text(
                  label,
                  style: Theme.of(
                    context,
                  ).textTheme.labelMedium!.copyWith(color: color),
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

class PrismeStatusBadge extends StatelessWidget {
  const PrismeStatusBadge({super.key, required this.status, this.label});
  final PrismeStatus status;
  final String? label;
  @override
  Widget build(BuildContext context) => PrismeBadge(
    label: label ?? status.label(context.prismeStrings),
    tone: status.tone,
    icon: status.icon,
  );
}

/// An observation is separate from resource health. Classification is supplied by
/// the domain's collection policy, never inferred from the absence of errors.
@immutable
class PrismeObservation {
  PrismeObservation({required this.freshness, this.observedAt}) {
    if ((freshness == PrismeFreshness.live ||
            freshness == PrismeFreshness.recent) &&
        observedAt == null) {
      throw ArgumentError('A recent observation needs a timestamp.');
    }
  }
  final PrismeFreshness freshness;
  final DateTime? observedAt;
  bool get isHistorical =>
      freshness == PrismeFreshness.stale ||
      freshness == PrismeFreshness.offline;
  PrismeStatus displayStatus(PrismeStatus resourceStatus) =>
      switch (freshness) {
        PrismeFreshness.stale || PrismeFreshness.offline => PrismeStatus.stale,
        PrismeFreshness.unknown => PrismeStatus.unknown,
        PrismeFreshness.partial when resourceStatus == PrismeStatus.healthy =>
          PrismeStatus.warning,
        _ => resourceStatus,
      };
}

class PrismeFreshnessBadge extends StatelessWidget {
  const PrismeFreshnessBadge({super.key, required this.observation});
  final PrismeObservation observation;
  @override
  Widget build(BuildContext context) {
    final s = context.prismeStrings;
    final label = switch (observation.freshness) {
      PrismeFreshness.live => s.live,
      PrismeFreshness.recent => s.recent,
      PrismeFreshness.stale => s.stale,
      PrismeFreshness.offline => s.offline,
      PrismeFreshness.partial => s.partial,
      PrismeFreshness.unknown => s.unknown,
    };
    final timestamp = observation.observedAt;
    return Tooltip(
      message: timestamp == null ? s.unknown : s.observedAt(timestamp),
      child: Semantics(
        label: timestamp == null ? label : '$label, ${s.observedAt(timestamp)}',
        excludeSemantics: true,
        child: PrismeBadge(
          label: label,
          icon: observation.isHistorical
              ? LucideIcons.clock
              : LucideIcons.radio,
          tone: observation.freshness == PrismeFreshness.partial
              ? PrismeTone.attention
              : PrismeTone.neutral,
        ),
      ),
    );
  }
}

class PrismeEnvironmentBadge extends StatelessWidget {
  const PrismeEnvironmentBadge({super.key, required this.environment});
  final PrismeEnvironment environment;
  @override
  Widget build(BuildContext context) => PrismeBadge(
    icon: LucideIcons.shield,
    label: switch (environment) {
      PrismeEnvironment.production => context.prismeStrings.production,
      PrismeEnvironment.staging => context.prismeStrings.staging,
      PrismeEnvironment.development => context.prismeStrings.development,
    },
  );
}
