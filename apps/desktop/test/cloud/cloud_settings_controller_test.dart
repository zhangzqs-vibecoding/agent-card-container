import 'dart:io';

import 'package:agent_card_desktop/src/cloud/cloud_connection_settings.dart';
import 'package:agent_card_desktop/src/cloud/cloud_settings_controller.dart';
import 'package:agent_card_desktop/src/cloud/cloud_settings_repository.dart';
import 'package:agent_card_desktop/src/cloud/cloud_settings_service.dart';
import 'package:agent_card_desktop/src/cloud/secret_store.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  late Directory root;
  late CloudSettingsRepository repository;
  late _MemorySecrets secrets;

  setUp(() {
    root = Directory.systemTemp.createTempSync('agent-card-cloud-controller-');
    repository = CloudSettingsRepository(
      File('${root.path}/cloud-config.json'),
    );
    secrets = _MemorySecrets();
  });

  tearDown(() => root.deleteSync(recursive: true));

  test('loads saved address without exposing the stored token', () async {
    await repository.save(
      const CloudUserConfig(baseUrl: 'https://saved.example.com'),
    );
    secrets.value = 'never-render-this-token';
    final controller = CloudSettingsController(
      CloudSettingsService(
        environment: const {},
        repository: repository,
        secretStore: secrets,
        connectionTester: (_, _) async {},
      ),
    );

    await controller.initialize();

    expect(controller.baseUrl, 'https://saved.example.com');
    expect(controller.accessToken, isEmpty);
    expect(controller.hasSavedCredential, isTrue);
    expect(controller.status, CloudSettingsStatus.ready);
  });

  test('environment-managed configuration is read only', () async {
    final controller = CloudSettingsController(
      CloudSettingsService(
        environment: const {
          'AGENTCARD_CLOUD_URL': 'https://managed.example.com',
          'AGENTCARD_ACCESS_TOKEN': 'managed-secret',
        },
        repository: repository,
        secretStore: secrets,
        connectionTester: (_, _) async {},
      ),
    );

    await controller.initialize();

    expect(controller.environmentManaged, isTrue);
    expect(controller.baseUrl, 'https://managed.example.com');
    expect(controller.accessToken, isEmpty);
    expect(controller.canEdit, isFalse);
  });

  test('tests and saves a valid draft then requests restart', () async {
    var tests = 0;
    final controller = CloudSettingsController(
      CloudSettingsService(
        environment: const {},
        repository: repository,
        secretStore: secrets,
        connectionTester: (config, token) async {
          tests++;
          expect(config.baseUrl, 'http://127.0.0.1:8080');
          expect(token, 'new-secret');
        },
      ),
    );
    await controller.initialize();
    controller
      ..baseUrl = 'http://127.0.0.1:8080'
      ..accessToken = 'new-secret'
      ..allowInsecureLoopback = true;

    await controller.testConnection();
    expect(controller.status, CloudSettingsStatus.connectionValid);

    await controller.save();

    expect(tests, 2);
    expect(controller.status, CloudSettingsStatus.savedPendingRestart);
    expect(secrets.value, 'new-secret');
  });

  test(
    'invalid trusted key JSON is reported without calling service',
    () async {
      var tested = false;
      final controller = CloudSettingsController(
        CloudSettingsService(
          environment: const {},
          repository: repository,
          secretStore: secrets,
          connectionTester: (_, _) async => tested = true,
        ),
      );
      await controller.initialize();
      controller
        ..baseUrl = 'https://saved.example.com'
        ..accessToken = 'new-secret'
        ..trustedKeysJson = '{broken';

      await controller.testConnection();

      expect(tested, isFalse);
      expect(controller.status, CloudSettingsStatus.error);
      expect(controller.message, contains('可信公钥'));
    },
  );

  test('clear reports partial failure accurately', () async {
    await repository.save(
      const CloudUserConfig(baseUrl: 'https://saved.example.com'),
    );
    secrets
      ..value = 'saved-secret'
      ..deleteFails = true;
    final controller = CloudSettingsController(
      CloudSettingsService(
        environment: const {},
        repository: repository,
        secretStore: secrets,
        connectionTester: (_, _) async {},
      ),
    );
    await controller.initialize();

    await controller.clear();

    expect(controller.status, CloudSettingsStatus.error);
    expect(controller.message, contains('部分'));
    expect(await repository.read(), isNull);
  });
}

class _MemorySecrets implements SecretStore {
  String? value;
  bool deleteFails = false;

  @override
  Future<String?> readAccessToken() async => value;

  @override
  Future<void> writeAccessToken(String value) async {
    this.value = value;
  }

  @override
  Future<void> deleteAccessToken() async {
    if (deleteFails) throw StateError('locked');
    value = null;
  }
}
