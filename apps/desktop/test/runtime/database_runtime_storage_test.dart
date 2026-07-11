import 'dart:convert';
import 'dart:io';

import 'package:agent_card_desktop/src/runtime/local_runtime_server.dart';
import 'package:agent_card_desktop/src/storage/database_runtime_storage.dart';
import 'package:agent_card_desktop/src/storage/local_database.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test(
    'CodeCard storage survives runtime session and server restart',
    () async {
      final database = LocalDatabase.inMemory();
      var server = await LocalRuntimeServer.start();
      addTearDown(() async {
        await server.close();
        database.close();
      });
      var session = server.createSession(
        instanceId: 'instance-1',
        cardId: 'card-1',
        versionId: 'version-1',
        resources: const {},
        declaredCapabilities: const {'storage'},
        storage: DatabaseRuntimeStorage(database, 'state-1'),
      );

      final stored = await _rpc(server, session, 'storage.set', {
        'key': 'draft',
        'value': {'text': '离线内容'},
      });
      expect(stored['error'], isNull);
      await server.close();

      server = await LocalRuntimeServer.start();
      session = server.createSession(
        instanceId: 'instance-1',
        cardId: 'card-1',
        versionId: 'version-1',
        resources: const {},
        declaredCapabilities: const {'storage'},
        storage: DatabaseRuntimeStorage(database, 'state-1'),
      );
      final restored = await _rpc(server, session, 'storage.get', {
        'key': 'draft',
      });

      expect(restored['result'], {
        'value': {'text': '离线内容'},
      });
    },
  );
}

Future<Map<String, Object?>> _rpc(
  LocalRuntimeServer server,
  RuntimeSession session,
  String method,
  Map<String, Object?> params,
) async {
  final client = HttpClient();
  try {
    final request = await client.post(
      InternetAddress.loopbackIPv4.address,
      server.port,
      '/v1/rpc',
    );
    request.headers
      ..set(HttpHeaders.hostHeader, session.authority)
      ..set('Origin', session.origin)
      ..set(HttpHeaders.authorizationHeader, 'Bearer ${session.token}')
      ..set(HttpHeaders.contentTypeHeader, 'application/json')
      ..set('X-AgentCard-RPC-Version', '1');
    request.add(
      utf8.encode(
        jsonEncode({
          'jsonrpc': '2.0',
          'id': 1,
          'method': method,
          'params': params,
        }),
      ),
    );
    final response = await request.close();
    return jsonDecode(await utf8.decoder.bind(response).join())
        as Map<String, Object?>;
  } finally {
    client.close(force: true);
  }
}
