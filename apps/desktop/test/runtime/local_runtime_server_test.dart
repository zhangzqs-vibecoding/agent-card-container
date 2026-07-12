import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:agent_card_desktop/src/runtime/local_runtime_server.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  group('LocalRuntimeServer', () {
    late LocalRuntimeServer server;

    setUp(() async {
      server = await LocalRuntimeServer.start();
    });

    tearDown(() async {
      await server.close();
    });

    test('binds only to IPv4 loopback with a random port', () {
      expect(server.address, InternetAddress.loopbackIPv4);
      expect(server.port, greaterThan(0));
    });

    test('creates isolated sessions', () {
      final first = server.createSession(
        instanceId: 'instance-1',
        cardId: 'card-one',
        versionId: 'version-one',
        resources: const {},
      );
      final second = server.createSession(
        instanceId: 'instance-2',
        cardId: 'card-two',
        versionId: 'version-two',
        resources: const {},
      );

      expect(first.id, isNot(second.id));
      expect(first.id, matches(RegExp(r'^[a-f0-9]{32}$')));
      expect(first.authority, isNot(second.authority));
      expect(first.token, isNot(second.token));
      expect(first.origin, startsWith('http://'));
    });

    test(
      'serves a dynamic bootstrap SDK without putting token in URL',
      () async {
        final session = server.createSession(
          instanceId: 'instance-1',
          cardId: 'card-one',
          versionId: 'version-one',
          resources: const {},
        );

        final response = await _request(
          server,
          '/runtime/bootstrap.js',
          authority: session.authority,
        );

        expect(response.statusCode, HttpStatus.ok);
        expect(
          response.headers.contentType?.mimeType,
          'application/javascript',
        );
        expect(response.headers.value('cache-control'), 'no-store');
        expect(response.body, contains('window.agentCard'));
        expect(response.body, contains(session.token));
        expect(response.body, contains('/v1/rpc'));
        expect(response.body, contains('/v1/events'));
        expect(session.origin, isNot(contains(session.token)));
        expect(session.resources.toString(), isNot(contains(session.token)));
      },
    );

    test(
      'serves registered resources only for the exact session host',
      () async {
        final session = server.createSession(
          instanceId: 'instance-1',
          cardId: 'card-one',
          versionId: 'version-one',
          resources: {
            '/bundle/hash/index.html': RuntimeResource.html('<h1>offline</h1>'),
          },
        );

        final rejected = await _request(server, '/bundle/hash/index.html');
        expect(rejected.statusCode, HttpStatus.notFound);

        final accepted = await _request(
          server,
          '/bundle/hash/index.html',
          authority: session.authority,
        );
        expect(accepted.statusCode, HttpStatus.ok);
        expect(accepted.body, '<h1>offline</h1>');
        expect(accepted.headers.contentType?.mimeType, 'text/html');
        expect(accepted.headers.value('content-security-policy'), isNotEmpty);
        expect(accepted.headers.value('x-content-type-options'), 'nosniff');
        expect(accepted.headers.value('referrer-policy'), 'no-referrer');
      },
    );

    test('rejects unauthenticated and cross-origin RPC requests', () async {
      final session = server.createSession(
        instanceId: 'instance-1',
        cardId: 'card-one',
        versionId: 'version-one',
        resources: const {},
      );
      const body = {'jsonrpc': '2.0', 'id': 1, 'method': 'runtime.getContext'};

      final unauthenticated = await _request(
        server,
        '/v1/rpc',
        authority: session.authority,
        method: 'POST',
        headers: {
          'Origin': session.origin,
          HttpHeaders.contentTypeHeader: 'application/json',
        },
        body: body,
      );
      expect(unauthenticated.statusCode, HttpStatus.unauthorized);

      final crossOrigin = await _request(
        server,
        '/v1/rpc',
        authority: session.authority,
        method: 'POST',
        headers: {
          'Origin': 'http://evil.localhost',
          HttpHeaders.authorizationHeader: 'Bearer ${session.token}',
          HttpHeaders.contentTypeHeader: 'application/json',
        },
        body: body,
      );
      expect(crossOrigin.statusCode, HttpStatus.forbidden);
    });

    test('serves context and isolates storage through JSON-RPC', () async {
      final session = server.createSession(
        instanceId: 'instance-1',
        cardId: 'card-one',
        versionId: 'version-one',
        resources: const {},
        declaredCapabilities: const {'storage'},
      );

      final context = await _rpc(
        server,
        session,
        id: 1,
        method: 'runtime.getContext',
      );
      expect(context['error'], isNull);
      expect(
        (context['result'] as Map<String, Object?>)['instanceId'],
        'instance-1',
      );

      final setResult = await _rpc(
        server,
        session,
        id: 2,
        method: 'storage.set',
        params: {
          'key': 'counter',
          'value': {'count': 1},
        },
      );
      expect(setResult['error'], isNull);

      final getResult = await _rpc(
        server,
        session,
        id: 3,
        method: 'storage.get',
        params: {'key': 'counter'},
      );
      expect(getResult['result'], {
        'value': {'count': 1},
      });

      final unknown = await _rpc(
        server,
        session,
        id: 4,
        method: 'system.shell',
      );
      expect((unknown['error'] as Map<String, Object?>)['code'], -32601);
    });

    test('expires a closed session immediately', () async {
      final session = server.createSession(
        instanceId: 'instance-1',
        cardId: 'card-one',
        versionId: 'version-one',
        resources: {'/bundle/hash/index.html': RuntimeResource.html('content')},
      );

      server.closeSession(session.id);

      final response = await _request(
        server,
        '/bundle/hash/index.html',
        authority: session.authority,
      );
      expect(response.statusCode, HttpStatus.notFound);
    });

    test('lists and deletes storage keys', () async {
      final session = server.createSession(
        instanceId: 'instance-1',
        cardId: 'card-one',
        versionId: 'version-one',
        resources: const {},
        declaredCapabilities: const {'storage'},
      );
      await _rpc(
        server,
        session,
        id: 1,
        method: 'storage.set',
        params: {'key': 'b', 'value': 2},
      );
      await _rpc(
        server,
        session,
        id: 2,
        method: 'storage.set',
        params: {'key': 'a', 'value': 1},
      );

      final listed = await _rpc(server, session, id: 3, method: 'storage.list');
      expect(listed['result'], {
        'keys': ['a', 'b'],
      });

      final deleted = await _rpc(
        server,
        session,
        id: 4,
        method: 'storage.delete',
        params: {'key': 'a'},
      );
      expect(deleted['result'], {'deleted': true});
    });

    test('rejects storage writes beyond the per-session quota', () async {
      final session = server.createSession(
        instanceId: 'instance-1',
        cardId: 'card-one',
        versionId: 'version-one',
        resources: const {},
        declaredCapabilities: const {'storage'},
      );
      final chunk = List.filled(200000, 'x').join();
      Map<String, Object?>? response;

      for (var index = 0; index < 30; index++) {
        response = await _rpc(
          server,
          session,
          id: index,
          method: 'storage.set',
          params: {'key': 'key-$index', 'value': chunk},
        );
        if (response['error'] != null) {
          break;
        }
      }

      expect(response, isNotNull);
      expect((response!['error'] as Map<String, Object?>)['code'], -32602);
    });

    test('rejects RPC bodies larger than 256 KiB', () async {
      final session = server.createSession(
        instanceId: 'instance-1',
        cardId: 'card-one',
        versionId: 'version-one',
        resources: const {},
      );
      final response = await _request(
        server,
        '/v1/rpc',
        authority: session.authority,
        method: 'POST',
        headers: {
          'Origin': session.origin,
          HttpHeaders.authorizationHeader: 'Bearer ${session.token}',
          HttpHeaders.contentTypeHeader: 'application/json',
          'X-AgentCard-RPC-Version': '1',
        },
        body: {
          'jsonrpc': '2.0',
          'id': 1,
          'method': 'storage.set',
          'params': {'key': 'large', 'value': List.filled(300000, 'x').join()},
        },
      );

      expect(response.statusCode, HttpStatus.requestEntityTooLarge);
    });

    test('rate limits each session with a stable error code', () async {
      await server.close();
      server = await LocalRuntimeServer.start(
        rateLimit: const RuntimeRateLimit(refillPerSecond: 0, burst: 2),
      );
      final session = server.createSession(
        instanceId: 'instance-1',
        cardId: 'card-one',
        versionId: 'version-one',
        resources: const {},
      );

      await _rpc(server, session, id: 1, method: 'runtime.getContext');
      await _rpc(server, session, id: 2, method: 'runtime.getContext');
      final limited = await _rpc(
        server,
        session,
        id: 3,
        method: 'runtime.getContext',
      );

      expect((limited['error'] as Map<String, Object?>)['data'], {
        'code': 'RATE_LIMITED',
      });
    });

    test('authenticates WebSocket events without token in the URL', () async {
      final session = server.createSession(
        instanceId: 'instance-1',
        cardId: 'card-one',
        versionId: 'version-one',
        resources: const {},
      );
      final socket = await WebSocket.connect(
        'ws://127.0.0.1:${server.port}/v1/events',
        headers: {
          HttpHeaders.hostHeader: session.authority,
          'Origin': session.origin,
        },
      );
      addTearDown(socket.close);
      final messages = StreamIterator<dynamic>(socket);

      socket.add(jsonEncode({'type': 'authenticate', 'token': session.token}));
      expect(await messages.moveNext(), isTrue);
      expect(jsonDecode(messages.current as String), {'type': 'authenticated'});

      final seq = server.publishEvent(session.id, 'theme.changed', {
        'theme': 'dark',
      });
      expect(seq, 1);
      expect(await messages.moveNext(), isTrue);
      expect(jsonDecode(messages.current as String), {
        'seq': 1,
        'event': 'theme.changed',
        'payload': {'theme': 'dark'},
      });
      await messages.cancel();
    });

    test('subscribes and unsubscribes bounded system metrics events', () async {
      await server.close();
      server = await LocalRuntimeServer.start(
        minimumMetricsInterval: const Duration(milliseconds: 10),
      );
      var reads = 0;
      final session = server.createSession(
        instanceId: 'instance-1',
        cardId: 'card-one',
        versionId: 'version-one',
        resources: const {},
        declaredCapabilities: const {'system.metrics.read'},
        rpcHandler: (_, method, params) async {
          expect(method, 'system.metrics.get');
          expect(params, isEmpty);
          return {'processorCount': 8, 'sample': ++reads};
        },
      );
      final socket = await WebSocket.connect(
        'ws://127.0.0.1:${server.port}/v1/events',
        headers: {
          HttpHeaders.hostHeader: session.authority,
          'Origin': session.origin,
        },
      );
      addTearDown(socket.close);
      final messages = StreamIterator<dynamic>(socket);
      socket.add(jsonEncode({'type': 'authenticate', 'token': session.token}));
      expect(await messages.moveNext(), isTrue);

      final subscribed = await _rpc(
        server,
        session,
        id: 1,
        method: 'system.metrics.subscribe',
        params: {'intervalMs': 10},
      );
      expect(subscribed['result'], {'subscribed': true, 'intervalMs': 10});
      expect(await messages.moveNext(), isTrue);
      final event = jsonDecode(messages.current as String) as Map;
      expect(event['event'], 'system.metrics');
      expect((event['payload'] as Map)['processorCount'], 8);

      final unsubscribed = await _rpc(
        server,
        session,
        id: 2,
        method: 'system.metrics.unsubscribe',
      );
      expect(unsubscribed['result'], {'subscribed': false});
      await messages.cancel();
    });

    test('rejects invalid event authentication and unknown events', () async {
      final session = server.createSession(
        instanceId: 'instance-1',
        cardId: 'card-one',
        versionId: 'version-one',
        resources: const {},
      );
      final socket = await WebSocket.connect(
        'ws://127.0.0.1:${server.port}/v1/events',
        headers: {
          HttpHeaders.hostHeader: session.authority,
          'Origin': session.origin,
        },
      );
      socket.add(jsonEncode({'type': 'authenticate', 'token': 'wrong'}));
      await socket.drain<void>().timeout(const Duration(seconds: 2));

      expect(socket.closeCode, WebSocketStatus.policyViolation);
      expect(
        () => server.publishEvent(session.id, 'shell.output', const {}),
        throwsArgumentError,
      );
    });

    test('forwards only registered capability methods to the host', () async {
      final calls = <String>[];
      final session = server.createSession(
        instanceId: 'instance-1',
        cardId: 'card-one',
        versionId: 'version-one',
        resources: const {},
        declaredCapabilities: const {'notification.show'},
        rpcHandler: (context, method, params) async {
          calls.add('${context.instanceId}:$method');
          return {'accepted': params['title']};
        },
      );

      final allowed = await _rpc(
        server,
        session,
        id: 1,
        method: 'notification.show',
        params: {'title': '完成'},
      );
      final forbidden = await _rpc(
        server,
        session,
        id: 2,
        method: 'shell.execute',
      );
      final undeclared = await _rpc(
        server,
        session,
        id: 3,
        method: 'storage.get',
        params: {'key': 'private'},
      );

      expect(allowed['result'], {'accepted': '完成'});
      expect(calls, ['instance-1:notification.show']);
      expect((forbidden['error'] as Map<String, Object?>)['code'], -32601);
      expect((undeclared['error'] as Map<String, Object?>)['data'], {
        'code': 'PERMISSION_DENIED',
      });
    });

    test(
      'maps host timeout and oversized responses to stable errors',
      () async {
        await server.close();
        server = await LocalRuntimeServer.start(
          invocationTimeout: const Duration(milliseconds: 10),
          maxResponseBytes: 128,
        );
        final session = server.createSession(
          instanceId: 'instance-1',
          cardId: 'card-one',
          versionId: 'version-one',
          resources: const {},
          declaredCapabilities: const {
            'notification.show',
            'system.metrics.read',
          },
          rpcHandler: (_, method, _) async {
            if (method == 'notification.show') {
              await Future<void>.delayed(const Duration(milliseconds: 50));
              return null;
            }
            return {'body': List.filled(256, 'x').join()};
          },
        );

        final timedOut = await _rpc(
          server,
          session,
          id: 1,
          method: 'notification.show',
        );
        final oversized = await _rpc(
          server,
          session,
          id: 2,
          method: 'system.metrics.get',
        );

        expect((timedOut['error'] as Map<String, Object?>)['data'], {
          'code': 'TIMEOUT',
        });
        expect((oversized['error'] as Map<String, Object?>)['data'], {
          'code': 'INTERNAL',
        });
      },
    );

    test('allows the documented network.fetch response budget', () async {
      final session = server.createSession(
        instanceId: 'instance-1',
        cardId: 'card-one',
        versionId: 'version-one',
        resources: const {},
        declaredCapabilities: const {'network.fetch'},
        rpcHandler: (_, _, _) async => {
          'status': 200,
          'body': List.filled(2 * 1024 * 1024, 'x').join(),
        },
      );

      final response = await _rpc(
        server,
        session,
        id: 1,
        method: 'network.fetch',
        params: {'url': 'https://api.example.test/data'},
      );

      expect((response['result'] as Map<String, Object?>)['status'], 200);
    });
  });
}

