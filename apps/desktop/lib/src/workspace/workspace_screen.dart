import 'dart:async';

import 'package:flutter/material.dart';

import '../adapters/in_app_webview_port.dart';
import '../agent_studio/agent_studio_controller.dart';
import '../cloud/card_catalog_controller.dart';
import '../cloud/card_version_lifecycle.dart';
import '../cloud/cloud_api_client.dart';
import '../cloud/cloud_settings_controller.dart';
import '../cloud/cloud_settings_section.dart';
import '../code_card/code_card_host.dart';
import '../capabilities/capability.dart';
import '../native_card/native_card_controller.dart';
import '../native_card/native_card_renderer.dart';
import '../runtime/runtime_activity_budget.dart';
import '../surfaces/surface.dart';
import 'workspace_card.dart';
import 'workspace_controller.dart';
import 'native_card_state_persistence.dart';

typedef NativeCardStateChanged =
    void Function(String namespace, Map<String, Object?> state);
typedef CardSurfaceAction =
    Future<void> Function(String instanceId, CardPlacement placement);
typedef CardPlacementEdit =
    void Function(String instanceId, CardPlacement placement);

class WorkspaceScreen extends StatefulWidget {
  const WorkspaceScreen({
    super.key,
    this.runtimePort,
    this.workspaceCards = const [],
    this.agentStudioController,
    this.workspaceController,
    this.cardCatalogController,
    this.cloudSettingsController,
    this.onNativeCardStateChanged,
    this.onDetachCard,
    this.onMoveCardToOverlay,
    this.runtimeVisible = true,
    this.onExportDiagnostics,
    this.webViewPortFactory,
  });

  final int? runtimePort;
  final List<WorkspaceCard> workspaceCards;
  final AgentStudioController? agentStudioController;
  final WorkspaceController? workspaceController;
  final CardCatalogController? cardCatalogController;
  final CloudSettingsController? cloudSettingsController;
  final NativeCardStateChanged? onNativeCardStateChanged;
  final CardSurfaceAction? onDetachCard;
  final CardSurfaceAction? onMoveCardToOverlay;
  final bool runtimeVisible;
  final Future<String> Function()? onExportDiagnostics;
  final InAppWebViewPortFactory? webViewPortFactory;

  @override
  State<WorkspaceScreen> createState() => _WorkspaceScreenState();
}

class _WorkspaceScreenState extends State<WorkspaceScreen> {
  bool _agentPanelOpen = true;
  int _selectedDestination = 0;
  final FocusNode _agentPromptFocusNode = FocusNode();

  @override
  void dispose() {
    _agentPromptFocusNode.dispose();
    super.dispose();
  }

