import 'dart:async';

import 'package:agent_card_desktop/src/cards/card_instance.dart';
import 'package:agent_card_desktop/src/native_card/native_card_spec.dart';
import 'package:agent_card_desktop/src/surfaces/surface.dart';
import 'package:agent_card_desktop/src/workspace/workspace_card.dart';
import 'package:agent_card_desktop/src/workspace/workspace_controller.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test('moves an in-memory card to the same surface persisted by the host', () {
    final controller = WorkspaceController([_card()]);
    addTearDown(controller.dispose);

    controller.moveInstance(
      'instance-1',
      surfaceId: 'detached-1',
      placement: const CardPlacement(x: 0, y: 0, width: 480, height: 320),
    );

    expect(controller.cards.single.instance.surfaceId, 'detached-1');
    expect(controller.cards.single.instance.placement.width, 480);
    expect(controller.workspaceCards, isEmpty);
  });

  test('refreshes persisted state received from a child surface', () {
    final controller = WorkspaceController([_card()]);

    controller.updatePersistedState('instance-1', {'count': 7});

    expect(controller.cards.single.persistedState, {'count': 7});
  });

  test('updates immediately and debounces placement persistence', () async {
    final writes = <CardPlacement>[];
    final controller = WorkspaceController(
      [_card()],
      debounce: const Duration(milliseconds: 20),
      persistPlacement: (_, _, placement) async => writes.add(placement),
    );
    addTearDown(controller.dispose);

    controller.editPlacement(
      'instance-1',
      const CardPlacement(x: 1, y: 0, width: 4, height: 3),
    );
    controller.editPlacement(
      'instance-1',
      const CardPlacement(x: 2, y: 0, width: 4, height: 3),
    );

    expect(controller.cards.single.instance.placement.x, 2);
    await Future<void>.delayed(const Duration(milliseconds: 10));
    expect(writes, isEmpty);
    await Future<void>.delayed(const Duration(milliseconds: 20));
    expect(writes, hasLength(1));
    expect(writes.single.x, 2);
  });

  test(
    'rolls back to the last committed placement after a write fails',
    () async {
      final controller = WorkspaceController(
        [_card()],
        debounce: Duration.zero,
        persistPlacement: (_, _, _) async => throw Exception('sqlite secret'),
      );
      addTearDown(controller.dispose);

      controller.editPlacement(
        'instance-1',
        const CardPlacement(x: 3, y: 2, width: 4, height: 3),
      );
      await Future<void>.delayed(Duration.zero);
      await Future<void>.delayed(Duration.zero);

      expect(controller.cards.single.instance.placement.x, 0);
      expect(controller.cards.single.instance.placement.y, 0);
      expect(controller.layoutErrorMessage, '布局保存失败，已恢复上次位置');
      expect(controller.layoutErrorMessage, isNot(contains('sqlite secret')));
    },
  );

  test('ignores completion from an older placement revision', () async {
    final firstWrite = Completer<void>();
    var writeCount = 0;
    final controller = WorkspaceController(
      [_card()],
      debounce: Duration.zero,
      persistPlacement: (_, _, _) {
        writeCount++;
        return writeCount == 1 ? firstWrite.future : Future.value();
      },
    );
    addTearDown(controller.dispose);

    controller.editPlacement(
      'instance-1',
      const CardPlacement(x: 1, y: 0, width: 4, height: 3),
    );
    await Future<void>.delayed(Duration.zero);
    controller.editPlacement(
      'instance-1',
      const CardPlacement(x: 2, y: 0, width: 4, height: 3),
    );
    await Future<void>.delayed(Duration.zero);
    firstWrite.completeError(Exception('stale failure'));
    await Future<void>.delayed(Duration.zero);

    expect(controller.cards.single.instance.placement.x, 2);
    expect(controller.layoutErrorMessage, isNull);
  });

  test(
    'rejects edits outside the main workspace and cancels on dispose',
    () async {
      var writes = 0;
      final controller = WorkspaceController(
        [_card()],
        debounce: const Duration(milliseconds: 20),
        persistPlacement: (_, _, _) async => writes++,
      );
      controller.moveInstance(
        'instance-1',
        surfaceId: 'detached-1',
        placement: const CardPlacement(x: 0, y: 0, width: 400, height: 300),
      );

      expect(
        () => controller.editPlacement(
          'instance-1',
          const CardPlacement(x: 1, y: 0, width: 4, height: 3),
        ),
        throwsStateError,
      );
      controller.moveInstance(
        'instance-1',
        surfaceId: 'workspace-main',
        placement: const CardPlacement(x: 0, y: 0, width: 4, height: 3),
      );
      controller.editPlacement(
        'instance-1',
        const CardPlacement(x: 1, y: 0, width: 4, height: 3),
      );
      controller.dispose();
      await Future<void>.delayed(const Duration(milliseconds: 30));
      expect(writes, 0);
    },
  );
}

WorkspaceCard _card() {
  return WorkspaceCard(
    instance: const CardInstance(
      instanceId: 'instance-1',
      cardId: 'card-1',
      versionId: 'version-1',
      surfaceId: 'workspace-main',
      placement: CardPlacement(x: 0, y: 0, width: 4, height: 3),
      stateNamespace: 'state-1',
      status: CardInstanceStatus.active,
    ),
    spec: NativeCardSpec.fromJson({
      'schemaVersion': 1,
      'initialState': <String, Object?>{},
      'root': {'id': 'root', 'type': 'Text'},
    }),
  );
}
