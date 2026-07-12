import 'package:flutter/widgets.dart';

typedef RuntimeVisibilityWidgetBuilder =
    Widget Function(BuildContext context, bool visible);

class RuntimeVisibilityBuilder extends StatefulWidget {
  const RuntimeVisibilityBuilder({required this.builder, super.key});

  final RuntimeVisibilityWidgetBuilder builder;

  @override
  State<RuntimeVisibilityBuilder> createState() =>
      _RuntimeVisibilityBuilderState();
}

class _RuntimeVisibilityBuilderState extends State<RuntimeVisibilityBuilder>
    with WidgetsBindingObserver {
  late bool _visible = _isVisible(WidgetsBinding.instance.lifecycleState);

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    final visible = _isVisible(state);
    if (visible == _visible) return;
    setState(() => _visible = visible);
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => widget.builder(context, _visible);
}

bool _isVisible(AppLifecycleState? state) {
  return state == null ||
      state == AppLifecycleState.resumed ||
      state == AppLifecycleState.inactive;
}
