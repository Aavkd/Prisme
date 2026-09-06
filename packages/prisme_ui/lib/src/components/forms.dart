import 'package:flutter/material.dart';
import 'package:lucide_icons_flutter/lucide_icons.dart';
import '../l10n/strings.dart';

class PrismeTextField extends StatelessWidget {
  const PrismeTextField({
    super.key,
    required this.label,
    this.controller,
    this.hint,
    this.helper,
    this.error,
    this.validator,
    this.onChanged,
    this.onSubmitted,
    this.enabled = true,
    this.readOnly = false,
    this.maxLines = 1,
    this.keyboardType,
    this.autofocus = false,
    this.focusNode,
  });
  final String label;
  final TextEditingController? controller;
  final String? hint, helper, error;
  final FormFieldValidator<String>? validator;
  final ValueChanged<String>? onChanged, onSubmitted;
  final bool enabled, readOnly, autofocus;
  final int maxLines;
  final TextInputType? keyboardType;
  final FocusNode? focusNode;
  @override
  Widget build(BuildContext context) => TextFormField(
    controller: controller,
    focusNode: focusNode,
    enabled: enabled,
    readOnly: readOnly,
    autofocus: autofocus,
    maxLines: maxLines,
    keyboardType: keyboardType,
    validator: validator,
    onChanged: onChanged,
    onFieldSubmitted: onSubmitted,
    decoration: InputDecoration(
      labelText: label,
      hintText: hint,
      helperText: helper,
      errorText: error,
    ),
  );
}

class PrismeOption<T> {
  const PrismeOption(this.value, this.label, {this.enabled = true});
  final T value;
  final String label;
  final bool enabled;
}

class PrismeSelect<T> extends StatelessWidget {
  const PrismeSelect({
    super.key,
    required this.label,
    required this.options,
    required this.value,
    required this.onChanged,
    this.error,
  });
  final String label;
  final List<PrismeOption<T>> options;
  final T? value;
  final ValueChanged<T?>? onChanged;
  final String? error;
  @override
  Widget build(BuildContext context) => InputDecorator(
    decoration: InputDecoration(
      labelText: label,
      errorText: error,
      enabled: onChanged != null,
    ),
    child: DropdownButtonHideUnderline(
      child: DropdownButton<T>(
        value: value,
        isExpanded: true,
        isDense: true,
        onChanged: onChanged,
        items: options
            .map(
              (option) => DropdownMenuItem<T>(
                value: option.value,
                enabled: option.enabled,
                child: Text(option.label),
              ),
            )
            .toList(),
      ),
    ),
  );
}

class PrismeSearchField extends StatelessWidget {
  const PrismeSearchField({
    super.key,
    required this.controller,
    required this.onChanged,
    this.label,
    this.autofocus = false,
  });
  final TextEditingController controller;
  final ValueChanged<String> onChanged;
  final String? label;
  final bool autofocus;
  @override
  Widget build(BuildContext context) =>
      ValueListenableBuilder<TextEditingValue>(
        valueListenable: controller,
        builder: (context, value, _) => TextField(
          controller: controller,
          autofocus: autofocus,
          onChanged: onChanged,
          decoration: InputDecoration(
            labelText: label ?? context.prismeStrings.search,
            prefixIcon: const Icon(LucideIcons.search, size: 18),
            suffixIcon: value.text.isEmpty
                ? null
                : IconButton(
                    tooltip: context.prismeStrings.clear,
                    icon: const Icon(LucideIcons.x, size: 18),
                    onPressed: () {
                      controller.clear();
                      onChanged('');
                    },
                  ),
          ),
        ),
      );
}

/// A concealed value field. No storage, clipboard action, or secret retrieval.
/// Revealed values are hidden again on focus loss or app inactivity.
class PrismeSecretField extends StatefulWidget {
  const PrismeSecretField({
    super.key,
    required this.label,
    required this.controller,
    this.enabled = true,
  });
  final String label;
  final TextEditingController controller;
  final bool enabled;
  @override
  State<PrismeSecretField> createState() => _PrismeSecretFieldState();
}

class _PrismeSecretFieldState extends State<PrismeSecretField>
    with WidgetsBindingObserver {
  final _focus = FocusNode();
  bool _visible = false;
  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    _focus.addListener(_blur);
  }

  void _blur() {
    if (!_focus.hasFocus && _visible) setState(() => _visible = false);
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    if (state != AppLifecycleState.resumed && _visible) {
      setState(() => _visible = false);
    }
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    _focus.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => TextField(
    controller: widget.controller,
    focusNode: _focus,
    enabled: widget.enabled,
    obscureText: !_visible,
    autocorrect: false,
    enableSuggestions: false,
    enableIMEPersonalizedLearning: false,
    decoration: InputDecoration(
      labelText: widget.label,
      suffixIcon: IconButton(
        tooltip: _visible
            ? context.prismeStrings.hideSecret
            : context.prismeStrings.showSecret,
        onPressed: widget.enabled
            ? () {
                _focus.requestFocus();
                setState(() => _visible = !_visible);
              }
            : null,
        icon: Icon(_visible ? LucideIcons.eyeOff : LucideIcons.eye, size: 18),
      ),
    ),
  );
}
