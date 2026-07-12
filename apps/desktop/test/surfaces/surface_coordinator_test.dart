import 'dart:convert';
import 'dart:io';

import 'package:agent_card_desktop/src/cards/card_instance.dart';
import 'package:agent_card_desktop/src/contracts/card_definition.dart';
import 'package:agent_card_desktop/src/storage/local_database.dart';
import 'package:agent_card_desktop/src/surfaces/surface.dart';
import 'package:agent_card_desktop/src/surfaces/surface_bridge.dart';
import 'package:agent_card_desktop/src/surfaces/surface_coordinator.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  late Directory root;
  late LocalDatabase database;
  late _FakeWindowBackend backend;
  late SurfaceCoordinator coordinator;
  late List<CardInstance> moved;
  late int displayModeRequests;
  late List<({String instanceId, String method, Map<String, Object?> params})>
  capabilityInvocations;

  setUp(() {
    root = Directory.systemTemp.createTempSync('surface-coordinator-');
    database = LocalDatabase.open('${root.path}/state.sqlite3');
    final definition = CardDefinition.fromJson(
      jsonDecode(
            File(
              '../../contracts/card/fixtures/native-card.json',
            ).readAsStringSync(),
          )
          as Map<String, Object?>,
    );
    database.registerInstallation(
      StoredInstallation(
        installation: CardInstallation(
          cardId: definition.cardId,
          versionId: definition.versionId,
          contentHash: 'content-hash',
          runtime: definition.runtime,
          installedAt: DateTime.utc(2026, 7, 12),
          verified: true,
        ),
        definition: definition,
        keyId: 'test-key',
      ),
    );
    database.upsertSurface(
      const CardSurface(id: 'workspace-main', type: SurfaceType.workspace),
    );
    database.upsertInstance(_instance);
    backend = _FakeWindowBackend();
    moved = [];
    displayModeRequests = 0;
    capabilityInvocations = [];
    coordinator = SurfaceCoordinator(
      database: database,
      windows: backend,
      newDetachedSurfaceId: () => 'detached-1',
      onInstanceMoved: moved.add,
      onOverlayDisplayRequested: () async {
        displayModeRequests++;
      },
      onCapabilityInvocation: (instanceId, method, params) async {
        capabilityInvocations.add((
          instanceId: instanceId,
          method: method,
          params: params,
        ));
        return {'opened': true};
      },
      now: () => DateTime.utc(2026, 7, 12, 10),
    );
  });

  test(
    'forwards child capability invocation to the authoritative host',
    () async {
      final result = await coordinator.handleBridgeMessage(
        SurfaceBridgeMessage.fromJson({
          'type': 'invokeCapability',
          'windowId': 'window-1',
          'instanceId': 'instance-1',
          'payload': {
            'method': 'host.openExternal',
            'params': {'url': 'https://example.com/help'},
          },
        }),
      );

      expect(result, {'opened': true});
      expect(capabilityInvocations.single.method, 'host.openExternal');
      expect(capabilityInvocations.single.instanceId, 'instance-1');
    },
  );

  tearDown(() {
    database.close();
    root.deleteSync(recursive: true);
  });

  test(
    'detaches and docks an instance without changing its state identity',
    () async {
      await coordinator.detach(
        'instance-1',
        const CardPlacement(x: 100, y: 80, width: 480, height: 320),
      );

      var instance = database.listInstances().single;
      expect(instance.surfaceId, 'detached-1');
      expect(instance.versionId, 'ver_pomodoro_1');
      expect(instance.stateNamespace, 'state-1');
      expect(backend.opened.single.surface.id, 'detached-1');
      expect(backend.opened.single.instanceIds, ['instance-1']);
      expect(moved.last.surfaceId, 'detached-1');

      await coordinator.dock('instance-1');

      instance = database.listInstances().single;
      expect(instance.surfaceId, 'workspace-main');
      expect(backend.closed, ['detached-1']);
      expect(moved.last.surfaceId, 'workspace-main');
    },
  );

  test('shares one overlay host per monitor', () async {
    await coordinator.moveToOverlay(
      'instance-1',
      monitorId: 'monitor-a',
      placement: const CardPlacement(x: 20, y: 30, width: 360, height: 240),
    );
    database.upsertInstance(
      const CardInstance(
        instanceId: 'instance-2',
        cardId: 'card_pomodoro',
        versionId: 'ver_pomodoro_1',
        surfaceId: 'workspace-main',
        placement: CardPlacement(x: 0, y: 0, width: 4, height: 3),
        stateNamespace: 'state-2',
        status: CardInstanceStatus.active,
      ),
    );
    await coordinator.moveToOverlay(
      'instance-2',
      monitorId: 'monitor-a',
      placement: const CardPlacement(x: 400, y: 30, width: 360, height: 240),
    );

    expect(backend.opened, hasLength(2));
    expect(backend.opened.map((entry) => entry.surface.id).toSet(), {
      'overlay-monitor-a',
    });
    expect(database.listInstances().map((item) => item.surfaceId).toSet(), {
      'overlay-monitor-a',
    });
  });

  test(
    'does not move persistent state when the platform window fails',
    () async {
      backend.failOpening = true;

      await expectLater(
        coordinator.detach(
          'instance-1',
          const CardPlacement(x: 0, y: 0, width: 480, height: 320),
        ),
        throwsStateError,
      );

      expect(database.listInstances().single.surfaceId, 'workspace-main');
    },
  );

  test('restores persisted non-workspace surfaces after startup', () async {
    database.moveInstanceToSurface(
      surface: const CardSurface(
        id: 'detached-restored',
        type: SurfaceType.detached,
        bounds: CardPlacement(x: 40, y: 50, width: 480, height: 320),
      ),
      instanceId: 'instance-1',
      placement: const CardPlacement(x: 0, y: 0, width: 480, height: 320),
    );

    await coordinator.restorePersistedSurfaces();

    expect(backend.opened.single.surface.id, 'detached-restored');
    expect(backend.opened.single.instanceIds, ['instance-1']);
  });

  test(
    'docks a detached instance after an authenticated close event',
    () async {
      await coordinator.detach(
        'instance-1',
        const CardPlacement(x: 40, y: 50, width: 480, height: 320),
      );

      final result = await coordinator.handleBridgeMessage(
        SurfaceBridgeMessage.fromJson({
          'type': 'hostEvent',
          'windowId': 'window-1',
          'instanceId': 'instance-1',
          'payload': {'event': 'windowCloseRequested'},
        }),
      );

      expect(result, {'allowClose': true});
      expect(database.listInstances().single.surfaceId, 'workspace-main');
      expect(backend.released, ['detached-1']);
      expect(backend.closed, isEmpty);
    },
  );

  test('forwards an authenticated overlay display-mode event', () async {
    await coordinator.moveToOverlay(
      'instance-1',
      monitorId: 'monitor-a',
      placement: const CardPlacement(x: 20, y: 30, width: 360, height: 240),
    );

    final result = await coordinator.handleBridgeMessage(
      SurfaceBridgeMessage.fromJson({
        'type': 'hostEvent',
        'windowId': 'window-1',
        'instanceId': 'instance-1',
        'payload': {'event': 'overlayDisplayRequested'},
      }),
    );

    expect(result, {'displayMode': true});
    expect(displayModeRequests, 1);
  });

  test(
    'persists child CodeCard launch failures and quarantines the third',
    () async {
      await coordinator.detach(
        'instance-1',
        const CardPlacement(x: 40, y: 50, width: 480, height: 320),
      );

      for (var attempt = 1; attempt <= 3; attempt++) {
        final result =
            await coordinator.handleBridgeMessage(
                  SurfaceBridgeMessage.fromJson({
                    'type': 'hostEvent',
                    'windowId': 'window-1',
                    'instanceId': 'instance-1',
                    'payload': {'event': 'codeCardLaunchFailed'},
                  }),
                )
                as Map<String, Object?>;
        expect(result['failureCount'], attempt);
      }

      expect(
        database.listInstances().single.status,
        CardInstanceStatus.quarantined,
      );
      expect(moved.last.status, CardInstanceStatus.quarantined);
    },
  );

  test(
    'clears child CodeCard launch failures after a successful mount',
    () async {
      database.recordLaunchFailure('instance-1');
      await coordinator.handleBridgeMessage(
        SurfaceBridgeMessage.fromJson({
          'type': 'hostEvent',
          'windowId': 'window-1',
          'instanceId': 'instance-1',
          'payload': {'event': 'codeCardLaunchSucceeded'},
        }),
      );

      expect(database.launchFailureCount('instance-1'), 0);
    },
  );

  test('persists detached window placement and last focus time', () async {
    await coordinator.detach(
      'instance-1',
      const CardPlacement(x: 40, y: 50, width: 480, height: 320),
    );

    await coordinator.handleBridgeMessage(
      SurfaceBridgeMessage.fromJson({
        'type': 'placementChanged',
        'windowId': 'window-1',
        'instanceId': 'instance-1',
        'payload': {
          'x': 80,
          'y': 90,
          'width': 640,
          'height': 480,
          'monitorId': 'monitor-b',
        },
      }),
    );
    await coordinator.handleBridgeMessage(
      SurfaceBridgeMessage.fromJson({
        'type': 'focusChanged',
        'windowId': 'window-1',
        'instanceId': 'instance-1',
        'payload': {'focused': true},
      }),
    );

    final surface = database.listSurfaces().singleWhere(
      (value) => value.id == 'detached-1',
    );
    expect(surface.bounds?.x, 80);
    expect(surface.bounds?.width, 640);
    expect(surface.monitorId, 'monitor-b');
    expect(surface.lastFocusedAt, DateTime.utc(2026, 7, 12, 10));
  });

  test('persists authenticated NativeCard state from a child window', () async {
    await coordinator.detach(
      'instance-1',
      const CardPlacement(x: 40, y: 50, width: 480, height: 320),
    );

    final result = await coordinator.handleBridgeMessage(
      SurfaceBridgeMessage.fromJson({
        'type': 'stateChanged',
        'windowId': 'window-1',
        'instanceId': 'instance-1',
        'payload': {
          'state': {'remaining': 42, 'running': true},
        },
      }),
    );

    expect(result, {'persisted': true});
    expect(database.readState('state-1'), {'remaining': 42, 'running': true});
  });

  test('removes a docked card from its previous overlay host', () async {
    await coordinator.moveToOverlay(
      'instance-1',
      monitorId: 'monitor-a',
      placement: const CardPlacement(x: 20, y: 30, width: 360, height: 240),
    );

    await coordinator.dock('instance-1');

    expect(database.listInstances().single.surfaceId, 'workspace-main');
    expect(backend.closed, ['overlay-monitor-a']);
  });

  test(
    'moves orphaned overlay cards onto the visible primary display',
    () async {
      await coordinator.moveToOverlay(
        'instance-1',
        monitorId: 'removed-monitor',
        placement: const CardPlacement(x: 900, y: 700, width: 500, height: 400),
      );

      await coordinator.reconcileDisplays(
        availableMonitorIds: const {'primary-monitor'},
        primaryMonitorId: 'primary-monitor',
        primaryBounds: const CardPlacement(
          x: 0,
          y: 0,
          width: 1024,
          height: 768,
        ),
      );

      final instance = database.listInstances().single;
      expect(instance.surfaceId, 'overlay-primary-monitor');
      expect(instance.placement.x, 524);
      expect(instance.placement.y, 368);
      expect(backend.closed, ['overlay-removed-monitor']);
    },
  );
}

