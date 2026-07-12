import 'dart:convert';
import 'dart:io';

enum CloudSettingsSource { environment, user, none }

enum CredentialPersistence { secure, sessionOnly, none }

class CloudUserConfig {
  const CloudUserConfig({
    required this.baseUrl,
    this.allowInsecureLoopback = false,
    this.trustedKeys = const {},
  });

  final String baseUrl;
  final bool allowInsecureLoopback;
  final Map<String, String> trustedKeys;

  Uri validate() {
    final uri = Uri.tryParse(baseUrl.trim());
    if (uri == null ||
        !uri.hasAuthority ||
        uri.userInfo.isNotEmpty ||
        uri.query.isNotEmpty ||
        uri.fragment.isNotEmpty ||
        (uri.path.isNotEmpty && uri.path != '/')) {
      throw ArgumentError.value(baseUrl, 'baseUrl', '云端地址格式无效');
    }
    final secure = uri.scheme == 'https';
    final insecureLoopback =
        allowInsecureLoopback && uri.scheme == 'http' && _isLoopback(uri.host);
    if (!secure && !insecureLoopback) {
      throw ArgumentError.value(
        baseUrl,
        'baseUrl',
        '必须使用 HTTPS；本地开发只允许显式启用的 loopback HTTP',
      );
    }
    for (final entry in trustedKeys.entries) {
      if (entry.key.trim().isEmpty || !_isEd25519PublicKey(entry.value)) {
        throw const FormatException('可信公钥必须是 keyId 到 Ed25519 公钥的映射');
      }
    }
    return uri;
  }

  Map<String, Object?> toJson() => {
    'schemaVersion': 1,
    'baseUrl': baseUrl.trim(),
    'allowInsecureLoopback': allowInsecureLoopback,
    'trustedKeys': Map<String, String>.from(trustedKeys),
  };

  factory CloudUserConfig.fromJson(Map<String, Object?> json) {
    if (json['schemaVersion'] != 1 ||
        json['baseUrl'] is! String ||
        json['allowInsecureLoopback'] is! bool ||
        json['trustedKeys'] is! Map) {
      throw const FormatException('云端配置格式无效');
    }
    final rawKeys = json['trustedKeys']! as Map;
    final keys = <String, String>{};
    for (final entry in rawKeys.entries) {
      if (entry.key is! String || entry.value is! String) {
        throw const FormatException('可信公钥格式无效');
      }
      keys[entry.key as String] = entry.value as String;
    }
    final config = CloudUserConfig(
      baseUrl: json['baseUrl']! as String,
      allowInsecureLoopback: json['allowInsecureLoopback']! as bool,
      trustedKeys: Map.unmodifiable(keys),
    );
    config.validate();
    return config;
  }

  @override
  bool operator ==(Object other) =>
      other is CloudUserConfig &&
      baseUrl == other.baseUrl &&
      allowInsecureLoopback == other.allowInsecureLoopback &&
      _mapsEqual(trustedKeys, other.trustedKeys);

  @override
  int get hashCode => Object.hash(
    baseUrl,
    allowInsecureLoopback,
    Object.hashAll(
      trustedKeys.entries.map((entry) => Object.hash(entry.key, entry.value)),
    ),
  );
}

class CloudConnectionSettings {
  const CloudConnectionSettings({
    required this.config,
    required this.accessToken,
    required this.source,
    required this.credentialPersistence,
  });

  final CloudUserConfig config;
  final String accessToken;
  final CloudSettingsSource source;
  final CredentialPersistence credentialPersistence;
}

bool _isLoopback(String host) {
  if (host == 'localhost' || host == '::1') return true;
  final address = InternetAddress.tryParse(host);
  return address?.isLoopback ?? false;
}

bool _isEd25519PublicKey(String encoded) {
  try {
    return base64Url.decode(base64Url.normalize(encoded)).length == 32;
  } on FormatException {
    return false;
  }
}

bool _mapsEqual(Map<String, String> left, Map<String, String> right) {
  if (left.length != right.length) return false;
  return left.entries.every((entry) => right[entry.key] == entry.value);
}
