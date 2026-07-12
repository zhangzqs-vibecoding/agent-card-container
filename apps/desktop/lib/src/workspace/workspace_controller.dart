import 'package:flutter/foundation.dart';

import 'workspace_card.dart';

class WorkspaceController extends ChangeNotifier {
  WorkspaceController([Iterable<WorkspaceCard> cards = const []])
    : _cards = List.of(cards);

  final List<WorkspaceCard> _cards;

  List<WorkspaceCard> get cards => List.unmodifiable(_cards);

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
}
