import 'dart:convert';
import 'dart:io';

import 'package:agent_card_desktop/src/native_card/native_card_spec.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  group('NativeCardSpec', () {
    test('parses the trusted pomodoro fixture', () {
      final spec = NativeCardSpec.fromJson(
        jsonDecode(_fixture()) as Map<String, Object?>,
      );

      expect(spec.schemaVersion, 1);
      expect(spec.initialState['remaining'], 1500);
      expect(spec.root.type, NativeComponentType.container);
      expect(spec.root.children.single.type, NativeComponentType.column);
      expect(spec.nodeCount, 5);
      expect(
        spec
            .root
            .children
            .single
            .children
            .last
            .events['onPressed']!
            .single
            .type,
        NativeActionType.toggle,
      );
    });

    test('rejects unknown components', () {
      final source = _baseNode(type: 'WebView');

      expect(
        () => NativeCardSpec.fromJson(_spec(source)),
        throwsA(isA<FormatException>()),
      );
    });

    test('rejects trees deeper than 32 levels', () {
      var node = _baseNode();
      for (var index = 0; index < 32; index++) {
        node = _baseNode(children: [node]);
      }

      expect(
        () => NativeCardSpec.fromJson(_spec(node)),
        throwsA(isA<FormatException>()),
      );
    });

    test('rejects trees with more than 500 nodes', () {
      final children = List.generate(500, (_) => _baseNode(type: 'Text'));

      expect(
        () => NativeCardSpec.fromJson(_spec(_baseNode(children: children))),
        throwsA(isA<FormatException>()),
      );
    });
  });
}

Map<String, Object?> _spec(Map<String, Object?> root) {
  return {
    'schemaVersion': 1,
    'initialState': <String, Object?>{},
    'root': root,
  };
}

Map<String, Object?> _baseNode({
  String type = 'Container',
  List<Map<String, Object?>> children = const [],
}) {
  return {
    'id': 'node',
    'type': type,
    if (children.isNotEmpty) 'children': children,
  };
}

String _fixture() {
  return File(
    '../../contracts/card/fixtures/pomodoro-native.json',
  ).readAsStringSync();
}