  void _openAgentPanel() {
    if (!_agentPanelOpen) {
      setState(() => _agentPanelOpen = true);
    }
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted) _agentPromptFocusNode.requestFocus();
    });
  }

  @override
  Widget build(BuildContext context) {
    final colors = Theme.of(context).colorScheme;
    return Scaffold(
      body: DecoratedBox(
        decoration: const BoxDecoration(
          gradient: LinearGradient(
            begin: Alignment.topLeft,
            end: Alignment.bottomRight,
            colors: [Color(0xFF101515), Color(0xFF080A0B)],
          ),
        ),
        child: SafeArea(
          child: Column(
            children: [
              _TopBar(runtimePort: widget.runtimePort),
              Expanded(
                child: Row(
                  children: [
                    _NavigationStrip(
                      selectedIndex: _selectedDestination,
                      onSelected: (index) {
                        setState(() => _selectedDestination = index);
                        if (index == 1 &&
                            widget.cardCatalogController?.cards.isEmpty ==
                                true) {
                          widget.cardCatalogController?.refresh();
                        }
                      },
                    ),
                    VerticalDivider(width: 1, color: colors.outlineVariant),
                    Expanded(child: _destinationContent()),
                    AnimatedContainer(
                      duration: const Duration(milliseconds: 220),
                      curve: Curves.easeOutCubic,
                      width: _agentPanelOpen ? 368 : 0,
                      clipBehavior: Clip.hardEdge,
                      decoration: const BoxDecoration(),
                      child: _agentPanelOpen
                          ? _AgentPanel(
                              controller: widget.agentStudioController,
                              promptFocusNode: _agentPromptFocusNode,
                              onCollapse: () {
                                setState(() => _agentPanelOpen = false);
                              },
                            )
                          : null,
                    ),
                  ],
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }

  Widget _destinationContent() {
    if (_selectedDestination == 1) {
      return _CardLibrary(
        controller: widget.cardCatalogController,
        onShowVersions: (cardId) async {
          await widget.cardCatalogController?.selectCard(cardId);
          if (mounted) {
            setState(() => _selectedDestination = 2);
          }
        },
      );
    }
    if (_selectedDestination == 2) {
      return _VersionHistory(controller: widget.cardCatalogController);
    }
    if (_selectedDestination == 3) {
      return _SettingsPage(
        cloudSettingsController: widget.cloudSettingsController,
        onExportDiagnostics: widget.onExportDiagnostics,
      );
    }
    return AnimatedBuilder(
      animation: widget.workspaceController ?? _NoopListenable.instance,
      builder: (context, _) => _WorkspaceCanvas(
        webViewPortFactory: widget.webViewPortFactory ?? InAppWebViewPort.new,
        runtimeVisible: widget.runtimeVisible,
        cards:
            widget.workspaceController?.workspaceCards ??
            widget.workspaceCards
                .where((card) => card.instance.surfaceId == 'workspace-main')
                .toList(growable: false),
        agentPanelOpen: _agentPanelOpen,
        onOpenAgentPanel: _openAgentPanel,
        onNativeCardStateChanged: widget.onNativeCardStateChanged,
        onDetachCard: widget.onDetachCard,
        onMoveCardToOverlay: widget.onMoveCardToOverlay,
        onEditPlacement: widget.workspaceController?.editPlacement,
        layoutErrorMessage: widget.workspaceController?.layoutErrorMessage,
        onDismissLayoutError: widget.workspaceController?.clearLayoutError,
      ),
    );
  }
}

class _CardLibrary extends StatelessWidget {
  const _CardLibrary({required this.controller, required this.onShowVersions});

  final CardCatalogController? controller;
  final Future<void> Function(String cardId) onShowVersions;

  @override
  Widget build(BuildContext context) {
    final catalog = controller;
    if (catalog == null) {
      return const _CloudUnavailable();
    }
    return AnimatedBuilder(
      animation: catalog,
      builder: (context, _) => _CatalogPage(
        title: '我的卡片',
        subtitle: '云端生成并签名的卡片',
        loading: catalog.loading,
        errorMessage: catalog.errorMessage,
        onRefresh: catalog.refresh,
        child: catalog.cards.isEmpty
            ? const Center(child: Text('还没有已生成的卡片'))
            : ListView.separated(
                padding: const EdgeInsets.fromLTRB(28, 8, 28, 28),
                itemCount: catalog.cards.length,
                separatorBuilder: (_, _) => const SizedBox(height: 12),
                itemBuilder: (context, index) {
                  final card = catalog.cards[index];
                  return Card(
                    child: ListTile(
                      contentPadding: const EdgeInsets.all(18),
                      leading: const CircleAvatar(
                        child: Icon(Icons.widgets_outlined),
                      ),
                      title: Text(card.title),
                      subtitle: Text(
                        '${card.description}\n最新版本 ${card.latestVersion.displayVersion} · ${card.latestVersion.runtime}',
                      ),
                      isThreeLine: true,
                      trailing: FilledButton.tonal(
                        onPressed: () => onShowVersions(card.cardId),
                        child: const Text('查看版本'),
                      ),
                    ),
                  );
                },
              ),
      ),
    );
  }
}

class _VersionHistory extends StatelessWidget {
  const _VersionHistory({required this.controller});

  final CardCatalogController? controller;

  @override
  Widget build(BuildContext context) {
    final catalog = controller;
    if (catalog == null) {
      return const _CloudUnavailable();
    }
    return AnimatedBuilder(
      animation: catalog,
      builder: (context, _) {
        final card = catalog.selectedCard;
        return _CatalogPage(
          title: '版本历史',
          subtitle: card?.title ?? '请先从卡片库选择一张卡片',
          loading: catalog.loading,
          errorMessage: catalog.errorMessage,
          onRefresh: card == null
              ? catalog.refresh
              : () => catalog.selectCard(card.cardId),
          child: card == null
              ? const Center(child: Text('暂无选中的卡片'))
              : ListView.separated(
                  padding: const EdgeInsets.fromLTRB(28, 8, 28, 28),
                  itemCount: card.versions.length,
                  separatorBuilder: (_, _) => const SizedBox(height: 10),
                  itemBuilder: (context, index) {
                    final version = card.versions[index];
                    final installing =
                        catalog.installingVersionId == version.versionId;
                    final changing =
                        catalog.changingVersionId == version.versionId;
                    final action = catalog.actionFor(version);
                    return Card(
                      child: ListTile(
                        contentPadding: const EdgeInsets.all(18),
                        title: Text(version.displayVersion),
                        subtitle: Text(
                          '${version.runtime} · ${version.createdAt.toLocal()}\n签名密钥 ${version.keyId}',
                        ),
                        isThreeLine: true,
                        trailing: Wrap(
                          spacing: 8,
                          children: [
                            FilledButton.tonal(
                              key: Key('preview-${version.versionId}'),
                              onPressed: () =>
                                  _showVersionPreview(context, version),
                              child: const Text('预览'),
                            ),
                            FilledButton(
                              onPressed: _versionActionEnabled(catalog, action)
                                  ? () => _handleVersionAction(
                                      context,
                                      catalog,
                                      version,
                                      action,
                                    )
                                  : null,
                              child: Text(
                                installing || changing
                                    ? '处理中…'
                                    : _versionActionLabel(action),
                              ),
                            ),
                          ],
                        ),
                      ),
                    );
                  },
                ),
        );
      },
    );
  }
}

bool _versionActionEnabled(
  CardCatalogController catalog,
  CatalogVersionAction action,
) {
  if (catalog.installingVersionId != null ||
      catalog.changingVersionId != null ||
      action == CatalogVersionAction.current) {
    return false;
  }
  return action == CatalogVersionAction.install
      ? catalog.installationAvailable
      : catalog.versionChangeAvailable;
}

String _versionActionLabel(CatalogVersionAction action) {
  return switch (action) {
    CatalogVersionAction.install => '安装此版本',
    CatalogVersionAction.current => '当前版本',
    CatalogVersionAction.upgrade => '升级到此版本',
    CatalogVersionAction.rollback => '回滚到此版本',
  };
}

Future<void> _handleVersionAction(
  BuildContext context,
  CardCatalogController catalog,
  CloudCardVersion version,
  CatalogVersionAction action,
) async {
  if (action == CatalogVersionAction.install) {
    await catalog.install(version.versionId);
    return;
  }
  final proposal = await catalog.prepareChange(version.versionId);
  if (proposal == null || !context.mounted) return;
  final decision = await _showVersionChangeDialog(context, proposal, action);
  if (decision == null) {
    catalog.cancelChange();
    return;
  }
  await catalog.applyChange(
    proposal,
    decision,
    _approvedDifferenceGrants(proposal),
  );
}

Set<PermissionGrant> _approvedDifferenceGrants(
  CatalogVersionChangeProposal proposal,
) {
  final capabilities = proposal.difference.addedCapabilities.toSet();
  if (proposal.difference.addedDomains.isNotEmpty) {
    capabilities.add('network.fetch');
  }
  return {
    for (final capability in capabilities)
      PermissionGrant(
        instanceId: proposal.instance.instanceId,
        versionId: proposal.targetVersion.versionId,
        capability: capability,
        domains: capability == 'network.fetch'
            ? proposal.difference.addedDomains.toSet()
            : const {},
      ),
  };
}

Future<CardVersionDecision?> _showVersionChangeDialog(
  BuildContext context,
  CatalogVersionChangeProposal proposal,
  CatalogVersionAction action,
) {
  final difference = proposal.difference;
  final incompatible = !difference.stateCompatible;
  final confirmDecision = incompatible
      ? action == CatalogVersionAction.rollback
            ? CardVersionDecision.restoreState
            : CardVersionDecision.resetState
      : CardVersionDecision.reuseState;
  final confirmLabel = incompatible
      ? action == CatalogVersionAction.rollback
            ? '回滚并恢复状态'
            : '安装并重置'
      : action == CatalogVersionAction.rollback
      ? '确认回滚'
      : '确认升级';
  return showDialog<CardVersionDecision>(
    context: context,
    barrierDismissible: false,
    builder: (context) => AlertDialog(
      title: const Text('版本能力与状态确认'),
      content: ConstrainedBox(
        constraints: const BoxConstraints(maxWidth: 520),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            _differenceLine('新增能力', difference.addedCapabilities),
            _differenceLine('移除能力', difference.removedCapabilities),
            _differenceLine('新增网络域', difference.addedDomains),
            _differenceLine('移除网络域', difference.removedDomains),
            const SizedBox(height: 12),
            Text(
              '状态 schema ${difference.currentStateSchemaVersion} → '
              '${difference.targetStateSchemaVersion}',
            ),
            const SizedBox(height: 6),
            Text(incompatible ? '状态结构不兼容；继续后会先备份当前状态。' : '状态结构兼容，将继续使用当前本地状态。'),
          ],
        ),
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.pop(context),
          child: Text(incompatible ? '继续使用当前版本' : '取消'),
        ),
        FilledButton(
          onPressed: () => Navigator.pop(context, confirmDecision),
          child: Text(confirmLabel),
        ),
      ],
    ),
  );
}

Widget _differenceLine(String label, List<String> values) {
  return Padding(
    padding: const EdgeInsets.only(bottom: 6),
    child: Text('$label：${values.isEmpty ? '无' : values.join('、')}'),
  );
}

Future<void> _showVersionPreview(
  BuildContext context,
  CloudCardVersion version,
) {
  final preview = version.preview;
  final title = _boundedPreviewText(preview['title'], version.title);
  final runtime = _boundedPreviewText(preview['runtime'], version.runtime);
  final reason = _boundedPreviewText(preview['reason'], version.description);
  return showDialog<void>(
    context: context,
    builder: (context) => AlertDialog(
      title: const Text('版本预览'),
      content: ConstrainedBox(
        constraints: const BoxConstraints(maxWidth: 480),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(title, style: Theme.of(context).textTheme.titleMedium),
            const SizedBox(height: 10),
            Chip(label: Text(runtime)),
            const SizedBox(height: 10),
            Text(reason),
            const SizedBox(height: 18),
            const Divider(),
            const SizedBox(height: 8),
            const Text('预览不会下载或执行制品代码'),
          ],
        ),
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.of(context).pop(),
          child: const Text('关闭'),
        ),
      ],
    ),
  );
}

