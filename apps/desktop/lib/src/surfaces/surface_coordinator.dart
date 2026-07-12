import '../cards/card_instance.dart';
import '../storage/local_database.dart';
import 'surface.dart';

abstract interface class WindowBackend {
  Future<void> ensureSurface(CardSurface surface, List<String> instanceIds);

  Future<void> closeSurface(String surfaceId);
}

class SurfaceCoordinator {
  const SurfaceCoordinator({
    required this.database,
    required this.windows,
    required this.newDetachedSurfaceId,
  });

  final LocalDatabase database;
  final WindowBackend windows;
  final String Function() newDetachedSurfaceId;

  Future<void> detach(String instanceId, CardPlacement bounds) async {
    _requireInstance(instanceId);
    final surface = CardSurface(
      id: newDetachedSurfaceId(),
      type: SurfaceType.detached,
      bounds: bounds,
    );
    await windows.ensureSurface(surface, [instanceId]);
    database.moveInstanceToSurface(
      surface: surface,
      instanceId: instanceId,
      placement: CardPlacement(
        x: 0,
        y: 0,
        width: bounds.width,
        height: bounds.height,
      ),
    );
  }

  Future<void> moveToOverlay(
    String instanceId, {
    required String monitorId,
    required CardPlacement placement,
  }) async {
    _requireInstance(instanceId);
    final surfaceId = 'overlay-${_safeID(monitorId)}';
    final instances = database
        .listInstances()
        .where((instance) => instance.surfaceId == surfaceId)
        .map((instance) => instance.instanceId)
        .toList();
    if (!instances.contains(instanceId)) {
      instances.add(instanceId);
    }
    final surface = CardSurface(
      id: surfaceId,
      type: SurfaceType.overlay,
      monitorId: monitorId,
      alwaysOnTop: true,
    );
    await windows.ensureSurface(surface, instances);
    database.moveInstanceToSurface(
      surface: surface,
      instanceId: instanceId,
      placement: placement,
    );
  }

  Future<void> dock(String instanceId) async {
    final instance = _requireInstance(instanceId);
    if (instance.surfaceId == 'workspace-main') {
      return;
    }
    if (instance.surfaceId.startsWith('detached-')) {
      await windows.closeSurface(instance.surfaceId);
    }
    database.moveInstanceToSurface(
      surface: const CardSurface(
        id: 'workspace-main',
        type: SurfaceType.workspace,
      ),
      instanceId: instanceId,
      placement: const CardPlacement(x: 0, y: 0, width: 4, height: 3),
    );
  }

  CardInstance _requireInstance(String instanceId) {
    for (final instance in database.listInstances()) {
      if (instance.instanceId == instanceId) {
        return instance;
      }
    }
    throw StateError('card instance does not exist');
  }
}

String _safeID(String value) {
  final sanitized = value.replaceAll(RegExp('[^a-zA-Z0-9_.-]'), '_');
  if (sanitized.isEmpty) {
    throw ArgumentError.value(value, 'monitorId', 'monitor ID is empty');
  }
  return sanitized;
}
