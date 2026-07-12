import 'dart:async';
import 'dart:collection';

import 'package:flutter/foundation.dart';

import 'capability.dart';

class PermissionRequest {
  const PermissionRequest({
    required this.cardId,
    required this.capability,
    this.domain,
  });

  final String cardId;
  final String capability;
  final String? domain;
}

class PermissionRequestController extends ChangeNotifier {
  final Queue<_PendingPermissionRequest> _pending = Queue();
  _PendingPermissionRequest? _current;

  PermissionRequest? get current => _current?.request;

  Future<PermissionGrant?> requestGrant(
    CardContext context,
    String capability,
    Map<String, Object?> params,
  ) {
    final pending = _PendingPermissionRequest(
      context: context,
      request: PermissionRequest(
        cardId: context.cardId,
        capability: capability,
        domain: _domain(capability, params),
      ),
    );
    _pending.add(pending);
    _showNext();
    return pending.completer.future;
  }

  void approve() {
    final pending = _takeCurrent();
    pending.completer.complete(
      PermissionGrant(
        instanceId: pending.context.instanceId,
        versionId: pending.context.versionId,
        capability: pending.request.capability,
        domains: pending.request.domain == null
            ? const {}
            : {pending.request.domain!},
      ),
    );
    _showNext();
  }

  void deny() {
    _takeCurrent().completer.complete(null);
    _showNext();
  }

  _PendingPermissionRequest _takeCurrent() {
    final pending = _current;
    if (pending == null) {
      throw StateError('no permission request is active');
    }
    _current = null;
    return pending;
  }

  void _showNext() {
    if (_current != null || _pending.isEmpty) {
      notifyListeners();
      return;
    }
    _current = _pending.removeFirst();
    notifyListeners();
  }

  @override
  void dispose() {
    _current?.completer.complete(null);
    for (final pending in _pending) {
      pending.completer.complete(null);
    }
    _current = null;
    _pending.clear();
    super.dispose();
  }
}

class _PendingPermissionRequest {
  _PendingPermissionRequest({required this.context, required this.request});

  final CardContext context;
  final PermissionRequest request;
  final Completer<PermissionGrant?> completer = Completer();
}

String? _domain(String capability, Map<String, Object?> params) {
  if (capability != 'network.fetch' && capability != 'host.openExternal') {
    return null;
  }
  final url = params['url'];
  final uri = url is String ? Uri.tryParse(url) : null;
  return uri?.host.toLowerCase();
}