String _boundedPreviewText(Object? value, String fallback) {
  final text = value is String && value.trim().isNotEmpty
      ? value.trim()
      : fallback;
  return text.length <= 500 ? text : '${text.substring(0, 500)}…';
}

class _CatalogPage extends StatelessWidget {
  const _CatalogPage({
    required this.title,
    required this.subtitle,
    required this.loading,
    required this.errorMessage,
    required this.onRefresh,
    required this.child,
  });

  final String title;
  final String subtitle;
  final bool loading;
  final String? errorMessage;
  final Future<void> Function() onRefresh;
  final Widget child;

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Padding(
          padding: const EdgeInsets.fromLTRB(28, 24, 28, 16),
          child: Row(
            children: [
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(title, style: Theme.of(context).textTheme.titleLarge),
                    const SizedBox(height: 4),
                    Text(subtitle),
                  ],
                ),
              ),
              IconButton(
                tooltip: '刷新',
                onPressed: loading ? null : onRefresh,
                icon: const Icon(Icons.refresh_rounded),
              ),
            ],
          ),
        ),
        if (loading) const LinearProgressIndicator(minHeight: 2),
        if (errorMessage != null)
          Padding(
            padding: const EdgeInsets.symmetric(horizontal: 28, vertical: 8),
            child: Text(
              errorMessage!,
              style: TextStyle(color: Theme.of(context).colorScheme.error),
            ),
          ),
        Expanded(child: child),
      ],
    );
  }
}

class _CloudUnavailable extends StatelessWidget {
  const _CloudUnavailable();

  @override
  Widget build(BuildContext context) {
    return const Center(child: Text('未配置云端连接，离线卡片仍可正常使用'));
  }
}

class _TopBar extends StatelessWidget {
  const _TopBar({required this.runtimePort});

  final int? runtimePort;

  @override
  Widget build(BuildContext context) {
    final colors = Theme.of(context).colorScheme;
    final runtimeLabel = runtimePort == null
        ? '测试模式'
        : '127.0.0.1:$runtimePort';
    return Container(
      height: 58,
      padding: const EdgeInsets.symmetric(horizontal: 18),
      decoration: BoxDecoration(
        color: const Color(0xD9101415),
        border: Border(bottom: BorderSide(color: colors.outlineVariant)),
      ),
      child: Row(
        children: [
          Container(
            width: 28,
            height: 28,
            decoration: BoxDecoration(
              color: const Color(0xFFE8FF47),
              borderRadius: BorderRadius.circular(7),
            ),
            child: const Icon(
              Icons.auto_awesome_mosaic_rounded,
              color: Color(0xFF101313),
              size: 17,
            ),
          ),
          const SizedBox(width: 11),
          const Text(
            'AGENT CARD',
            style: TextStyle(
              fontFamily: 'Bahnschrift',
              fontWeight: FontWeight.w700,
              letterSpacing: 1.7,
              fontSize: 13,
            ),
          ),
          const SizedBox(width: 24),
          Container(width: 1, height: 18, color: colors.outlineVariant),
          const SizedBox(width: 18),
          Text(
            '工作区',
            style: TextStyle(
              color: colors.onSurfaceVariant,
              fontWeight: FontWeight.w600,
              fontSize: 13,
            ),
          ),
          const Spacer(),
          _StatusPill(
            label: '本地运行时',
            value: runtimeLabel,
            color: const Color(0xFF7BF5A7),
          ),
          const SizedBox(width: 10),
          IconButton(
            tooltip: '通知',
            onPressed: () {},
            icon: const Icon(Icons.notifications_none_rounded, size: 19),
          ),
          const CircleAvatar(
            radius: 14,
            backgroundColor: Color(0xFF293132),
            child: Text(
              'Z',
              style: TextStyle(fontSize: 11, fontWeight: FontWeight.w700),
            ),
          ),
        ],
      ),
    );
  }
}

class _StatusPill extends StatelessWidget {
  const _StatusPill({
    required this.label,
    required this.value,
    required this.color,
  });

  final String label;
  final String value;
  final Color color;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 6),
      decoration: BoxDecoration(
        color: const Color(0xFF171D1E),
        borderRadius: BorderRadius.circular(8),
        border: Border.all(color: const Color(0xFF303738)),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Container(
            width: 6,
            height: 6,
            decoration: BoxDecoration(color: color, shape: BoxShape.circle),
          ),
          const SizedBox(width: 7),
          Text(
            label,
            style: const TextStyle(fontSize: 11, fontWeight: FontWeight.w600),
          ),
          const SizedBox(width: 6),
          Text(
            value,
            style: TextStyle(
              color: Theme.of(context).colorScheme.onSurfaceVariant,
              fontFamily: 'monospace',
              fontSize: 10,
            ),
          ),
        ],
      ),
    );
  }
}

