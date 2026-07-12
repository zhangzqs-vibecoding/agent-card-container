import 'dart:async';
import 'dart:convert';
import 'dart:io';
import 'dart:math';

import '../contracts/local_rpc_contract.dart';

export 'runtime_session.dart';

import 'runtime_session.dart';

void _requireLocale(String locale) {
  if (locale.isEmpty) {
    throw ArgumentError.value(locale, 'locale', 'must not be empty');
  }
}

void _requireTheme(String theme) {
  if (!const {'light', 'dark', 'system'}.contains(theme)) {
    throw ArgumentError.value(theme, 'theme', 'is invalid');
  }
}

void _requireSurface(String surface) {
  if (!const {'workspace', 'overlay', 'detached'}.contains(surface)) {
    throw ArgumentError.value(surface, 'surface', 'is invalid');
  }
}

class LocalRuntimeServer {
  LocalRuntimeServer._(
    this._server,
    this._rateLimit,
    this._invocationTimeout,
    this._maxResponseBytes,
    this._minimumMetricsInterval,
    this._locale,
    this._theme,
    this._online,
  ) {
    _subscription = _server.listen(_handleRequest);
  }

  static Future<LocalRuntimeServer> start({
    RuntimeRateLimit rateLimit = const RuntimeRateLimit(),
    Duration invocationTimeout = const Duration(seconds: 10),
    int maxResponseBytes = 1024 * 1024,
    Duration minimumMetricsInterval = const Duration(seconds: 1),
    String initialLocale = 'en-US',
    String initialTheme = 'system',
    bool initialOnline = true,
  }) async {
    _requireLocale(initialLocale);
    _requireTheme(initialTheme);
    final server = await HttpServer.bind(
      InternetAddress.loopbackIPv4,
      0,
      shared: false,
    );
    return LocalRuntimeServer._(
      server,
      rateLimit,
      invocationTimeout,
      maxResponseBytes,
      minimumMetricsInterval,
      initialLocale,
      initialTheme,
      initialOnline,
    );
  }

  static const _contentSecurityPolicy =
      "default-src 'none'; "
      "script-src 'self'; "
      "style-src 'self' 'unsafe-inline'; "
      "img-src 'self' data: blob:; "
      "font-src 'self'; "
      "connect-src 'self'; "
      "media-src 'self' blob:; "
      "worker-src 'self' blob:; "
      "frame-src 'none'; "
      "object-src 'none'; "
      "base-uri 'none'; "
      "form-action 'none'";

  final HttpServer _server;
  final RuntimeRateLimit _rateLimit;
  final Duration _invocationTimeout;
  final int _maxResponseBytes;
  final Duration _minimumMetricsInterval;
  String _locale;
  String _theme;
  bool _online;
  final _sessionsByAuthority = <String, RuntimeSession>{};
  final _metricsSubscriptions = <String, _MetricsSubscription>{};
  late final StreamSubscription<HttpRequest> _subscription;
  final Random _random = Random.secure();

  InternetAddress get address => _server.address;
  int get port => _server.port;

  RuntimeSession createSession({
    required String instanceId,
    required String cardId,
    required String versionId,
    required Map<String, RuntimeResource> resources,
    Set<String> declaredCapabilities = const {},
    String? locale,
    String? theme,
    String surface = 'workspace',
    bool? online,
    RuntimeStorage? storage,
    RuntimeRpcHandler? rpcHandler,
  }) {
    _requireLocale(locale ?? _locale);
    _requireTheme(theme ?? _theme);
    _requireSurface(surface);
    final id = _randomHex(16);
    final authority = '$id.localhost:$port';
    final session = RuntimeSession(
      id: id,
      authority: authority,
      token: _randomToken(32),
      instanceId: instanceId,
      cardId: cardId,
      versionId: versionId,
      resources: resources,
      declaredCapabilities: declaredCapabilities,
      locale: locale ?? _locale,
      theme: theme ?? _theme,
      surface: surface,
      online: online ?? _online,
      storage: storage,
      rateLimit: _rateLimit,
      rpcHandler: rpcHandler,
    );
    _sessionsByAuthority[authority] = session;
    return session;
  }

  void closeSession(String id) {
    final sessions = _sessionsByAuthority.values
        .where((session) => session.id == id)
        .toList(growable: false);
    for (final session in sessions) {
      _metricsSubscriptions.remove(session.id)?.cancel();
      session.closeEventSockets();
      _sessionsByAuthority.remove(session.authority);
    }
  }

