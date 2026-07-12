enum SurfaceBridgeMessageType {
  mount('mount'),
  unmount('unmount'),
  placementChanged('placementChanged'),
  focusChanged('focusChanged'),
  invokeCapability('invokeCapability'),
  hostEvent('hostEvent');

  const SurfaceBridgeMessageType(this.wireName);

  final String wireName;
}

class SurfaceBridgeMessage {
  const SurfaceBridgeMessage({
    required this.type,
    required this.windowId,
    required this.instanceId,
    required this.payload,
  });

  factory SurfaceBridgeMessage.fromJson(Map<String, Object?> json) {
    const allowed = {'type', 'windowId', 'instanceId', 'payload'};
    final unknown = json.keys.where((key) => !allowed.contains(key));
    if (unknown.isNotEmpty) {
      throw FormatException(
        'surface bridge has unknown fields: ${unknown.join(', ')}',
      );
    }
    final rawType = json['type'];
    final type = SurfaceBridgeMessageType.values
        .where((candidate) => candidate.wireName == rawType)
        .firstOrNull;
    final windowId = json['windowId'];
    final instanceId = json['instanceId'];
    final payload = json['payload'];
    if (type == null) {
      throw const FormatException('unknown surface bridge message type');
    }
    if (windowId is! String || windowId.isEmpty) {
      throw const FormatException('surface bridge windowId is invalid');
    }
    if (instanceId is! String || instanceId.isEmpty) {
      throw const FormatException('surface bridge instanceId is invalid');
    }
    if (payload is! Map<String, Object?>) {
      throw const FormatException('surface bridge payload is invalid');
    }
    return SurfaceBridgeMessage(
      type: type,
      windowId: windowId,
      instanceId: instanceId,
      payload: Map.unmodifiable(payload),
    );
  }

  final SurfaceBridgeMessageType type;
  final String windowId;
  final String instanceId;
  final Map<String, Object?> payload;

  Map<String, Object?> toJson() {
    return {
      'type': type.wireName,
      'windowId': windowId,
      'instanceId': instanceId,
      'payload': payload,
    };
  }
}

class SurfaceBridgeBindings {
  final Map<String, Set<String>> _windowInstances = {};

  void replaceWindowInstances(String windowId, Iterable<String> instanceIds) {
    if (windowId.isEmpty ||
        instanceIds.any((instanceId) => instanceId.isEmpty)) {
      throw const FormatException('surface bridge binding is invalid');
    }
    _windowInstances[windowId] = Set.unmodifiable(instanceIds);
  }

  void removeWindow(String windowId) {
    _windowInstances.remove(windowId);
  }

  bool owns(String windowId, String instanceId) {
    return _windowInstances[windowId]?.contains(instanceId) ?? false;
  }

  bool accepts(SurfaceBridgeMessage message) {
    return owns(message.windowId, message.instanceId);
  }
}
