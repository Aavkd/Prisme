import 'package:flutter/material.dart';
import '../foundation/colors.dart';
import '../foundation/theme.dart';
import '../l10n/strings.dart';
import 'status.dart';
import 'surfaces.dart';

class PrismeTableColumn<T> {
  const PrismeTableColumn({
    required this.label,
    required this.value,
    this.numeric = false,
  });
  final String label;
  final String Function(T row) value;
  final bool numeric;
}

/// Short summary tables. On narrow screens each row becomes a labelled card;
/// no column disappears. Feature layers own pagination and server-side sorting.
class PrismeDataTable<T> extends StatelessWidget {
  const PrismeDataTable({
    super.key,
    required this.columns,
    required this.rows,
    this.onOpen,
    this.onSort,
    this.sortColumnIndex,
    this.sortAscending = true,
  }) : assert(columns.length > 0);
  final List<PrismeTableColumn<T>> columns;
  final List<T> rows;
  final ValueChanged<T>? onOpen;
  final void Function(int column, bool ascending)? onSort;
  final int? sortColumnIndex;
  final bool sortAscending;
  @override
  Widget build(BuildContext context) {
    if (rows.isEmpty) {
      return Padding(
        padding: const EdgeInsets.all(24),
        child: Text(context.prismeStrings.noData),
      );
    }
    return LayoutBuilder(
      builder: (context, box) {
        final narrow =
            box.maxWidth / MediaQuery.textScalerOf(context).scale(1) < 720;
        if (narrow) {
          return Column(
            children: [
              for (final row in rows)
                Padding(
                  padding: const EdgeInsets.only(bottom: 12),
                  child: PrismeSurface(
                    onTap: onOpen == null ? null : () => onOpen!(row),
                    semanticLabel: columns.first.value(row),
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        for (final column in columns)
                          Padding(
                            padding: const EdgeInsets.only(bottom: 8),
                            child: PrismeKeyValue(
                              label: column.label,
                              value: column.value(row),
                            ),
                          ),
                      ],
                    ),
                  ),
                ),
            ],
          );
        }
        return SingleChildScrollView(
          scrollDirection: Axis.horizontal,
          child: ConstrainedBox(
            constraints: BoxConstraints(minWidth: box.maxWidth),
            child: DataTable(
              showCheckboxColumn: false,
              sortColumnIndex: sortColumnIndex,
              sortAscending: sortAscending,
              columns: [
                for (var i = 0; i < columns.length; i++)
                  DataColumn(
                    label: Text(columns[i].label),
                    numeric: columns[i].numeric,
                    onSort: onSort == null
                        ? null
                        : (_, ascending) => onSort!(i, ascending),
                  ),
              ],
              rows: rows
                  .map(
                    (row) => DataRow(
                      onSelectChanged: onOpen == null
                          ? null
                          : (_) => onOpen!(row),
                      cells: columns
                          .map((col) => DataCell(Text(col.value(row))))
                          .toList(),
                    ),
                  )
                  .toList(),
            ),
          ),
        );
      },
    );
  }
}

@immutable
class PrismeTimelineEvent {
  const PrismeTimelineEvent({
    required this.title,
    required this.time,
    required this.icon,
    this.description,
    this.tone = PrismeTone.neutral,
  });
  final String title;
  final String? description;
  final DateTime time;
  final IconData icon;
  final PrismeTone tone;
}

class PrismeTimeline extends StatelessWidget {
  const PrismeTimeline({super.key, required this.events});
  final List<PrismeTimelineEvent> events;
  @override
  Widget build(BuildContext context) => Column(
    children: [
      for (final event in events)
        Padding(
          padding: const EdgeInsets.only(bottom: 18),
          child: Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Icon(
                event.icon,
                size: 16,
                color: event.tone.foreground(context.prismeColors),
              ),
              const SizedBox(width: 10),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      event.title,
                      style: Theme.of(context).textTheme.titleSmall,
                    ),
                    if (event.description != null)
                      Text(
                        event.description!,
                        style: Theme.of(context).textTheme.bodySmall,
                      ),
                    Text(
                      context.prismeStrings.observedAt(event.time),
                      style: Theme.of(context).textTheme.labelSmall,
                    ),
                  ],
                ),
              ),
            ],
          ),
        ),
    ],
  );
}

/// Plain, selectable text only. No terminal execution or interpretation of logs.
class PrismeCodeBlock extends StatelessWidget {
  const PrismeCodeBlock({
    super.key,
    required this.text,
    this.label,
    this.maxHeight = 280,
  });
  final String text;
  final String? label;
  final double maxHeight;
  @override
  Widget build(BuildContext context) => PrismeSurface(
    raised: true,
    child: Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        if (label != null) ...[
          Text(label!, style: Theme.of(context).textTheme.bodySmall),
          const SizedBox(height: 10),
        ],
        ConstrainedBox(
          constraints: BoxConstraints(maxHeight: maxHeight),
          child: SingleChildScrollView(
            child: SelectableText(
              text,
              style: PrismeTheme.mono(
                context,
              ).copyWith(color: context.prismeColors.text),
            ),
          ),
        ),
      ],
    ),
  );
}

enum PrismeDiffKind { context, added, removed }

class PrismeDiffLine {
  const PrismeDiffLine(this.text, this.kind);
  final String text;
  final PrismeDiffKind kind;
}

class PrismeDiffView extends StatelessWidget {
  const PrismeDiffView({super.key, required this.lines, this.height = 260});
  final List<PrismeDiffLine> lines;
  final double height;
  @override
  Widget build(BuildContext context) => PrismeSurface(
    padding: EdgeInsets.zero,
    child: SizedBox(
      height: height,
      child: ListView.builder(
        itemCount: lines.length,
        itemBuilder: (context, i) {
          final line = lines[i], c = context.prismeColors;
          final (prefix, color, bg) = switch (line.kind) {
            PrismeDiffKind.context => (' ', c.text, c.surface),
            PrismeDiffKind.added => ('+', c.healthy, c.healthySubtle),
            PrismeDiffKind.removed => ('−', c.critical, c.criticalSubtle),
          };
          return Container(
            color: bg,
            padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 5),
            child: SelectableText(
              '$prefix ${line.text}',
              style: PrismeTheme.mono(context).copyWith(color: color),
            ),
          );
        },
      ),
    ),
  );
}
