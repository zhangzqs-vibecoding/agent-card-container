import 'dart:convert';
import 'dart:io';

import 'package:agent_card_desktop/src/cloud/cloud_connection_settings.dart';
import 'package:agent_card_desktop/src/cloud/cloud_settings_repository.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  late Directory root;
  late File file;
  late CloudSettingsRepository repository;

  setUp(() {
    root = Directory.systemTemp.createTempSync('agent-card-cloud-settings-');
    file = File('${root.path}${Platform.pathSeparator}cloud-config.json');
    repository = CloudSettingsRepository(file);
  });

  tearDown(() => root.deleteSync(recursive: true));

  test('accepts HTTPS and round trips trusted keys without secrets', () async {
    const config = CloudUserConfig(
      baseUrl: 'https://agent-card.example.com',
      allowInsecureLoopback: false,
      trustedKeys: {
        'release-key': 'AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA',
      },
    );

    await repository.save(config);

    expect(await repository.read(), config);
    final persisted = file.readAsStringSync();
    expect(persisted, isNot(contains('accessToken')));
    expect(persisted, isNot(contains('secret-token')));
    expect(jsonDecode(persisted), {
      'schemaVersion': 1,
      'baseUrl': 'https://agent-card.example.com',
      'allowInsecureLoopback': false,
      'trustedKeys': {
        'release-key': 'AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA',
      },
    });
  });

  test('allows HTTP only for loopback when explicitly enabled', () {
    expect(
      () => CloudUserConfig(
        baseUrl: 'http://127.0.0.1:8080',
        allowInsecureLoopback: false,
      ).validate(),
      throwsArgumentError,
    );
    expect(
      () => CloudUserConfig(
        baseUrl: 'http://192.168.1.10:8080',
        allowInsecureLoopback: true,
      ).validate(),
      throwsArgumentError,
    );
    expect(
      CloudUserConfig(
        baseUrl: 'http://localhost:8080',
        allowInsecureLoopback: true,
      ).validate,
      returnsNormally,
    );
  });

  test('rejects malformed trusted key objects', () async {
    file.writeAsStringSync(
      jsonEncode({
        'schemaVersion': 1,
        'baseUrl': 'https://agent-card.example.com',
        'allowInsecureLoopback': false,
        'trustedKeys': {'': 'key'},
      }),
    );

    expect(repository.read, throwsFormatException);
  });

  test('rejects corrupt or oversized settings files', () async {
    file.writeAsStringSync('{broken');
    expect(repository.read, throwsFormatException);

    file.writeAsBytesSync(List<int>.filled(70 * 1024, 65));
    expect(repository.read, throwsFormatException);
  });

  test('invalid replacement preserves the previous settings', () async {
    const original = CloudUserConfig(
      baseUrl: 'https://old.agent-card.example.com',
    );
    await repository.save(original);

    await expectLater(
      repository.save(
        const CloudUserConfig(baseUrl: 'http://remote.example.com'),
      ),
      throwsArgumentError,
    );

    expect(await repository.read(), original);
  });

  test('clear removes the user settings file', () async {
    await repository.save(
      const CloudUserConfig(baseUrl: 'https://agent-card.example.com'),
    );

    await repository.clear();

    expect(await repository.read(), isNull);
  });
}
