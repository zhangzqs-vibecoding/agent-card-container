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
  });
}
