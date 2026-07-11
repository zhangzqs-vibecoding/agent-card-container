import 'dart:async';
import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';

typedef RuntimeRpcHandler =
    FutureOr<Object?> Function(
      RuntimeRpcContext context,
      String method,
      Map<String, Object?> params,
    );

class RuntimeRpcContext {
  const RuntimeRpcContext({
    required this.instanceId,
    required this.cardId,
    required this.versionId,
  });

  final String instanceId;
  final String cardId;
  final String versionId;
}

class RuntimeResource {
  RuntimeResource({required this.bytes, required this.contentType});

  factory RuntimeResource.html(String source) {
    return RuntimeResource(
      bytes: Uint8List.fromList(utf8.encode(source)),
      contentType: 'text/html; charset=utf-8',
    );
  }

  final Uint8List bytes;
  final String contentType;
}

class RuntimeSession {
  RuntimeSession({
    required this.id,
    required this.authority,
    required this.token,
    required this.instanceId,
    required this.cardId,
    required this.versionId,
    required Map<String, RuntimeResource> resources,
    Set<String> declaredCapabilities = const {},
    RuntimeRateLimit rateLimit = const RuntimeRateLimit(),
    this.rpcHandler,
  }) : resources = Map.unmodifiable(resources),
       declaredCapabilities = Set.unmodifiable(declaredCapabilities),
       _rateLimiter = _TokenBucket(rateLimit);

  final String id;
  final String authority;
  final String token;
  final String instanceId;
  final String cardId;
  final String versionId;
  final Map<String, RuntimeResource> resources;
  final Set<String> declaredCapabilities;
  final RuntimeRpcHandler? rpcHandler;
  final Map<String, Object?> storage = {};
  final _TokenBucket _rateLimiter;
  final Set<WebSocket> _eventSockets = {};
  var _eventSequence = 0;

  String get origin => 'http://$authority';

  bool allowRpc() => _rateLimiter.take();

  RuntimeRpcContext get rpcContext => RuntimeRpcContext(
    instanceId: instanceId,
    cardId: cardId,
    versionId: versionId,
  );

  void attachEventSocket(WebSocket socket) {
    _eventSockets.add(socket);
  }

  void detachEventSocket(WebSocket socket) {
    _eventSockets.remove(socket);
  }

  int publishEvent(String event, Object? payload) {
    final sequence = ++_eventSequence;
    final message = jsonEncode({
      'seq': sequence,
      'event': event,
      'payload': payload,
    });
    for (final socket in _eventSockets.toList(growable: false)) {
      if (socket.readyState == WebSocket.open) {
        socket.add(message);
      } else {
        _eventSockets.remove(socket);
      }
    }
    return sequence;
  }

  void closeEventSockets() {
    for (final socket in _eventSockets.toList(growable: false)) {
      unawaited(
        socket.close(WebSocketStatus.goingAway, 'runtime session closed'),
      );
    }
    _eventSockets.clear();
  }
}

class RuntimeRateLimit {
  const RuntimeRateLimit({this.refillPerSecond = 30, this.burst = 60})
    : assert(refillPerSecond >= 0),
      assert(burst > 0);

  final double refillPerSecond;
  final int burst;
}

class _TokenBucket {
  _TokenBucket(this.configuration)
    : _tokens = configuration.burst.toDouble(),
      _lastRefill = DateTime.now();

  final RuntimeRateLimit configuration;
  double _tokens;
  DateTime _lastRefill;

  bool take() {
    final now = DateTime.now();
    final elapsed = now.difference(_lastRefill).inMicroseconds / 1000000;
    _lastRefill = now;
    _tokens = (_tokens + elapsed * configuration.refillPerSecond).clamp(
      0,
      configuration.burst.toDouble(),
    );
    if (_tokens < 1) {
      return false;
    }
    _tokens -= 1;
    return true;
  }
}