const _instance = CardInstance(
  instanceId: 'instance-1',
  cardId: 'card_pomodoro',
  versionId: 'ver_pomodoro_1',
  surfaceId: 'workspace-main',
  placement: CardPlacement(x: 0, y: 0, width: 4, height: 3),
  stateNamespace: 'state-1',
  status: CardInstanceStatus.active,
);

class _OpenedSurface {
  const _OpenedSurface(this.surface, this.instanceIds);

  final CardSurface surface;
  final List<String> instanceIds;
}

class _FakeWindowBackend implements WindowBackend {
  final opened = <_OpenedSurface>[];
  final closed = <String>[];
  final released = <String>[];
  var failOpening = false;

  @override
  Future<void> ensureSurface(
    CardSurface surface,
    List<String> instanceIds,
  ) async {
    if (failOpening) {
      throw StateError('platform failure');
    }
    opened.add(_OpenedSurface(surface, List.of(instanceIds)));
  }

  @override
  Future<void> closeSurface(String surfaceId) async {
    closed.add(surfaceId);
  }

  @override
  void releaseSurface(String surfaceId) {
    released.add(surfaceId);
  }

  @override
  Future<void> setSurfaceAlwaysOnTop(String surfaceId, bool value) async {}

  @override
  Future<void> requestSurfaceAttention(String surfaceId) async {}
}