class _NavigationStrip extends StatelessWidget {
  const _NavigationStrip({
    required this.selectedIndex,
    required this.onSelected,
  });

  final int selectedIndex;
  final ValueChanged<int> onSelected;

  @override
  Widget build(BuildContext context) {
    const destinations = [
      (Icons.grid_view_rounded, '工作区'),
      (Icons.widgets_outlined, '卡片库'),
      (Icons.history_rounded, '版本'),
    ];
    return SizedBox(
      width: 78,
      child: Column(
        children: [
          const SizedBox(height: 16),
          for (var index = 0; index < destinations.length; index++)
            _NavItem(
              icon: destinations[index].$1,
              label: destinations[index].$2,
              selected: selectedIndex == index,
              onTap: () => onSelected(index),
            ),
          const Spacer(),
          _NavItem(
            icon: Icons.settings_outlined,
            label: '设置',
            selected: selectedIndex == 3,
            onTap: () => onSelected(3),
          ),
          const SizedBox(height: 14),
        ],
      ),
    );
  }
}

class _SettingsPage extends StatefulWidget {
  const _SettingsPage({
    required this.cloudSettingsController,
    required this.onExportDiagnostics,
  });

  final CloudSettingsController? cloudSettingsController;
  final Future<String> Function()? onExportDiagnostics;

  @override
  State<_SettingsPage> createState() => _SettingsPageState();
}

class _SettingsPageState extends State<_SettingsPage> {
  bool _exporting = false;
  String? _result;

  Future<void> _export() async {
    final export = widget.onExportDiagnostics;
    if (export == null || _exporting) return;
    setState(() {
      _exporting = true;
      _result = null;
    });
    try {
      final path = await export();
      if (mounted) setState(() => _result = '已导出至 $path');
    } catch (_) {
      if (mounted) setState(() => _result = '导出失败，请稍后重试');
    } finally {
      if (mounted) setState(() => _exporting = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final colors = Theme.of(context).colorScheme;
    return SingleChildScrollView(
      padding: const EdgeInsets.all(32),
      child: Align(
        alignment: Alignment.topLeft,
        child: ConstrainedBox(
          constraints: const BoxConstraints(maxWidth: 640),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              if (widget.cloudSettingsController case final controller?) ...[
                CloudSettingsSection(controller: controller),
                const SizedBox(height: 36),
                const Divider(),
                const SizedBox(height: 28),
              ],
              Text('诊断与支持', style: Theme.of(context).textTheme.headlineMedium),
              const SizedBox(height: 12),
              Text(
                '导出本地运行时状态。诊断包不包含密钥、令牌或卡片内容。',
                style: TextStyle(color: colors.onSurfaceVariant),
              ),
              const SizedBox(height: 20),
              FilledButton.icon(
                key: const Key('export-diagnostics'),
                onPressed: widget.onExportDiagnostics == null || _exporting
                    ? null
                    : _export,
                icon: _exporting
                    ? const SizedBox.square(
                        dimension: 16,
                        child: CircularProgressIndicator(strokeWidth: 2),
                      )
                    : const Icon(Icons.file_download_outlined),
                label: Text(_exporting ? '正在导出' : '导出诊断包'),
              ),
              if (_result case final result?) ...[
                const SizedBox(height: 16),
                SelectableText(result),
              ],
            ],
          ),
        ),
      ),
    );
  }
}

class _NavItem extends StatelessWidget {
  const _NavItem({
    required this.icon,
    required this.label,
    required this.selected,
    required this.onTap,
  });

  final IconData icon;
  final String label;
  final bool selected;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 8),
      child: Tooltip(
        message: label,
        child: InkWell(
          borderRadius: BorderRadius.circular(12),
          onTap: onTap,
          child: AnimatedContainer(
            duration: const Duration(milliseconds: 160),
            width: 52,
            height: 48,
            decoration: BoxDecoration(
              color: selected ? const Color(0xFFE8FF47) : Colors.transparent,
              borderRadius: BorderRadius.circular(12),
            ),
            child: Icon(
              icon,
              size: 20,
              color: selected
                  ? const Color(0xFF121515)
                  : Theme.of(context).colorScheme.onSurfaceVariant,
            ),
          ),
        ),
      ),
    );
  }
}

class _WorkspaceCanvas extends StatelessWidget {
  const _WorkspaceCanvas({
    required this.webViewPortFactory,
    required this.cards,
    required this.runtimeVisible,
    required this.agentPanelOpen,
    required this.onOpenAgentPanel,
    this.onNativeCardStateChanged,
    this.onDetachCard,
    this.onMoveCardToOverlay,
    this.onEditPlacement,
    this.layoutErrorMessage,
    this.onDismissLayoutError,
  });

  final InAppWebViewPortFactory webViewPortFactory;
  final List<WorkspaceCard> cards;
  final bool runtimeVisible;
  final bool agentPanelOpen;
  final VoidCallback onOpenAgentPanel;
  final NativeCardStateChanged? onNativeCardStateChanged;
  final CardSurfaceAction? onDetachCard;
  final CardSurfaceAction? onMoveCardToOverlay;
  final CardPlacementEdit? onEditPlacement;
  final String? layoutErrorMessage;
  final VoidCallback? onDismissLayoutError;

