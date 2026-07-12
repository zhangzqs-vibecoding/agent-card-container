enum SurfaceType { workspace, overlay, detached }

class CardPlacement {
  const CardPlacement({
    required this.x,
    required this.y,
    required this.width,
    required this.height,
  });

  final double x;
  final double y;
  final double width;
  final double height;
}

class CardSurface {
  const CardSurface({
    required this.id,
    required this.type,
    this.monitorId,
    this.bounds,
    this.alwaysOnTop = false,
    this.lastFocusedAt,
  });

  final String id;
  final SurfaceType type;
  final String? monitorId;
  final CardPlacement? bounds;
  final bool alwaysOnTop;
  final DateTime? lastFocusedAt;
}
