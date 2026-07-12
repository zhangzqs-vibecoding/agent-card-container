import 'dart:convert';

import 'package:agent_card_desktop/src/adapters/in_app_webview_port.dart';
import 'package:agent_card_desktop/src/surfaces/surface_window.dart';
import 'package:agent_card_desktop/src/surfaces/surface_bridge.dart';
import 'package:flutter/material.dart';
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

  testWidgets('pauses NativeCards beyond the active runtime budget', (
    tester,
  ) async {
    final cards = List.generate(
      21,
      (index) => {
        'instanceId': 'instance-$index',
        'runtime': 'native',
        'spec': {
          'schemaVersion': 1,
          'initialState': const <String, Object?>{},
          'root': {
            'id': 'label',
            'type': 'Text',
            'props': {'text': 'card-$index'},
          },
        },
        'state': const <String, Object?>{},
      },
    );
    final arguments = SurfaceWindowArguments.tryParse(
      jsonEncode({
        'kind': 'surface',
        'surfaceId': 'detached-many',
        'surfaceType': 'detached',
        'ownerWindowId': 'main-window',
        'alwaysOnTop': false,
        'instanceIds': [
          for (var index = 0; index < 21; index++) 'instance-$index',
        ],
        'cards': cards,
      }),
    )!;
    final model = SurfaceWindowModel(arguments);
    addTearDown(model.dispose);

    await tester.pumpWidget(
      SurfaceWindowApp(
        model: model,
        webViewPortFactory: () => InAppWebViewPort(platformSupported: false),
      ),
    );

    expect(find.text('card-19'), findsOneWidget);
    expect(find.text('card-20'), findsNothing);
    expect(find.text('已暂停：卡片不可见或超过活动上限'), findsOneWidget);
  });

  testWidgets('routes child NativeCard capabilities through the owner', (
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
              'initialState': {'result': 'pending'},
              'root': {
                'id': 'button',
                'type': 'Button',
                'props': {'label': '调用能力'},
                'events': {
                  'onPressed': [
                    {
                      'type': 'capability.invoke',
                      'method': 'system.metrics.get',
                      'path': 'result',
                    },
                  ],
                },
              },
            },
            'state': const <String, Object?>{},
          },
        ],
      }),
    )!;
    final model = SurfaceWindowModel(arguments);
    addTearDown(model.dispose);
    final invocations = <String>[];

    await tester.pumpWidget(
      SurfaceWindowApp(
        model: model,
        onCapabilityInvocation: (instanceId, method, params) async {
          invocations.add('$instanceId:$method');
          return {'cpuCount': 8};
        },
      ),
    );
    await tester.tap(find.text('调用能力'));
    await tester.pump();

    expect(invocations, ['instance-1:system.metrics.get']);
  });

  testWidgets('publishes child NativeCard state changes to the owner', (
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
              'initialState': {'count': 0},
              'root': {
                'id': 'button',
                'type': 'Button',
                'props': {'label': '增加'},
                'events': {
                  'onPressed': [
                    {'type': 'increment', 'path': 'count'},
                  ],
                },
              },
            },
            'state': const <String, Object?>{},
          },
        ],
      }),
    )!;
    final model = SurfaceWindowModel(arguments);
    addTearDown(model.dispose);
    final states = <Map<String, Object?>>[];

    await tester.pumpWidget(
      SurfaceWindowApp(
        model: model,
        onStateChanged: (instanceId, state) async {
          expect(instanceId, 'instance-1');
          states.add(state);
        },
      ),
    );
    await tester.tap(find.text('增加'));
    await tester.pump();

    expect(states.single['count'], 1);
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

  testWidgets('suspends child-engine CodeCards while its window is hidden', (
    tester,
  ) async {
    addTearDown(() async {
      tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.resumed);
    });
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
    tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.hidden);
    await tester.pump();

    expect(find.text('已暂停：卡片不可见或超过活动上限'), findsOneWidget);
    expect(find.text('当前平台无法安全挂载 CodeCard'), findsNothing);
  });

  test('builds a close request scoped to the child window and instance', () {
    final arguments = SurfaceWindowArguments.tryParse(
      jsonEncode({
        'kind': 'surface',
        'surfaceId': 'detached-1',
        'surfaceType': 'detached',
        'ownerWindowId': 'main-window',
        'alwaysOnTop': false,
        'instanceIds': ['instance-1'],
      }),
    )!;

    final message = buildSurfaceCloseRequest(arguments, 'child-window');

    expect(message.type, SurfaceBridgeMessageType.hostEvent);
    expect(message.windowId, 'child-window');
    expect(message.instanceId, 'instance-1');
    expect(message.payload, {'event': 'windowCloseRequested'});
  });

  test('builds an overlay display request for one owned instance', () {
    final arguments = SurfaceWindowArguments.tryParse(
      jsonEncode({
        'kind': 'surface',
        'surfaceId': 'overlay-primary',
        'surfaceType': 'overlay',
        'ownerWindowId': 'main-window',
        'alwaysOnTop': true,
        'instanceIds': ['instance-1', 'instance-2'],
      }),
    )!;

    final message = buildOverlayDisplayRequest(arguments, 'child-window');

    expect(message.windowId, 'child-window');
    expect(message.instanceId, 'instance-1');
    expect(message.payload, {'event': 'overlayDisplayRequested'});
  });

  testWidgets('overlay edit mode exposes an explicit display-mode action', (
    tester,
  ) async {
    var requested = false;
    final arguments = SurfaceWindowArguments.tryParse(
      jsonEncode({
        'kind': 'surface',
        'surfaceId': 'overlay-primary',
        'surfaceType': 'overlay',
        'ownerWindowId': 'main-window',
        'alwaysOnTop': true,
        'instanceIds': ['instance-1'],
      }),
    )!;
    final model = SurfaceWindowModel(arguments);
    addTearDown(model.dispose);
    await tester.pumpWidget(
      SurfaceWindowApp(
        model: model,
        onEnterOverlayDisplayMode: () async {
          requested = true;
        },
      ),
    );

    await tester.tap(find.byKey(const Key('enter-overlay-display-mode')));
    await tester.pump();

    expect(requested, isTrue);
  });

  test('builds placement and focus messages for every mounted instance', () {
    final arguments = SurfaceWindowArguments.tryParse(
      jsonEncode({
        'kind': 'surface',
        'surfaceId': 'overlay-primary',
        'surfaceType': 'overlay',
        'ownerWindowId': 'main-window',
        'alwaysOnTop': true,
        'instanceIds': ['instance-1', 'instance-2'],
      }),
    )!;

    final placement = buildSurfacePlacementMessages(
      arguments,
      'child-window',
      const Rect.fromLTWH(20, 30, 800, 600),
    );
    final focus = buildSurfaceFocusMessages(
      arguments,
      'child-window',
      focused: true,
    );

    expect(placement, hasLength(2));
    expect(placement.last.type, SurfaceBridgeMessageType.placementChanged);
    expect(placement.last.payload['width'], 800);
    expect(focus.map((message) => message.instanceId), [
      'instance-1',
      'instance-2',
    ]);
    expect(focus.first.payload, {'focused': true});
  });
}
