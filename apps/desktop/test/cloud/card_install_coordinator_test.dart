import 'dart:convert';
import 'dart:io';

import 'package:agent_card_desktop/src/artifacts/artifact_crypto.dart';
import 'package:agent_card_desktop/src/artifacts/artifact_installer.dart';
import 'package:agent_card_desktop/src/cloud/card_install_coordinator.dart';
import 'package:agent_card_desktop/src/cloud/cloud_api_client.dart';
import 'package:agent_card_desktop/src/storage/local_database.dart';
import 'package:flutter_test/flutter_test.dart';

import '../support/signed_artifact_fixture.dart';

void main() {
  test('downloads, verifies and installs a ready NativeCard', () async {
    final artifact = buildSignedNativeArtifact();
    final server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
    addTearDown(() => server.close(force: true));
    server.listen((request) async {
      if (request.uri.path == '/v1/cards') {
        request.response
          ..headers.contentType = ContentType.json
          ..write(
            jsonEncode({
              'cards': [
                {
                  'cardId': 'card_pomodoro',
                  'title': '番茄钟',
                  'description': '离线',
                  'latestVersion': {
                    'versionId': 'ver_pomodoro_2',
                    'cardId': 'card_pomodoro',
                    'runtime': 'native',
                    'displayVersion': '1.0.0',
                    'title': '番茄钟',
                    'description': '离线',
                    'artifactSha256': List.filled(64, 'f').join(),
                    'keyId': artifact.keyId,
                    'preview': {},
                    'createdAt': '2026-07-12T13:00:00Z',
                  },
                },
              ],
            }),
          );
      } else if (request.uri.path == '/v1/cards/card_pomodoro') {
        request.response
          ..headers.contentType = ContentType.json
          ..write(
            jsonEncode({
              'cardId': 'card_pomodoro',
              'title': '番茄钟',
              'description': '离线',
              'versions': [
                {
                  'versionId': 'ver_pomodoro_1',
                  'cardId': 'card_pomodoro',
                  'runtime': 'native',
                  'displayVersion': '1.0.0',
                  'title': '番茄钟',
                  'description': '离线',
                  'artifactSha256': artifact.sha256,
                  'keyId': artifact.keyId,
                  'preview': {},
                  'createdAt': '2026-07-12T13:00:00Z',
                },
              ],
            }),
          );
      } else if (request.uri.path.endsWith('/artifact')) {
        request.response
          ..headers.contentType = ContentType.json
          ..write(
            jsonEncode({
              'url': 'http://127.0.0.1:${server.port}/download',
              'sha256': artifact.sha256,
              'keyId': artifact.keyId,
              'expiresAt': '2026-07-12T14:00:00Z',
            }),
          );
      } else if (request.uri.path == '/download') {
        request.response.add(artifact.bytes);
      } else {
        request.response.statusCode = HttpStatus.notFound;
      }
      await request.response.close();
    });
    final root = Directory.systemTemp.createTempSync('agent-card-install-');
    addTearDown(() => root.deleteSync(recursive: true));
    final database = LocalDatabase.open('${root.path}/state.sqlite3');
    addTearDown(database.close);
    final client = CloudApiClient(
      baseUri: Uri.parse('http://127.0.0.1:${server.port}'),
      tokenProvider: () async => 'token',
      allowInsecureForDevelopment: true,
    );
    addTearDown(client.close);
    final coordinator = CardInstallCoordinator(
      client: client,
      installer: ArtifactInstaller(
        root: root,
        crypto: ArtifactCrypto.native(),
        trustedKeys: {artifact.keyId: artifact.publicKey},
      ),
      database: database,
      newInstanceId: () => 'instance-1',
      newStateNamespace: () => 'state-1',
      now: () => DateTime.utc(2026, 7, 12),
    );

    final installed = await coordinator.installCardVersion(
      'card_pomodoro',
      'ver_pomodoro_1',
    );

    expect(installed.workspaceCard, isNotNull);
    expect(installed.workspaceCard?.spec.initialState['title'], '专注时间');
    expect(database.listInstances().single.instanceId, 'instance-1');
    expect(database.installation('ver_pomodoro_1')?.keyId, artifact.keyId);
    expect(
      database
          .grantsForInstance('instance-1')
          .map((grant) => grant.capability)
          .toSet(),
      containsAll({'storage', 'window.manageSelf'}),
    );
  });
}
