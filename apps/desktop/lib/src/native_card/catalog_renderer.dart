import 'package:flutter/material.dart';

import 'native_card_controller.dart';
import 'native_card_spec.dart';

class CatalogRenderer {
  const CatalogRenderer({required this.controller});

  final NativeCardController controller;

  Widget render(BuildContext context, NativeNode node) {
    return switch (node.type) {
      NativeComponentType.container => _container(context, node),
      NativeComponentType.row => _row(context, node),
      NativeComponentType.column => _column(context, node),
      NativeComponentType.stack => _stack(context, node),
      NativeComponentType.grid => _grid(context, node),
      NativeComponentType.scroll => _scroll(context, node),
      NativeComponentType.divider => const Divider(),
      NativeComponentType.text => _text(context, node),
      NativeComponentType.icon => _icon(node),
      NativeComponentType.image => _image(node),
      NativeComponentType.badge => _badge(context, node),
      NativeComponentType.progress => _progress(node),
      NativeComponentType.chart => _chart(context, node),
      NativeComponentType.button => _button(node),
      NativeComponentType.textInput => _textInput(node),
      NativeComponentType.checkbox => _checkbox(node),
      NativeComponentType.select => _select(node),
      NativeComponentType.slider => _slider(node),
      NativeComponentType.list => _list(context, node),
      NativeComponentType.keyValue => _keyValue(context, node),
      NativeComponentType.emptyState => _emptyState(context, node),
      NativeComponentType.errorState => _errorState(context, node),
    };
  }

  Widget _container(BuildContext context, NativeNode node) {
    final padding = _number(node.props['padding'], fallback: 0);
    final child = node.children.isEmpty
        ? const SizedBox.shrink()
        : node.children.length == 1
        ? render(context, node.children.first)
        : _column(context, node);
    return Container(padding: EdgeInsets.all(padding), child: child);
  }