  @override
  Widget build(BuildContext context) {
    final colors = Theme.of(context).colorScheme;
    return LayoutBuilder(
      builder: (context, constraints) {
        final columnWidth = (constraints.maxWidth - 56) / 12;
        final viewport = Offset.zero & constraints.biggest;
        final activeIndexes = RuntimeActivityBudget.activeIndexes(
          cards.map(
            (card) => card.nativeSpec == null
                ? CardRuntimeKind.code
                : CardRuntimeKind.native,
          ),
          eligible: cards.map((card) {
            if (card.nativeSpec != null) return true;
            if (!runtimeVisible) return false;
            final placement = card.instance.placement;
            return Rect.fromLTWH(
              28 + placement.x * columnWidth,
              80 + placement.y * 80,
              placement.width * columnWidth - 12,
              placement.height * 80 - 12,
            ).overlaps(viewport);
          }),
        );
        return Stack(
          children: [
            const Positioned.fill(child: CustomPaint(painter: _GridPainter())),
            Positioned(
              left: 28,
              top: 24,
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  const Text(
                    '工作区',
                    style: TextStyle(
                      fontFamily: 'Bahnschrift',
                      fontWeight: FontWeight.w700,
                      fontSize: 22,
                      letterSpacing: .2,
                    ),
                  ),
                  const SizedBox(height: 4),
                  Text(
                    '卡片会吸附到 12 列网格 · ${cards.length} 个实例',
                    style: TextStyle(
                      color: colors.onSurfaceVariant,
                      fontSize: 12,
                    ),
                  ),
                ],
              ),
            ),
            if (cards.isEmpty)
              Center(
                child: Container(
                  width: 430,
                  padding: const EdgeInsets.fromLTRB(36, 32, 36, 34),
                  decoration: BoxDecoration(
                    color: const Color(0xE6121718),
                    borderRadius: BorderRadius.circular(22),
                    border: Border.all(color: const Color(0xFF303839)),
                    boxShadow: const [
                      BoxShadow(
                        color: Color(0x73000000),
                        blurRadius: 40,
                        offset: Offset(0, 20),
                      ),
                    ],
                  ),
                  child: Column(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      Container(
                        width: 58,
                        height: 58,
                        decoration: BoxDecoration(
                          color: const Color(0xFF202728),
                          borderRadius: BorderRadius.circular(17),
                          border: Border.all(color: const Color(0xFF3B4445)),
                        ),
                        child: const Icon(
                          Icons.dashboard_customize_outlined,
                          color: Color(0xFFE8FF47),
                          size: 27,
                        ),
                      ),
                      const SizedBox(height: 22),
                      const Text(
                        '和 Agent 对话，生成你的第一张卡片',
                        textAlign: TextAlign.center,
                        style: TextStyle(
                          fontSize: 18,
                          fontWeight: FontWeight.w600,
                          height: 1.35,
                        ),
                      ),
                      const SizedBox(height: 9),
                      Text(
                        '优先生成原生卡片，复杂的本地逻辑会自动切换为 CodeCard。',
                        textAlign: TextAlign.center,
                        style: TextStyle(
                          color: colors.onSurfaceVariant,
                          fontSize: 12,
                          height: 1.55,
                        ),
                      ),
                      const SizedBox(height: 24),
                      FilledButton.icon(
                        key: const Key('generate-card-empty-state'),
                        onPressed: onOpenAgentPanel,
                        icon: const Icon(Icons.add_rounded, size: 18),
                        label: const Text('生成卡片'),
                        style: FilledButton.styleFrom(
                          foregroundColor: const Color(0xFF111414),
                          backgroundColor: const Color(0xFFE8FF47),
                          padding: const EdgeInsets.symmetric(
                            horizontal: 20,
                            vertical: 14,
                          ),
                          textStyle: const TextStyle(
                            fontWeight: FontWeight.w700,
                          ),
                        ),
                      ),
                    ],
                  ),
                ),
              ),
            for (final (index, card) in cards.indexed)
              Positioned(
                left: 28 + card.instance.placement.x * columnWidth,
                top: 80 + card.instance.placement.y * 80,
                width: card.instance.placement.width * columnWidth - 12,
                height: card.instance.placement.height * 80 - 12,
                child: _WorkspaceCardView(
                  key: ValueKey(card.instance.instanceId),
                  card: card,
                  active: activeIndexes.contains(index),
                  webViewPortFactory: webViewPortFactory,
                  onNativeCardStateChanged: onNativeCardStateChanged,
                  surfacePlacement: CardPlacement(
                    x: 100 + card.instance.placement.x * columnWidth,
                    y: 80 + card.instance.placement.y * 80,
                    width: card.instance.placement.width * columnWidth - 12,
                    height: card.instance.placement.height * 80 - 12,
                  ),
                  onDetachCard: onDetachCard,
                  onMoveCardToOverlay: onMoveCardToOverlay,
                  gridPlacement: card.instance.placement,
                  columnWidth: columnWidth,
                  rowHeight: 80,
                  onEditPlacement: onEditPlacement,
                ),
              ),
            if (layoutErrorMessage case final message?)
              Positioned(
                left: 28,
                right: 28,
                top: 68,
                child: Material(
                  color: colors.errorContainer,
                  borderRadius: BorderRadius.circular(12),
                  child: Padding(
                    padding: const EdgeInsets.symmetric(
                      horizontal: 14,
                      vertical: 8,
                    ),
                    child: Row(
                      children: [
                        Icon(
                          Icons.restore_rounded,
                          size: 18,
                          color: colors.onErrorContainer,
                        ),
                        const SizedBox(width: 9),
                        Expanded(
                          child: Text(
                            message,
                            style: TextStyle(color: colors.onErrorContainer),
                          ),
                        ),
                        IconButton(
                          key: const Key('dismiss-layout-error'),
                          tooltip: '关闭提示',
                          onPressed: onDismissLayoutError,
                          icon: const Icon(Icons.close_rounded, size: 18),
                        ),
                      ],
                    ),
                  ),
                ),
              ),
            if (!agentPanelOpen)
              Positioned(
                key: const Key('open-agent-panel'),
                right: 18,
                top: 18,
                child: IconButton.filledTonal(
                  tooltip: '打开 Agent Studio',
                  onPressed: onOpenAgentPanel,
                  icon: const Icon(Icons.auto_awesome_rounded),
                ),
              ),
          ],
        );
      },
    );
  }
}

class _WorkspaceCardView extends StatefulWidget {
  const _WorkspaceCardView({
    required this.card,
    required this.active,
    required this.surfacePlacement,
    required this.webViewPortFactory,
    required this.gridPlacement,
    required this.columnWidth,
    required this.rowHeight,
    this.onNativeCardStateChanged,
    this.onDetachCard,
    this.onMoveCardToOverlay,
    this.onEditPlacement,
    super.key,
  });

  final WorkspaceCard card;
  final bool active;
  final CardPlacement surfacePlacement;
  final InAppWebViewPortFactory webViewPortFactory;
  final CardPlacement gridPlacement;
  final double columnWidth;
  final double rowHeight;
  final NativeCardStateChanged? onNativeCardStateChanged;
  final CardSurfaceAction? onDetachCard;
  final CardSurfaceAction? onMoveCardToOverlay;
  final CardPlacementEdit? onEditPlacement;

  @override
  State<_WorkspaceCardView> createState() => _WorkspaceCardViewState();
}

