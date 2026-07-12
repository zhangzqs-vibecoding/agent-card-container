import 'dart:convert';

import '../surfaces/surface.dart';
import '../surfaces/surface_coordinator.dart';
import '../surfaces/surface_window.dart';

typedef SurfaceSnapshotProvider = SurfaceCardSnapshot Function(String);

class SurfaceWindowConfiguration {
  const SurfaceWindowConfiguration({
    required this.arguments,
    this.hiddenAtLaunch = true,
  });

  final String arguments;
  final bool hiddenAtLaunch;
}

abstract interface class MultiWindowDriver {
  Future<String> create(SurfaceWindowConfiguration configuration);

  Future<void> show(String windowId);

  Future<void> updateCards(String windowId, List<SurfaceCardSnapshot> cards);

  Future<void> close(String windowId);
}

class MultiWindowBackend implements WindowBackend {
  MultiWindowBackend(this.driver, {required this.snapshotProvider});

  final MultiWindowDriver driver;
  final SurfaceSnapshotProvider snapshotProvider;
  final Map<String, String> _surfaceWindows = {};

  @override
  Future<void> ensureSurface(
    CardSurface surface,
    List<String> instanceIds,
  ) async {
    final cards = instanceIds.map(snapshotProvider).toList(growable: false);
    final existing = _surfaceWindows[surface.id];
    if (existing != null) {
      await driver.updateCards(existing, cards);
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
    await driver.show(windowId);
  }

  @override
  Future<void> closeSurface(String surfaceId) async {
    final windowId = _surfaceWindows.remove(surfaceId);
    if (windowId != null) {
      await driver.close(windowId);
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
