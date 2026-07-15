import 'dart:convert';
import 'dart:io';

import 'package:agent_card_desktop/src/capabilities/capability.dart';
import 'package:agent_card_desktop/src/cards/card_instance.dart';
import 'package:agent_card_desktop/src/contracts/card_definition.dart';
import 'package:agent_card_desktop/src/storage/local_database.dart';
import 'package:agent_card_desktop/src/surfaces/surface.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  group('LocalDatabase', () {
    late LocalDatabase database;

    setUp(() {
      database = LocalDatabase.inMemory();
    });

    tearDown(() {
      database.close();
    });

    test('migrates a new database to schema version four', () {
      expect(database.schemaVersion, 4);
    });

    test('restores the immutable CardDefinition installation index', () {
      final definition = CardDefinition.fromJson(
        jsonDecode(
              File(
                '../../contracts/card/fixtures/web-card.json',
              ).readAsStringSync(),
            )
            as Map<String, Object?>,
      );
      database.registerInstallation(
        StoredInstallation(
          installation: _installation('ver_local_canvas_1'),
          definition: definition,
          keyId: 'release-2026',
        ),
      );

      final restored = database.installation('ver_local_canvas_1');

      expect(restored?.definition.title, '本地画板');
      expect(restored?.definition.capabilities, contains('storage'));
      expect(restored?.installation.contentHash, 'hash-ver_local_canvas_1');
      expect(restored?.keyId, 'release-2026');
    });

    test('persists installations, surfaces and card instances', () {
      database.upsertInstallation(_installation('version-1'));
      database.upsertSurface(_workspace());
      database.upsertSurface(
        CardSurface(
          id: 'overlay-monitor-a',
          type: SurfaceType.overlay,
          monitorId: 'monitor-a',
          bounds: CardPlacement(x: 10, y: 20, width: 800, height: 600),
          alwaysOnTop: true,
          lastFocusedAt: DateTime.utc(2026, 7, 12, 8, 30),
        ),
      );
      database.upsertInstance(_instance());

      final instances = database.listInstances();
      final surfaces = database.listSurfaces();

      expect(instances, hasLength(1));
      expect(instances.single.instanceId, 'instance-1');
      expect(instances.single.surfaceId, 'workspace-main');
      expect(instances.single.placement.width, 4);
      expect(instances.single.status, CardInstanceStatus.active);
      expect(surfaces, hasLength(2));
      final overlay = surfaces.singleWhere(
        (surface) => surface.id == 'overlay-monitor-a',
      );
      expect(overlay.monitorId, 'monitor-a');
      expect(overlay.bounds?.width, 800);
      expect(overlay.alwaysOnTop, isTrue);
      expect(overlay.lastFocusedAt, DateTime.utc(2026, 7, 12, 8, 30));
    });

    test('updates window bounds and focus without replacing its identity', () {
      database.upsertSurface(
        const CardSurface(id: 'detached-1', type: SurfaceType.detached),
      );

      database.updateSurfaceWindowState(
        'detached-1',
        bounds: const CardPlacement(x: 40, y: 50, width: 640, height: 480),
        focusedAt: DateTime.utc(2026, 7, 12, 9),
        monitorId: 'monitor-b',
      );

      final surface = database.listSurfaces().single;
      expect(surface.id, 'detached-1');
      expect(surface.type, SurfaceType.detached);
      expect(surface.bounds?.width, 640);
      expect(surface.lastFocusedAt, DateTime.utc(2026, 7, 12, 9));
      expect(surface.monitorId, 'monitor-b');
    });

    test('moves an instance and switches versions atomically', () {
      database.upsertInstallation(_installation('version-1'));
      database.upsertInstallation(_installation('version-2'));
      database.upsertSurface(_workspace());
      database.upsertSurface(
        const CardSurface(
          id: 'detached-1',
          type: SurfaceType.detached,
          alwaysOnTop: true,
        ),
      );
      database.upsertInstance(_instance());

      database.moveInstance(
        instanceId: 'instance-1',
        surfaceId: 'detached-1',
        placement: const CardPlacement(x: 10, y: 20, width: 500, height: 320),
      );
      database.switchInstanceVersion('instance-1', 'version-2');

      final instance = database.listInstances().single;
      expect(instance.surfaceId, 'detached-1');
      expect(instance.versionId, 'version-2');
      expect(instance.placement.x, 10);
      expect(instance.placement.width, 500);
    });

    test('rejects moving an unknown instance', () {
      expect(
        () => database.moveInstance(
          instanceId: 'missing',
          surfaceId: 'workspace-main',
          placement: const CardPlacement(x: 1, y: 1, width: 4, height: 3),
        ),
        throwsStateError,
      );
    });

    test('restores a moved placement after reopening the database', () {
      final root = Directory.systemTemp.createTempSync('agent-card-layout-');
      addTearDown(() => root.deleteSync(recursive: true));
      final path = '${root.path}/layout.sqlite3';
      final first = LocalDatabase.open(path);
      first.upsertInstallation(_installation('version-1'));
      first.upsertSurface(_workspace());
      first.upsertInstance(_instance());
      first.moveInstance(
        instanceId: 'instance-1',
        surfaceId: 'workspace-main',
        placement: const CardPlacement(x: 6, y: 4, width: 5, height: 2),
      );
      first.close();

      final reopened = LocalDatabase.open(path);
      addTearDown(reopened.close);
      final instance = reopened.listInstances().single;
      expect(instance.placement.x, 6);
      expect(instance.placement.y, 4);
      expect(instance.placement.width, 5);
      expect(instance.placement.height, 2);
    });

    test('persists namespaced JSON state and permission grants', () {
      database.upsertInstallation(_installation('version-1'));
      database.upsertSurface(_workspace());
      database.upsertInstance(_instance());
      database.putState('state-1', 'timer', {
        'remaining': 1500,
        'running': false,
      });
      database.upsertGrant(
        const PermissionGrant(
          instanceId: 'instance-1',
          versionId: 'version-1',
          capability: 'network.fetch',
          domains: {'api.example.com'},
        ),
      );

      expect(database.readState('state-1'), {
        'timer': {'remaining': 1500, 'running': false},
      });
      database.deleteState('state-1', 'timer');
      expect(database.readState('state-1'), isEmpty);
      expect(database.grantsForInstance('instance-1'), {
        const PermissionGrant(
          instanceId: 'instance-1',
          versionId: 'version-1',
          capability: 'network.fetch',
          domains: {'api.example.com'},
        ),
      });
    });

    test('replaces a complete state snapshot and removes stale keys', () {
      database.putState('state-1', 'kept', 1);
      database.putState('state-1', 'stale', true);

      database.replaceState('state-1', {'kept': 2, 'added': 'value'});

      expect(database.readState('state-1'), {'added': 'value', 'kept': 2});
    });

    test('persists launch failures and quarantines one card instance', () {
      database.upsertInstallation(_installation('version-1'));
      database.upsertSurface(_workspace());
      database.upsertInstance(_instance());

      expect(database.recordLaunchFailure('instance-1'), 1);
      expect(database.recordLaunchFailure('instance-1'), 2);
      expect(database.launchFailureCount('instance-1'), 2);
      database.quarantineInstance('instance-1');
      expect(
        database.listInstances().single.status,
        CardInstanceStatus.quarantined,
      );
      database.clearLaunchFailures('instance-1');
      expect(database.launchFailureCount('instance-1'), 0);
    });
  });
}

CardInstallation _installation(String versionId) {
  return CardInstallation(
    cardId: 'card-1',
    versionId: versionId,
    contentHash: 'hash-$versionId',
    runtime: CardRuntime.native,
    installedAt: DateTime.utc(2026, 7, 12),
    verified: true,
  );
}

CardSurface _workspace() {
  return const CardSurface(id: 'workspace-main', type: SurfaceType.workspace);
}

CardInstance _instance() {
  return const CardInstance(
    instanceId: 'instance-1',
    cardId: 'card-1',
    versionId: 'version-1',
    surfaceId: 'workspace-main',
    placement: CardPlacement(x: 0, y: 0, width: 4, height: 3),
    stateNamespace: 'state-1',
    status: CardInstanceStatus.active,
  );
}
