import 'dart:convert';
import 'dart:io';

import 'package:agent_card_desktop/src/native_card/expression.dart';
import 'package:agent_card_desktop/src/native_card/native_card_catalog_generated.dart';
import 'package:agent_card_desktop/src/native_card/native_card_spec.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  final catalog =
      jsonDecode(
            File(
              '../../contracts/card/native-card-catalog.v1.json',
            ).readAsStringSync(),
          )
          as Map<String, Object?>;

  test('generated Dart semantics match the root catalog', () {
    final components = catalog['components']! as Map<String, Object?>;
    final actions = catalog['actions']! as Map<String, Object?>;
    final expressions = catalog['expressions']! as Map<String, Object?>;
    final limits = catalog['limits']! as Map<String, Object?>;

    expect(nativeCatalogMaxActionsPerEvent, limits['maxActionsPerEvent']);

    expect(nativeCatalogComponentProps.keys.toSet(), components.keys.toSet());
    expect(
      nativeCatalogActionRequiredFields.keys.toSet(),
      actions.keys.toSet(),
    );
    expect(nativeCatalogExpressionArity.keys.toSet(), expressions.keys.toSet());
    for (final entry in components.entries) {
      final rule = entry.value! as Map<String, Object?>;
      expect(
        nativeCatalogComponentProps[entry.key],
        (rule['allowedProps']! as Map<String, Object?>).keys.toSet(),
      );
      expect(
        nativeCatalogComponentRequiredProps[entry.key],
        (rule['requiredProps']! as List).cast<String>().toSet(),
      );
      expect(
        nativeCatalogComponentEvents[entry.key],
        (rule['allowedEvents']! as List).cast<String>().toSet(),
      );
    }
    for (final entry in actions.entries) {
      final rule = entry.value! as Map<String, Object?>;
      expect(
        nativeCatalogActionRequiredFields[entry.key],
        (rule['requiredFields']! as List).cast<String>().toSet(),
      );
      expect(
        nativeCatalogActionOptionalFields[entry.key],
        (rule['optionalFields']! as List).cast<String>().toSet(),
      );
    }
    for (final entry in expressions.entries) {
      final rule = entry.value! as Map<String, Object?>;
      expect(nativeCatalogExpressionArity[entry.key], (
        min: rule['minArgs']! as int,
        max: rule['maxArgs'] as int?,
      ));
    }
  });

  test('Flutter parser enforces the catalog actions-per-event limit', () {
    final action = <String, Object?>{
      'type': 'capability.invoke',
      'method': 'notification.show',
    };
    Map<String, Object?> specWithActions(int count) => _spec(
      'Button',
      const {},
      {'onPressed': List<Object?>.generate(count, (_) => action)},
      const {},
    );

    expect(
      () => NativeCardSpec.fromJson(
        specWithActions(nativeCatalogMaxActionsPerEvent),
      ),
      returnsNormally,
    );
    expect(
      () => NativeCardSpec.fromJson(
        specWithActions(nativeCatalogMaxActionsPerEvent + 1),
      ),
      throwsA(isA<FormatException>()),
    );
  });

  test(
    'Flutter parser accepts exactly catalog components props and events',
    () {
      final components = catalog['components']! as Map<String, Object?>;
      for (final entry in components.entries) {
        final rule = entry.value! as Map<String, Object?>;
        final allowedProps = rule['allowedProps']! as Map<String, Object?>;
        final props = {
          for (final prop in allowedProps.entries)
            prop.key: _sampleValue(prop.value! as Map<String, Object?>),
        };
        final events = {
          for (final event in (rule['allowedEvents']! as List).cast<String>())
            event: <Object?>[],
        };
        final state = <String, Object?>{
          'value': switch (entry.key) {
            'Checkbox' => false,
            'Select' => 'a',
            _ => 0,
          },
        };
        expect(
          () => NativeCardSpec.fromJson(_spec(entry.key, props, events, state)),
          returnsNormally,
          reason: entry.key,
        );
        expect(
          () => NativeCardSpec.fromJson(
            _spec(entry.key, {...props, '_unknown': true}, events, state),
          ),
          throwsA(isA<FormatException>()),
          reason: '${entry.key} unknown prop',
        );
      }
    },
  );

  test('Flutter actions and expressions cover the catalog', () {
    final actions = catalog['actions']! as Map<String, Object?>;
    for (final entry in actions.entries) {
      final rule = entry.value! as Map<String, Object?>;
      final action = <String, Object?>{'type': entry.key};
      for (final field in (rule['requiredFields']! as List).cast<String>()) {
        action[field] = _actionField(field, rule);
      }
      expect(
        () => NativeAction.fromJson(action),
        returnsNormally,
        reason: entry.key,
      );
      for (final field in (rule['requiredFields']! as List).cast<String>()) {
        final missing = Map<String, Object?>.from(action)..remove(field);
        expect(
          () => NativeAction.fromJson(missing),
          throwsA(isA<FormatException>()),
          reason: '${entry.key} missing $field',
        );
      }
    }

    const evaluator = NativeExpressionEvaluator();
    final expressions = catalog['expressions']! as Map<String, Object?>;
    for (final entry in expressions.entries) {
      final rule = entry.value! as Map<String, Object?>;
      final count = rule['minArgs']! as int;
      final argumentType = rule['argumentType']! as String;
      final args = List<Object?>.generate(
        count,
        (index) => switch (argumentType) {
          'number' => index == 1 ? 1 : 2,
          'boolean' => true,
          _ => 'value',
        },
      );
      expect(
        () => evaluator.evaluate({'op': entry.key, 'args': args}, const {}),
        returnsNormally,
        reason: entry.key,
      );
    }
    final literalWithExpressionFields = <String, Object?>{
      'op': 'add',
      'args': [1],
      'extra': true,
    };
    expect(
      const NativeExpressionEvaluator().evaluate(
        literalWithExpressionFields,
        const {},
      ),
      same(literalWithExpressionFields),
    );
  });
}

Map<String, Object?> _spec(
  String type,
  Map<String, Object?> props,
  Map<String, Object?> events,
  Map<String, Object?> state,
) {
  return {
    'schemaVersion': 1,
    'initialState': state,
    'root': {
      'id': 'root',
      'type': type,
      if (props.isNotEmpty) 'props': props,
      if (events.isNotEmpty) 'events': events,
    },
  };
}

Object? _sampleValue(Map<String, Object?> rule) {
  final values = (rule['enum'] as List?)?.cast<String>();
  if (values != null && values.isNotEmpty) {
    return values.first;
  }
  return switch (rule['type']) {
    'string' => rule['pattern'] == null ? 'value' : 'value',
    'number' => 0.5,
    'integer' => 1,
    'boolean' => true,
    'stringList' => ['a'],
    'numberList' => [1],
    'object' => <String, Object?>{},
    'timerConfiguration' => {'intervalMs': 1000, 'delta': -1},
    _ => 'value',
  };
}

Object? _actionField(String field, Map<String, Object?> rule) {
  return switch (field) {
    'path' => 'value',
    'method' => 'notification.show',
    'params' => <String, Object?>{},
    'value' => _sampleValue(rule['valueRule']! as Map<String, Object?>),
    _ => throw StateError('unsupported action field $field'),
  };
}