class _WorkspaceCardViewState extends State<_WorkspaceCardView> {
  NativeCardController? _nativeController;
  NativeCardStatePersistence? _nativeStatePersistence;
  InAppWebViewPort? _webViewPort;
  CodeCardHost? _codeCardHost;
  Object? _codeCardError;
  var _codeCardMounted = false;
  CardPlacement? _gestureStart;
  Offset _gestureDelta = Offset.zero;

  @override
  void initState() {
    super.initState();
    _createController();
  }

  @override
  void didUpdateWidget(covariant _WorkspaceCardView oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.card.instance.versionId != widget.card.instance.versionId) {
      _disposeRuntime();
      _createController();
      return;
    }
    if (oldWidget.active == widget.active) {
      return;
    }
    if (widget.card.nativeSpec != null) {
      _disposeRuntime();
      _createController();
      return;
    }
    final host = _codeCardHost;
    if (host == null) {
      _createController();
    } else if (widget.active && host.state == CodeCardHostState.suspended) {
      unawaited(_resumeCodeCard(host));
    } else if (!widget.active && host.state == CodeCardHostState.mounted) {
      unawaited(_suspendCodeCard(host));
    }
  }

  void _createController() {
    if (!widget.active) {
      return;
    }
    final nativeSpec = widget.card.nativeSpec;
    if (nativeSpec != null) {
      final controller = NativeCardController(
        {...nativeSpec.initialState, ...widget.card.persistedState},
        capabilityBroker: widget.card.capabilityBroker,
        cardContext: widget.card.cardContext,
      );
      _nativeController = controller;
      if (widget.onNativeCardStateChanged case final write?) {
        _nativeStatePersistence = NativeCardStatePersistence(
          controller: controller,
          write: (state) => write(widget.card.instance.stateNamespace, state),
        );
      }
      return;
    }
    final descriptor = widget.card.codeCard!;
    final port = widget.webViewPortFactory();
    final host = CodeCardHost(
      session: descriptor.session,
      entrypoint: descriptor.entrypoint,
      webView: port,
      onLaunchFailure: descriptor.onLaunchFailure,
      onLaunchSuccess: descriptor.onLaunchSuccess,
      onQuarantine: descriptor.onQuarantine,
      onSuspend: descriptor.onSuspend,
      onResume: descriptor.onResume,
    );
    _webViewPort = port;
    _codeCardHost = host;
    unawaited(_mountCodeCard(host));
  }

  Future<void> _mountCodeCard(CodeCardHost host) async {
    try {
      await host.mount();
      if (!widget.active && identical(host, _codeCardHost)) {
        await host.suspend();
      }
      if (mounted && identical(host, _codeCardHost)) {
        setState(() => _codeCardMounted = true);
      }
    } catch (error) {
      if (mounted && identical(host, _codeCardHost)) {
        setState(() => _codeCardError = error);
      }
    }
  }

  Future<void> _suspendCodeCard(CodeCardHost host) async {
    try {
      await host.suspend();
    } catch (error) {
      if (mounted && identical(host, _codeCardHost)) {
        setState(() => _codeCardError = error);
      }
    }
  }

  Future<void> _resumeCodeCard(CodeCardHost host) async {
    try {
      await host.resume();
    } catch (error) {
      if (mounted && identical(host, _codeCardHost)) {
        setState(() => _codeCardError = error);
      }
    }
  }

  void _disposeRuntime() {
    _nativeStatePersistence?.dispose();
    _nativeStatePersistence = null;
    _nativeController?.dispose();
    _nativeController = null;
    final host = _codeCardHost;
    _codeCardHost = null;
    _webViewPort = null;
    _codeCardMounted = false;
    _codeCardError = null;
    if (host != null) {
      unawaited(host.dispose());
    }
  }

  @override
  void dispose() {
    _disposeRuntime();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(14),
      clipBehavior: Clip.antiAlias,
      decoration: BoxDecoration(
        color: const Color(0xF21A2021),
        borderRadius: BorderRadius.circular(18),
        border: Border.all(color: const Color(0xFF394142)),
        boxShadow: const [
          BoxShadow(
            color: Color(0x4D000000),
            blurRadius: 24,
            offset: Offset(0, 12),
          ),
        ],
      ),
      child: Stack(
        children: [
          Positioned.fill(child: _cardContent()),
          if (widget.onEditPlacement != null)
            Positioned(
              left: 0,
              top: 0,
              child: MouseRegion(
                cursor: SystemMouseCursors.move,
                child: GestureDetector(
                  key: Key('card-drag-${widget.card.instance.instanceId}'),
                  behavior: HitTestBehavior.opaque,
                  onPanStart: (_) => _startPlacementGesture(),
                  onPanUpdate: _dragPlacement,
                  onPanEnd: (_) => _endPlacementGesture(),
                  onPanCancel: _endPlacementGesture,
                  child: const Tooltip(
                    message: '拖动卡片',
                    child: SizedBox(
                      width: 42,
                      height: 34,
                      child: Icon(Icons.drag_indicator_rounded, size: 18),
                    ),
                  ),
                ),
              ),
            ),
          if (widget.onEditPlacement != null)
            Positioned(
              right: 0,
              bottom: 0,
              child: MouseRegion(
                cursor: SystemMouseCursors.resizeDownRight,
                child: GestureDetector(
                  key: Key('card-resize-${widget.card.instance.instanceId}'),
                  behavior: HitTestBehavior.opaque,
                  onPanStart: (_) => _startPlacementGesture(),
                  onPanUpdate: _resizePlacement,
                  onPanEnd: (_) => _endPlacementGesture(),
                  onPanCancel: _endPlacementGesture,
                  child: const Tooltip(
                    message: '调整卡片大小',
                    child: SizedBox(
                      width: 34,
                      height: 34,
                      child: Icon(Icons.south_east_rounded, size: 15),
                    ),
                  ),
                ),
              ),
            ),
          if (widget.onDetachCard != null || widget.onMoveCardToOverlay != null)
            Positioned(
              right: 0,
              top: 0,
              child: PopupMenuButton<_SurfaceAction>(
                key: Key('surface-menu-${widget.card.instance.instanceId}'),
                tooltip: '卡片窗口选项',
                onSelected: _runSurfaceAction,
                itemBuilder: (context) => [
                  if (widget.onDetachCard != null)
                    const PopupMenuItem(
                      value: _SurfaceAction.detach,
                      child: Text('分离为独立窗口'),
                    ),
                  if (widget.onMoveCardToOverlay != null)
                    const PopupMenuItem(
                      value: _SurfaceAction.overlay,
                      child: Text('移到桌面悬浮层'),
                    ),
                ],
              ),
            ),
        ],
      ),
    );
  }

  void _startPlacementGesture() {
    _gestureStart = widget.gridPlacement;
    _gestureDelta = Offset.zero;
  }

  void _dragPlacement(DragUpdateDetails details) {
    final start = _gestureStart;
    if (start == null) return;
    _gestureDelta += details.delta;
    widget.onEditPlacement?.call(
      widget.card.instance.instanceId,
      CardPlacement(
        x: start.x + _gestureDelta.dx / widget.columnWidth,
        y: start.y + _gestureDelta.dy / widget.rowHeight,
        width: start.width,
        height: start.height,
      ),
    );
  }

  void _resizePlacement(DragUpdateDetails details) {
    final start = _gestureStart;
    if (start == null) return;
    _gestureDelta += details.delta;
    widget.onEditPlacement?.call(
      widget.card.instance.instanceId,
      CardPlacement(
        x: start.x,
        y: start.y,
        width: (start.width + _gestureDelta.dx / widget.columnWidth)
            .clamp(0.01, double.infinity)
            .toDouble(),
        height: (start.height + _gestureDelta.dy / widget.rowHeight)
            .clamp(0.01, double.infinity)
            .toDouble(),
      ),
    );
  }

  void _endPlacementGesture() {
    _gestureStart = null;
    _gestureDelta = Offset.zero;
  }

  Future<void> _runSurfaceAction(_SurfaceAction action) async {
    final instanceId = widget.card.instance.instanceId;
    switch (action) {
      case _SurfaceAction.detach:
        await widget.onDetachCard?.call(instanceId, widget.surfacePlacement);
      case _SurfaceAction.overlay:
        await widget.onMoveCardToOverlay?.call(
          instanceId,
          widget.surfacePlacement,
        );
    }
  }

  Widget _cardContent() {
    if (!widget.active) {
      return const Center(child: Text('已暂停：卡片不可见或超过活动上限'));
    }
    final nativeSpec = widget.card.nativeSpec;
    final nativeController = _nativeController;
    if (nativeSpec != null && nativeController != null) {
      return NativeCardRenderer(spec: nativeSpec, controller: nativeController);
    }
    if (_codeCardError != null) {
      return const Center(child: Text('当前平台无法满足 CodeCard 的安全隔离要求'));
    }
    final port = _webViewPort;
    if (!_codeCardMounted || port == null) {
      return const Center(child: CircularProgressIndicator());
    }
    return CodeCardWebView(port: port);
  }
}