Future<Map<String, Object?>> _rpc(
  LocalRuntimeServer server,
  RuntimeSession session, {
  required int id,
  required String method,
  Map<String, Object?>? params,
}) async {
  final response = await _request(
    server,
    '/v1/rpc',
    authority: session.authority,
    method: 'POST',
    headers: {
      'Origin': session.origin,
      HttpHeaders.authorizationHeader: 'Bearer ${session.token}',
      HttpHeaders.contentTypeHeader: 'application/json',
      'X-AgentCard-RPC-Version': '1',
    },
    body: {
      'jsonrpc': '2.0',
      'id': id,
      'method': method,
      if (params != null) 'params': params,
    },
  );
  expect(response.statusCode, HttpStatus.ok);
  return jsonDecode(response.body) as Map<String, Object?>;
}

Future<_Response> _request(
  LocalRuntimeServer server,
  String path, {
  String? authority,
  String method = 'GET',
  Map<String, String> headers = const {},
  Object? body,
}) async {
  final client = HttpClient();
  try {
    final uri = Uri(
      scheme: 'http',
      host: InternetAddress.loopbackIPv4.address,
      port: server.port,
      path: path,
    );
    final request = await client.openUrl(method, uri);
    if (authority != null) {
      request.headers.set(HttpHeaders.hostHeader, authority);
    }
    for (final entry in headers.entries) {
      request.headers.set(entry.key, entry.value);
    }
    if (body != null) {
      request.add(utf8.encode(jsonEncode(body)));
    }
    final response = await request.close();
    final responseBody = await utf8.decoder.bind(response).join();
    return _Response(
      statusCode: response.statusCode,
      headers: response.headers,
      body: responseBody,
    );
  } finally {
    client.close(force: true);
  }
}

class _Response {
  const _Response({
    required this.statusCode,
    required this.headers,
    required this.body,
  });

  final int statusCode;
  final HttpHeaders headers;
  final String body;
}
