import 'dart:convert';

import 'cloud_connection_settings.dart';
import 'cloud_settings_repository.dart';
import 'secret_store.dart';

class CloudSettingsLoadResult {
  const CloudSettingsLoadResult({
    required this.settings,
    required this.environmentManaged,
    this.issueCode,
    this.issueMessage,
  });

  final CloudConnectionSettings? settings;
  final bool environmentManaged;
  final String? issueCode;
  final String? issueMessage;
}

class CloudSettingsService {
  CloudSettingsService({
    required Map<String, String> environment,
    required this.repository,
    required this.secretStore,
  }) : environment = Map.unmodifiable(environment);

  final Map<String, String> environment;
  final CloudSettingsRepository repository;
  final SecretStore secretStore;

  Future<CloudSettingsLoadResult> load() async {
    final url = _nonEmpty(environment['AGENTCARD_CLOUD_URL']);
    final token = _nonEmpty(environment['AGENTCARD_ACCESS_TOKEN']);
    final environmentManaged = url != null || token != null;
    if (environmentManaged) {
      if (url == null || token == null) {
        return const CloudSettingsLoadResult(
          settings: null,
          environmentManaged: true,
          issueCode: 'INCOMPLETE_ENVIRONMENT_CONFIGURATION',
          issueMessage: '环境变量中的云端地址和访问令牌必须同时配置',
        );
      }
      try {
        final config = CloudUserConfig(
          baseUrl: url,
          allowInsecureLoopback:
              environment['AGENTCARD_ALLOW_INSECURE_CLOUD'] == 'true',
          trustedKeys: _environmentTrustedKeys(
            environment['AGENTCARD_TRUSTED_KEYS_JSON'],
          ),
        );
        config.validate();
        return CloudSettingsLoadResult(
          settings: CloudConnectionSettings(
            config: config,
            accessToken: token,
            source: CloudSettingsSource.environment,
            credentialPersistence: CredentialPersistence.none,
          ),
          environmentManaged: true,
        );
      } catch (_) {
        return const CloudSettingsLoadResult(
          settings: null,
          environmentManaged: true,
          issueCode: 'INVALID_ENVIRONMENT_CONFIGURATION',
          issueMessage: '环境变量中的云端配置无效',
        );
      }
    }

    CloudUserConfig? config;
    try {
      config = await repository.read();
    } catch (_) {
      return const CloudSettingsLoadResult(
        settings: null,
        environmentManaged: false,
        issueCode: 'INVALID_USER_CONFIGURATION',
        issueMessage: '已保存的云端配置无效，请重新配置',
      );
    }
    if (config == null) {
      return const CloudSettingsLoadResult(
        settings: null,
        environmentManaged: false,
      );
    }

    try {
      final storedToken = _nonEmpty(await secretStore.readAccessToken());
      if (storedToken == null) {
        return const CloudSettingsLoadResult(
          settings: null,
          environmentManaged: false,
          issueCode: 'ACCESS_TOKEN_MISSING',
          issueMessage: '安全凭据库中没有云端访问令牌',
        );
      }
      return CloudSettingsLoadResult(
        settings: CloudConnectionSettings(
          config: config,
          accessToken: storedToken,
          source: CloudSettingsSource.user,
          credentialPersistence: CredentialPersistence.secure,
        ),
        environmentManaged: false,
      );
    } catch (_) {
      return const CloudSettingsLoadResult(
        settings: null,
        environmentManaged: false,
        issueCode: 'SECURE_STORE_UNAVAILABLE',
        issueMessage: '系统安全凭据库当前不可用',
      );
    }
  }
}

String? _nonEmpty(String? value) {
  final trimmed = value?.trim();
  return trimmed == null || trimmed.isEmpty ? null : trimmed;
}

Map<String, String> _environmentTrustedKeys(String? encoded) {
  if (_nonEmpty(encoded) == null) return const {};
  final decoded = jsonDecode(encoded!);
  if (decoded is! Map) throw const FormatException('trusted keys must be map');
  final result = <String, String>{};
  for (final entry in decoded.entries) {
    if (entry.key is! String || entry.value is! String) {
      throw const FormatException('trusted keys must contain strings');
    }
    result[entry.key as String] = entry.value as String;
  }
  return Map.unmodifiable(result);
}