enum _SurfaceAction { detach, overlay }

class _AgentPanel extends StatefulWidget {
  const _AgentPanel({
    required this.onCollapse,
    required this.promptFocusNode,
    this.controller,
  });

  final VoidCallback onCollapse;
  final FocusNode promptFocusNode;
  final AgentStudioController? controller;

  @override
  State<_AgentPanel> createState() => _AgentPanelState();
}

class _AgentPanelState extends State<_AgentPanel> {
  final TextEditingController _promptController = TextEditingController();

  @override
  void dispose() {
    _promptController.dispose();
    super.dispose();
  }

  Future<void> _submit() async {
    final controller = widget.controller;
    if (controller == null) {
      return;
    }
    final prompt = _promptController.text;
    _promptController.clear();
    await controller.submit(prompt);
  }

  @override
  Widget build(BuildContext context) {
    final colors = Theme.of(context).colorScheme;
    return Container(
      width: 368,
      decoration: BoxDecoration(
        color: const Color(0xF2131718),
        border: Border(left: BorderSide(color: colors.outlineVariant)),
      ),
      child: Column(
        children: [
          Padding(
            padding: const EdgeInsets.fromLTRB(20, 18, 10, 14),
            child: Row(
              children: [
                const Icon(
                  Icons.auto_awesome_rounded,
                  color: Color(0xFFE8FF47),
                  size: 17,
                ),
                const SizedBox(width: 9),
                const Text(
                  'AGENT STUDIO',
                  style: TextStyle(
                    fontFamily: 'Bahnschrift',
                    fontSize: 12,
                    fontWeight: FontWeight.w700,
                    letterSpacing: 1.4,
                  ),
                ),
                const Spacer(),
                IconButton(
                  key: const Key('collapse-agent-panel'),
                  tooltip: '收起',
                  onPressed: widget.onCollapse,
                  icon: const Icon(Icons.keyboard_double_arrow_right_rounded),
                ),
              ],
            ),
          ),
          Divider(height: 1, color: colors.outlineVariant),
          Expanded(
            child: AnimatedBuilder(
              animation: widget.controller ?? _NoopListenable.instance,
              builder: (context, _) => Padding(
                padding: const EdgeInsets.all(20),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    const _AgentMessage(
                      label: 'AGENT',
                      body: '描述你想放在桌面上的工具。我会先确认需求，再选择 NativeCard 或 CodeCard。',
                    ),
                    const SizedBox(height: 16),
                    Text(
                      '试试这些',
                      style: TextStyle(
                        color: colors.onSurfaceVariant,
                        fontSize: 11,
                        fontWeight: FontWeight.w700,
                      ),
                    ),
                    const SizedBox(height: 10),
                    Wrap(
                      spacing: 7,
                      runSpacing: 7,
                      children: [
                        _PromptChip(
                          label: '离线番茄钟',
                          onTap: () => _promptController.text = '做一个离线番茄钟',
                        ),
                        _PromptChip(
                          label: '系统状态',
                          onTap: () => _promptController.text = '做一个系统状态卡片',
                        ),
                        _PromptChip(
                          label: '本地待办',
                          onTap: () => _promptController.text = '做一个本地待办卡片',
                        ),
                      ],
                    ),
                    if (widget.controller?.session != null) ...[
                      const SizedBox(height: 18),
                      _GenerationStatusCard(controller: widget.controller!),
                    ],
                    const Spacer(),
                    Container(
                      padding: const EdgeInsets.fromLTRB(14, 10, 8, 8),
                      decoration: BoxDecoration(
                        color: const Color(0xFF0D1112),
                        borderRadius: BorderRadius.circular(15),
                        border: Border.all(color: const Color(0xFF343C3D)),
                      ),
                      child: Row(
                        crossAxisAlignment: CrossAxisAlignment.end,
                        children: [
                          Expanded(
                            child: TextField(
                              key: const Key('agent-prompt-field'),
                              focusNode: widget.promptFocusNode,
                              controller: _promptController,
                              maxLines: 4,
                              minLines: 1,
                              decoration: InputDecoration(
                                hintText: '描述一张新卡片…',
                                border: InputBorder.none,
                                isDense: true,
                              ),
                            ),
                          ),
                          const SizedBox(width: 8),
                          IconButton.filled(
                            key: const Key('agent-submit'),
                            tooltip: '发送',
                            onPressed: widget.controller?.canSubmit == true
                                ? _submit
                                : null,
                            icon: const Icon(Icons.arrow_upward_rounded),
                          ),
                        ],
                      ),
                    ),
                    const SizedBox(height: 9),
                    Text(
                      '本地状态不会作为 AI 上下文上传',
                      style: TextStyle(
                        color: colors.onSurfaceVariant,
                        fontSize: 10,
                      ),
                    ),
                  ],
                ),
              ),
            ),
          ),
        ],
      ),
    );
  }
}