  int publishEvent(String sessionId, String event, Object? payload) {
    _validateEvent(event);
    final session = _sessionsByAuthority.values
        .where((candidate) => candidate.id == sessionId)
        .firstOrNull;
    if (session == null) {
      throw StateError('runtime session is expired');
    }
    return session.publishEvent(event, payload);
  }

  int publishEventForInstance(
    String instanceId,
    String event,
    Object? payload,
  ) {
    _validateEvent(event);
    var published = 0;
    for (final session in _sessionsByAuthority.values) {
      if (session.instanceId == instanceId) {
        session.publishEvent(event, payload);
        published++;
      }
    }
    return published;
  }

  int updateEnvironment({String? locale, String? theme, bool? online}) {
    if (locale != null) _requireLocale(locale);
    if (theme != null) _requireTheme(theme);
    _locale = locale ?? _locale;
    _theme = theme ?? _theme;
    _online = online ?? _online;
    var changedSessions = 0;
    for (final session in _sessionsByAuthority.values) {
      final previous = session.context;
      final next = previous.copyWith(
        locale: _locale,
        theme: _theme,
        online: _online,
      );
      if (next.locale == previous.locale &&
          next.theme == previous.theme &&
          next.online == previous.online) {
        continue;
      }
      session.context = next;
      if (next.theme != previous.theme) {
        session.publishEvent('theme.changed', {'theme': next.theme});
      }
      if (next.online != previous.online) {
        session.publishEvent('online.changed', {'online': next.online});
      }
      session.publishEvent('context.changed', next.toJson());
      changedSessions++;
    }
    return changedSessions;
  }

  int updateInstanceSurface(
    String instanceId,
    String surface, {
    Map<String, Object?> payload = const {},
  }) {
    _requireSurface(surface);
    var changedSessions = 0;
    for (final session in _sessionsByAuthority.values) {
      if (session.instanceId != instanceId) continue;
      final contextChanged = session.context.surface != surface;
      if (contextChanged) {
        session.context = session.context.copyWith(surface: surface);
      }
      session.publishEvent('surface.changed', {...payload, 'surface': surface});
      if (contextChanged) {
        session.publishEvent('context.changed', session.context.toJson());
      }
      changedSessions++;
    }
    return changedSessions;
  }

  void _validateEvent(String event) {
    if (!LocalRpcContract.events.contains(event)) {
      throw ArgumentError.value(event, 'event', 'unknown runtime event');
    }
  }

  Future<void> close() async {
    for (final subscription in _metricsSubscriptions.values) {
      subscription.cancel();
    }
    _metricsSubscriptions.clear();
    for (final session in _sessionsByAuthority.values) {
      session.closeEventSockets();
    }
    _sessionsByAuthority.clear();
    await _subscription.cancel();
    await _server.close(force: true);
  }

  Future<void> _handleRequest(HttpRequest request) async {
    final authority = request.headers.value(HttpHeaders.hostHeader);
    final session = _sessionsByAuthority[authority];
    if (session == null) {
      await _closeWithStatus(request, HttpStatus.notFound);
      return;
    }

    if (request.uri.path == '/v1/rpc') {
      await _handleRpc(request, session);
      return;
    }

    if (request.uri.path == '/v1/events') {
      await _handleEvents(request, session);
      return;
    }

    if (request.uri.path == '/runtime/bootstrap.js') {
      if (request.method != 'GET') {
        await _closeWithStatus(request, HttpStatus.methodNotAllowed);
        return;
      }
      await _serveBootstrap(request, session);
      return;
    }

    if (request.method != 'GET') {
      await _closeWithStatus(request, HttpStatus.methodNotAllowed);
      return;
    }

    final resource = session.resources[request.uri.path];
    if (resource == null) {
      await _closeWithStatus(request, HttpStatus.notFound);
      return;
    }

    final response = request.response;
    response.statusCode = HttpStatus.ok;
    response.headers.contentType = ContentType.parse(resource.contentType);
    response.headers.set('Content-Security-Policy', _contentSecurityPolicy);
    response.headers.set('X-Content-Type-Options', 'nosniff');
    response.headers.set('Referrer-Policy', 'no-referrer');
    response.headers.set(
      'Cache-Control',
      'public, max-age=31536000, immutable',
    );
    response.add(resource.bytes);
    await response.close();
  }

