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
