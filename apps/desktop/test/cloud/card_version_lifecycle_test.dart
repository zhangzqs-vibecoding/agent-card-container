import 'dart:io';

import 'package:agent_card_desktop/src/artifacts/artifact_installer.dart';
import 'package:agent_card_desktop/src/cards/card_instance.dart';
import 'package:agent_card_desktop/src/capabilities/capability.dart';
import 'package:agent_card_desktop/src/cloud/card_version_lifecycle.dart';
import 'package:agent_card_desktop/src/contracts/card_definition.dart';
import 'package:agent_card_desktop/src/storage/local_database.dart';
import 'package:agent_card_desktop/src/surfaces/surface.dart';
import 'package:agent_card_desktop/src/native_card/native_card_spec.dart';
import 'package:agent_card_desktop/src/workspace/workspace_card.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  late Directory root;
  late LocalDatabase database;

  setUp(() {
    root = Directory.systemTemp.createTempSync('card-version-lifecycle-');
    database = LocalDatabase.open('${root.path}/state.sqlite3');
    database.upsertSurface(
      const CardSurface(id: 'workspace-main', type: SurfaceType.workspace),
    );
  });

  tearDown(() {
    database.close();
    root.deleteSync(recursive: true);
  });

  test('prepares stable capability domain and schema differences', () {
    final current = _definition(
      versionId: 'ver_card_1',
      stateSchemaVersion: 1,
      capabilities: const ['storage', 'network.fetch'],
      domains: const ['api.example.com', 'old.example.com'],
    );
    final target = _definition(
      versionId: 'ver_card_2',
      stateSchemaVersion: 2,
      capabilities: const ['clipboard.read', 'network.fetch'],
      domains: const ['api.example.com', 'new.example.com'],
    );
    _register(database, current);
    _register(database, target);
    database.upsertInstance(_instance('ver_card_1'));
    final lifecycle = _lifecycle(database);

    final prepared = lifecycle.prepare('instance-1', _artifact(root, target));

    expect(prepared.difference.addedCapabilities, ['clipboard.read']);
    expect(prepared.difference.removedCapabilities, ['storage']);
    expect(prepared.difference.addedDomains, ['new.example.com']);
    expect(prepared.difference.removedDomains, ['old.example.com']);
    expect(prepared.difference.currentStateSchemaVersion, 1);
    expect(prepared.difference.targetStateSchemaVersion, 2);
    expect(prepared.difference.stateCompatible, isFalse);
    expect(database.listInstances().single.versionId, 'ver_card_1');
  });

  test('rejects an artifact for a different card without changing state', () {
    final current = _definition(versionId: 'ver_card_1');
    final target = _definition(versionId: 'ver_card_2', cardId: 'card_other');
    _register(database, current);
    _register(database, target);
    database.upsertInstance(_instance('ver_card_1'));

    expect(
      () => _lifecycle(database).prepare('instance-1', _artifact(root, target)),
      throwsA(
        isA<CardVersionLifecycleException>().having(
          (error) => error.code,
          'code',
          'CARD_VERSION_MISMATCH',
        ),
      ),
    );
    expect(database.listInstances().single.versionId, 'ver_card_1');
  });

  test('applies a compatible change with explicit capability approval', () {
    final current = _definition(
      versionId: 'ver_card_1',
      capabilities: const ['network.fetch', 'storage'],
      domains: const ['api.example.com', 'old.example.com'],
    );
    final target = _definition(
      versionId: 'ver_card_2',
      capabilities: const ['clipboard.read', 'network.fetch'],
      domains: const ['api.example.com', 'new.example.com'],
    );
    _register(database, current);
    _register(database, target);
    database.upsertInstance(_instance('ver_card_1'));
    database.putState('state-1', 'counter', 7);
    database.upsertGrant(
      const PermissionGrant(
        instanceId: 'instance-1',
        versionId: 'ver_card_1',
        capability: 'network.fetch',
        domains: {'api.example.com', 'old.example.com'},
      ),
    );
    final lifecycle = _lifecycle(database);
    final prepared = lifecycle.prepare('instance-1', _artifact(root, target));

    final switched = lifecycle.apply(
      prepared,
      decision: CardVersionDecision.reuseState,
      approvedGrants: {
        const PermissionGrant(
          instanceId: 'instance-1',
          versionId: 'ver_card_2',
          capability: 'clipboard.read',
        ),
        const PermissionGrant(
          instanceId: 'instance-1',
          versionId: 'ver_card_2',
          capability: 'network.fetch',
          domains: {'new.example.com'},
        ),
      },
    );

    expect(switched.versionId, 'ver_card_2');
    expect(switched.stateNamespace, 'state-1');
    expect(database.readState('state-1'), {'counter': 7});
    expect(database.grantsForInstance('instance-1'), {
      const PermissionGrant(
        instanceId: 'instance-1',
        versionId: 'ver_card_2',
        capability: 'clipboard.read',
      ),
      const PermissionGrant(
        instanceId: 'instance-1',
        versionId: 'ver_card_2',
        capability: 'network.fetch',
        domains: {'api.example.com', 'new.example.com'},
      ),
    });
  });

  test('rejects missing approval without changing the instance', () {
    final current = _definition(versionId: 'ver_card_1');
    final target = _definition(
      versionId: 'ver_card_2',
      capabilities: const ['clipboard.read', 'storage'],
    );
    _register(database, current);
    _register(database, target);
    database.upsertInstance(_instance('ver_card_1'));
    final lifecycle = _lifecycle(database);
    final prepared = lifecycle.prepare('instance-1', _artifact(root, target));

    expect(
      () => lifecycle.apply(prepared, decision: CardVersionDecision.reuseState),
      throwsA(
        isA<CardVersionLifecycleException>().having(
          (error) => error.code,
          'code',
          'PERMISSION_APPROVAL_REQUIRED',
        ),
      ),
    );
    expect(database.listInstances().single.versionId, 'ver_card_1');
  });

  test('backs up and resets incompatible state only after confirmation', () {
    final current = _definition(versionId: 'ver_card_1');
    final target = _definition(versionId: 'ver_card_2', stateSchemaVersion: 2);
    _register(database, current);
    _register(database, target);
    database.upsertInstance(_instance('ver_card_1'));
    database.putState('state-1', 'counter', 7);
    final lifecycle = _lifecycle(database);
    final prepared = lifecycle.prepare('instance-1', _artifact(root, target));

    final cancelled = lifecycle.apply(
      prepared,
      decision: CardVersionDecision.cancel,
    );
    expect(cancelled.versionId, 'ver_card_1');
    expect(
      database.stateBackups(instanceId: 'instance-1', versionId: 'ver_card_1'),
      isEmpty,
    );

    final switched = lifecycle.apply(
      prepared,
      decision: CardVersionDecision.resetState,
    );
    expect(switched.versionId, 'ver_card_2');
    expect(switched.stateNamespace, 'state-2');
    expect(database.readState('state-2'), isEmpty);
    expect(
      database
          .stateBackups(instanceId: 'instance-1', versionId: 'ver_card_1')
          .single
          .snapshot,
      {'counter': 7},
    );
  });

  test('rejects a stale prepared change after the instance moved', () {
    final current = _definition(versionId: 'ver_card_1');
    final target = _definition(versionId: 'ver_card_2');
    _register(database, current);
    _register(database, target);
    database.upsertInstance(_instance('ver_card_1'));
    final lifecycle = _lifecycle(database);
    final prepared = lifecycle.prepare('instance-1', _artifact(root, target));
    database.switchInstanceVersion('instance-1', 'ver_card_2');

    expect(
      () => lifecycle.apply(prepared, decision: CardVersionDecision.reuseState),
      throwsA(
        isA<CardVersionLifecycleException>().having(
          (error) => error.code,
          'code',
          'VERSION_CHANGE_STALE',
        ),
      ),
    );
  });

  test('restores the newest compatible backup when rolling back', () {
    final target = _definition(versionId: 'ver_card_1');
    final current = _definition(versionId: 'ver_card_2', stateSchemaVersion: 2);
    _register(database, target);
    _register(database, current);
    database.putState('old-v1', 'counter', 4);
    database.backupState(
      backupId: 'old-backup',
      instanceId: 'instance-1',
      cardId: 'card_1',
      versionId: 'ver_card_1',
      stateSchemaVersion: 1,
      stateNamespace: 'old-v1',
      createdAt: DateTime.utc(2026, 7, 15),
    );
    database.upsertInstance(
      _instance('ver_card_2', stateNamespace: 'state-v2'),
    );
    database.putState('state-v2', 'title', 'current');
    final lifecycle = _lifecycle(database);
    final prepared = lifecycle.prepare('instance-1', _artifact(root, target));

    final restored = lifecycle.apply(
      prepared,
      decision: CardVersionDecision.restoreState,
    );

    expect(restored.versionId, 'ver_card_1');
    expect(restored.stateNamespace, 'state-2');
    expect(database.readState('state-2'), {'counter': 4});
    expect(
      database
          .stateBackups(instanceId: 'instance-1', versionId: 'ver_card_2')
          .single
          .snapshot,
      {'title': 'current'},
    );
  });

  test('builds and replaces the runtime exactly once after commit', () {
    final current = _definition(versionId: 'ver_card_1');
    final target = _definition(versionId: 'ver_card_2');
    _register(database, current);
    _register(database, target);
    database.upsertInstance(_instance('ver_card_1'));
    var builds = 0;
    var replacements = 0;
    final lifecycle = _lifecycle(
      database,
      buildWorkspaceCard: (_, instance) {
        builds++;
        return _workspaceCard(instance);
      },
      replaceWorkspaceCard: (card) {
        replacements++;
        expect(card.instance.versionId, 'ver_card_2');
        expect(database.listInstances().single.versionId, 'ver_card_2');
      },
    );
    final prepared = lifecycle.prepare('instance-1', _artifact(root, target));

    final switched = lifecycle.applyAndActivate(
      prepared,
      decision: CardVersionDecision.reuseState,
    );

    expect(switched.versionId, 'ver_card_2');
    expect(builds, 1);
    expect(replacements, 1);
  });

  test('restores database state when target runtime construction fails', () {
    final current = _definition(versionId: 'ver_card_1');
    final target = _definition(versionId: 'ver_card_2', stateSchemaVersion: 2);
    _register(database, current);
    _register(database, target);
    database.upsertInstance(_instance('ver_card_1'));
    database.putState('state-1', 'counter', 7);
    var replacements = 0;
    var sequence = 0;
    final lifecycle = CardVersionLifecycle(
      database: database,
      newBackupId: () => 'backup-${sequence++}',
      newStateNamespace: () => sequence == 1 ? 'state-2' : 'state-recovery',
      now: () => DateTime.utc(2026, 7, 16),
      buildWorkspaceCard: (_, _) => throw FormatException('broken payload'),
      replaceWorkspaceCard: (_) => replacements++,
    );
    final prepared = lifecycle.prepare('instance-1', _artifact(root, target));

    expect(
      () => lifecycle.applyAndActivate(
        prepared,
        decision: CardVersionDecision.resetState,
      ),
      throwsA(
        isA<CardVersionLifecycleException>().having(
          (error) => error.code,
          'code',
          'VERSION_RUNTIME_BUILD_FAILED',
        ),
      ),
    );
    final restored = database.listInstances().single;
    expect(restored.versionId, 'ver_card_1');
    expect(restored.stateNamespace, 'state-1');
    expect(database.readState('state-1'), {'counter': 7});
    expect(replacements, 0);
  });
}