  Future<void> _handleRpc(HttpRequest request, RuntimeSession session) async {
    if (request.method != 'POST') {
      await _closeWithStatus(request, HttpStatus.methodNotAllowed);
      return;
    }
    if (request.headers.value(HttpHeaders.authorizationHeader) !=
        'Bearer ${session.token}') {
      await _closeWithStatus(request, HttpStatus.unauthorized);
      return;
    }
    if (request.headers.value('Origin') != session.origin) {
      await _closeWithStatus(request, HttpStatus.forbidden);
      return;
    }
    if (request.headers.value('X-AgentCard-RPC-Version') != '1') {
      await _closeWithStatus(request, HttpStatus.badRequest);
      return;
    }

    final bytes = <int>[];
    await for (final chunk in request) {
      bytes.addAll(chunk);
      if (bytes.length > 256 * 1024) {
        await _closeWithStatus(request, HttpStatus.requestEntityTooLarge);
        return;
      }
    }

    Object? decoded;
    try {
      decoded = jsonDecode(utf8.decode(bytes));
    } on FormatException {
      await _writeRpcError(request, null, -32700, 'Parse error');
      return;
    }
    if (decoded is! Map<String, Object?> ||
        decoded['jsonrpc'] != '2.0' ||
        decoded['method'] is! String) {
      await _writeRpcError(request, null, -32600, 'Invalid Request');
      return;
    }

    final id = decoded['id'];
    final method = decoded['method']! as String;
    if (!LocalRpcContract.methods.contains(method)) {
      await _writeRpcError(request, id, -32601, 'Method not found');
      return;
    }
    final rawParams = decoded['params'];
    if (rawParams != null && rawParams is! Map<String, Object?>) {
      await _writeRpcError(request, id, -32602, 'Invalid params');
      return;
    }
    final params = rawParams as Map<String, Object?>? ?? const {};

    if (!session.allowRpc()) {
      await _writeRpcError(
        request,
        id,
        -32000,
        'Rate limited',
        stableCode: 'RATE_LIMITED',
      );
      return;
    }

    try {
      final result = await _invoke(
        session,
        method,
        params,
      ).timeout(_invocationTimeout);
      await _writeRpcResult(
        request,
        id,
        result,
        maxBytes: method == 'network.fetch'
            ? 6 * 1024 * 1024
            : _maxResponseBytes,
      );
    } on _InvalidParams catch (error) {
      await _writeRpcError(request, id, -32602, error.message);
    } on _UnknownMethod {
      await _writeRpcError(request, id, -32601, 'Method not found');
    } on RuntimeRpcException catch (error) {
      await _writeRpcError(
        request,
        id,
        -32000,
        error.message,
        stableCode: error.code,
      );
    } on TimeoutException {
      await _writeRpcError(
        request,
        id,
        -32000,
        'Invocation timed out',
        stableCode: 'TIMEOUT',
      );
    } catch (_) {
      await _writeRpcError(
        request,
        id,
        -32000,
        'Internal error',
        stableCode: 'INTERNAL',
      );
    }
  }

  Future<void> _handleEvents(
    HttpRequest request,
    RuntimeSession session,
  ) async {
    if (request.headers.value('Origin') != session.origin) {
      await _closeWithStatus(request, HttpStatus.forbidden);
      return;
    }
    if (!WebSocketTransformer.isUpgradeRequest(request)) {
      await _closeWithStatus(request, HttpStatus.badRequest);
      return;
    }
    final socket = await WebSocketTransformer.upgrade(request);
    var authenticated = false;
    socket.listen(
      (message) {
        if (authenticated || message is! String) {
          return;
        }
        try {
          final decoded = jsonDecode(message);
          if (decoded is Map<String, Object?> &&
              decoded['type'] == 'authenticate' &&
              decoded['token'] == session.token) {
            authenticated = true;
            session.attachEventSocket(socket);
            socket.add(jsonEncode({'type': 'authenticated'}));
            return;
          }
        } on FormatException {
          // Invalid authentication frames are closed below.
        }
        unawaited(
          socket.close(
            WebSocketStatus.policyViolation,
            'invalid event authentication',
          ),
        );
      },
      onDone: () => session.detachEventSocket(socket),
      onError: (_) => session.detachEventSocket(socket),
      cancelOnError: true,
    );
  }

