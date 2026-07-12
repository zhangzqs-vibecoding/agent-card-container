import 'dart:convert';

import 'package:agent_card_desktop/src/surfaces/surface_window.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test('accepts only explicit surface-engine arguments', () {
    expect(SurfaceWindowArguments.tryParse(''), isNull);
    expect(
      SurfaceWindowArguments.tryParse(jsonEncode({'kind': 'other'})),
      isNull,
    );

    final arguments = SurfaceWindowArguments.tryParse(
      jsonEncode({
        'kind': 'surface',
        'surfaceId': 'overlay-monitor-a',
        'surfaceType': 'overlay',
        'ownerWindowId': 'main-window',
        'alwaysOnTop': true,
        'instanceIds': ['instance-1'],
      }),
    );

    expect(arguments?.surfaceId, 'overlay-monitor-a');
    expect(arguments?.ownerWindowId, 'main-window');
    expect(arguments?.instanceIds, ['instance-1']);
  });

  test('updates only the instances mounted by the owning surface', () {
    final model = SurfaceWindowModel(
      SurfaceWindowArguments.tryParse(
        jsonEncode({
          'kind': 'surface',
          'surfaceId': 'detached-1',
          'surfaceType': 'detached',
          'ownerWindowId': 'main-window',
          'alwaysOnTop': false,
          'instanceIds': ['instance-1'],
        }),
      )!,
    );
    addTearDown(model.dispose);

    model.updateInstances(['instance-2']);

    expect(model.instanceIds, ['instance-2']);
  });

  test('round trips a NativeCard snapshot without runtime credentials', () {
    final snapshot = SurfaceCardSnapshot.fromJson({
      'instanceId': 'instance-1',
      'runtime': 'native',
      'spec': {
        'schemaVersion': 1,
        'initialState': {'count': 0},
        'root': {'id': 'value', 'type': 'Text'},
      },
      'state': {'count': 2},
    });

    final encoded = jsonEncode(snapshot.toJson());
    final restored = SurfaceCardSnapshot.fromJson(
      jsonDecode(encoded) as Map<String, Object?>,
    );

    expect(restored.instanceId, 'instance-1');
    expect(restored.nativeSpec?.initialState['count'], 0);
    expect(restored.state['count'], 2);
    expect(encoded, isNot(contains('token')));
  });

  test('accepts a CodeCard snapshot using only its local origin', () {
    final snapshot = SurfaceCardSnapshot.fromJson({
      'instanceId': 'instance-2',
      'runtime': 'code',
      'sessionId': 'session-2',
      'origin': 'http://127.0.0.1:43125',
      'entrypoint': '/bundle/hash/index.html',
    });

    expect(snapshot.origin, Uri.parse('http://127.0.0.1:43125'));
    expect(snapshot.entrypoint, '/bundle/hash/index.html');
    expect(snapshot.toJson(), isNot(contains('token')));
  });

  test('rejects remote or credential-bearing CodeCard snapshots', () {
    expect(
      () => SurfaceCardSnapshot.fromJson({
        'instanceId': 'instance-2',
        'runtime': 'code',
        'sessionId': 'session-2',
        'origin': 'https://example.com',
        'entrypoint': '/index.html',
      }),
      throwsA(isA<FormatException>()),
    );
    expect(
      () => SurfaceCardSnapshot.fromJson({
        'instanceId': 'instance-2',
        'runtime': 'code',
        'sessionId': 'session-2',
        'origin': 'http://127.0.0.1:43125',
        'entrypoint': '/index.html',
        'token': 'secret',
      }),
      throwsA(isA<FormatException>()),
    );
  });

  testWidgets('renders a NativeCard snapshot inside the child engine', (
    tester,
  ) async {
    final arguments = SurfaceWindowArguments.tryParse(
      jsonEncode({
        'kind': 'surface',
        'surfaceId': 'detached-1',
        'surfaceType': 'detached',
        'ownerWindowId': 'main-window',
        'alwaysOnTop': false,
        'instanceIds': ['instance-1'],
        'cards': [
          {
            'instanceId': 'instance-1',
            'runtime': 'native',
            'spec': {
              'schemaVersion': 1,
              'initialState': {'label': 'initial'},
              'root': {
                'id': 'label',
                'type': 'Text',
                'props': {
                  'text': {'path': 'state.label'},
                },
              },
            },
            'state': {'label': '独立窗口卡片'},
          },
        ],
      }),
    )!;
    final model = SurfaceWindowModel(arguments);
    addTearDown(model.dispose);

    await tester.pumpWidget(SurfaceWindowApp(model: model));

    expect(find.text('独立窗口卡片'), findsOneWidget);
    expect(find.textContaining('正在挂载'), findsNothing);
  });

  testWidgets('fails closed when child-engine CodeCard isolation is absent', (
    tester,
  ) async {
    final arguments = SurfaceWindowArguments.tryParse(
      jsonEncode({
        'kind': 'surface',
        'surfaceId': 'detached-1',
        'surfaceType': 'detached',
        'ownerWindowId': 'main-window',
        'alwaysOnTop': false,
        'instanceIds': ['instance-1'],
        'cards': [
          {
            'instanceId': 'instance-1',
            'runtime': 'code',
            'sessionId': 'session-1',
            'origin': 'http://127.0.0.1:43125',
            'entrypoint': '/bundle/hash/index.html',
          },
        ],
      }),
    )!;
    final model = SurfaceWindowModel(arguments);
    addTearDown(model.dispose);

    await tester.pumpWidget(SurfaceWindowApp(model: model));
    await tester.pump();

    expect(find.text('当前平台无法安全挂载 CodeCard'), findsOneWidget);
  });
}
