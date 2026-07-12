import 'package:flutter/material.dart';

import 'cloud_settings_controller.dart';

class CloudSettingsSection extends StatefulWidget {
  const CloudSettingsSection({required this.controller, super.key});

  final CloudSettingsController controller;

  @override
  State<CloudSettingsSection> createState() => _CloudSettingsSectionState();
}

class _CloudSettingsSectionState extends State<CloudSettingsSection> {
  late final TextEditingController _urlController;
  late final TextEditingController _tokenController;
  late final TextEditingController _keysController;
  bool _showToken = false;

  @override
  void initState() {
    super.initState();
    _urlController = TextEditingController(text: widget.controller.baseUrl);
    _tokenController = TextEditingController();
    _keysController = TextEditingController(
      text: widget.controller.trustedKeysJson,
    );
  }

  @override
  void dispose() {
    _urlController.dispose();
    _tokenController.dispose();
    _keysController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: widget.controller,
      builder: (context, _) {
        final controller = widget.controller;
        final colors = Theme.of(context).colorScheme;
        return Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text('云端服务', style: Theme.of(context).textTheme.headlineMedium),
            const SizedBox(height: 8),
            Text(
              controller.environmentManaged
                  ? '当前连接由环境变量管理，界面不会显示或覆盖访问令牌。'
                  : '连接 Go 云端以使用 Agent Studio 和云端卡片库。保存后重启应用生效。',
              style: TextStyle(color: colors.onSurfaceVariant),
            ),
            const SizedBox(height: 18),
            TextField(
              key: const Key('cloud-base-url'),
              controller: _urlController,
              enabled: controller.canEdit,
              decoration: const InputDecoration(
                labelText: '服务地址',
                hintText: 'https://agent-card.example.com',
                border: OutlineInputBorder(),
              ),
              onChanged: (value) => controller.baseUrl = value,
            ),
            const SizedBox(height: 14),
            TextField(
              key: const Key('cloud-access-token'),
              controller: _tokenController,
              enabled: controller.canEdit,
              obscureText: !_showToken,
              autocorrect: false,
              enableSuggestions: false,
              decoration: InputDecoration(
                labelText: controller.hasSavedCredential
                    ? 'Access Token（已安全保存，留空表示不更改）'
                    : 'Access Token',
                border: const OutlineInputBorder(),
                suffixIcon: IconButton(
                  key: const Key('cloud-toggle-token-visibility'),
                  onPressed: controller.canEdit
                      ? () => setState(() => _showToken = !_showToken)
                      : null,
                  icon: Icon(
                    _showToken ? Icons.visibility_off : Icons.visibility,
                  ),
                ),
              ),
              onChanged: (value) => controller.accessToken = value,
            ),
            const SizedBox(height: 8),
            CheckboxListTile(
              key: const Key('cloud-allow-insecure'),
              contentPadding: EdgeInsets.zero,
              value: controller.allowInsecureLoopback,
              onChanged: controller.canEdit
                  ? (value) => controller.allowInsecureLoopback = value ?? false
                  : null,
              title: const Text('允许本机 HTTP 开发服务'),
              subtitle: const Text('仅允许 localhost、127.0.0.0/8 或 ::1'),
              controlAffinity: ListTileControlAffinity.leading,
            ),
            ExpansionTile(
              tilePadding: EdgeInsets.zero,
              title: const Text('高级设置'),
              children: [
                TextField(
                  key: const Key('cloud-trusted-keys'),
                  controller: _keysController,
                  enabled: controller.canEdit,
                  minLines: 3,
                  maxLines: 8,
                  style: const TextStyle(fontFamily: 'monospace'),
                  decoration: const InputDecoration(
                    labelText: '可信 Ed25519 公钥 JSON',
                    border: OutlineInputBorder(),
                  ),
                  onChanged: (value) => controller.trustedKeysJson = value,
                ),
              ],
            ),
            const SizedBox(height: 16),
            Wrap(
              spacing: 10,
              runSpacing: 10,
              children: [
                OutlinedButton.icon(
                  key: const Key('cloud-test-connection'),
                  onPressed: controller.canEdit
                      ? controller.testConnection
                      : null,
                  icon: const Icon(Icons.wifi_tethering),
                  label: const Text('测试连接'),
                ),
                FilledButton.icon(
                  key: const Key('cloud-save'),
                  onPressed: controller.canEdit ? controller.save : null,
                  icon: const Icon(Icons.save_outlined),
                  label: const Text('保存'),
                ),
                TextButton.icon(
                  key: const Key('cloud-clear'),
                  onPressed: controller.canEdit ? controller.clear : null,
                  icon: const Icon(Icons.delete_outline),
                  label: const Text('清除配置'),
                ),
              ],
            ),
            if (controller.message case final message?) ...[
              const SizedBox(height: 12),
              Text(
                message,
                key: const Key('cloud-settings-message'),
                style: TextStyle(
                  color: controller.status == CloudSettingsStatus.error
                      ? colors.error
                      : colors.primary,
                ),
              ),
            ],
          ],
        );
      },
    );
  }
}
