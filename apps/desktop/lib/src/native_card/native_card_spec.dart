import 'native_card_catalog_generated.dart';

enum NativeComponentType {
  container,
  row,
  column,
  stack,
  grid,
  scroll,
  divider,
  text,
  icon,
  image,
  badge,
  progress,
  chart,
  button,
  textInput,
  checkbox,
  select,
  slider,
  list,
  keyValue,
  emptyState,
  errorState,
}

enum NativeActionType {
  set,
  increment,
  toggle,
  append,
  remove,
  startTimer,
  stopTimer,
  capabilityInvoke,
}

class NativeAction {
  const NativeAction({
    required this.type,
    this.path,
    this.value,
    this.method,
    this.params = const {},
  });

  factory NativeAction.fromJson(Map<String, Object?> json) {
    final wireType = _requiredString(json, 'type');
    final requiredFields = nativeCatalogActionRequiredFields[wireType];
    final optionalFields = nativeCatalogActionOptionalFields[wireType];
    if (requiredFields == null || optionalFields == null) {
      throw FormatException('unknown native action: $wireType');
    }
    _rejectUnknown(json, {
      'type',
      ...requiredFields,
      ...optionalFields,
    }, 'action');
    for (final field in requiredFields) {
      if (!json.containsKey(field)) {
        throw FormatException('$wireType action requires $field');
      }
    }
    final type = _actionType(wireType);
    final path = _optionalString(json, 'path');
    final method = _optionalString(json, 'method');
    final params = !json.containsKey('params')
        ? const <String, Object?>{}
        : _object(json['params'], 'action.params');

    if (type == NativeActionType.capabilityInvoke &&
        (method == null || method.isEmpty)) {
      throw const FormatException('capability.invoke action requires method');
    }
    if (type != NativeActionType.capabilityInvoke &&
        (path == null || path.isEmpty)) {
      throw FormatException('${_actionName(type)} action requires path');
    }

    return NativeAction(
      type: type,
      path: path,
      value: json['value'],
      method: method,
      params: Map.unmodifiable(params),
    );
  }

  final NativeActionType type;
  final String? path;
  final Object? value;
  final String? method;
  final Map<String, Object?> params;

  Map<String, Object?> toJson() {
    final wireType = _actionWireName(type);
    final valueRequired = nativeCatalogActionRequiredFields[wireType]!.contains(
      'value',
    );
    return {
      'type': wireType,
      if (path != null) 'path': path,
      if (_actionCarriesValue(type) && (valueRequired || value != null))
        'value': value,
      if (method != null) 'method': method,
      if (params.isNotEmpty) 'params': params,
    };
  }
}

class NativeNode {
  const NativeNode({
    required this.id,
    required this.type,
    required this.props,
    required this.events,
    required this.children,
  });

  final String id;
  final NativeComponentType type;
  final Map<String, Object?> props;
  final Map<String, List<NativeAction>> events;
  final List<NativeNode> children;

  Map<String, Object?> toJson() {
    return {
      'id': id,
      'type': _componentWireName(type),
      if (props.isNotEmpty) 'props': props,
      if (events.isNotEmpty)
        'events': events.map(
          (name, actions) =>
              MapEntry(name, actions.map((action) => action.toJson()).toList()),
        ),
      if (children.isNotEmpty)
        'children': children.map((child) => child.toJson()).toList(),
    };
  }
}

class NativeCardSpec {
  const NativeCardSpec({
    required this.schemaVersion,
    required this.initialState,
    required this.root,
    required this.nodeCount,
  });

  factory NativeCardSpec.fromJson(Map<String, Object?> json) {
    _rejectUnknown(json, const {
      'schemaVersion',
      'initialState',
      'root',
    }, 'NativeCardSpec');
    final schemaVersion = _requiredInt(json, 'schemaVersion');
    if (schemaVersion != 1) {
      throw const FormatException('schemaVersion must be 1');
    }

    final counter = _NodeCounter();
    final root = _parseNode(
      _object(json['root'], 'root'),
      depth: 1,
      counter: counter,
    );
    return NativeCardSpec(
      schemaVersion: schemaVersion,
      initialState: Map.unmodifiable(
        _object(json['initialState'], 'initialState'),
      ),
      root: root,
      nodeCount: counter.value,
    );
  }

  final int schemaVersion;
  final Map<String, Object?> initialState;
  final NativeNode root;
  final int nodeCount;

