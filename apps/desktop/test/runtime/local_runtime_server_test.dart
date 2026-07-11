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
      request.write(jsonEncode(body));
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
