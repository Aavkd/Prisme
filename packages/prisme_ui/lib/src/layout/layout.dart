import 'package:flutter/material.dart';
import '../foundation/tokens.dart';

class PrismePageFrame extends StatelessWidget {
  const PrismePageFrame({
    super.key,
    required this.children,
    this.header,
    this.controller,
  });
  final List<Widget> children;
  final Widget? header;
  final ScrollController? controller;
  @override
  Widget build(BuildContext context) => ListView(
    controller: controller,
    padding: EdgeInsets.all(
      MediaQuery.sizeOf(context).width < PrismeBreakpoints.compact ? 16 : 24,
    ),
    children: [
      if (header != null) ...[header!, const SizedBox(height: 24)],
      for (var i = 0; i < children.length; i++) ...[
        children[i],
        if (i < children.length - 1) const SizedBox(height: 24),
      ],
    ],
  );
}

/// Responds to available pane width, including reduced space inside an inspector.
/// Large text automatically stacks the panes instead of shrinking typography.
class PrismeAdaptiveColumns extends StatelessWidget {
  const PrismeAdaptiveColumns({
    super.key,
    required this.primary,
    required this.secondary,
    this.breakpoint = 720,
    this.gap = 24,
  });
  final Widget primary, secondary;
  final double breakpoint, gap;
  @override
  Widget build(BuildContext context) => LayoutBuilder(
    builder: (context, box) {
      if (box.maxWidth / MediaQuery.textScalerOf(context).scale(1) <
          breakpoint) {
        return Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            primary,
            SizedBox(height: gap),
            secondary,
          ],
        );
      }
      return Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Expanded(child: primary),
          SizedBox(width: gap),
          Expanded(child: secondary),
        ],
      );
    },
  );
}
