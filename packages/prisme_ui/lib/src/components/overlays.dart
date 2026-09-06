import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:lucide_icons_flutter/lucide_icons.dart';
import '../foundation/tokens.dart';
import '../l10n/strings.dart';
import 'actions.dart';
import 'status.dart';
import 'surfaces.dart';
import 'forms.dart';

Future<T?> showPrismeDialog<T>({
  required BuildContext context,
  required String title,
  required WidgetBuilder builder,
  bool dismissible = true,
  double width = 480,
}) => showDialog<T>(
  context: context,
  barrierDismissible: dismissible,
  builder: (dialogContext) => Dialog(
    child: ConstrainedBox(
      constraints: BoxConstraints(
        maxWidth: width,
        maxHeight: MediaQuery.sizeOf(dialogContext).height * .85,
      ),
      child: _OverlayBody(title: title, child: builder(dialogContext)),
    ),
  ),
);

/// Right-hand modal on desktop; native bottom sheet on compact screens.
Future<T?> showPrismeInspector<T>({
  required BuildContext context,
  required String title,
  required WidgetBuilder builder,
}) {
  if (MediaQuery.sizeOf(context).width >= PrismeBreakpoints.desktop) {
    return showDialog<T>(
      context: context,
      builder: (dialogContext) => Align(
        alignment: AlignmentDirectional.centerEnd,
        child: Dialog(
          insetPadding: const EdgeInsets.all(16),
          child: SizedBox(
            width: PrismeBreakpoints.inspector,
            child: _OverlayBody(title: title, child: builder(dialogContext)),
          ),
        ),
      ),
    );
  }
  return showModalBottomSheet<T>(
    context: context,
    isScrollControlled: true,
    useSafeArea: true,
    builder: (sheetContext) => Padding(
      padding: EdgeInsets.only(
        bottom: MediaQuery.viewInsetsOf(sheetContext).bottom,
      ),
      child: ConstrainedBox(
        constraints: BoxConstraints(
          maxHeight: MediaQuery.sizeOf(sheetContext).height * .85,
        ),
        child: _OverlayBody(title: title, child: builder(sheetContext)),
      ),
    ),
  );
}

class _OverlayBody extends StatelessWidget {
  const _OverlayBody({required this.title, required this.child});
  final String title;
  final Widget child;
  @override
  Widget build(BuildContext context) => CallbackShortcuts(
    bindings: {
      const SingleActivator(LogicalKeyboardKey.escape): () =>
          Navigator.of(context).pop(),
    },
    child: FocusScope(
      autofocus: true,
      child: FocusTraversalGroup(
        child: Padding(
          padding: const EdgeInsets.all(24),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              Row(
                children: [
                  Expanded(
                    child: Semantics(
                      header: true,
                      child: Text(
                        title,
                        style: Theme.of(context).textTheme.headlineSmall,
                      ),
                    ),
                  ),
                  PrismeIconButton(
                    icon: LucideIcons.x,
                    label: context.prismeStrings.close,
                    onPressed: () => Navigator.of(context).pop(),
                  ),
                ],
              ),
              const SizedBox(height: 16),
              Flexible(child: SingleChildScrollView(child: child)),
            ],
          ),
        ),
      ),
    ),
  );
}

@immutable
class PrismeActionPlan {
  const PrismeActionPlan({
    required this.title,
    required this.actionLabel,
    required this.target,
    required this.environment,
    required this.impact,
    required this.verification,
    required this.rollback,
    this.requiredPhrase,
  });
  final String title, actionLabel, target, impact, verification, rollback;
  final PrismeEnvironment environment;
  final String? requiredPhrase;
}

/// Returns the user's local decision only. The host must revalidate permissions,
/// freshness, target, policy and (on Android) biometrics before an API mutation.
Future<bool> showPrismeConfirmation({
  required BuildContext context,
  required PrismeActionPlan plan,
}) async =>
    await showPrismeDialog<bool>(
      context: context,
      title: plan.title,
      dismissible: false,
      builder: (_) => _ConfirmationBody(plan: plan),
    ) ??
    false;

class _ConfirmationBody extends StatefulWidget {
  const _ConfirmationBody({required this.plan});
  final PrismeActionPlan plan;
  @override
  State<_ConfirmationBody> createState() => _ConfirmationBodyState();
}

class _ConfirmationBodyState extends State<_ConfirmationBody> {
  final _phrase = TextEditingController();
  @override
  void dispose() {
    _phrase.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final p = widget.plan, s = context.prismeStrings;
    final ready = p.requiredPhrase == null || _phrase.text == p.requiredPhrase;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Wrap(children: [PrismeEnvironmentBadge(environment: p.environment)]),
        const SizedBox(height: 16),
        PrismeKeyValue(label: s.target, value: p.target),
        const SizedBox(height: 16),
        PrismeKeyValue(label: s.impact, value: p.impact),
        const SizedBox(height: 16),
        PrismeKeyValue(label: s.verification, value: p.verification),
        const SizedBox(height: 16),
        PrismeKeyValue(label: s.rollback, value: p.rollback),
        if (p.requiredPhrase != null) ...[
          const SizedBox(height: 20),
          PrismeTextField(
            label: s.typeToConfirm(p.requiredPhrase!),
            controller: _phrase,
            onChanged: (_) => setState(() {}),
          ),
        ],
        const SizedBox(height: 24),
        Wrap(
          alignment: WrapAlignment.end,
          spacing: 8,
          runSpacing: 8,
          children: [
            PrismeButton(
              label: s.cancel,
              onPressed: () => Navigator.of(context).pop(false),
            ),
            PrismeButton(
              label: p.actionLabel,
              variant: PrismeButtonVariant.danger,
              onPressed: ready ? () => Navigator.of(context).pop(true) : null,
            ),
          ],
        ),
      ],
    );
  }
}
