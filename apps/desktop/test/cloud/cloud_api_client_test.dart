import 'dart:convert';
import 'dart:io';

import 'package:agent_card_desktop/src/cloud/cloud_api_client.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  group('CloudApiClient', () {
    late HttpServer server;
    late CloudApiClient client;
    late List<HttpRequest> requests;

    setUp(() async {
      requests = [];
      server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
      server.listen((request) async {
        requests.add(request);
        if (request.uri.path == '/artifact.agentcard') {
          expect(
            request.headers.value(HttpHeaders.authorizationHeader),
            isNull,
          );
          request.response
            ..statusCode = HttpStatus.ok
            ..add([1, 2, 3, 4]);
          await request.response.close();
          return;
        }
        if (request.uri.path == '/v1/cards') {
          request.response
            ..headers.contentType = ContentType.json
            ..statusCode = HttpStatus.ok
            ..write(
              jsonEncode({
                'cards': [
                  {
                    'cardId': 'card_01',
                    'title': '番茄钟',
                    'description': '离线',
                    'latestVersion': {
                      'versionId': 'ver_01',
                      'cardId': 'card_01',
                      'runtime': 'native',
                      'displayVersion': '1.0.0',
                      'title': '番茄钟',
                      'description': '离线',
                      'artifactSha256': List.filled(64, 'a').join(),
                      'keyId': 'key-1',
                      'preview': {},
                      'createdAt': '2026-07-12T13:00:00Z',
                    },
                  },
                ],
              }),
            );
          await request.response.close();
          return;
        }
        if (request.uri.path.endsWith('/artifact')) {
          request.response
            ..headers.contentType = ContentType.json
            ..statusCode = HttpStatus.ok
            ..write(
              jsonEncode({
                'url': 'http://127.0.0.1:${server.port}/artifact.agentcard',
                'sha256': List.filled(64, 'a').join(),
                'keyId': 'key-1',
                'expiresAt': '2026-07-12T14:00:00Z',
              }),
            );
          await request.response.close();
          return;
        }
        if (request.uri.path.endsWith('/events')) {
          request.response
            ..statusCode = HttpStatus.ok
            ..headers.contentType = ContentType(
              'text',
              'event-stream',
              charset: 'utf-8',
            );
          request.response.write(
            'id: 2\n'
            'event: status.changed\n'
            'data: {"eventId":2,"sessionId":"gen_01","type":"status.changed","stage":"generating","message":"生成中","progress":0.4,"timestamp":"2026-07-12T13:00:00Z"}\n\n',
          );
          await request.response.close();
          return;
        }
        final body = await utf8.decoder.bind(request).join();
        final decoded = body.isEmpty
            ? const <String, Object?>{}
            : jsonDecode(body) as Map<String, Object?>;
        request.response.headers.contentType = ContentType.json;
        if (request.uri.path == '/v1/generations') {
          expect(decoded['prompt'], '离线番茄钟');
          request.response
            ..statusCode = HttpStatus.created
            ..write(
              jsonEncode({
                'id': 'gen_01',
                'prompt': decoded['prompt'],
                'target': 'auto',
                'locale': 'zh-CN',
                'status': 'awaiting_confirmation',
                'summary': {
                  'goal': '离线番茄钟',
                  'constraints': ['可离线'],
                  'locale': 'zh-CN',
                },
                'messages': [],
                'createdAt': '2026-07-12T13:00:00Z',
                'updatedAt': '2026-07-12T13:00:00Z',
              }),
            );
        } else {
          request.response
            ..statusCode = HttpStatus.ok
            ..write(
              jsonEncode({
                'id': 'gen_01',
                'prompt': '离线番茄钟',
                'target': 'auto',
                'locale': 'zh-CN',
                'status': 'queued',
                'summary': {
                  'goal': '离线番茄钟',
                  'constraints': ['可离线'],
                  'locale': 'zh-CN',
                },
                'messages': [],
                'createdAt': '2026-07-12T13:00:00Z',
                'updatedAt': '2026-07-12T13:00:00Z',
              }),
            );
        }
        await request.response.close();
      });
      client = CloudApiClient(
        baseUri: Uri.parse('http://127.0.0.1:${server.port}'),
        tokenProvider: () async => 'access-token',
        allowInsecureForDevelopment: true,
      );
    });

    tearDown(() async {
      client.close();
      await server.close(force: true);
    });

    test('creates and confirms a generation with bearer auth', () async {
      final created = await client.createGeneration(
        prompt: '离线番茄钟',
        target: GenerationTarget.auto,
      );
      final confirmed = await client.confirmGeneration(created.id);

      expect(created.status, GenerationStatus.awaitingConfirmation);
      expect(created.summary.goal, '离线番茄钟');
      expect(confirmed.status, GenerationStatus.queued);
      expect(
        requests.every(
          (request) =>
              request.headers.value(HttpHeaders.authorizationHeader) ==
              'Bearer access-token',
        ),
        isTrue,
      );
    });

    test('parses one SSE connection and sends Last-Event-ID', () async {
      final events = await client
          .connectGenerationEvents('gen_01', lastEventId: 1)
          .toList();

      expect(events, hasLength(1));
      expect(events.single.eventId, 2);
      expect(events.single.stage, 'generating');
      expect(requests.single.headers.value('Last-Event-ID'), '1');
    });

    test(
      'lists cards and downloads artifact without forwarding bearer',
      () async {
        final cards = await client.listCards();
        final download = await client.artifactDownload('card_01', 'ver_01');
        final bytes = await client.downloadArtifact(download.url);

        expect(cards.single.latestVersion.versionId, 'ver_01');
        expect(download.keyId, 'key-1');
        expect(bytes, [1, 2, 3, 4]);
      },
    );
  });
}
