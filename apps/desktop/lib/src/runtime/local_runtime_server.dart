import 'dart:async';
import 'dart:convert';
import 'dart:io';
import 'dart:math';

export 'runtime_session.dart';

import 'runtime_session.dart';

class LocalRuntimeServer {
  LocalRuntimeServer._(this._server) {
    _subscription = _server.listen(_handleRequest);
  }

  static Future<LocalRuntimeServer> start() async {
    final server = await HttpServer.bind(
      InternetAddress.loopbackIPv4,
      0,
      shared: false,
    );
    return LocalRuntimeServer._(server);
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
  final _sessionsByAuthority = <String, RuntimeSession>{};
  late final StreamSubscription<HttpRequest> _subscription;
  final Random _random = Random.secure();

  InternetAddress get address => _server.address;
  int get port => _server.port;

  RuntimeSession createSession({
    required String instanceId,
    required String cardId,
    required String versionId,
    required Map<String, RuntimeResource> resources,
  }) {
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
    );
    _sessionsByAuthority[authority] = session;
    return session;
  }

  void closeSession(String id) {
    _sessionsByAuthority.removeWhere((_, session) => session.id == id);
  }

  Future<void> close() async {
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
    final rawParams = decoded['params'];
    if (rawParams != null && rawParams is! Map<String, Object?>) {
      await _writeRpcError(request, id, -32602, 'Invalid params');
      return;
    }
    final params = rawParams as Map<String, Object?>? ?? const {};

    try {
      final result = _invoke(session, method, params);
      await _writeRpcResult(request, id, result);
    } on _InvalidParams catch (error) {
      await _writeRpcError(request, id, -32602, error.message);
    } on _UnknownMethod {
      await _writeRpcError(request, id, -32601, 'Method not found');
    }
  }

  Object? _invoke(
    RuntimeSession session,
    String method,
    Map<String, Object?> params,
  ) {
    switch (method) {
      case 'runtime.getContext':
        return {
          'instanceId': session.instanceId,
          'cardId': session.cardId,
          'versionId': session.versionId,
          'online': true,
        };
      case 'storage.get':
        final key = _storageKey(params);
        return {'value': session.storage[key]};
      case 'storage.set':
        final key = _storageKey(params);
        if (!params.containsKey('value')) {
          throw const _InvalidParams('value is required');
        }
        final candidate = Map<String, Object?>.from(session.storage)
          ..[key] = params['value'];
        if (utf8.encode(jsonEncode(candidate)).length > 5 * 1024 * 1024) {
          throw const _InvalidParams('storage quota exceeded');
        }
        session.storage[key] = params['value'];
        return {'stored': true};
      case 'storage.delete':
        final key = _storageKey(params);
        return {'deleted': session.storage.remove(key) != null};
      case 'storage.list':
        final keys = session.storage.keys.toList()..sort();
        return {'keys': keys};
      default:
        throw const _UnknownMethod();
    }
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
    Object? result,
  ) {
    return _writeRpcResponse(request, {
      'jsonrpc': '2.0',
      'id': id,
      'result': result,
    });
  }

  Future<void> _writeRpcError(
    HttpRequest request,
    Object? id,
    int code,
    String message,
  ) {
    return _writeRpcResponse(request, {
      'jsonrpc': '2.0',
      'id': id,
      'error': {'code': code, 'message': message},
    });
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

class _InvalidParams implements Exception {
  const _InvalidParams(this.message);

  final String message;
}

class _UnknownMethod implements Exception {
  const _UnknownMethod();
}
