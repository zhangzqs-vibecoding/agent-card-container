import '../cards/card_instance.dart';
import '../storage/local_database.dart';
import 'surface.dart';
import 'surface_bridge.dart';

typedef SurfaceCapabilityInvocation =
    Future<Object?> Function(
      String instanceId,
      String method,
      Map<String, Object?> params,
    );

abstract interface class WindowBackend {
  Future<void> ensureSurface(CardSurface surface, List<String> instanceIds);

  Future<void> closeSurface(String surfaceId);

  void releaseSurface(String surfaceId);
}

class SurfaceCoordinator {
  SurfaceCoordinator({
    required this.database,
    required this.windows,
    required this.newDetachedSurfaceId,
    this.onInstanceMoved,
    this.onOverlayDisplayRequested,
    this.onCapabilityInvocation,
    DateTime Function()? now,
  }) : now = now ?? DateTime.now;

  final LocalDatabase database;
  final WindowBackend windows;
  final String Function() newDetachedSurfaceId;
  final void Function(CardInstance instance)? onInstanceMoved;
  final Future<void> Function()? onOverlayDisplayRequested;
  final SurfaceCapabilityInvocation? onCapabilityInvocation;
  final DateTime Function() now;

  Future<Object?> handleBridgeMessage(SurfaceBridgeMessage message) async {
    final instance = _requireInstance(message.instanceId);
    if (message.type == SurfaceBridgeMessageType.invokeCapability) {
      final invocation = onCapabilityInvocation;
      if (invocation == null) {
        throw StateError('surface capability invocation is unavailable');
      }
      if (message.payload.keys.toSet().difference(const {
            'method',
            'params',
          }).isNotEmpty ||
          message.payload['method'] is! String ||
          (message.payload['method']! as String).isEmpty ||
          message.payload['params'] is! Map<String, Object?>) {
        throw const FormatException('invalid invokeCapability payload');
      }
      return invocation(
        instance.instanceId,
        message.payload['method']! as String,
        message.payload['params']! as Map<String, Object?>,
      );
    }
    if (message.type == SurfaceBridgeMessageType.placementChanged) {
      final placement = _placement(message.payload);
      database.updateSurfaceWindowState(
        instance.surfaceId,
        bounds: placement.bounds,
        monitorId: placement.monitorId,
      );
      return const {'persisted': true};
    }
    if (message.type == SurfaceBridgeMessageType.focusChanged) {
      if (message.payload.keys.length != 1 ||
          message.payload['focused'] is! bool) {
        throw const FormatException('invalid focusChanged payload');
      }
      if (message.payload['focused'] == true) {
        database.updateSurfaceWindowState(instance.surfaceId, focusedAt: now());
      }
      return const {'persisted': true};
    }
    if (message.type != SurfaceBridgeMessageType.hostEvent) {
      throw const FormatException('unsupported surface host event');
    }
    if (message.payload['event'] == 'overlayDisplayRequested') {
      if (!instance.surfaceId.startsWith('overlay-') ||
          onOverlayDisplayRequested == null) {
        throw StateError('overlay display mode is unavailable');
      }
      await onOverlayDisplayRequested!();
      return const {'displayMode': true};
    }
    if (message.payload['event'] != 'windowCloseRequested') {
      throw const FormatException('unsupported surface host event');
    }
    if (!instance.surfaceId.startsWith('detached-')) {
      throw StateError('only detached windows can request close docking');
    }
    final surfaceId = instance.surfaceId;
    _moveToWorkspace(message.instanceId);
    windows.releaseSurface(surfaceId);
    return const {'allowClose': true};
  }

  Future<void> restorePersistedSurfaces() async {
    final instances = database.listInstances();
    for (final surface in database.listSurfaces()) {
      if (surface.type == SurfaceType.workspace) {
        continue;
      }
      final instanceIds = instances
          .where(
            (instance) =>
                instance.surfaceId == surface.id &&
                instance.status != CardInstanceStatus.quarantined,
          )
          .map((instance) => instance.instanceId)
          .toList(growable: false);
      if (instanceIds.isNotEmpty) {
        await windows.ensureSurface(surface, instanceIds);
      }
    }
  }

