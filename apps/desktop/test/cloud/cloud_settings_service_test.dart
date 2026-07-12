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

  test('tests candidate before mutating saved configuration', () async {
    await repository.save(
      const CloudUserConfig(baseUrl: 'https://old.example.com'),
    );
    secrets.value = 'old-secret';
    final service = CloudSettingsService(
      environment: const {},
      repository: repository,
      secretStore: secrets,
      connectionTester: (_, _) async => throw StateError('offline'),
    );

    await expectLater(
      service.save(
        config: const CloudUserConfig(baseUrl: 'https://new.example.com'),
        accessToken: 'new-secret',
      ),
      throwsStateError,
    );

    expect((await repository.read())?.baseUrl, 'https://old.example.com');
    expect(secrets.value, 'old-secret');
  });

  test('secure store failure preserves previous configuration', () async {
    await repository.save(
      const CloudUserConfig(baseUrl: 'https://old.example.com'),
    );
    secrets
      ..value = 'old-secret'
      ..writeError = StateError('vault locked');
    final service = CloudSettingsService(
      environment: const {},
      repository: repository,
      secretStore: secrets,
      connectionTester: (_, _) async {},
    );

    await expectLater(
      service.save(
        config: const CloudUserConfig(baseUrl: 'https://new.example.com'),
        accessToken: 'new-secret',
      ),
      throwsStateError,
    );

    expect((await repository.read())?.baseUrl, 'https://old.example.com');
    expect(secrets.value, 'old-secret');
  });

  test('repository failure restores the previous secure token', () async {
    final store = FailingSettingsStore(
      const CloudUserConfig(baseUrl: 'https://old.example.com'),
    );
    secrets.value = 'old-secret';
    final service = CloudSettingsService(
      environment: const {},
      repository: store,
      secretStore: secrets,
      connectionTester: (_, _) async {},
    );

    await expectLater(
      service.save(
        config: const CloudUserConfig(baseUrl: 'https://new.example.com'),
        accessToken: 'new-secret',
      ),
      throwsA(isA<FileSystemException>()),
    );

    expect(secrets.value, 'old-secret');
  });

  test(
    'clear reports credential and configuration results separately',
    () async {
      final store = FailingSettingsStore(
        const CloudUserConfig(baseUrl: 'https://old.example.com'),
        clearFails: true,
      );
      secrets.value = 'old-secret';
      final service = CloudSettingsService(
        environment: const {},
        repository: store,
        secretStore: secrets,
        connectionTester: (_, _) async {},
      );

      final result = await service.clear();

      expect(result.credentialCleared, isTrue);
      expect(result.configurationCleared, isFalse);
    },
  );
}

class MemorySecretStore implements SecretStore {
  String? value;
  Object? readError;
  Object? writeError;
  int readCount = 0;

  @override
  Future<String?> readAccessToken() async {
    readCount++;
    if (readError case final error?) throw error;
    return value;
  }

  @override
  Future<void> writeAccessToken(String value) async {
    if (writeError case final error?) throw error;
    this.value = value;
  }

  @override
  Future<void> deleteAccessToken() async {
    value = null;
  }
}

class FailingSettingsStore implements CloudSettingsStore {
  FailingSettingsStore(this.value, {this.clearFails = false});

  CloudUserConfig? value;
  final bool clearFails;

  @override
  Future<CloudUserConfig?> read() async => value;

  @override
  Future<void> save(CloudUserConfig config) async {
    throw const FileSystemException('disk unavailable');
  }

  @override
  Future<void> clear() async {
    if (clearFails) throw const FileSystemException('disk unavailable');
    value = null;
  }
}
