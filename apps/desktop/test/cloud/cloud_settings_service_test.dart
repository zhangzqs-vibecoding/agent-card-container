import 'dart:io';

import 'package:agent_card_desktop/src/cloud/cloud_connection_settings.dart';
import 'package:agent_card_desktop/src/cloud/cloud_settings_repository.dart';
import 'package:agent_card_desktop/src/cloud/cloud_settings_service.dart';
import 'package:agent_card_desktop/src/cloud/secret_store.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  late Directory root;
  late CloudSettingsRepository repository;
  late MemorySecretStore secrets;

  setUp(() {
    root = Directory.systemTemp.createTempSync('agent-card-cloud-service-');
    repository = CloudSettingsRepository(
      File('${root.path}/cloud-config.json'),
    );
    secrets = MemorySecretStore();
  });

  tearDown(() => root.deleteSync(recursive: true));

  test('complete environment configuration overrides user settings', () async {
    await repository.save(
      const CloudUserConfig(baseUrl: 'https://user.example.com'),
    );
    secrets.value = 'user-secret';
    final service = CloudSettingsService(
      environment: const {
        'AGENTCARD_CLOUD_URL': 'https://environment.example.com',
        'AGENTCARD_ACCESS_TOKEN': 'environment-secret',
      },
      repository: repository,
      secretStore: secrets,
    );

    final result = await service.load();

    expect(result.settings?.config.baseUrl, 'https://environment.example.com');
    expect(result.settings?.accessToken, 'environment-secret');
    expect(result.settings?.source, CloudSettingsSource.environment);
    expect(secrets.readCount, 0);
    expect(result.environmentManaged, isTrue);
  });

  test(
    'partial environment configuration never mixes with user data',
    () async {
      await repository.save(
        const CloudUserConfig(baseUrl: 'https://user.example.com'),
      );
      secrets.value = 'user-secret';
      final service = CloudSettingsService(
        environment: const {
          'AGENTCARD_CLOUD_URL': 'https://managed.example.com',
        },
        repository: repository,
        secretStore: secrets,
      );

      final result = await service.load();

      expect(result.settings, isNull);
      expect(result.environmentManaged, isTrue);
      expect(result.issueCode, 'INCOMPLETE_ENVIRONMENT_CONFIGURATION');
      expect(secrets.readCount, 0);
    },
  );

  test('loads user configuration only with a secure token', () async {
    await repository.save(
      const CloudUserConfig(baseUrl: 'https://user.example.com'),
    );
    final service = CloudSettingsService(
      environment: const {},
      repository: repository,
      secretStore: secrets,
    );

    expect((await service.load()).settings, isNull);

    secrets.value = 'stored-secret';
    final loaded = await service.load();
    expect(loaded.settings?.accessToken, 'stored-secret');
    expect(loaded.settings?.source, CloudSettingsSource.user);
    expect(
      loaded.settings?.credentialPersistence,
      CredentialPersistence.secure,
    );
  });

  test('locked credential store returns a redacted offline state', () async {
    await repository.save(
      const CloudUserConfig(baseUrl: 'https://user.example.com'),
    );
    secrets.readError = StateError('vault failed for super-secret-token');
    final service = CloudSettingsService(
      environment: const {},
      repository: repository,
      secretStore: secrets,
    );

    final result = await service.load();

    expect(result.settings, isNull);
    expect(result.issueCode, 'SECURE_STORE_UNAVAILABLE');
    expect(result.issueMessage, isNot(contains('super-secret-token')));
  });
}

class MemorySecretStore implements SecretStore {
  String? value;
  Object? readError;
  int readCount = 0;

  @override
  Future<String?> readAccessToken() async {
    readCount++;
    if (readError case final error?) throw error;
    return value;
  }

  @override
  Future<void> writeAccessToken(String value) async {
    this.value = value;
  }

  @override
  Future<void> deleteAccessToken() async {
    value = null;
  }
}