  Future<Object?> _invoke(
    RuntimeSession session,
    String method,
    Map<String, Object?> params,
  ) async {
    final capability = _requiredCapability(method);
    if (capability != null &&
        !session.declaredCapabilities.contains(capability)) {
      throw const RuntimeRpcException(
        'PERMISSION_DENIED',
        'Capability was not declared by the card',
      );
    }
    switch (method) {
      case 'runtime.getContext':
        return session.context.toJson();
      case 'storage.get':
        final key = _storageKey(params);
        return {'value': session.storage.get(key)};
      case 'storage.set':
        final key = _storageKey(params);
        if (!params.containsKey('value')) {
          throw const _InvalidParams('value is required');
        }
        final candidate = Map<String, Object?>.from(session.storage.snapshot())
          ..[key] = params['value'];
        if (utf8.encode(jsonEncode(candidate)).length > 5 * 1024 * 1024) {
          throw const _InvalidParams('storage quota exceeded');
        }
        session.storage.set(key, params['value']);
        return {'stored': true};
      case 'storage.delete':
        final key = _storageKey(params);
        return {'deleted': session.storage.delete(key)};
      case 'storage.list':
        return {'keys': session.storage.listKeys()};
      case 'system.metrics.subscribe':
        if (params.keys.toSet().difference(const {'intervalMs'}).isNotEmpty) {
          throw const _InvalidParams('unknown metrics subscription parameter');
        }
        final intervalMs = params['intervalMs'] ?? 5000;
        if (intervalMs is! int ||
            intervalMs < _minimumMetricsInterval.inMilliseconds ||
            intervalMs > const Duration(minutes: 1).inMilliseconds) {
          throw _InvalidParams(
            'intervalMs must be between '
            '${_minimumMetricsInterval.inMilliseconds} and 60000',
          );
        }
        _metricsSubscriptions.remove(session.id)?.cancel();
        _metricsSubscriptions[session.id] = _MetricsSubscription(
          Duration(milliseconds: intervalMs),
          () => _publishMetrics(session),
        );
        return {'subscribed': true, 'intervalMs': intervalMs};
      case 'system.metrics.unsubscribe':
        if (params.isNotEmpty) {
          throw const _InvalidParams(
            'system.metrics.unsubscribe does not accept parameters',
          );
        }
        _metricsSubscriptions.remove(session.id)?.cancel();
        return {'subscribed': false};
      default:
        final handler = session.rpcHandler;
        if (handler == null) {
          throw const RuntimeRpcException(
            'CAPABILITY_UNAVAILABLE',
            'Capability is unavailable',
          );
        }
        return handler(session.rpcContext, method, params);
    }
  }

  Future<void> _publishMetrics(RuntimeSession session) async {
    if (!_sessionsByAuthority.containsKey(session.authority)) {
      return;
    }
    try {
      final metrics = await _invoke(
        session,
        'system.metrics.get',
        const {},
      ).timeout(_invocationTimeout);
      if (_metricsSubscriptions.containsKey(session.id)) {
        session.publishEvent('system.metrics', metrics);
      }
    } catch (_) {
      // A transient metrics read must not terminate the runtime or leak details.
    }
  }