  Future<void> reconcileDisplays({
    required Set<String> availableMonitorIds,
    required String primaryMonitorId,
    required CardPlacement primaryBounds,
  }) async {
    if (!availableMonitorIds.contains(primaryMonitorId)) {
      throw StateError('primary monitor is not available');
    }
    final targetId = 'overlay-${_safeID(primaryMonitorId)}';
    final targetSurface = CardSurface(
      id: targetId,
      type: SurfaceType.overlay,
      monitorId: primaryMonitorId,
      bounds: primaryBounds,
      alwaysOnTop: true,
    );
    for (final surface in database.listSurfaces()) {
      final monitorId = surface.monitorId;
      if (surface.type != SurfaceType.overlay ||
          monitorId == null ||
          availableMonitorIds.contains(monitorId)) {
        continue;
      }
      final allInstances = database.listInstances();
      final orphaned = allInstances
          .where((instance) => instance.surfaceId == surface.id)
          .toList(growable: false);
      final targetIds = allInstances
          .where((instance) => instance.surfaceId == targetId)
          .map((instance) => instance.instanceId)
          .toList();
      targetIds.addAll(orphaned.map((instance) => instance.instanceId));
      await windows.ensureSurface(targetSurface, targetIds);
      for (final instance in orphaned) {
        database.moveInstanceToSurface(
          surface: targetSurface,
          instanceId: instance.instanceId,
          placement: _constrain(instance.placement, primaryBounds),
        );
        _notifyMoved(instance.instanceId);
      }
      await windows.closeSurface(surface.id);
    }
  }

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
    _notifyMoved(instanceId);
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
    _notifyMoved(instanceId);
  }

  Future<void> dock(String instanceId) async {
    final instance = _requireInstance(instanceId);
    if (instance.surfaceId == 'workspace-main') {
      return;
    }
    if (instance.surfaceId.startsWith('detached-')) {
      await windows.closeSurface(instance.surfaceId);
    }
    _moveToWorkspace(instanceId);
  }

  void _moveToWorkspace(String instanceId) {
    database.moveInstanceToSurface(
      surface: const CardSurface(
        id: 'workspace-main',
        type: SurfaceType.workspace,
      ),
      instanceId: instanceId,
      placement: const CardPlacement(x: 0, y: 0, width: 4, height: 3),
    );
    _notifyMoved(instanceId);
  }

  void _notifyMoved(String instanceId) {
    final listener = onInstanceMoved;
    if (listener != null) {
      listener(_requireInstance(instanceId));
    }
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

({CardPlacement bounds, String? monitorId}) _placement(
  Map<String, Object?> payload,
) {
  const keys = {'x', 'y', 'width', 'height', 'monitorId'};
  if (payload.keys.toSet().difference(keys).isNotEmpty ||
      !payload.keys.toSet().containsAll(const {'x', 'y', 'width', 'height'})) {
    throw const FormatException('invalid placementChanged payload');
  }
  final x = payload['x'];
  final y = payload['y'];
  final width = payload['width'];
  final height = payload['height'];
  final monitorId = payload['monitorId'];
  if (x is! num ||
      y is! num ||
      width is! num ||
      height is! num ||
      !x.isFinite ||
      !y.isFinite ||
      !width.isFinite ||
      !height.isFinite ||
      width <= 0 ||
      height <= 0 ||
      (monitorId != null && (monitorId is! String || monitorId.isEmpty))) {
    throw const FormatException('invalid placementChanged payload');
  }
  return (
    bounds: CardPlacement(
      x: x.toDouble(),
      y: y.toDouble(),
      width: width.toDouble(),
      height: height.toDouble(),
    ),
    monitorId: monitorId as String?,
  );
}

CardPlacement _constrain(CardPlacement placement, CardPlacement visibleBounds) {
  final width = placement.width.clamp(1, visibleBounds.width).toDouble();
  final height = placement.height.clamp(1, visibleBounds.height).toDouble();
  return CardPlacement(
    x: placement.x.clamp(0, visibleBounds.width - width).toDouble(),
    y: placement.y.clamp(0, visibleBounds.height - height).toDouble(),
    width: width,
    height: height,
  );
}

String _safeID(String value) {
  final sanitized = value.replaceAll(RegExp('[^a-zA-Z0-9_.-]'), '_');
  if (sanitized.isEmpty) {
    throw ArgumentError.value(value, 'monitorId', 'monitor ID is empty');
  }
  return sanitized;
}
