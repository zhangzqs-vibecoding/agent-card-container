import 'dart:convert';
import 'dart:io';

import 'package:agent_card_desktop/src/app/desktop_bootstrap.dart';
import 'package:agent_card_desktop/src/cards/card_instance.dart';
import 'package:agent_card_desktop/src/contracts/card_definition.dart';
import 'package:agent_card_desktop/src/cloud/cloud_connection_settings.dart';
import 'package:agent_card_desktop/src/cloud/cloud_settings_repository.dart';
import 'package:agent_card_desktop/src/cloud/secret_store.dart';
import 'package:agent_card_desktop/src/storage/local_database.dart';
import 'package:agent_card_desktop/src/surfaces/surface.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test(
    'restores an offline NativeCard before building the workspace',
    () async {
      final root = Directory.systemTemp.createTempSync('agent-card-bootstrap-');
      addTearDown(() => root.deleteSync(recursive: true));
      final definition = CardDefinition.fromJson(
        jsonDecode(
              File(
                '../../contracts/card/fixtures/native-card.json',
              ).readAsStringSync(),
            )
            as Map<String, Object?>,
      );
      final database = LocalDatabase.open('${root.path}/agent-card.sqlite3');
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
      database.upsertInstance(
        CardInstance(
          instanceId: 'instance-1',
          cardId: definition.cardId,
          versionId: definition.versionId,
          surfaceId: 'workspace-main',
          placement: const CardPlacement(x: 0, y: 0, width: 4, height: 3),
          stateNamespace: 'state-1',
          status: CardInstanceStatus.active,
        ),
      );
      database.close();
      final payload = File(
        '${root.path}/artifacts/sha256/content-hash/payload/native.json',
      );
      payload.parent.createSync(recursive: true);
      payload.writeAsStringSync(
        File(
          '../../contracts/card/fixtures/pomodoro-native.json',
        ).readAsStringSync(),
      );

      final runtime = await DesktopBootstrap.start(appDataDirectory: root);
      addTearDown(runtime.close);

      expect(runtime.runtimeServer.port, greaterThan(0));
      expect(runtime.workspaceCards, hasLength(1));
      expect(runtime.workspaceCards.single.instance.instanceId, 'instance-1');
      expect(runtime.workspaceCards.single.spec.initialState['title'], '专注时间');
      expect(runtime.recoveryErrors, isEmpty);
      expect(runtime.surfaceCoordinator, isNotNull);
      final diagnostics =
          jsonDecode(
                runtime.createDiagnosticBundle(
                  now: () => DateTime.utc(2026, 7, 12),
                ),
              )
              as Map<String, Object?>;
      final diagnosticRuntime = diagnostics['runtime']! as Map;
      expect(diagnosticRuntime['installedCardCount'], 1);
      expect(diagnosticRuntime['activeSurfaceCount'], 1);
      expect(diagnostics, isNot(contains('cardState')));
      final diagnosticFile = runtime.exportDiagnosticBundle(
        now: () => DateTime.utc(2026, 7, 12, 14, 30),
      );
      expect(diagnosticFile.existsSync(), isTrue);
      expect(diagnosticFile.path, contains('diagnostics'));
      expect(diagnosticFile.readAsStringSync(), contains('schemaVersion'));

      runtime.workspaceController.editPlacement(
        'instance-1',
        const CardPlacement(x: 5, y: 2, width: 4, height: 3),
      );
      await Future<void>.delayed(const Duration(milliseconds: 350));
      final persisted = runtime.database.listInstances().single;
      expect(persisted.placement.x, 5);
      expect(persisted.placement.y, 2);
    },
  );

  test('isolates a corrupt card during startup recovery', () async {
    final root = Directory.systemTemp.createTempSync('agent-card-bootstrap-');
    addTearDown(() => root.deleteSync(recursive: true));
    final definition = CardDefinition.fromJson(
      jsonDecode(
            File(
              '../../contracts/card/fixtures/native-card.json',
            ).readAsStringSync(),
          )
          as Map<String, Object?>,
    );
    final database = LocalDatabase.open('${root.path}/agent-card.sqlite3');
    database.registerInstallation(
      StoredInstallation(
        installation: CardInstallation(
          cardId: definition.cardId,
          versionId: definition.versionId,
          contentHash: 'missing-content',
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
    database.upsertInstance(
      CardInstance(
        instanceId: 'broken-instance',
        cardId: definition.cardId,
        versionId: definition.versionId,
        surfaceId: 'workspace-main',
        placement: const CardPlacement(x: 0, y: 0, width: 4, height: 3),
        stateNamespace: 'broken-state',
        status: CardInstanceStatus.active,
      ),
    );
    database.close();

    final runtime = await DesktopBootstrap.start(appDataDirectory: root);
    addTearDown(runtime.close);

    expect(runtime.workspaceCards, isEmpty);
    expect(runtime.runtimeServer.port, greaterThan(0));
    expect(runtime.recoveryErrors, hasLength(1));
    expect(runtime.recoveryErrors.single.instanceId, 'broken-instance');
  });

  test(
    'reports a previous unclean shutdown without blocking startup',
    () async {
      final root = Directory.systemTemp.createTempSync('agent-card-bootstrap-');
      addTearDown(() => root.deleteSync(recursive: true));
      File('${root.path}/run.marker').writeAsStringSync('running\n');

      final runtime = await DesktopBootstrap.start(appDataDirectory: root);
      addTearDown(runtime.close);

      expect(runtime.runtimeServer.port, greaterThan(0));
      expect(
        runtime.recoveryErrors.map((error) => error.instanceId),
        contains('desktop-runtime'),
      );
      expect(File('${root.path}/run.marker').existsSync(), isTrue);
      await runtime.close();
      expect(File('${root.path}/run.marker').existsSync(), isFalse);
    },
  );

  test(
    'restores an offline CodeCard into an isolated runtime session',
    () async {
      final root = Directory.systemTemp.createTempSync('agent-card-bootstrap-');
      addTearDown(() => root.deleteSync(recursive: true));
      final definition = CardDefinition.fromJson(
        jsonDecode(
              File(
                '../../contracts/card/fixtures/web-card.json',
              ).readAsStringSync(),
            )
            as Map<String, Object?>,
      );
      final database = LocalDatabase.open('${root.path}/agent-card.sqlite3');
      database.registerInstallation(
        StoredInstallation(
          installation: CardInstallation(
            cardId: definition.cardId,
            versionId: definition.versionId,
            contentHash: 'web-content-hash',
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
      database.upsertInstance(
        CardInstance(
          instanceId: 'web-instance',
          cardId: definition.cardId,
          versionId: definition.versionId,
          surfaceId: 'workspace-main',
          placement: const CardPlacement(x: 0, y: 0, width: 4, height: 3),
          stateNamespace: 'web-state',
          status: CardInstanceStatus.active,
        ),
      );
      database.close();
      final payload = File(
        '${root.path}/artifacts/sha256/web-content-hash/payload/web/index.html',
      );
      payload.parent.createSync(recursive: true);
      payload.writeAsStringSync(
        '<script src="/runtime/bootstrap.js"></script><h1>Offline CodeCard</h1>',
      );

      final runtime = await DesktopBootstrap.start(appDataDirectory: root);
      addTearDown(runtime.close);

      expect(runtime.workspaceCards, hasLength(1));
      expect(runtime.workspaceCards.single.nativeSpec, isNull);
      expect(runtime.workspaceCards.single.codeCard, isNotNull);
      expect(
        runtime.workspaceCards.single.codeCard?.entrypoint,
        '/bundle/web-content-hash/payload/web/index.html',
      );
      expect(runtime.recoveryErrors, isEmpty);
    },
  );

  test(
    'enables Agent Studio only from process environment configuration',
    () async {
      final root = Directory.systemTemp.createTempSync('agent-card-bootstrap-');
      addTearDown(() => root.deleteSync(recursive: true));

      final runtime = await DesktopBootstrap.start(
        appDataDirectory: root,
        environment: const {
          'AGENTCARD_CLOUD_URL': 'https://api.agentcard.example',
          'AGENTCARD_ACCESS_TOKEN': 'ephemeral-test-token',
        },
      );
      addTearDown(runtime.close);

      expect(runtime.agentStudioController, isNotNull);
      expect(runtime.cloudClient, isNotNull);
      expect(runtime.cardCatalogController, isNotNull);
    },
  );

  test('enables Agent Studio from saved config and secure token', () async {
    final root = Directory.systemTemp.createTempSync('agent-card-bootstrap-');
    addTearDown(() => root.deleteSync(recursive: true));
    await CloudSettingsRepository(
      File('${root.path}/cloud-config.json'),
    ).save(const CloudUserConfig(baseUrl: 'https://saved.agentcard.example'));

    final runtime = await DesktopBootstrap.start(
      appDataDirectory: root,
      environment: const {},
      secretStore: _BootstrapSecretStore('saved-access-token'),
    );
    addTearDown(runtime.close);

    expect(runtime.agentStudioController, isNotNull);
    expect(runtime.cloudClient?.baseUri.host, 'saved.agentcard.example');
    expect(runtime.cardCatalogController, isNotNull);
  });
}

class _BootstrapSecretStore implements SecretStore {
  _BootstrapSecretStore(this.value);

  String? value;

  @override
  Future<String?> readAccessToken() async => value;

  @override
  Future<void> writeAccessToken(String value) async {
    this.value = value;
  }

  @override
  Future<void> deleteAccessToken() async {
    value = null;
  }
}