  Widget _row(BuildContext context, NativeNode node) {
    final spacing = _number(node.props['spacing'], fallback: 0);
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: _spaced(
        node.children.map((child) => render(context, child)).toList(),
        spacing,
        Axis.horizontal,
      ),
    );
  }

  Widget _column(BuildContext context, NativeNode node) {
    final spacing = _number(node.props['spacing'], fallback: 0);
    return Column(
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: _spaced(
        node.children.map((child) => render(context, child)).toList(),
        spacing,
        Axis.vertical,
      ),
    );
  }

  Widget _stack(BuildContext context, NativeNode node) {
    return Stack(
      children: node.children.map((child) => render(context, child)).toList(),
    );
  }

  Widget _grid(BuildContext context, NativeNode node) {
    final columns = _integer(
      node.props['columns'],
      fallback: 2,
    ).clamp(1, 6).toInt();
    final spacing = _number(node.props['spacing'], fallback: 8);
    return GridView.count(
      crossAxisCount: columns,
      shrinkWrap: true,
      physics: const NeverScrollableScrollPhysics(),
      crossAxisSpacing: spacing,
      mainAxisSpacing: spacing,
      childAspectRatio: 3,
      children: node.children.map((child) => render(context, child)).toList(),
    );
  }

  Widget _scroll(BuildContext context, NativeNode node) {
    return SingleChildScrollView(
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: node.children.map((child) => render(context, child)).toList(),
      ),
    );
  }

  Widget _text(BuildContext context, NativeNode node) {
    final text = controller.resolve(node.props['text'])?.toString() ?? '';
    final styleName = node.props['style'];
    final theme = Theme.of(context).textTheme;
    final style = switch (styleName) {
      'display' => theme.displaySmall?.copyWith(fontWeight: FontWeight.w700),
      'title' => theme.titleLarge?.copyWith(fontWeight: FontWeight.w700),
      _ => theme.bodyMedium,
    };
    return Text(text, style: style);
  }

  Widget _icon(NativeNode node) {
    final icon = switch (node.props['name']) {
      'timer' => Icons.timer_outlined,
      'check' => Icons.check_circle_outline,
      'warning' => Icons.warning_amber_rounded,
      'settings' => Icons.settings_outlined,
      _ => Icons.widgets_outlined,
    };
    return Icon(icon);
  }

  Widget _image(NativeNode node) {
    final alt = node.props['alt']?.toString() ?? 'card image';
    return Semantics(
      label: alt,
      image: true,
      child: const Icon(Icons.image_outlined, size: 32),
    );
  }

  Widget _badge(BuildContext context, NativeNode node) {
    final label = controller.resolve(node.props['label'])?.toString() ?? '';
    return DecoratedBox(
      decoration: BoxDecoration(
        color: Theme.of(context).colorScheme.secondaryContainer,
        borderRadius: BorderRadius.circular(999),
      ),
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
        child: Text(label, style: Theme.of(context).textTheme.labelSmall),
      ),
    );
  }

  Widget _progress(NativeNode node) {
    final value = controller.resolve(node.props['value']);
    final progress = value is num
        ? value.toDouble().clamp(0, 1).toDouble()
        : null;
    return LinearProgressIndicator(value: progress);
  }

  Widget _chart(BuildContext context, NativeNode node) {
    final rawValues = controller.resolve(node.props['values']);
    final values = rawValues is List
        ? rawValues.whereType<num>().map((value) => value.toDouble()).toList()
        : const <double>[];
    return SizedBox(
      height: 72,
      child: CustomPaint(
        painter: _BarChartPainter(
          values: values,
          color: Theme.of(context).colorScheme.primary,
        ),
      ),
    );
  }

  Widget _button(NativeNode node) {
    final label = controller.resolve(node.props['label'])?.toString() ?? '';
    return FilledButton(
      onPressed: () async {
        await controller.applyActionsAsync(
          node.events['onPressed'] ?? const [],
        );
      },
      child: Text(label),
    );
  }

  Widget _textInput(NativeNode node) {
    final path = _requiredPath(node);
    final current = _stateValue(path)?.toString() ?? '';
    return TextFormField(
      key: ValueKey(node.id),
      initialValue: current,
      decoration: InputDecoration(labelText: node.props['label']?.toString()),
      onChanged: (value) => _setState(path, value),
    );
  }

  Widget _checkbox(NativeNode node) {
    final path = _requiredPath(node);
    final current = _stateValue(path);
    return CheckboxListTile(
      contentPadding: EdgeInsets.zero,
      title: Text(node.props['label']?.toString() ?? ''),
      value: current is bool ? current : false,
      onChanged: (value) => _setState(path, value ?? false),
    );
  }

  Widget _select(NativeNode node) {
    final path = _requiredPath(node);
    final options = _strings(node.props['options']);
    final current = _stateValue(path)?.toString();
    return DropdownButton<String>(
      value: options.contains(current) ? current : null,
      hint: Text(node.props['label']?.toString() ?? ''),
      isExpanded: true,
      items: options
          .map(
            (option) =>
                DropdownMenuItem<String>(value: option, child: Text(option)),
          )
          .toList(),
      onChanged: (value) {
        if (value != null) {
          _setState(path, value);
        }
      },
    );
  }

  Widget _slider(NativeNode node) {
    final path = _requiredPath(node);
    final minimum = _number(node.props['min'], fallback: 0);
    final maximum = _number(node.props['max'], fallback: 1);
    final rawValue = _stateValue(path);
    final value = (rawValue is num ? rawValue.toDouble() : minimum)
        .clamp(minimum, maximum)
        .toDouble();
    return Slider(
      value: value,
      min: minimum,
      max: maximum,
      onChanged: (next) => _setState(path, next),
    );
  }

  Widget _list(BuildContext context, NativeNode node) {
    return Column(
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: node.children
          .map(
            (child) => Padding(
              padding: const EdgeInsets.symmetric(vertical: 4),
              child: render(context, child),
            ),
          )
          .toList(),
    );
  }

  Widget _keyValue(BuildContext context, NativeNode node) {
    return Row(
      children: [
        Expanded(
          child: Text(
            controller.resolve(node.props['label'])?.toString() ?? '',
            style: Theme.of(context).textTheme.bodySmall,
          ),
        ),
        Text(
          controller.resolve(node.props['value'])?.toString() ?? '',
          style: Theme.of(
            context,
          ).textTheme.bodyMedium?.copyWith(fontWeight: FontWeight.w700),
        ),
      ],
    );
  }

  Widget _emptyState(BuildContext context, NativeNode node) {
    return _messageState(
      Icons.inbox_outlined,
      node.props['title']?.toString() ?? 'No data',
      Theme.of(context).colorScheme.onSurfaceVariant,
    );
  }

  Widget _errorState(BuildContext context, NativeNode node) {
    return _messageState(
      Icons.error_outline_rounded,
      node.props['message']?.toString() ?? 'Card error',
      Theme.of(context).colorScheme.error,
    );
  }

  Widget _messageState(IconData icon, String message, Color color) {
    return Padding(
      padding: const EdgeInsets.all(12),
      child: Row(
        mainAxisAlignment: MainAxisAlignment.center,
        children: [
          Icon(icon, color: color, size: 18),
          const SizedBox(width: 8),
          Flexible(child: Text(message)),
        ],
      ),
    );
  }

  String _requiredPath(NativeNode node) {
    final path = node.props['valuePath'];
    if (path is! String || path.isEmpty) {
      throw FlutterError('${node.type.name} requires valuePath');
    }
    return path;
  }

  Object? _stateValue(String path) {
    return controller.resolve({'path': 'state.$path'});
  }

  void _setState(String path, Object? value) {
    controller.applyAction(
      NativeAction(type: NativeActionType.set, path: path, value: value),
    );
  }

  List<String> _strings(Object? value) {
    return value is List ? value.whereType<String>().toList() : const [];
  }

  List<Widget> _spaced(List<Widget> children, double spacing, Axis axis) {
    if (children.length < 2 || spacing <= 0) {
      return children;
    }
    final result = <Widget>[];
    for (var index = 0; index < children.length; index++) {
      if (index > 0) {
        result.add(
          axis == Axis.vertical
              ? SizedBox(height: spacing)
              : SizedBox(width: spacing),
        );
      }
      result.add(children[index]);
    }
    return result;
  }

  double _number(Object? value, {required double fallback}) {
    return value is num ? value.toDouble() : fallback;
  }

  int _integer(Object? value, {required int fallback}) {
    return value is int ? value : fallback;
  }
}

class _BarChartPainter extends CustomPainter {
  const _BarChartPainter({required this.values, required this.color});

  final List<double> values;
  final Color color;

  @override
  void paint(Canvas canvas, Size size) {
    if (values.isEmpty) {
      return;
    }
    final maximum = values.reduce((a, b) => a > b ? a : b);
    if (maximum <= 0) {
      return;
    }
    const gap = 4.0;
    final barWidth = (size.width - gap * (values.length - 1)) / values.length;
    final paint = Paint()..color = color;
    for (var index = 0; index < values.length; index++) {
      final height = size.height * (values[index] / maximum).clamp(0, 1);
      canvas.drawRRect(
        RRect.fromRectAndRadius(
          Rect.fromLTWH(
            index * (barWidth + gap),
            size.height - height,
            barWidth,
            height,
          ),
          const Radius.circular(3),
        ),
        paint,
      );
    }
  }

  @override
  bool shouldRepaint(covariant _BarChartPainter oldDelegate) {
    return oldDelegate.values != values || oldDelegate.color != color;
  }
}
