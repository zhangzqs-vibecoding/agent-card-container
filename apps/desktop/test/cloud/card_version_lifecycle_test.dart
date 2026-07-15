import 'dart:io';

import 'package:agent_card_desktop/src/artifacts/artifact_installer.dart';
import 'package:agent_card_desktop/src/cards/card_instance.dart';
import 'package:agent_card_desktop/src/cloud/card_version_lifecycle.dart';
import 'package:agent_card_desktop/src/contracts/card_definition.dart';
import 'package:agent_card_desktop/src/storage/local_database.dart';
import 'package:agent_card_desktop/src/surfaces/surface.dart';
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
}

CardVersionLifecycle _lifecycle(LocalDatabase database) {
  return CardVersionLifecycle(
    database: database,
    newBackupId: () => 'backup-1',
    newStateNamespace: () => 'state-2',
    now: () => DateTime.utc(2026, 7, 16),
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

CardInstance _instance(String versionId) {
  return CardInstance(
    instanceId: 'instance-1',
    cardId: 'card_1',
    versionId: versionId,
    surfaceId: 'workspace-main',
    placement: const CardPlacement(x: 0, y: 0, width: 4, height: 3),
    stateNamespace: 'state-1',
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
