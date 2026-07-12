import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:agent_card_desktop/src/artifacts/artifact_installer.dart';
import 'package:agent_card_desktop/src/cards/card_instance.dart';
import 'package:agent_card_desktop/src/capabilities/capability.dart';
import 'package:agent_card_desktop/src/capabilities/capability_broker.dart';
import 'package:agent_card_desktop/src/contracts/card_definition.dart';
import 'package:agent_card_desktop/src/runtime/local_runtime_server.dart';
import 'package:agent_card_desktop/src/storage/local_database.dart';
import 'package:agent_card_desktop/src/surfaces/surface.dart';
import 'package:agent_card_desktop/src/workspace/installed_workspace_card_factory.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test('creates an isolated local runtime session for a CodeCard', () async {
    final root = Directory.systemTemp.createTempSync('code-workspace-card-');
    addTearDown(() => root.deleteSync(recursive: true));
    final database = LocalDatabase.open('${root.path}/state.sqlite3');
    addTearDown(database.close);
    final server = await LocalRuntimeServer.start();
    addTearDown(server.close);
    final definition = CardDefinition.fromJson(
      jsonDecode(
            File(
              '../../contracts/card/fixtures/web-card.json',
            ).readAsStringSync(),
          )
          as Map<String, Object?>,
    );
    File('${root.path}/artifact/payload/web/index.html')
      ..createSync(recursive: true)
      ..writeAsStringSync(
        '<script src="/runtime/bootstrap.js"></script><h1>Offline</h1>',
      );
    final artifact = InstalledArtifact(
      definition: definition,
      contentHash: 'content-hash',
      directory: Directory('${root.path}/artifact'),
      keyId: 'key-1',
    );
    const instance = CardInstance(
      instanceId: 'instance-1',
      cardId: 'card_local_canvas',
      versionId: 'ver_local_canvas_1',
      surfaceId: 'workspace-main',
      placement: CardPlacement(x: 0, y: 0, width: 4, height: 3),
      stateNamespace: 'state-1',
      status: CardInstanceStatus.active,
    );
    database.registerInstalledInstance(
      installation: StoredInstallation(
        installation: CardInstallation(
          cardId: definition.cardId,
          versionId: definition.versionId,
          contentHash: 'content-hash',
          runtime: definition.runtime,
          installedAt: DateTime.utc(2026, 7, 12),
          verified: true,
        ),
        definition: definition,
        keyId: 'key-1',
      ),
      surface: const CardSurface(
        id: 'workspace-main',
        type: SurfaceType.workspace,
      ),
      instance: instance,
    );
    final factory = InstalledWorkspaceCardFactory(
      runtimeServer: server,
      database: database,
      capabilityRuntimeFactory: (instance, definition) {
        return (
          broker: CapabilityBroker(),
          context: CardContext(
            instanceId: instance.instanceId,
            cardId: instance.cardId,
            versionId: instance.versionId,
            declaredCapabilities: definition.capabilities.toSet(),
          ),
        );
      },
    );

    final card = factory.create(artifact, instance);

    expect(card.codeCard, isNotNull);
    expect(card.nativeSpec, isNull);
    expect(card.capabilityBroker, isNotNull);
    expect(card.cardContext?.instanceId, 'instance-1');
    expect(
      card.codeCard?.entrypoint,
      '/bundle/content-hash/payload/web/index.html',
    );
    final session = card.codeCard!.session;
    final response = await _get(session, card.codeCard!.entrypoint);
    expect(response.statusCode, HttpStatus.ok);
    expect(await utf8.decoder.bind(response).join(), contains('Offline'));
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
    await card.codeCard!.onSuspend!();
    expect(await messages.moveNext(), isTrue);
    expect(jsonDecode(messages.current as String)['event'], 'runtime.suspend');
    await card.codeCard!.onResume!();
    expect(await messages.moveNext(), isTrue);
    expect(jsonDecode(messages.current as String)['event'], 'runtime.resume');
    await messages.cancel();
    expect(await card.codeCard!.onLaunchFailure!(), 1);
    await card.codeCard!.onLaunchSuccess!();
    expect(database.launchFailureCount('instance-1'), 0);
    await card.codeCard!.onQuarantine!();
    expect(
      database.listInstances().single.status,
      CardInstanceStatus.quarantined,
    );
  });
}

Future<HttpClientResponse> _get(RuntimeSession session, String path) async {
  final client = HttpClient();
  addTearDown(() => client.close(force: true));
  final request = await client.getUrl(
    Uri.parse('http://127.0.0.1:${session.authority.split(':').last}$path'),
  );
  request.headers.set(HttpHeaders.hostHeader, session.authority);
  return request.close();
}
