import 'package:flutter/foundation.dart';

import '../cards/card_instance.dart';
import '../surfaces/surface.dart';
import 'workspace_card.dart';

class WorkspaceController extends ChangeNotifier {
  WorkspaceController([Iterable<WorkspaceCard> cards = const []])
    : _cards = List.of(cards);

  final List<WorkspaceCard> _cards;

  List<WorkspaceCard> get cards => List.unmodifiable(_cards);
  List<WorkspaceCard> get workspaceCards => List.unmodifiable(
    _cards.where((card) => card.instance.surfaceId == 'workspace-main'),
  );

  void add(WorkspaceCard card) {
    final existing = _cards.indexWhere(
      (candidate) => candidate.instance.instanceId == card.instance.instanceId,
    );
    if (existing >= 0) {
      _cards[existing] = card;
    } else {
      _cards.add(card);
    }
    notifyListeners();
  }

  void moveInstance(
    String instanceId, {
    required String surfaceId,
    required CardPlacement placement,
  }) {
    final index = _cards.indexWhere(
      (card) => card.instance.instanceId == instanceId,
    );
    if (index < 0) {
      throw StateError('workspace card instance does not exist');
    }
    final card = _cards[index];
    final instance = card.instance;
    final moved = CardInstance(
      instanceId: instance.instanceId,
      cardId: instance.cardId,
      versionId: instance.versionId,
      surfaceId: surfaceId,
      placement: placement,
      stateNamespace: instance.stateNamespace,
      status: instance.status,
    );
    _cards[index] = card.nativeSpec != null
        ? WorkspaceCard(
            instance: moved,
            spec: card.spec,
            persistedState: card.persistedState,
            capabilityBroker: card.capabilityBroker,
            cardContext: card.cardContext,
          )
        : WorkspaceCard.code(
            instance: moved,
            codeCard: card.codeCard!,
            persistedState: card.persistedState,
            capabilityBroker: card.capabilityBroker,
            cardContext: card.cardContext,
          );
    notifyListeners();
  }
}
