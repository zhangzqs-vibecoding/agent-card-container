import 'dart:async';

import 'package:flutter/foundation.dart';

import '../cards/card_instance.dart';
import '../surfaces/surface.dart';
import 'workspace_card.dart';
import 'workspace_layout_engine.dart';

typedef WorkspacePlacementPersistence =
    Future<void> Function(
      String instanceId,
      String surfaceId,
      CardPlacement placement,
    );

class WorkspaceController extends ChangeNotifier {
  WorkspaceController(
    Iterable<WorkspaceCard> cards, {
    WorkspacePlacementPersistence? persistPlacement,
    WorkspaceLayoutEngine layoutEngine = const WorkspaceLayoutEngine(),
    this.debounce = const Duration(milliseconds: 300),
  }) : _cards = List.of(cards),
       _persistPlacement = persistPlacement,
       _layoutEngine = layoutEngine {
    for (final card in _cards) {
      _committedPlacements[card.instance.instanceId] = card.instance.placement;
    }
  }

  final List<WorkspaceCard> _cards;
  final WorkspacePlacementPersistence? _persistPlacement;
  final WorkspaceLayoutEngine _layoutEngine;
  final Duration debounce;
  final Map<String, CardPlacement> _committedPlacements = {};
  final Map<String, Timer> _placementTimers = {};
  final Map<String, int> _placementRevisions = {};
  String? _layoutErrorMessage;
  bool _disposed = false;

  List<WorkspaceCard> get cards => List.unmodifiable(_cards);
  List<WorkspaceCard> get workspaceCards => List.unmodifiable(
    _cards.where((card) => card.instance.surfaceId == 'workspace-main'),
  );
  String? get layoutErrorMessage => _layoutErrorMessage;

  void add(WorkspaceCard card) {
    final existing = _cards.indexWhere(
      (candidate) => candidate.instance.instanceId == card.instance.instanceId,
    );
    if (existing >= 0) {
      _cards[existing] = card;
    } else {
      _cards.add(card);
    }
    _committedPlacements[card.instance.instanceId] = card.instance.placement;
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
    final moved = _copyInstance(
      _cards[index].instance,
      surfaceId: surfaceId,
      placement: placement,
    );
    _cards[index] = _copyCard(_cards[index], instance: moved);
    _placementTimers.remove(instanceId)?.cancel();
    _placementRevisions[instanceId] =
        (_placementRevisions[instanceId] ?? 0) + 1;
    _committedPlacements[instanceId] = placement;
    notifyListeners();
  }

  void editPlacement(String instanceId, CardPlacement candidate) {
    final index = _indexOf(instanceId);
    final card = _cards[index];
    if (card.instance.surfaceId != 'workspace-main') {
      throw StateError('only workspace instances can be edited');
    }
    final placement = _layoutEngine.resolve(
      candidate,
      occupied: [
        for (final other in workspaceCards)
          WorkspaceLayoutItem(
            instanceId: other.instance.instanceId,
            placement: other.instance.placement,
          ),
      ],
      ignoreInstanceId: instanceId,
    );
    _cards[index] = _copyCard(
      card,
      instance: _copyInstance(card.instance, placement: placement),
    );
    final revision = (_placementRevisions[instanceId] ?? 0) + 1;
    _placementRevisions[instanceId] = revision;
    _placementTimers.remove(instanceId)?.cancel();
    final persist = _persistPlacement;
    if (persist == null) {
      _committedPlacements[instanceId] = placement;
    } else {
      _placementTimers[instanceId] = Timer(
        debounce,
        () => unawaited(_persistEdit(instanceId, placement, revision, persist)),
      );
    }
    notifyListeners();
  }

  void clearLayoutError() {
    if (_layoutErrorMessage == null) return;
    _layoutErrorMessage = null;
    notifyListeners();
  }

  Future<void> _persistEdit(
    String instanceId,
    CardPlacement placement,
    int revision,
    WorkspacePlacementPersistence persist,
  ) async {
    _placementTimers.remove(instanceId);
    try {
      await persist(instanceId, 'workspace-main', placement);
      if (_disposed || _placementRevisions[instanceId] != revision) return;
      _committedPlacements[instanceId] = placement;
      if (_layoutErrorMessage != null) {
        _layoutErrorMessage = null;
        notifyListeners();
      }
    } catch (_) {
      if (_disposed || _placementRevisions[instanceId] != revision) return;
      final committed = _committedPlacements[instanceId];
      if (committed != null) {
        final index = _indexOf(instanceId);
        final card = _cards[index];
        _cards[index] = _copyCard(
          card,
          instance: _copyInstance(card.instance, placement: committed),
        );
      }
      _layoutErrorMessage = '布局保存失败，已恢复上次位置';
      notifyListeners();
    }
  }

  void updatePersistedState(
    String instanceId,
    Map<String, Object?> persistedState,
  ) {
    final index = _cards.indexWhere(
      (card) => card.instance.instanceId == instanceId,
    );
    if (index < 0) {
      throw StateError('workspace card instance does not exist');
    }
    final card = _cards[index];
    _cards[index] = card.nativeSpec != null
        ? WorkspaceCard(
            instance: card.instance,
            spec: card.spec,
            persistedState: persistedState,
            capabilityBroker: card.capabilityBroker,
            cardContext: card.cardContext,
          )
        : WorkspaceCard.code(
            instance: card.instance,
            codeCard: card.codeCard!,
            persistedState: persistedState,
            capabilityBroker: card.capabilityBroker,
            cardContext: card.cardContext,
          );
    notifyListeners();
  }

  int _indexOf(String instanceId) {
    final index = _cards.indexWhere(
      (card) => card.instance.instanceId == instanceId,
    );
    if (index < 0) {
      throw StateError('workspace card instance does not exist');
    }
    return index;
  }

  @override
  void dispose() {
    _disposed = true;
    for (final timer in _placementTimers.values) {
      timer.cancel();
    }
    _placementTimers.clear();
    super.dispose();
  }
}

CardInstance _copyInstance(
  CardInstance instance, {
  String? surfaceId,
  CardPlacement? placement,
}) {
  return CardInstance(
    instanceId: instance.instanceId,
    cardId: instance.cardId,
    versionId: instance.versionId,
    surfaceId: surfaceId ?? instance.surfaceId,
    placement: placement ?? instance.placement,
    stateNamespace: instance.stateNamespace,
    status: instance.status,
  );
}

WorkspaceCard _copyCard(WorkspaceCard card, {required CardInstance instance}) {
  return card.nativeSpec != null
      ? WorkspaceCard(
          instance: instance,
          spec: card.spec,
          persistedState: card.persistedState,
          capabilityBroker: card.capabilityBroker,
          cardContext: card.cardContext,
        )
      : WorkspaceCard.code(
          instance: instance,
          codeCard: card.codeCard!,
          persistedState: card.persistedState,
          capabilityBroker: card.capabilityBroker,
          cardContext: card.cardContext,
        );
}
