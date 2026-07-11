import '../runtime/runtime_session.dart';
import 'local_database.dart';

class DatabaseRuntimeStorage implements RuntimeStorage {
  const DatabaseRuntimeStorage(this.database, this.namespace);

  final LocalDatabase database;
  final String namespace;

  @override
  bool delete(String key) {
    final existed = database.readState(namespace).containsKey(key);
    database.deleteState(namespace, key);
    return existed;
  }

  @override
  Object? get(String key) => database.readState(namespace)[key];

  @override
  List<String> listKeys() {
    final keys = database.readState(namespace).keys.toList()..sort();
    return keys;
  }

  @override
  void set(String key, Object? value) {
    database.putState(namespace, key, value);
  }

  @override
  Map<String, Object?> snapshot() => database.readState(namespace);
}