CardVersionLifecycle _lifecycle(
  LocalDatabase database, {
  WorkspaceCard Function(InstalledArtifact, CardInstance)? buildWorkspaceCard,
  void Function(WorkspaceCard)? replaceWorkspaceCard,
}) {
  return CardVersionLifecycle(
    database: database,
    newBackupId: () => 'backup-1',
    newStateNamespace: () => 'state-2',
    now: () => DateTime.utc(2026, 7, 16),
    buildWorkspaceCard: buildWorkspaceCard,
    replaceWorkspaceCard: replaceWorkspaceCard,
  );
}

WorkspaceCard _workspaceCard(CardInstance instance) {
  return WorkspaceCard(
    instance: instance,
    spec: NativeCardSpec.fromJson({
      'schemaVersion': 1,
      'initialState': <String, Object?>{},
      'root': {'id': 'root', 'type': 'Text'},
    }),
  );
}

void _register(LocalDatabase database, CardDefinition definition) {
  database.registerInstallation(
    StoredInstallation(
      installation: CardInstallation(
        cardId: definition.cardId,
        versionId: definition.versionId,
        contentHash: 'hash-${definition.versionId}',
        runtime: definition.runtime,
        installedAt: DateTime.utc(2026, 7, 16),
        verified: true,
      ),
      definition: definition,
      keyId: 'fixture-key',
    ),
  );
}

