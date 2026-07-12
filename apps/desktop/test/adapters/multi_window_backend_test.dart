import 'dart:convert';

import 'package:agent_card_desktop/src/adapters/multi_window_backend.dart';
import 'package:agent_card_desktop/src/surfaces/surface.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test(
    'creates one platform window and updates its mounted instances',
    () async {
      final driver = _FakeMultiWindowDriver();
      final backend = MultiWindowBackend(driver);
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
      expect(driver.shown, ['window-1']);
      expect(driver.updates.single.instanceIds, ['instance-1', 'instance-2']);
    },
  );

  test('closes only the window belonging to the requested surface', () async {
    final driver = _FakeMultiWindowDriver();
    final backend = MultiWindowBackend(driver);
    await backend.ensureSurface(
      const CardSurface(id: 'detached-1', type: SurfaceType.detached),
      ['instance-1'],
    );

    await backend.closeSurface('detached-1');
    await backend.closeSurface('unknown');

    expect(driver.closed, ['window-1']);
  });
}

class _FakeMultiWindowDriver implements MultiWindowDriver {
  final created = <SurfaceWindowConfiguration>[];
  final shown = <String>[];
  final updates = <({String windowId, List<String> instanceIds})>[];
  final closed = <String>[];

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
  Future<void> updateInstances(
    String windowId,
    List<String> instanceIds,
  ) async {
    updates.add((windowId: windowId, instanceIds: List.of(instanceIds)));
  }

  @override
  Future<void> close(String windowId) async {
    closed.add(windowId);
  }
}
