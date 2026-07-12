import '../surfaces/surface.dart';
import '../surfaces/surface_coordinator.dart';
import 'capability.dart';

class WindowCapabilityHandlers {
  const WindowCapabilityHandlers(this.coordinator);

  final SurfaceCoordinator coordinator;

  Future<Object?> getState(
    CardContext context,
    Map<String, Object?> params,
  ) async {
    _requireKeys(params, const {});
    return coordinator.getState(context.instanceId);
  }

  Future<Object?> detach(
    CardContext context,
    Map<String, Object?> params,
  ) async {
    _requireKeys(params, const {'x', 'y', 'width', 'height'});
    final placement = _placement(params);
    await _translateStateError(
      () => coordinator.detach(context.instanceId, placement),
    );
    return coordinator.getState(context.instanceId);
  }

  Future<Object?> dock(CardContext context, Map<String, Object?> params) async {
    _requireKeys(params, const {});
    await _translateStateError(() => coordinator.dock(context.instanceId));
    return coordinator.getState(context.instanceId);
  }

  Future<Object?> setAlwaysOnTop(
    CardContext context,
    Map<String, Object?> params,
  ) async {
    _requireKeys(params, const {'value'});
    final value = params['value'];
    if (value is! bool) {
      throw const CapabilityException(
        CapabilityErrorCode.invalidParams,
        'window.setAlwaysOnTop requires a boolean value',
      );
    }
    await _translateStateError(
      () => coordinator.setAlwaysOnTop(context.instanceId, value),
    );
    return coordinator.getState(context.instanceId);
  }

  Future<Object?> requestAttention(
    CardContext context,
    Map<String, Object?> params,
  ) async {
    _requireKeys(params, const {});
    await _translateStateError(
      () => coordinator.requestAttention(context.instanceId),
    );
    return const {'requested': true};
  }
}

void _requireKeys(Map<String, Object?> params, Set<String> expected) {
  if (params.keys.toSet().length != expected.length ||
      !params.keys.toSet().containsAll(expected)) {
    throw const CapabilityException(
      CapabilityErrorCode.invalidParams,
      'window capability parameters are invalid',
    );
  }
}

CardPlacement _placement(Map<String, Object?> params) {
  final x = params['x'];
  final y = params['y'];
  final width = params['width'];
  final height = params['height'];
  if (x is! num ||
      y is! num ||
      width is! num ||
      height is! num ||
      !x.isFinite ||
      !y.isFinite ||
      !width.isFinite ||
      !height.isFinite ||
      width <= 0 ||
      height <= 0) {
    throw const CapabilityException(
      CapabilityErrorCode.invalidParams,
      'window.detach requires finite positive bounds',
    );
  }
  return CardPlacement(
    x: x.toDouble(),
    y: y.toDouble(),
    width: width.toDouble(),
    height: height.toDouble(),
  );
}

Future<void> _translateStateError(Future<void> Function() action) async {
  try {
    await action();
  } on StateError catch (error) {
    throw CapabilityException(
      CapabilityErrorCode.capabilityUnavailable,
      error.message.toString(),
    );
  }
}
