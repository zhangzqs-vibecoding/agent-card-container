import 'dart:io';

import 'capability.dart';

typedef CapabilityHandler =
    Future<Object?> Function(CardContext context, Map<String, Object?> params);
typedef CapabilityGrantRequester =
    Future<PermissionGrant?> Function(
      CardContext context,
      String capability,
      Map<String, Object?> params,
    );
typedef CapabilityGrantPersister = void Function(PermissionGrant grant);

class CapabilityBroker {
  CapabilityBroker({this.requestGrant, this.persistGrant});

  final CapabilityGrantRequester? requestGrant;
  final CapabilityGrantPersister? persistGrant;
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

    if (method == 'clipboard.read' && !context.userGesture) {
      throw const CapabilityException(
        CapabilityErrorCode.permissionRequired,
        'clipboard.read requires a user gesture',
      );
    }
    final requestedDomain = _validatedRequestedDomain(context, method, params);

    var grant = _matchingGrant(context, capability);
    if (grant == null && _isAutomaticallyGranted(capability)) {
      grant = PermissionGrant(
        instanceId: context.instanceId,
        versionId: context.versionId,
        capability: capability,
      );
    }
    final requiresPerUseConfirmation = capability == 'clipboard.read';
    final requiresDomainGrant =
        requestedDomain != null &&
        (grant == null || !grant.domains.contains(requestedDomain));
    if (grant == null || requiresPerUseConfirmation || requiresDomainGrant) {
      final requester = requestGrant;
      if (requester == null) {
        throw const CapabilityException(
          CapabilityErrorCode.permissionRequired,
          'capability requires a grant',
        );
      }
      grant = await requester(context, capability, params);
      if (grant == null) {
        throw const CapabilityException(
          CapabilityErrorCode.permissionDenied,
          'capability grant was denied',
        );
      }
      _validateRequestedGrant(context, capability, grant);
      if (requestedDomain != null && !grant.domains.contains(requestedDomain)) {
        throw const CapabilityException(
          CapabilityErrorCode.permissionDenied,
          'requested grant does not cover the requested domain',
        );
      }
      if (!requiresPerUseConfirmation) {
        grant = PermissionGrant(
          instanceId: grant.instanceId,
          versionId: grant.versionId,
          capability: grant.capability,
          domains: {
            ...?_matchingGrant(context, capability)?.domains,
            ...grant.domains,
          },
        );
        _grants = {
          for (final existing in _grants)
            if (!(existing.instanceId == grant.instanceId &&
                existing.versionId == grant.versionId &&
                existing.capability == grant.capability))
              existing,
          grant,
        };
        persistGrant?.call(grant);
      }
    }
    if (method == 'network.fetch') {
      _validateNetworkGrant(grant, requestedDomain!);
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

  String? _validatedRequestedDomain(
    CardContext context,
    String method,
    Map<String, Object?> params,
  ) {
    if (method != 'network.fetch' && method != 'host.openExternal') {
      return null;
    }
    final rawUrl = params['url'];
    final uri = rawUrl is String ? Uri.tryParse(rawUrl) : null;
    if (uri == null ||
        !uri.hasAuthority ||
        uri.userInfo.isNotEmpty ||
        (uri.scheme != 'https' && uri.scheme != 'http')) {
      throw CapabilityException(
        CapabilityErrorCode.invalidParams,
        '$method requires an http or https URL without credentials',
      );
    }
    final host = uri.host.toLowerCase();
    if (InternetAddress.tryParse(host) != null || _isLocalHost(host)) {
      throw const CapabilityException(
        CapabilityErrorCode.permissionDenied,
        'local and IP literal domains are not allowed',
      );
    }
    if (method == 'network.fetch' && !context.networkDomains.contains(host)) {
      throw const CapabilityException(
        CapabilityErrorCode.permissionDenied,
        'network domain is not declared',
      );
    }
    return host;
  }

  void _validateRequestedGrant(
    CardContext context,
    String capability,
    PermissionGrant grant,
  ) {
    if (grant.instanceId != context.instanceId ||
        grant.versionId != context.versionId ||
        grant.capability != capability ||
        (capability == 'network.fetch' &&
            !context.networkDomains.containsAll(grant.domains))) {
      throw const CapabilityException(
        CapabilityErrorCode.permissionDenied,
        'requested grant exceeds the card manifest',
      );
    }
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

  void _validateNetworkGrant(PermissionGrant grant, String host) {
    if (!grant.domains.contains(host)) {
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

  bool _isAutomaticallyGranted(String capability) {
    return capability == 'storage' || capability == 'window.manageSelf';
  }

  bool _isLocalHost(String host) {
    return host == 'localhost' ||
        host.endsWith('.localhost') ||
        host == '127.0.0.1' ||
        host == '::1';
  }
}
