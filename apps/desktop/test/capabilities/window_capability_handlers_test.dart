import 'dart:io';
import 'dart:convert';

import 'package:agent_card_desktop/src/capabilities/capability.dart';
import 'package:agent_card_desktop/src/capabilities/window_capability_handlers.dart';
import 'package:agent_card_desktop/src/cards/card_instance.dart';
import 'package:agent_card_desktop/src/contracts/card_definition.dart';
import 'package:agent_card_desktop/src/storage/local_database.dart';
import 'package:agent_card_desktop/src/surfaces/surface.dart';
import 'package:agent_card_desktop/src/surfaces/surface_coordinator.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  late Directory root;
  late LocalDatabase database;
  late _WindowBackend backend;
  late SurfaceCoordinator coordinator;
  late WindowCapabilityHandlers handlers;

  setUp(() {
    root = Directory.systemTemp.createTempSync('window-capabilities-');
    database = LocalDatabase.open('${root.path}/state.sqlite3');
    final definition = CardDefinition.fromJson(
      jsonDecode(
            File(
              '../../contracts/card/fixtures/native-card.json',
            ).readAsStringSync(),
          )
          as Map<String, Object?>,
    );
    database
      ..registerInstallation(
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
      )
      ..upsertSurface(
        const CardSurface(id: 'workspace-main', type: SurfaceType.workspace),
      )
      ..upsertInstance(_instance);
    backend = _WindowBackend();
    coordinator = SurfaceCoordinator(
      database: database,
      windows: backend,
      newDetachedSurfaceId: () => 'detached-1',
    );
    handlers = WindowCapabilityHandlers(coordinator);
  });

  tearDown(() {
    database.close();
    root.deleteSync(recursive: true);
  });

  test('gets only the invoking card surface state', () async {
    final result = await handlers.getState(_context, const {});

    expect(result, {
      'surfaceId': 'workspace-main',
      'surfaceType': 'workspace',
      'alwaysOnTop': false,
      'placement': {'x': 0.0, 'y': 0.0, 'width': 4.0, 'height': 3.0},
    });
  });

  test('detaches and docks the invoking card with validated bounds', () async {
    await handlers.detach(_context, const {
      'x': 40,
      'y': 50,
      'width': 480,
      'height': 320,
    });
    expect(database.listInstances().single.surfaceId, 'detached-1');

    await handlers.dock(_context, const {});
    expect(database.listInstances().single.surfaceId, 'workspace-main');
  });

  test(
    'changes always-on-top and requests attention for its own surface',
    () async {
      await coordinator.detach(
        'instance-1',
        const CardPlacement(x: 0, y: 0, width: 480, height: 320),
      );

      await handlers.setAlwaysOnTop(_context, const {'value': true});
      await handlers.requestAttention(_context, const {});

      expect(
        database
            .listSurfaces()
            .singleWhere((s) => s.id == 'detached-1')
            .alwaysOnTop,
        isTrue,
      );
      expect(backend.alwaysOnTop, [('detached-1', true)]);
      expect(backend.attention, ['detached-1']);
    },
  );

  test('rejects malformed bounds and unknown parameters', () async {
    await expectLater(
      () => handlers.detach(_context, const {'width': -1, 'height': 20}),
      throwsA(isA<CapabilityException>()),
    );
    await expectLater(
      () => handlers.getState(_context, const {'instanceId': 'other'}),
      throwsA(isA<CapabilityException>()),
    );
  });
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

final _context = CardContext(
  instanceId: 'instance-1',
  cardId: 'card_pomodoro',
  versionId: 'ver_pomodoro_1',
  declaredCapabilities: const {'window.manageSelf'},
);

class _WindowBackend implements WindowBackend {
  final List<(String, bool)> alwaysOnTop = [];
  final List<String> attention = [];

  @override
  Future<void> ensureSurface(
    CardSurface surface,
    List<String> instanceIds,
  ) async {}

  @override
  Future<void> closeSurface(String surfaceId) async {}

  @override
  void releaseSurface(String surfaceId) {}

  @override
  Future<void> setSurfaceAlwaysOnTop(String surfaceId, bool value) async {
    alwaysOnTop.add((surfaceId, value));
  }

  @override
  Future<void> requestSurfaceAttention(String surfaceId) async {
    attention.add(surfaceId);
  }
}
