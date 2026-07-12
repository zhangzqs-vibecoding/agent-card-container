import 'dart:convert';

import '../surfaces/surface.dart';
import '../surfaces/surface_bridge.dart';
import '../surfaces/surface_coordinator.dart';
import '../surfaces/surface_window.dart';
import '../surfaces/overlay_mode_controller.dart';

typedef SurfaceSnapshotProvider = SurfaceCardSnapshot Function(String);
typedef SurfaceBridgeHandler = Future<Object?> Function(SurfaceBridgeMessage);

class SurfaceWindowConfiguration {
  const SurfaceWindowConfiguration({
    required this.arguments,
    this.hiddenAtLaunch = true,
  });

  final String arguments;
  final bool hiddenAtLaunch;
}

abstract interface class MultiWindowDriver {
  Future<void> setBridgeHandler(
    Future<Object?> Function(Map<String, Object?> message) handler,
  );

  Future<String> create(SurfaceWindowConfiguration configuration);

  Future<void> show(String windowId);

  Future<void> updateCards(String windowId, List<SurfaceCardSnapshot> cards);

  Future<void> setOverlayEditing(String windowId, bool editing);

  Future<void> setAlwaysOnTop(String windowId, bool value);

  Future<void> requestAttention(String windowId);

  Future<void> close(String windowId);
}

class MultiWindowBackend implements WindowBackend, OverlaySurfaceModePort {
  MultiWindowBackend(
    this.driver, {
    required this.snapshotProvider,
    this.onBridgeMessage,
  });

  final MultiWindowDriver driver;
  final SurfaceSnapshotProvider snapshotProvider;
  final SurfaceBridgeHandler? onBridgeMessage;
  final Map<String, String> _surfaceWindows = {};
  final Map<String, SurfaceType> _surfaceTypes = {};
  final SurfaceBridgeBindings _bindings = SurfaceBridgeBindings();

  Future<void> initializeBridge() {
    return driver.setBridgeHandler((json) async {
      final message = SurfaceBridgeMessage.fromJson(json);
      if (!_bindings.accepts(message)) {
        throw StateError('surface bridge window does not own instance');
      }
      final handler = onBridgeMessage;
      if (handler == null) {
        throw StateError('surface bridge handler is unavailable');
      }
      return handler(message);
    });
  }

  @override
  Future<void> ensureSurface(
    CardSurface surface,
    List<String> instanceIds,
  ) async {
    final cards = instanceIds.map(snapshotProvider).toList(growable: false);
    final existing = _surfaceWindows[surface.id];
    if (existing != null) {
      await driver.updateCards(existing, cards);
      _bindings.replaceWindowInstances(existing, instanceIds);
      return;
    }
    final arguments = jsonEncode({
      'kind': 'surface',
      'surfaceId': surface.id,
      'surfaceType': surface.type.name,
      'monitorId': surface.monitorId,
      'alwaysOnTop': surface.alwaysOnTop,
      'bounds': _placementJson(surface.bounds),
      'instanceIds': instanceIds,
      'cards': cards.map((card) => card.toJson()).toList(),
    });
    final windowId = await driver.create(
      SurfaceWindowConfiguration(arguments: arguments),
    );
    _surfaceWindows[surface.id] = windowId;
    _surfaceTypes[surface.id] = surface.type;
    _bindings.replaceWindowInstances(windowId, instanceIds);
    await driver.show(windowId);
  }

  @override
  Future<void> closeSurface(String surfaceId) async {
    final windowId = _surfaceWindows.remove(surfaceId);
    _surfaceTypes.remove(surfaceId);
    if (windowId != null) {
      _bindings.removeWindow(windowId);
      await driver.close(windowId);
    }
  }

  @override
  void releaseSurface(String surfaceId) {
    final windowId = _surfaceWindows.remove(surfaceId);
    _surfaceTypes.remove(surfaceId);
    if (windowId != null) {
      _bindings.removeWindow(windowId);
    }
  }

  @override
  Future<void> setSurfaceAlwaysOnTop(String surfaceId, bool value) async {
    final windowId = _surfaceWindows[surfaceId];
    if (windowId == null) {
      throw StateError('surface window is not registered');
    }
    await driver.setAlwaysOnTop(windowId, value);
  }

  @override
  Future<void> requestSurfaceAttention(String surfaceId) async {
    final windowId = _surfaceWindows[surfaceId];
    if (windowId == null) {
      throw StateError('surface window is not registered');
    }
    await driver.requestAttention(windowId);
  }

  @override
  Future<void> setEditing(bool editing) async {
    for (final entry in _surfaceWindows.entries) {
      if (_surfaceTypes[entry.key] == SurfaceType.overlay) {
        await driver.setOverlayEditing(entry.value, editing);
      }
    }
  }
}

Map<String, double>? _placementJson(CardPlacement? placement) {
  if (placement == null) {
    return null;
  }
  return {
    'x': placement.x,
    'y': placement.y,
    'width': placement.width,
    'height': placement.height,
  };
}
