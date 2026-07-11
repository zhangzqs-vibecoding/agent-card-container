import 'dart:convert';
import 'dart:io';

import 'package:agent_card_desktop/src/native_card/native_card_controller.dart';
import 'package:agent_card_desktop/src/native_card/native_card_renderer.dart';
import 'package:agent_card_desktop/src/native_card/native_card_spec.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  testWidgets('renders and interacts with the pomodoro NativeCard', (
    tester,
  ) async {
    final spec = NativeCardSpec.fromJson(
      jsonDecode(_fixture()) as Map<String, Object?>,
    );
    final controller = NativeCardController(spec.initialState);
    addTearDown(controller.dispose);

    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: SizedBox(
            width: 360,
            height: 240,
            child: NativeCardRenderer(spec: spec, controller: controller),
          ),
        ),
      ),
    );

    expect(find.text('专注时间'), findsOneWidget);
    expect(find.text('25:00'), findsOneWidget);
    expect(find.text('开始 / 暂停'), findsOneWidget);

    await tester.tap(find.text('开始 / 暂停'));
    await tester.pump();
    expect(controller.state['running'], isTrue);
  });

  testWidgets('renders the trusted display and layout catalog', (tester) async {
    final spec = NativeCardSpec.fromJson({
      'schemaVersion': 1,
      'initialState': {'progress': 0.5},
      'root': _node(
        'root',
        'Scroll',
        children: [
          _node(
            'column',
            'Column',
            props: {'spacing': 8},
            children: [
              _node(
                'row',
                'Row',
                children: [
                  _node('row-text', 'Text', props: {'text': 'row'}),
                  _node('badge', 'Badge', props: {'label': 'LOCAL'}),
                ],
              ),
              _node('divider', 'Divider'),
              _node(
                'stack',
                'Stack',
                children: [
                  _node('stack-text', 'Text', props: {'text': 'stack'}),
                ],
              ),
              _node(
                'grid',
                'Grid',
                props: {'columns': 2},
                children: [
                  _node('grid-a', 'Text', props: {'text': 'grid-a'}),
                  _node('grid-b', 'Text', props: {'text': 'grid-b'}),
                ],
              ),
              _node('icon', 'Icon', props: {'name': 'timer'}),
              _node('image', 'Image', props: {'alt': 'local image'}),
              _node(
                'progress',
                'Progress',
                props: {
                  'value': {'path': 'state.progress'},
                },
              ),
              _node(
                'chart',
                'Chart',
                props: {
                  'values': [1, 3, 2],
                },
              ),
              _node(
                'list',
                'List',
                children: [
                  _node('list-item', 'Text', props: {'text': 'list-item'}),
                ],
              ),
              _node(
                'key-value',
                'KeyValue',
                props: {'label': 'Mode', 'value': 'Offline'},
              ),
              _node('empty', 'EmptyState', props: {'title': 'Nothing here'}),
              _node(
                'error',
                'ErrorState',
                props: {'message': 'Something failed'},
              ),
            ],
          ),
        ],
      ),
    });
    final controller = NativeCardController(spec.initialState);
    addTearDown(controller.dispose);

    await tester.pumpWidget(
      MaterialApp(
        home: SizedBox(
          width: 600,
          height: 800,
          child: NativeCardRenderer(spec: spec, controller: controller),
        ),
      ),
    );

    expect(find.byType(ErrorWidget), findsNothing);
    expect(find.text('row'), findsOneWidget);
    expect(find.text('LOCAL'), findsOneWidget);
    expect(find.text('grid-a'), findsOneWidget);
    expect(find.text('grid-b'), findsOneWidget);
    expect(find.text('list-item'), findsOneWidget);
    expect(find.text('Mode'), findsOneWidget);
    expect(find.text('Offline'), findsOneWidget);
    expect(find.text('Nothing here'), findsOneWidget);
    expect(find.text('Something failed'), findsOneWidget);
    expect(find.byType(LinearProgressIndicator), findsOneWidget);
    expect(find.byType(CustomPaint), findsWidgets);
  });

  testWidgets('input components update isolated card state', (tester) async {
    final spec = NativeCardSpec.fromJson({
      'schemaVersion': 1,
      'initialState': {
        'name': 'before',
        'done': false,
        'choice': 'a',
        'level': 2,
      },
      'root': _node(
        'root',
        'Column',
        children: [
          _node(
            'input',
            'TextInput',
            props: {'label': 'Name', 'valuePath': 'name'},
          ),
          _node(
            'check',
            'Checkbox',
            props: {'label': 'Done', 'valuePath': 'done'},
          ),
          _node(
            'select',
            'Select',
            props: {
              'label': 'Choice',
              'valuePath': 'choice',
              'options': ['a', 'b'],
            },
          ),
          _node(
            'slider',
            'Slider',
            props: {'valuePath': 'level', 'min': 0, 'max': 10},
          ),
        ],
      ),
    });
    final controller = NativeCardController(spec.initialState);
    addTearDown(controller.dispose);

    await tester.pumpWidget(
      MaterialApp(
        home: SizedBox(
          width: 500,
          height: 500,
          child: NativeCardRenderer(spec: spec, controller: controller),
        ),
      ),
    );

    expect(find.byType(ErrorWidget), findsNothing);
    await tester.enterText(find.byType(TextField), 'after');
    await tester.tap(find.byType(Checkbox));
    await tester.drag(find.byType(Slider), const Offset(100, 0));
    await tester.pump();

    expect(controller.state['name'], 'after');
    expect(controller.state['done'], isTrue);
    expect(controller.state['level'], greaterThan(2));
    expect(find.byType(DropdownButton<String>), findsOneWidget);
  });
}

Map<String, Object?> _node(
  String id,
  String type, {
  Map<String, Object?> props = const {},
  List<Map<String, Object?>> children = const [],
}) {
  return {
    'id': id,
    'type': type,
    if (props.isNotEmpty) 'props': props,
    if (children.isNotEmpty) 'children': children,
  };
}

String _fixture() {
  return File(
    '../../contracts/card/fixtures/pomodoro-native.json',
  ).readAsStringSync();
}
