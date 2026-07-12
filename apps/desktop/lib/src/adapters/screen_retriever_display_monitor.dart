import 'dart:async';

import 'package:flutter/widgets.dart';
import 'package:screen_retriever/screen_retriever.dart';

import '../surfaces/surface.dart';
import '../surfaces/surface_coordinator.dart';

class ScreenRetrieverDisplayMonitor with ScreenListener {
  ScreenRetrieverDisplayMonitor(this.coordinator);

  final SurfaceCoordinator coordinator;
  var _started = false;
  var _reconciling = false;

  Future<void> start() async {
    if (_started) {
      return;
    }
    _started = true;
    screenRetriever.addListener(this);
    await _reconcile();
  }

  @override
  void onScreenEvent(String eventName) {
    unawaited(_reconcile());
  }

  Future<void> _reconcile() async {
    if (!_started || _reconciling) {
      return;
    }
    _reconciling = true;
    try {
      final primary = await screenRetriever.getPrimaryDisplay();
      final displays = await screenRetriever.getAllDisplays();
      final position = primary.visiblePosition ?? Offset.zero;
      final size = primary.visibleSize ?? primary.size;
      await coordinator.reconcileDisplays(
        availableMonitorIds: displays.map((display) => display.id).toSet(),
        primaryMonitorId: primary.id,
        primaryBounds: CardPlacement(
          x: position.dx,
          y: position.dy,
          width: size.width,
          height: size.height,
        ),
      );
    } finally {
      _reconciling = false;
    }
  }

  void dispose() {
    if (!_started) {
      return;
    }
    _started = false;
    screenRetriever.removeListener(this);
  }
}
