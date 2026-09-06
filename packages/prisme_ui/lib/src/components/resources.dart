import 'package:flutter/material.dart';
import '../foundation/colors.dart';
import 'actions.dart';
import 'metrics.dart';
import 'status.dart';
import 'surfaces.dart';

/// Common frame for machines, services, projects and backups. Data is supplied
/// by the feature layer. Cached health is never painted as currently healthy.
class PrismeResourceCard extends StatelessWidget {
  const PrismeResourceCard({
    super.key,
    required this.title,
    required this.icon,
    required this.observation,
    required this.status,
    this.subtitle,
    this.child,
    this.footer,
    this.onTap,
  });
  final String title;
  final IconData icon;
  final PrismeObservation observation;
  final PrismeStatus status;
  final String? subtitle;
  final Widget? child, footer;
  final VoidCallback? onTap;
  @override
  Widget build(BuildContext context) => PrismeSurface(
    onTap: onTap,
    semanticLabel: onTap == null ? null : title,
    child: Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Icon(icon, size: 20, color: context.prismeColors.muted),
            const SizedBox(width: 10),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(title, style: Theme.of(context).textTheme.titleSmall),
                  if (subtitle != null)
                    Text(
                      subtitle!,
                      style: Theme.of(context).textTheme.bodySmall,
                    ),
                ],
              ),
            ),
          ],
        ),
        const SizedBox(height: 12),
        Wrap(
          spacing: 8,
          runSpacing: 8,
          children: [
            PrismeStatusBadge(status: observation.displayStatus(status)),
            PrismeFreshnessBadge(observation: observation),
          ],
        ),
        if (child != null) ...[const SizedBox(height: 16), child!],
        if (footer != null) ...[
          const SizedBox(height: 16),
          const Divider(),
          const SizedBox(height: 12),
          footer!,
        ],
      ],
    ),
  );
}

class PrismeWorkerCard extends StatelessWidget {
  const PrismeWorkerCard({
    super.key,
    required this.name,
    required this.task,
    required this.icon,
    required this.observation,
    required this.status,
    required this.contextLabel,
    this.progress,
    this.progressLabel,
    this.resources,
    this.request,
    this.reviewLabel,
    this.onReview,
    this.onOpen,
  }) : assert(onReview == null || (request != null && reviewLabel != null));
  final String name, task, contextLabel;
  final IconData icon;
  final PrismeObservation observation;
  final PrismeStatus status;
  final double? progress;
  final String? progressLabel, resources, request, reviewLabel;
  final VoidCallback? onReview, onOpen;
  @override
  Widget build(BuildContext context) => PrismeResourceCard(
    title: name,
    icon: icon,
    observation: observation,
    status: status,
    subtitle: contextLabel,
    child: Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        if (onOpen == null)
          Text(task, style: Theme.of(context).textTheme.titleSmall)
        else
          PrismeButton(
            label: task,
            onPressed: onOpen,
            variant: PrismeButtonVariant.ghost,
          ),
        const SizedBox(height: 10),
        PrismeProgress(value: progress, label: progressLabel),
        if (resources != null) ...[
          const SizedBox(height: 8),
          Text(resources!, style: Theme.of(context).textTheme.bodySmall),
        ],
        if (request != null) ...[
          const SizedBox(height: 12),
          Container(
            padding: const EdgeInsets.all(12),
            decoration: BoxDecoration(
              color: context.prismeColors.attentionSubtle,
              borderRadius: BorderRadius.circular(6),
            ),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  request!,
                  style: TextStyle(color: context.prismeColors.attention),
                ),
                if (onReview != null) ...[
                  const SizedBox(height: 8),
                  PrismeButton(label: reviewLabel!, onPressed: onReview),
                ],
              ],
            ),
          ),
        ],
      ],
    ),
  );
}
