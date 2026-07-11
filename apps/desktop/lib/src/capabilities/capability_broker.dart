import 'capability.dart';

typedef CapabilityHandler =
    Future<Object?> Function(CardContext context, Map<String, Object?> params);

class CapabilityBroker {
  final Map<String, CapabilityHandler> _handlers = {};
  Set<PermissionGrant> _grants = {};

  void register(String method, CapabilityHandler handler) {
    _handlers[method] = handler;
  }

  void replaceGrants(Set<PermissionGrant> grants) {
    _grants = Set.unmodifiable(grants);
  }

  Future<Object?> invoke(
    CardContext context,
    String method,
    Map<String, Object?> params,
  ) async {
    final capability = _requiredCapability(method);
    if (!context.declaredCapabilities.contains(capability)) {
      throw CapabilityException(
        CapabilityErrorCode.permissionDenied,
        'card did not declare $capability',
      );
    }

    final grant = _matchingGrant(context, capability);
    if (grant == null) {
      throw const CapabilityException(
        CapabilityErrorCode.permissionRequired,
        'capability requires a grant',
      );
    }
    if (method == 'clipboard.read' && !context.userGesture) {
      throw const CapabilityException(
        CapabilityErrorCode.permissionRequired,
        'clipboard.read requires a user gesture',
      );
    }
    if (method == 'network.fetch') {
      _validateNetworkRequest(context, grant, params);
    }

    final handler = _handlers[method];
    if (handler == null) {
      throw const CapabilityException(
        CapabilityErrorCode.capabilityUnavailable,
        'capability handler is unavailable',
      );
    }
    return handler(context, params);
  }

  PermissionGrant? _matchingGrant(CardContext context, String capability) {
    for (final grant in _grants) {
      if (grant.instanceId == context.instanceId &&
          grant.versionId == context.versionId &&
          grant.capability == capability) {
        return grant;
      }
    }
    return null;
  }

  void _validateNetworkRequest(
    CardContext context,
    PermissionGrant grant,
    Map<String, Object?> params,
  ) {
    final rawUrl = params['url'];
    final uri = rawUrl is String ? Uri.tryParse(rawUrl) : null;
    if (uri == null ||
        !uri.hasAuthority ||
        (uri.scheme != 'https' && uri.scheme != 'http')) {
      throw const CapabilityException(
        CapabilityErrorCode.invalidParams,
        'network.fetch requires an http or https URL',
      );
    }
    final host = uri.host.toLowerCase();
    if (!context.networkDomains.contains(host) ||
        !grant.domains.contains(host) ||
        _isLocalHost(host)) {
      throw const CapabilityException(
        CapabilityErrorCode.permissionDenied,
        'network domain is not allowed',
      );
    }
  }

  String _requiredCapability(String method) {
    if (method.startsWith('storage.')) {
      return 'storage';
    }
    if (method.startsWith('window.')) {
      return 'window.manageSelf';
    }
    if (method.startsWith('system.metrics.')) {
      return 'system.metrics.read';
    }
    return method;
  }

  bool _isLocalHost(String host) {
    return host == 'localhost' ||
        host.endsWith('.localhost') ||
        host == '127.0.0.1' ||
        host == '::1';
  }
}