class _AgentMessage extends StatelessWidget {
  const _AgentMessage({required this.label, required this.body});

  final String label;
  final String body;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(15),
      decoration: BoxDecoration(
        color: const Color(0xFF1B2223),
        borderRadius: const BorderRadius.only(
          topLeft: Radius.circular(4),
          topRight: Radius.circular(16),
          bottomLeft: Radius.circular(16),
          bottomRight: Radius.circular(16),
        ),
        border: Border.all(color: const Color(0xFF313A3B)),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            label,
            style: const TextStyle(
              color: Color(0xFFE8FF47),
              fontSize: 9,
              fontWeight: FontWeight.w800,
              letterSpacing: 1.2,
            ),
          ),
          const SizedBox(height: 8),
          Text(body, style: const TextStyle(fontSize: 12, height: 1.55)),
        ],
      ),
    );
  }
}

class _PromptChip extends StatelessWidget {
  const _PromptChip({required this.label, this.onTap});

  final String label;
  final VoidCallback? onTap;

  @override
  Widget build(BuildContext context) {
    return InkWell(
      onTap: onTap,
      borderRadius: BorderRadius.circular(8),
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 7),
        decoration: BoxDecoration(
          borderRadius: BorderRadius.circular(8),
          border: Border.all(color: const Color(0xFF343C3D)),
        ),
        child: Text(label, style: const TextStyle(fontSize: 11)),
      ),
    );
  }
}

class _GenerationStatusCard extends StatelessWidget {
  const _GenerationStatusCard({required this.controller});

  final AgentStudioController controller;

  @override
  Widget build(BuildContext context) {
    final session = controller.session!;
    final awaiting = controller.phase == AgentStudioPhase.awaitingConfirmation;
    final running =
        controller.phase == AgentStudioPhase.queued ||
        controller.phase == AgentStudioPhase.generating ||
        controller.phase == AgentStudioPhase.validating;
    return Container(
      padding: const EdgeInsets.all(14),
      decoration: BoxDecoration(
        color: const Color(0xFF1B2223),
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: const Color(0xFF343C3D)),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            _phaseLabel(controller.phase),
            style: const TextStyle(
              color: Color(0xFFE8FF47),
              fontSize: 10,
              fontWeight: FontWeight.w800,
              letterSpacing: 1,
            ),
          ),
          const SizedBox(height: 8),
          Text(session.summary.goal, style: const TextStyle(fontSize: 12)),
          if (session.summary.constraints.isNotEmpty) ...[
            const SizedBox(height: 7),
            for (final constraint in session.summary.constraints)
              Text(
                '· $constraint',
                style: TextStyle(
                  color: Theme.of(context).colorScheme.onSurfaceVariant,
                  fontSize: 10,
                ),
              ),
          ],
          if (running) ...[
            const SizedBox(height: 12),
            LinearProgressIndicator(
              value: controller.events.isEmpty
                  ? null
                  : controller.events.last.progress,
            ),
          ],
          if (controller.errorMessage.isNotEmpty) ...[
            const SizedBox(height: 8),
            Text(
              controller.errorMessage,
              style: const TextStyle(color: Color(0xFFFF8A80), fontSize: 10),
            ),
          ],
          if (awaiting) ...[
            const SizedBox(height: 12),
            Row(
              children: [
                Expanded(
                  child: FilledButton(
                    key: const Key('confirm-generation'),
                    onPressed: controller.confirm,
                    child: const Text('确认生成'),
                  ),
                ),
                const SizedBox(width: 8),
                TextButton(
                  onPressed: controller.cancel,
                  child: const Text('取消'),
                ),
              ],
            ),
          ],
          if (controller.phase == AgentStudioPhase.ready &&
              session.versionId != null) ...[
            const SizedBox(height: 9),
            Text(
              '版本 ${session.versionId} 已就绪',
              style: const TextStyle(fontSize: 10),
            ),
          ],
        ],
      ),
    );
  }
}

String _phaseLabel(AgentStudioPhase phase) {
  return switch (phase) {
    AgentStudioPhase.idle => 'READY',
    AgentStudioPhase.submitting => 'ANALYZING',
    AgentStudioPhase.awaitingConfirmation => 'CONFIRM REQUIREMENTS',
    AgentStudioPhase.queued => 'QUEUED',
    AgentStudioPhase.generating => 'GENERATING',
    AgentStudioPhase.validating => 'VALIDATING',
    AgentStudioPhase.ready => 'READY',
    AgentStudioPhase.failed => 'FAILED',
    AgentStudioPhase.cancelled => 'CANCELLED',
    AgentStudioPhase.error => 'CONNECTION ERROR',
  };
}

class _NoopListenable extends ChangeNotifier {
  _NoopListenable._();

  static final instance = _NoopListenable._();
}

class _GridPainter extends CustomPainter {
  const _GridPainter();

  @override
  void paint(Canvas canvas, Size size) {
    final minor = Paint()
      ..color = const Color(0x0DFFFFFF)
      ..strokeWidth = 1;
    final major = Paint()
      ..color = const Color(0x16FFFFFF)
      ..strokeWidth = 1;

    for (var x = 0.0; x <= size.width; x += 32) {
      canvas.drawLine(
        Offset(x, 0),
        Offset(x, size.height),
        x % 128 == 0 ? major : minor,
      );
    }
    for (var y = 0.0; y <= size.height; y += 32) {
      canvas.drawLine(
        Offset(0, y),
        Offset(size.width, y),
        y % 128 == 0 ? major : minor,
      );
    }
  }

  @override
  bool shouldRepaint(covariant CustomPainter oldDelegate) => false;
}
