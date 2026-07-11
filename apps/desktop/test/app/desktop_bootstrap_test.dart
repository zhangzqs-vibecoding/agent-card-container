import 'dart:convert';
import 'dart:io';

import 'package:agent_card_desktop/src/app/desktop_bootstrap.dart';
import 'package:agent_card_desktop/src/cards/card_instance.dart';
import 'package:agent_card_desktop/src/contracts/card_definition.dart';
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
}
