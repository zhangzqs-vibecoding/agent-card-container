abstract interface class SecretStore {
  Future<String?> readAccessToken();

  Future<void> writeAccessToken(String value);

  Future<void> deleteAccessToken();
}
