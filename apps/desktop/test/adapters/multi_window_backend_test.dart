import 'dart:convert';

import 'package:agent_card_desktop/src/adapters/multi_window_backend.dart';
import 'package:agent_card_desktop/src/surfaces/surface.dart';
import 'package:agent_card_desktop/src/surfaces/surface_bridge.dart';
import 'package:agent_card_desktop/src/surfaces/surface_window.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test(
    'creates one platform window and updates its mounted instances',
    () async {
      final driver = _FakeMultiWindowDriver();
      final backend = MultiWindowBackend(driver, snapshotProvider: _snapshot);
      const surface = CardSurface(
        id: 'overlay-monitor-a',
        type: SurfaceType.overlay,
        monitorId: 'monitor-a',
        alwaysOnTop: true,
      );

      await backend.ensureSurface(surface, ['instance-1']);
      await backend.ensureSurface(surface, ['instance-1', 'instance-2']);

      expect(driver.created, hasLength(1));
      final arguments =
          jsonDecode(driver.created.single.arguments) as Map<String, Object?>;
      expect(arguments['surfaceId'], 'overlay-monitor-a');
      expect(arguments['surfaceType'], 'overlay');
      expect(arguments['instanceIds'], ['instance-1']);
      expect(
        (arguments['cards'] as List).single,
        containsPair('instanceId', 'instance-1'),
      );
      expect(driver.shown, ['window-1']);
      expect(driver.updates.single.cards.map((card) => card.instanceId), [
        'instance-1',
        'instance-2',
      ]);
    },
  );

  test('closes only the window belonging to the requested surface', () async {
    final driver = _FakeMultiWindowDriver();
    final backend = MultiWindowBackend(driver, snapshotProvider: _snapshot);
    await backend.ensureSurface(
      const CardSurface(id: 'detached-1', type: SurfaceType.detached),
      ['instance-1'],
    );

    await backend.closeSurface('detached-1');
    await backend.closeSurface('unknown');

    expect(driver.closed, ['window-1']);
  });

  test(
    'dispatches bridge messages only for the owning window instance',
    () async {
      final driver = _FakeMultiWindowDriver();
      final accepted = <SurfaceBridgeMessage>[];
      final backend = MultiWindowBackend(
        driver,
        snapshotProvider: _snapshot,
        onBridgeMessage: (message) async {
          accepted.add(message);
          return {'accepted': true};
        },
      );
      await backend.initializeBridge();
      await backend.ensureSurface(
        const CardSurface(id: 'detached-1', type: SurfaceType.detached),
        ['instance-1'],
      );

      final result = await driver.sendBridge({
        'type': 'hostEvent',
        'windowId': 'window-1',
        'instanceId': 'instance-1',
        'payload': {'event': 'windowCloseRequested'},
      });

      expect(result, {'accepted': true});
      expect(accepted.single.instanceId, 'instance-1');
      await expectLater(
        driver.sendBridge({
          'type': 'hostEvent',
          'windowId': 'window-1',
          'instanceId': 'instance-2',
          'payload': {'event': 'windowCloseRequested'},
        }),
        throwsStateError,
      );
    },
  );

  test('changes click-through mode only on overlay host windows', () async {
    final driver = _FakeMultiWindowDriver();
    final backend = MultiWindowBackend(driver, snapshotProvider: _snapshot);
    await backend.ensureSurface(
      const CardSurface(id: 'overlay-a', type: SurfaceType.overlay),
      ['instance-1'],
    );
    await backend.ensureSurface(
      const CardSurface(id: 'detached-1', type: SurfaceType.detached),
      ['instance-2'],
    );

    await backend.setEditing(false);

    expect(driver.overlayModes, [(windowId: 'window-1', editing: false)]);
  });

  test(
    'routes self-window commands only to the owning surface window',
    () async {
      final driver = _FakeMultiWindowDriver();
      final backend = MultiWindowBackend(driver, snapshotProvider: _snapshot);
      await backend.ensureSurface(
        const CardSurface(id: 'detached-1', type: SurfaceType.detached),
        ['instance-1'],
      );

      await backend.setSurfaceAlwaysOnTop('detached-1', true);
      await backend.requestSurfaceAttention('detached-1');

      expect(driver.alwaysOnTop, [(windowId: 'window-1', value: true)]);
      expect(driver.attention, ['window-1']);
      await expectLater(
        () => backend.requestSurfaceAttention('workspace-main'),
        throwsStateError,
      );
    },
  );
}

class _FakeMultiWindowDriver implements MultiWindowDriver {
  final created = <SurfaceWindowConfiguration>[];
  final shown = <String>[];
  final updates = <({String windowId, List<SurfaceCardSnapshot> cards})>[];
  final closed = <String>[];
  final overlayModes = <({String windowId, bool editing})>[];
  final alwaysOnTop = <({String windowId, bool value})>[];
  final attention = <String>[];
  Future<Object?> Function(Map<String, Object?>)? bridgeHandler;

  Future<Object?> sendBridge(Map<String, Object?> message) {
    final handler = bridgeHandler;
    if (handler == null) {
      throw StateError('bridge handler is missing');
    }
    return handler(message);
  }

  @override
  Future<void> setBridgeHandler(
    Future<Object?> Function(Map<String, Object?> message) handler,
  ) async {
    bridgeHandler = handler;
  }

  @override
  Future<String> create(SurfaceWindowConfiguration configuration) async {
    created.add(configuration);
    return 'window-${created.length}';
  }

  @override
  Future<void> show(String windowId) async {
    shown.add(windowId);
  }

  @override
  Future<void> updateCards(
    String windowId,
    List<SurfaceCardSnapshot> cards,
  ) async {
    updates.add((windowId: windowId, cards: List.of(cards)));
  }

  @override
  Future<void> close(String windowId) async {
    closed.add(windowId);
  }

  @override
  Future<void> setOverlayEditing(String windowId, bool editing) async {
    overlayModes.add((windowId: windowId, editing: editing));
  }

  @override
  Future<void> setAlwaysOnTop(String windowId, bool value) async {
    alwaysOnTop.add((windowId: windowId, value: value));
  }

  @override
  Future<void> requestAttention(String windowId) async {
    attention.add(windowId);
  }
}

SurfaceCardSnapshot _snapshot(String instanceId) {
  return SurfaceCardSnapshot.fromJson({
    'instanceId': instanceId,
    'runtime': 'native',
    'spec': {
      'schemaVersion': 1,
      'initialState': <String, Object?>{},
      'root': {'id': 'root', 'type': 'Text'},
    },
    'state': <String, Object?>{},
  });
}