  Map<String, Object?> toJson() {
    return {
      'schemaVersion': schemaVersion,
      'initialState': initialState,
      'root': root.toJson(),
    };
  }
}

sealed class BindingValue {
  const BindingValue();

  factory BindingValue.fromJson(Object? value) {
    if (value is Map<String, Object?> && value.length == 1) {
      if (value['path'] case final String path) {
        return PathBindingValue(path);
      }
      if (value['expr'] case final Map<String, Object?> expression) {
        return ExpressionBindingValue(Map.unmodifiable(expression));
      }
    }
    return LiteralBindingValue(value);
  }
}

class LiteralBindingValue extends BindingValue {
  const LiteralBindingValue(this.value);

  final Object? value;
}

class PathBindingValue extends BindingValue {
  const PathBindingValue(this.path);

  final String path;
}

class ExpressionBindingValue extends BindingValue {
  const ExpressionBindingValue(this.expression);

  final Map<String, Object?> expression;
}

NativeNode _parseNode(
  Map<String, Object?> json, {
  required int depth,
  required _NodeCounter counter,
}) {
  if (depth > 32) {
    throw const FormatException('NativeCard tree exceeds depth 32');
  }
  counter.value++;
  if (counter.value > 500) {
    throw const FormatException('NativeCard tree exceeds 500 nodes');
  }

  _rejectUnknown(json, const {
    'id',
    'type',
    'props',
    'events',
    'children',
  }, 'node');
  final id = _requiredString(json, 'id');
  final wireType = _requiredString(json, 'type');
  final type = _componentType(wireType);
  final allowedProps = nativeCatalogComponentProps[wireType];
  final requiredProps = nativeCatalogComponentRequiredProps[wireType];
  final allowedEvents = nativeCatalogComponentEvents[wireType];
  if (allowedProps == null || requiredProps == null || allowedEvents == null) {
    throw FormatException('unknown native component: $wireType');
  }
  final props = json['props'] == null
      ? const <String, Object?>{}
      : _object(json['props'], 'node.props');
  _rejectUnknown(props, allowedProps, '$wireType props');
  for (final prop in requiredProps) {
    if (!props.containsKey(prop)) {
      throw FormatException('$wireType requires prop $prop');
    }
  }
  final childrenValue = json['children'];
  final childMaps = childrenValue == null
      ? const <Map<String, Object?>>[]
      : _objectList(childrenValue, 'node.children');
  if (childMaps.length > 200) {
    throw const FormatException('node children exceed 200');
  }

  final events = <String, List<NativeAction>>{};
  final rawEvents = json['events'];
  if (rawEvents != null) {
    for (final entry in _object(rawEvents, 'node.events').entries) {
      if (!allowedEvents.contains(entry.key)) {
        throw FormatException('$wireType does not support event ${entry.key}');
      }
      if (entry.value is! List) {
        throw FormatException('event ${entry.key} must be an array');
      }
      if ((entry.value! as List).length > nativeCatalogMaxActionsPerEvent) {
        throw FormatException(
          'event ${entry.key} exceeds $nativeCatalogMaxActionsPerEvent actions',
        );
      }
      events[entry.key] = List.unmodifiable(
        (entry.value! as List).map(
          (action) => NativeAction.fromJson(_object(action, 'event action')),
        ),
      );
    }
  }

  return NativeNode(
    id: id,
    type: type,
    props: Map.unmodifiable(props),
    events: Map.unmodifiable(events),
    children: List.unmodifiable(
      childMaps.map(
        (child) => _parseNode(child, depth: depth + 1, counter: counter),
      ),
    ),
  );
}

NativeComponentType _componentType(String value) {
  const values = {
    'Container': NativeComponentType.container,
    'Row': NativeComponentType.row,
    'Column': NativeComponentType.column,
    'Stack': NativeComponentType.stack,
    'Grid': NativeComponentType.grid,
    'Scroll': NativeComponentType.scroll,
    'Divider': NativeComponentType.divider,
    'Text': NativeComponentType.text,
    'Icon': NativeComponentType.icon,
    'Image': NativeComponentType.image,
    'Badge': NativeComponentType.badge,
    'Progress': NativeComponentType.progress,
    'Chart': NativeComponentType.chart,
    'Button': NativeComponentType.button,
    'TextInput': NativeComponentType.textInput,
    'Checkbox': NativeComponentType.checkbox,
    'Select': NativeComponentType.select,
    'Slider': NativeComponentType.slider,
    'List': NativeComponentType.list,
    'KeyValue': NativeComponentType.keyValue,
    'EmptyState': NativeComponentType.emptyState,
    'ErrorState': NativeComponentType.errorState,
  };
  final type = values[value];
  if (type == null) {
    throw FormatException('unknown native component: $value');
  }
  return type;
}