InstalledArtifact _artifact(Directory root, CardDefinition definition) {
  return InstalledArtifact(
    definition: definition,
    contentHash: 'hash-${definition.versionId}',
    directory: root,
    keyId: 'fixture-key',
  );
}

CardInstance _instance(String versionId, {String stateNamespace = 'state-1'}) {
  return CardInstance(
    instanceId: 'instance-1',
    cardId: 'card_1',
    versionId: versionId,
    surfaceId: 'workspace-main',
    placement: const CardPlacement(x: 0, y: 0, width: 4, height: 3),
    stateNamespace: stateNamespace,
    status: CardInstanceStatus.active,
  );
}

CardDefinition _definition({
  required String versionId,
  String cardId = 'card_1',
  int stateSchemaVersion = 1,
  List<String> capabilities = const ['storage'],
  List<String> domains = const [],
}) {
  return CardDefinition(
    formatVersion: 1,
    minHostVersion: '1.0.0',
    cardId: cardId,
    versionId: versionId,
    displayVersion: versionId == 'ver_card_1' ? '1.0.0' : '1.0.1',
    runtime: CardRuntime.native,
    stateSchemaVersion: stateSchemaVersion,
    title: 'Test card',
    description: 'Test card version',
    entrypoint: 'payload/native.json',
    catalogVersion: '1.0.0',
    minSize: const CardSize(width: 1, height: 1),
    preferredSize: const CardSize(width: 4, height: 3),
    maxSize: const CardSize(width: 12, height: 12),
    capabilities: capabilities,
    networkPolicy: NetworkPolicy(
      mode: domains.isEmpty ? 'none' : 'proxy',
      domains: domains,
    ),
    files: const [
      CardFile(
        path: 'payload/native.json',
        sha256:
            '0000000000000000000000000000000000000000000000000000000000000000',
        size: 0,
      ),
    ],
    createdAt: DateTime.utc(2026, 7, 16),
  );
}
