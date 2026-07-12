import '../runtime/runtime_session.dart';

enum CodeCardHostState {
  created,
  mounted,
  suspended,
  error,
  quarantined,
  disposed,
}

enum WebPermission { camera, microphone, geolocation, notifications, clipboard }

class WebViewCapabilities {
  const WebViewCapabilities({
    required this.embedded,
    required this.requestInterception,
    required this.isolatedOrigin,
  });

  const WebViewCapabilities.secure()
    : embedded = true,
      requestInterception = true,
      isolatedOrigin = true;

  final bool embedded;
  final bool requestInterception;
  final bool isolatedOrigin;

  bool get canEnforceCodeCardPolicy =>
      embedded && requestInterception && isolatedOrigin;
}

class CodeCardWebPolicy {
  const CodeCardWebPolicy(this.origin);

  final Uri origin;

  bool get allowPopups => false;
  bool get allowDownloads => false;

  bool allowsNavigation(Uri target) => _sameOrigin(target);

  bool allowsResource(Uri target) => _sameOrigin(target);

  bool allowsPermission(WebPermission permission) => false;

  bool _sameOrigin(Uri target) {
    return target.scheme == origin.scheme &&
        target.host == origin.host &&
        target.port == origin.port &&
        (target.scheme == 'http' || target.scheme == 'https');
  }
}

class WebViewConfiguration {
  const WebViewConfiguration({
    required this.policy,
    required this.developerToolsEnabled,
    required this.userDataKey,
  });

  final CodeCardWebPolicy policy;
  final bool developerToolsEnabled;
  final String userDataKey;
}

abstract interface class WebViewPort {
  Future<WebViewCapabilities> inspectCapabilities();

  Future<void> mount(WebViewConfiguration configuration, Uri initialUri);

  Future<void> suspend();

  Future<void> resume();

  Future<void> dispose();
}

class CodeCardUnavailableException implements Exception {
  const CodeCardUnavailableException(this.message);

  final String message;

  @override
  String toString() => 'CodeCardUnavailableException: $message';
}

class CodeCardHost {
  CodeCardHost({
    required this.session,
    required this.entrypoint,
    required this.webView,
    this.diagnosticMode = false,
    this.onLaunchFailure,
    this.onLaunchSuccess,
    this.onQuarantine,
    this.onSuspend,
    this.onResume,
  });

  final RuntimeSession session;
  final String entrypoint;
  final WebViewPort webView;
  final bool diagnosticMode;
  final Future<int> Function()? onLaunchFailure;
  final Future<void> Function()? onLaunchSuccess;
  final Future<void> Function()? onQuarantine;
  final Future<void> Function()? onSuspend;
  final Future<void> Function()? onResume;

  CodeCardHostState _state = CodeCardHostState.created;
  var _launchFailures = 0;

  CodeCardHostState get state => _state;

  Future<void> mount() async {
    _requireState(CodeCardHostState.created);
    final capabilities = await webView.inspectCapabilities();
    if (!capabilities.canEnforceCodeCardPolicy) {
      _state = CodeCardHostState.error;
      throw const CodeCardUnavailableException(
        'platform WebView cannot enforce CodeCard isolation policy',
      );
    }
    if (!entrypoint.startsWith('/') || entrypoint.contains('..')) {
      _state = CodeCardHostState.error;
      throw const CodeCardUnavailableException('invalid CodeCard entrypoint');
    }
    final origin = Uri.parse(session.origin);
    final initialUri = origin.replace(path: entrypoint);
    final configuration = WebViewConfiguration(
      policy: CodeCardWebPolicy(origin),
      developerToolsEnabled: diagnosticMode,
      userDataKey: session.id,
    );
    try {
      await webView.mount(configuration, initialUri);
      _launchFailures = 0;
      await onLaunchSuccess?.call();
      _state = CodeCardHostState.mounted;
    } catch (_) {
      await _recordLaunchFailure();
      rethrow;
    }
  }

  Future<void> suspend() async {
    _requireState(CodeCardHostState.mounted);
    await webView.suspend();
    _state = CodeCardHostState.suspended;
    await onSuspend?.call();
  }

  Future<void> resume() async {
    _requireState(CodeCardHostState.suspended);
    await webView.resume();
    _state = CodeCardHostState.mounted;
    await onResume?.call();
  }

  Future<void> dispose() async {
    if (_state == CodeCardHostState.disposed) {
      return;
    }
    await webView.dispose();
    _state = CodeCardHostState.disposed;
  }

  Future<void> recordLaunchFailure() async {
    if (_state == CodeCardHostState.disposed ||
        _state == CodeCardHostState.quarantined) {
      return;
    }
    await _recordLaunchFailure();
  }

  Future<void> _recordLaunchFailure() async {
    _launchFailures = await onLaunchFailure?.call() ?? _launchFailures + 1;
    if (_launchFailures < 3) {
      _state = CodeCardHostState.error;
      return;
    }
    _state = CodeCardHostState.quarantined;
    await onQuarantine?.call();
  }

  void _requireState(CodeCardHostState expected) {
    if (_state != expected) {
      throw StateError(
        'CodeCardHost expected ${expected.name}, was ${_state.name}',
      );
    }
  }
}
