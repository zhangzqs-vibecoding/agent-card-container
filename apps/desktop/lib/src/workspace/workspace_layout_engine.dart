import '../surfaces/surface.dart';

class WorkspaceLayoutItem {
  const WorkspaceLayoutItem({
    required this.instanceId,
    required this.placement,
  });

  final String instanceId;
  final CardPlacement placement;
}

class WorkspaceLayoutEngine {
  const WorkspaceLayoutEngine({
    this.columns = 12,
    this.minWidth = 2,
    this.minHeight = 2,
  }) : assert(columns > 0),
       assert(minWidth > 0 && minWidth <= columns),
       assert(minHeight > 0);

  final int columns;
  final int minWidth;
  final int minHeight;

  CardPlacement resolve(
    CardPlacement candidate, {
    Iterable<WorkspaceLayoutItem> occupied = const [],
    String? ignoreInstanceId,
  }) {
    _validate(candidate);
    final width = candidate.width.round().clamp(minWidth, columns);
    final height = candidate.height.round().clamp(minHeight, 1 << 30);
    final x = candidate.x.round().clamp(0, columns - width);
    var y = candidate.y.round().clamp(0, 1 << 30);
    final blockers = occupied
        .where((item) => item.instanceId != ignoreInstanceId)
        .map((item) => resolve(item.placement))
        .toList(growable: false);
    final lastBlockedRow = blockers.fold<int>(
      y,
      (maximum, placement) => maximum > placement.y + placement.height
          ? maximum
          : (placement.y + placement.height).round(),
    );
    while (y <= lastBlockedRow) {
      final placement = CardPlacement(
        x: x.toDouble(),
        y: y.toDouble(),
        width: width.toDouble(),
        height: height.toDouble(),
      );
      if (!blockers.any((blocker) => _intersects(placement, blocker))) {
        return placement;
      }
      y++;
    }
    return CardPlacement(
      x: x.toDouble(),
      y: y.toDouble(),
      width: width.toDouble(),
      height: height.toDouble(),
    );
  }

  CardPlacement autoPlace({
    required double width,
    required double height,
    Iterable<WorkspaceLayoutItem> occupied = const [],
  }) {
    final normalized = resolve(
      CardPlacement(x: 0, y: 0, width: width, height: height),
    );
    final blockers = occupied.toList(growable: false);
    final lastRow = blockers.fold<int>(
      0,
      (maximum, item) => maximum > item.placement.y + item.placement.height
          ? maximum
          : (item.placement.y + item.placement.height).ceil(),
    );
    for (var y = 0; y <= lastRow; y++) {
      for (var x = 0; x <= columns - normalized.width; x++) {
        final candidate = CardPlacement(
          x: x.toDouble(),
          y: y.toDouble(),
          width: normalized.width,
          height: normalized.height,
        );
        if (!blockers.any(
          (item) => _intersects(candidate, resolve(item.placement)),
        )) {
          return candidate;
        }
      }
    }
    throw StateError('workspace layout search exhausted');
  }

  void _validate(CardPlacement placement) {
    final values = [
      placement.x,
      placement.y,
      placement.width,
      placement.height,
    ];
    if (values.any((value) => !value.isFinite) ||
        placement.width <= 0 ||
        placement.height <= 0) {
      throw const FormatException('invalid workspace placement');
    }
  }

  bool _intersects(CardPlacement left, CardPlacement right) {
    return left.x < right.x + right.width &&
        left.x + left.width > right.x &&
        left.y < right.y + right.height &&
        left.y + left.height > right.y;
  }
}