NativeActionType _actionType(String value) {
  const values = {
    'set': NativeActionType.set,
    'increment': NativeActionType.increment,
    'toggle': NativeActionType.toggle,
    'append': NativeActionType.append,
    'remove': NativeActionType.remove,
    'startTimer': NativeActionType.startTimer,
    'stopTimer': NativeActionType.stopTimer,
    'capability.invoke': NativeActionType.capabilityInvoke,
  };
  final type = values[value];
  if (type == null) {
    throw FormatException('unknown native action: $value');
  }
  return type;
}

String _actionName(NativeActionType type) {
  return type.name;
}

String _componentWireName(NativeComponentType type) {
  return switch (type) {
    NativeComponentType.container => 'Container',
    NativeComponentType.row => 'Row',
    NativeComponentType.column => 'Column',
    NativeComponentType.stack => 'Stack',
    NativeComponentType.grid => 'Grid',
    NativeComponentType.scroll => 'Scroll',
    NativeComponentType.divider => 'Divider',
    NativeComponentType.text => 'Text',
    NativeComponentType.icon => 'Icon',
    NativeComponentType.image => 'Image',
    NativeComponentType.badge => 'Badge',
    NativeComponentType.progress => 'Progress',
    NativeComponentType.chart => 'Chart',
    NativeComponentType.button => 'Button',
    NativeComponentType.textInput => 'TextInput',
    NativeComponentType.checkbox => 'Checkbox',
    NativeComponentType.select => 'Select',
    NativeComponentType.slider => 'Slider',
    NativeComponentType.list => 'List',
    NativeComponentType.keyValue => 'KeyValue',
    NativeComponentType.emptyState => 'EmptyState',
    NativeComponentType.errorState => 'ErrorState',
  };
}

String _actionWireName(NativeActionType type) {
  return switch (type) {
    NativeActionType.set => 'set',
    NativeActionType.increment => 'increment',
    NativeActionType.toggle => 'toggle',
    NativeActionType.append => 'append',
    NativeActionType.remove => 'remove',
    NativeActionType.startTimer => 'startTimer',
    NativeActionType.stopTimer => 'stopTimer',
    NativeActionType.capabilityInvoke => 'capability.invoke',
  };
}

bool _actionCarriesValue(NativeActionType type) {
  return switch (type) {
    NativeActionType.set ||
    NativeActionType.increment ||
    NativeActionType.append ||
    NativeActionType.remove ||
    NativeActionType.startTimer => true,
    NativeActionType.toggle ||
    NativeActionType.stopTimer ||
    NativeActionType.capabilityInvoke => false,
  };
}

void _rejectUnknown(
  Map<String, Object?> json,
  Set<String> allowed,
  String context,
) {
  final unknown = json.keys.where((key) => !allowed.contains(key)).toList();
  if (unknown.isNotEmpty) {
    throw FormatException('$context has unknown fields: ${unknown.join(', ')}');
  }
}

String _requiredString(Map<String, Object?> json, String key) {
  final value = json[key];
  if (value is! String || value.isEmpty) {
    throw FormatException('$key must be a non-empty string');
  }
  return value;
}

String? _optionalString(Map<String, Object?> json, String key) {
  final value = json[key];
  if (value == null) {
    return null;
  }
  if (value is! String) {
    throw FormatException('$key must be a string');
  }
  return value;
}

int _requiredInt(Map<String, Object?> json, String key) {
  final value = json[key];
  if (value is! int) {
    throw FormatException('$key must be an integer');
  }
  return value;
}

Map<String, Object?> _object(Object? value, String context) {
  if (value is! Map<String, Object?>) {
    throw FormatException('$context must be an object');
  }
  return value;
}

List<Map<String, Object?>> _objectList(Object? value, String context) {
  if (value is! List) {
    throw FormatException('$context must be an array');
  }
  return value.map((item) => _object(item, context)).toList();
}

class _NodeCounter {
  int value = 0;
}