  String? _requiredCapability(String method) {
    if (method == 'runtime.getContext') {
      return null;
    }
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

  String _storageKey(Map<String, Object?> params) {
    final key = params['key'];
    if (key is! String || key.isEmpty || key.length > 256) {
      throw const _InvalidParams('key must be a non-empty string');
    }
    return key;
  }

  Future<void> _writeRpcResult(
    HttpRequest request,
    Object? id,
    Object? result, {
    required int maxBytes,
  }) async {
    final body = {'jsonrpc': '2.0', 'id': id, 'result': result};
    if (utf8.encode(jsonEncode(body)).length > maxBytes) {
      await _writeRpcError(
        request,
        id,
        -32000,
        'Response exceeds size limit',
        stableCode: 'INTERNAL',
      );
      return;
    }
    await _writeRpcResponse(request, body);
  }

  Future<void> _writeRpcError(
    HttpRequest request,
    Object? id,
    int code,
    String message, {
    String? stableCode,
  }) {
    return _writeRpcResponse(request, {
      'jsonrpc': '2.0',
      'id': id,
      'error': {
        'code': code,
        'message': message,
        if (stableCode != null) 'data': {'code': stableCode},
      },
    });
  }

  Future<void> _serveBootstrap(
    HttpRequest request,
    RuntimeSession session,
  ) async {
    final token = jsonEncode(session.token);
    final source =
        '(function(){\n'
        '  "use strict";\n'
        '  const token = $token;\n'
        '  let nextId = 1;\n'
        '  const handlers = new Map();\n'
        '  async function invoke(method, params) {\n'
        '    const response = await fetch("/v1/rpc", {\n'
        '      method: "POST",\n'
        '      headers: {\n'
        '        "Authorization": "Bearer " + token,\n'
        '        "Content-Type": "application/json",\n'
        '        "X-AgentCard-RPC-Version": "1"\n'
        '      },\n'
        '      body: JSON.stringify({jsonrpc:"2.0",id:nextId++,method:method,params:params||{}})\n'
        '    });\n'
        '    const payload = await response.json();\n'
        '    if (payload.error) { const error = new Error(payload.error.message); error.code = payload.error.data && payload.error.data.code; throw error; }\n'
        '    return payload.result;\n'
        '  }\n'
        '  const socket = new WebSocket((location.protocol==="https:"?"wss://":"ws://")+location.host+"/v1/events");\n'
        '  socket.addEventListener("open", function(){socket.send(JSON.stringify({type:"authenticate",token:token}));});\n'
        '  socket.addEventListener("message", function(event){const message=JSON.parse(event.data);const list=handlers.get(message.event)||[];list.forEach(function(handler){handler(message.payload);});});\n'
        '  window.agentCard = Object.freeze({\n'
        '    getContext: function(){return invoke("runtime.getContext");},\n'
        '    invoke: invoke,\n'
        '    subscribe: function(event, handler){const list=handlers.get(event)||[];list.push(handler);handlers.set(event,list);return function(){handlers.set(event,(handlers.get(event)||[]).filter(function(item){return item!==handler;}));};}\n'
        '  });\n'
        '})();\n';
    request.response.statusCode = HttpStatus.ok;
    request.response.headers.contentType = ContentType(
      'application',
      'javascript',
      charset: 'utf-8',
    );
    request.response.headers.set('Cache-Control', 'no-store');
    request.response.headers.set('X-Content-Type-Options', 'nosniff');
    request.response.headers.set('Referrer-Policy', 'no-referrer');
    request.response.write(source);
    await request.response.close();
  }

  Future<void> _writeRpcResponse(
    HttpRequest request,
    Map<String, Object?> body,
  ) async {
    request.response.statusCode = HttpStatus.ok;
    request.response.headers.contentType = ContentType.json;
    request.response.headers.set('X-Content-Type-Options', 'nosniff');
    request.response.headers.set('Referrer-Policy', 'no-referrer');
    request.response.write(jsonEncode(body));
    await request.response.close();
  }

  Future<void> _closeWithStatus(HttpRequest request, int statusCode) async {
    request.response.statusCode = statusCode;
    await request.response.close();
  }

  List<int> _randomBytes(int byteCount) {
    return List<int>.generate(
      byteCount,
      (_) => _random.nextInt(256),
      growable: false,
    );
  }

  String _randomHex(int byteCount) {
    return _randomBytes(
      byteCount,
    ).map((byte) => byte.toRadixString(16).padLeft(2, '0')).join();
  }

  String _randomToken(int byteCount) {
    return base64Url.encode(_randomBytes(byteCount)).replaceAll('=', '');
  }
}

class _MetricsSubscription {
  _MetricsSubscription(Duration interval, Future<void> Function() sample)
    : _sample = sample {
    _timer = Timer.periodic(interval, (_) => _tick());
  }

  final Future<void> Function() _sample;
  late final Timer _timer;
  var _sampling = false;

  void _tick() {
    if (_sampling) {
      return;
    }
    _sampling = true;
    unawaited(_sample().whenComplete(() => _sampling = false));
  }

  void cancel() => _timer.cancel();
}

class _InvalidParams implements Exception {
  const _InvalidParams(this.message);

  final String message;
}

class _UnknownMethod implements Exception {
  const _UnknownMethod();
}

class RuntimeRpcException implements Exception {
  const RuntimeRpcException(this.code, this.message);

  final String code;
  final String message;
}
