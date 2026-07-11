enum CapabilityErrorCode {
  invalidParams,
  permissionRequired,
  permissionDenied,
  capabilityUnavailable,
  offline,
  rateLimited,
  timeout,
  sessionExpired,
  internal,
}

class CapabilityException implements Exception {
  const CapabilityException(this.code, this.message);

  final CapabilityErrorCode code;
  final String message;

  @override
  String toString() {
    return 'CapabilityException(${code.name}): $message';
  }
}

class CardContext {
  CardContext({
    required this.instanceId,
    required this.cardId,
    required this.versionId,
    required Set<String> declaredCapabilities,
    Set<String> networkDomains = const {},
    this.userGesture = false,
  }) : declaredCapabilities = Set.unmodifiable(declaredCapabilities),
       networkDomains = Set.unmodifiable(networkDomains);

  final String instanceId;
  final String cardId;
  final String versionId;
  final Set<String> declaredCapabilities;
  final Set<String> networkDomains;
  final bool userGesture;

  CardContext withUserGesture(bool value) {
    return CardContext(
      instanceId: instanceId,
      cardId: cardId,
      versionId: versionId,
      declaredCapabilities: declaredCapabilities,
      networkDomains: networkDomains,
      userGesture: value,
    );
  }
}

class PermissionGrant {
  const PermissionGrant({
    required this.instanceId,
    required this.versionId,
    required this.capability,
    this.domains = const {},
  });

  final String instanceId;
  final String versionId;
  final String capability;
  final Set<String> domains;

  @override
  bool operator ==(Object other) {
    return other is PermissionGrant &&
        other.instanceId == instanceId &&
        other.versionId == versionId &&
        other.capability == capability &&
        _sameSet(other.domains, domains);
  }

  @override
  int get hashCode => Object.hash(
    instanceId,
    versionId,
    capability,
    Object.hashAllUnordered(domains),
  );
}

bool _sameSet(Set<String> left, Set<String> right) {
  return left.length == right.length && left.containsAll(right);
}
