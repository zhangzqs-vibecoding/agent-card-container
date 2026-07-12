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

    test('migrates a new database to schema version two', () {
      expect(database.schemaVersion, 2);
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
      database.upsertInstance(_instance());

      final instances = database.listInstances();

      expect(instances, hasLength(1));
      expect(instances.single.instanceId, 'instance-1');
      expect(instances.single.surfaceId, 'workspace-main');
      expect(instances.single.placement.width, 4);
      expect(instances.single.status, CardInstanceStatus.active);
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
