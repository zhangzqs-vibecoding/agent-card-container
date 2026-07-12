import 'package:flutter_secure_storage/flutter_secure_storage.dart';

import '../cloud/secret_store.dart';

class SecureTokenStore implements SecretStore {
  SecureTokenStore({FlutterSecureStorage? storage})
    : _storage = storage ?? const FlutterSecureStorage();

  static const _accessTokenKey = 'agent-card.cloud.access-token';

  final FlutterSecureStorage _storage;

  @override
  Future<String?> readAccessToken() => _storage.read(key: _accessTokenKey);

  @override
  Future<void> writeAccessToken(String value) =>
      _storage.write(key: _accessTokenKey, value: value);

  @override
  Future<void> deleteAccessToken() => _storage.delete(key: _accessTokenKey);
}
