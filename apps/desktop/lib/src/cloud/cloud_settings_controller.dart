import 'dart:convert';

import 'package:flutter/foundation.dart';

import 'cloud_connection_settings.dart';
import 'cloud_settings_service.dart';

enum CloudSettingsStatus {
  loading,
  ready,
  testing,
  connectionValid,
  saving,
  savedPendingRestart,
  clearing,
  error,
}

class CloudSettingsController extends ChangeNotifier {
  CloudSettingsController(this.service);

  final CloudSettingsService service;

  CloudSettingsStatus status = CloudSettingsStatus.loading;
  String? message;
  bool environmentManaged = false;
  bool hasSavedCredential = false;
  String _baseUrl = '';
  String _accessToken = '';
  String _storedToken = '';
  bool _allowInsecureLoopback = false;
  String _trustedKeysJson = '{}';

  bool get canEdit => !environmentManaged && !_busy;
  bool get _busy =>
      status == CloudSettingsStatus.loading ||
      status == CloudSettingsStatus.testing ||
      status == CloudSettingsStatus.saving ||
      status == CloudSettingsStatus.clearing;

  String get baseUrl => _baseUrl;
  set baseUrl(String value) => _update(() => _baseUrl = value);

  String get accessToken => _accessToken;
  set accessToken(String value) => _update(() => _accessToken = value);

  bool get allowInsecureLoopback => _allowInsecureLoopback;
  set allowInsecureLoopback(bool value) =>
      _update(() => _allowInsecureLoopback = value);

  String get trustedKeysJson => _trustedKeysJson;
  set trustedKeysJson(String value) => _update(() => _trustedKeysJson = value);

  Future<void> initialize() async {
    status = CloudSettingsStatus.loading;
    notifyListeners();
    final result = await service.load();
    environmentManaged = result.environmentManaged;
    final settings = result.settings;
    if (settings != null) {
      _baseUrl = settings.config.baseUrl;
      _allowInsecureLoopback = settings.config.allowInsecureLoopback;
      _trustedKeysJson = const JsonEncoder.withIndent(
        '  ',
      ).convert(settings.config.trustedKeys);
      _storedToken = settings.accessToken;
      hasSavedCredential = settings.source == CloudSettingsSource.user;
    }
    status = result.issueCode == null
        ? CloudSettingsStatus.ready
        : CloudSettingsStatus.error;
    message = result.issueMessage;
    notifyListeners();
  }

  Future<void> testConnection() async {
    await _perform(CloudSettingsStatus.testing, () async {
      final draft = _draft();
      await service.test(config: draft, accessToken: _effectiveToken());
      status = CloudSettingsStatus.connectionValid;
      message = '连接正常，认证有效';
    });
  }

  Future<void> save() async {
    await _perform(CloudSettingsStatus.saving, () async {
      final token = _effectiveToken();
      await service.save(config: _draft(), accessToken: token);
      _storedToken = token;
      _accessToken = '';
      hasSavedCredential = true;
      status = CloudSettingsStatus.savedPendingRestart;
      message = '配置已保存，重启应用后生效';
    });
  }

  Future<void> clear() async {
    await _perform(CloudSettingsStatus.clearing, () async {
      final result = await service.clear();
      if (!result.credentialCleared || !result.configurationCleared) {
        status = CloudSettingsStatus.error;
        message = '配置仅完成部分清除，请检查系统安全凭据库后重试';
        return;
      }
      _baseUrl = '';
      _accessToken = '';
      _storedToken = '';
      _allowInsecureLoopback = false;
      _trustedKeysJson = '{}';
      hasSavedCredential = false;
      status = CloudSettingsStatus.savedPendingRestart;
      message = '云端配置已清除，重启应用后生效';
    });
  }

  CloudUserConfig _draft() {
    try {
      final decoded = jsonDecode(
        _trustedKeysJson.trim().isEmpty ? '{}' : _trustedKeysJson,
      );
      if (decoded is! Map) throw const FormatException();
      final keys = <String, String>{};
      for (final entry in decoded.entries) {
        if (entry.key is! String || entry.value is! String) {
          throw const FormatException();
        }
        keys[entry.key as String] = entry.value as String;
      }
      final config = CloudUserConfig(
        baseUrl: _baseUrl,
        allowInsecureLoopback: _allowInsecureLoopback,
        trustedKeys: Map.unmodifiable(keys),
      );
      config.validate();
      return config;
    } catch (_) {
      throw const FormatException('可信公钥 JSON 或云端地址无效');
    }
  }

  String _effectiveToken() {
    final token = _accessToken.trim().isEmpty ? _storedToken : _accessToken;
    if (token.trim().isEmpty) throw const FormatException('访问令牌不能为空');
    return token;
  }

  Future<void> _perform(
    CloudSettingsStatus busyStatus,
    Future<void> Function() operation,
  ) async {
    if (environmentManaged || _busy) return;
    status = busyStatus;
    message = null;
    notifyListeners();
    try {
      await operation();
    } catch (_) {
      status = CloudSettingsStatus.error;
      message = busyStatus == CloudSettingsStatus.testing
          ? '连接失败，请检查地址、令牌和可信公钥配置'
          : '操作失败，原有配置未改变';
    }
    notifyListeners();
  }

  void _update(VoidCallback update) {
    if (!canEdit) return;
    update();
    if (status != CloudSettingsStatus.ready) {
      status = CloudSettingsStatus.ready;
      message = null;
    }
    notifyListeners();
  }
}
