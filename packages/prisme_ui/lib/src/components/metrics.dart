import 'dart:math' as math;
import 'package:flutter/material.dart';
import '../foundation/colors.dart';
import '../foundation/theme.dart';
import '../l10n/strings.dart';
import 'status.dart';

class PrismeMetric extends StatelessWidget {
  const PrismeMetric({
    super.key,
    required this.label,
    this.value,
    this.unit,
    this.detail,
  });
  final String label;
  final String? value, unit, detail;
  @override
  Widget build(BuildContext context) => Column(
    crossAxisAlignment: CrossAxisAlignment.start,
    children: [
      Text(label, style: Theme.of(context).textTheme.bodySmall),
      const SizedBox(height: 6),
      Wrap(
        crossAxisAlignment: WrapCrossAlignment.end,
        spacing: 5,
        children: [
          Text(value ?? '—', style: PrismeTheme.metric(context)),
          if (value != null && unit != null)
            Text(unit!, style: Theme.of(context).textTheme.bodySmall),
        ],
      ),
      if (value == null || detail != null) ...[
        const SizedBox(height: 4),
        Text(
          value == null ? detail ?? context.prismeStrings.unavailable : detail!,
          style: Theme.of(context).textTheme.bodySmall,
        ),
      ],
    ],
  );
}

class PrismeCapacityBar extends StatelessWidget {
  const PrismeCapacityBar({
    super.key,
    required this.label,
    required this.unit,
    this.used,
    this.total,
    this.tone = PrismeTone.neutral,
  }) : assert(used == null || (used >= 0 && used < double.infinity)),
       assert(total == null || (total > 0 && total < double.infinity));
  final String label, unit;
  final double? used, total;
  final PrismeTone tone;
  @override
  Widget build(BuildContext context) {
    final known = used != null && total != null;
    final s = context.prismeStrings;
    final value = known
        ? '${s.number(used!, decimals: 1)} / ${s.number(total!, decimals: 1)} $unit'
        : s.unavailable;
    return Semantics(
      label: label,
      value: value,
      excludeSemantics: true,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Wrap(
            alignment: WrapAlignment.spaceBetween,
            spacing: 8,
            children: [
              Text(label, style: Theme.of(context).textTheme.bodySmall),
              Text(value, style: PrismeTheme.mono(context)),
            ],
          ),
          const SizedBox(height: 8),
          if (known)
            LinearProgressIndicator(
              value: (used! / total!).clamp(0, 1),
              minHeight: 3,
              color: tone.foreground(context.prismeColors),
              borderRadius: BorderRadius.circular(2),
            ),
        ],
      ),
    );
  }
}

/// Null means progress is not measurable, not zero. Domain progress must be 0..1.
class PrismeProgress extends StatelessWidget {
  const PrismeProgress({super.key, this.value, this.label})
    : assert(value == null || (value >= 0 && value <= 1));
  final double? value;
  final String? label;
  @override
  Widget build(BuildContext context) {
    final text =
        label ??
        (value == null
            ? context.prismeStrings.progressUnavailable
            : context.prismeStrings.percent(value!));
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Text(text, style: Theme.of(context).textTheme.bodySmall),
        if (value != null) ...[
          const SizedBox(height: 8),
          LinearProgressIndicator(
            value: value,
            semanticsLabel: label,
            semanticsValue: context.prismeStrings.percent(value!),
            minHeight: 3,
          ),
        ],
      ],
    );
  }
}

/// Small read-only time series. Null samples are gaps, never joined or replaced
/// with zero. Axes/units and a textual range accompany the graphical mark.
class PrismeSparkline extends StatelessWidget {
  const PrismeSparkline({
    super.key,
    required this.samples,
    required this.label,
    required this.unit,
    required this.startLabel,
    required this.endLabel,
    this.height = 48,
  });
  final List<double?> samples;
  final String label, unit, startLabel, endLabel;
  final double height;
  @override
  Widget build(BuildContext context) {
    final finite = samples
        .whereType<double>()
        .where((n) => n.isFinite)
        .toList();
    if (finite.isEmpty) {
      return Text(
        context.prismeStrings.noSamples,
        style: Theme.of(context).textTheme.bodySmall,
      );
    }
    final min = finite.reduce(math.min), max = finite.reduce(math.max);
    final s = context.prismeStrings;
    final range =
        '${s.number(min, decimals: 1)}–${s.number(max, decimals: 1)} $unit';
    return Semantics(
      label: '$label, $startLabel – $endLabel, $range',
      excludeSemantics: true,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Text(
            '$label · $range',
            style: Theme.of(context).textTheme.labelSmall,
          ),
          SizedBox(
            height: height,
            child: CustomPaint(
              painter: _SparkPainter(
                samples: List.unmodifiable(samples),
                color: context.prismeColors.accent,
                grid: context.prismeColors.border,
              ),
            ),
          ),
          Wrap(
            alignment: WrapAlignment.spaceBetween,
            spacing: 8,
            children: [
              Text(startLabel, style: Theme.of(context).textTheme.labelSmall),
              Text(endLabel, style: Theme.of(context).textTheme.labelSmall),
            ],
          ),
        ],
      ),
    );
  }
}

class _SparkPainter extends CustomPainter {
  _SparkPainter({
    required this.samples,
    required this.color,
    required this.grid,
  });
  final List<double?> samples;
  final Color color, grid;
  @override
  void paint(Canvas canvas, Size size) {
    final values = samples
        .whereType<double>()
        .where((n) => n.isFinite)
        .toList();
    if (values.isEmpty || size.isEmpty) return;
    final low = values.reduce(math.min), high = values.reduce(math.max);
    final span = high == low ? 1.0 : high - low;
    final line = Paint()
      ..color = color
      ..strokeWidth = 1.5
      ..style = PaintingStyle.stroke;
    canvas.drawLine(
      Offset(0, size.height - 2),
      Offset(size.width, size.height - 2),
      Paint()..color = grid,
    );
    var path = Path(), connected = false;
    for (var i = 0; i < samples.length; i++) {
      final value = samples[i];
      if (value == null || !value.isFinite) {
        canvas.drawPath(path, line);
        path = Path();
        connected = false;
        continue;
      }
      final x = samples.length == 1
          ? size.width / 2
          : 2 + i * (size.width - 4) / (samples.length - 1);
      final y = high == low
          ? size.height / 2
          : size.height - 6 - (value - low) / span * (size.height - 12);
      if (connected) {
        path.lineTo(x, y);
      } else {
        path.moveTo(x, y);
        connected = true;
      }
      canvas.drawCircle(Offset(x, y), 1.3, Paint()..color = color);
    }
    canvas.drawPath(path, line);
  }

  @override
  bool shouldRepaint(covariant _SparkPainter old) =>
      old.samples != samples || old.color != color || old.grid != grid;
}
