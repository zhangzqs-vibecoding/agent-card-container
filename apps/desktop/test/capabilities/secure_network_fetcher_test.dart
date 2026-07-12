import 'dart:io';

import 'package:agent_card_desktop/src/capabilities/capability.dart';
import 'package:agent_card_desktop/src/capabilities/secure_network_fetcher.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test(
    'rejects DNS names resolving to private or loopback addresses',
    () async {
      final fetcher = SecureNetworkFetcher(
        resolver: (_) async => [InternetAddress.loopbackIPv4],
      );
      addTearDown(fetcher.close);

      await expectLater(
        fetcher.handle(_context, {'url': 'https://api.example.test/data'}),
        throwsA(
          isA<CapabilityException>().having(
            (error) => error.code,
            'code',
            CapabilityErrorCode.permissionDenied,
          ),
        ),
      );
    },
  );

  test(
    'connects only to the validated address and returns a bounded response',
    () async {
      final server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
      addTearDown(() => server.close(force: true));
      server.listen((request) async {
        expect(request.headers.host, 'api.example.test');
        request.response
          ..headers.set('x-secret', 'hidden')
          ..headers.contentType = ContentType.json
          ..write('{"ok":true}');
        await request.response.close();
      });
      final fetcher = SecureNetworkFetcher(
        resolver: (_) async => [InternetAddress('93.184.216.34')],
        connector: (_, _) =>
            Socket.startConnect(InternetAddress.loopbackIPv4, server.port),
      );
      addTearDown(fetcher.close);

      final result = await fetcher.handle(_context, {
        'url': 'http://api.example.test/data',
      });

      expect(result, {
        'status': 200,
        'contentType': 'application/json; charset=utf-8',
        'body': '{"ok":true}',
      });
    },
  );

  test('rejects redirects and responses exceeding the byte budget', () async {
    final server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
    addTearDown(() => server.close(force: true));
    server.listen((request) async {
      if (request.uri.path == '/redirect') {
        request.response
          ..statusCode = HttpStatus.found
          ..headers.set(HttpHeaders.locationHeader, 'http://127.0.0.1/private');
      } else {
        request.response.add(List.filled(33, 65));
      }
      await request.response.close();
    });
    final fetcher = SecureNetworkFetcher(
      resolver: (_) async => [InternetAddress('93.184.216.34')],
      connector: (_, _) =>
          Socket.startConnect(InternetAddress.loopbackIPv4, server.port),
      maxResponseBytes: 32,
    );
    addTearDown(fetcher.close);

    await expectLater(
      fetcher.handle(_context, {'url': 'http://api.example.test/redirect'}),
      throwsA(isA<CapabilityException>()),
    );
    await expectLater(
      fetcher.handle(_context, {'url': 'http://api.example.test/large'}),
      throwsA(isA<CapabilityException>()),
    );
  });
}

final _context = CardContext(
  instanceId: 'instance-1',
  cardId: 'card-1',
  versionId: 'version-1',
  declaredCapabilities: const {'network.fetch'},
  networkDomains: const {'api.example.test'},
);
