import 'dart:convert';
import 'dart:io';

import 'cloud_connection_settings.dart';

abstract interface class CloudSettingsStore {
  Future<CloudUserConfig?> read();

  Future<void> save(CloudUserConfig config);

  Future<void> clear();
}

class CloudSettingsRepository implements CloudSettingsStore {
  CloudSettingsRepository(this.file);

  static const maximumBytes = 64 * 1024;

  final File file;

  @override
  Future<CloudUserConfig?> read() async {
    if (!await file.exists()) return null;
    if (await file.length() > maximumBytes) {
      throw const FormatException('云端配置文件过大');
    }
    try {
      final decoded = jsonDecode(await file.readAsString());
      if (decoded is! Map<String, Object?>) {
        throw const FormatException('云端配置必须是 JSON 对象');
      }
      return CloudUserConfig.fromJson(decoded);
    } on FormatException {
      rethrow;
    } catch (_) {
      throw const FormatException('无法读取云端配置');
    }
  }

  @override
  Future<void> save(CloudUserConfig config) async {
    config.validate();
    await file.parent.create(recursive: true);
    final temporary = File('${file.path}.tmp');
    final backup = File('${file.path}.bak');
    await temporary.writeAsString(jsonEncode(config.toJson()), flush: true);
    if (await backup.exists()) await backup.delete();
    final hadPrevious = await file.exists();
    if (hadPrevious) await file.rename(backup.path);
    try {
      await temporary.rename(file.path);
      if (await backup.exists()) await backup.delete();
    } catch (_) {
      if (await file.exists()) await file.delete();
      if (hadPrevious && await backup.exists()) await backup.rename(file.path);
      rethrow;
    } finally {
      if (await temporary.exists()) await temporary.delete();
    }
  }

  @override
  Future<void> clear() async {
    if (await file.exists()) await file.delete();
  }
}
