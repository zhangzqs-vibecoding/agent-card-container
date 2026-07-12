import 'package:flutter/material.dart';

import '../agent_studio/agent_studio_controller.dart';
import '../native_card/native_card_controller.dart';
import '../native_card/native_card_renderer.dart';
import 'workspace_card.dart';
import 'workspace_controller.dart';

class WorkspaceScreen extends StatefulWidget {
  const WorkspaceScreen({
    super.key,
    this.runtimePort,
    this.workspaceCards = const [],
    this.agentStudioController,
    this.workspaceController,
  });

  final int? runtimePort;
  final List<WorkspaceCard> workspaceCards;
  final AgentStudioController? agentStudioController;
  final WorkspaceController? workspaceController;

  @override
  State<WorkspaceScreen> createState() => _WorkspaceScreenState();
}

class _WorkspaceScreenState extends State<WorkspaceScreen> {
  bool _agentPanelOpen = true;
  int _selectedDestination = 0;

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
                      },
                    ),
                    VerticalDivider(width: 1, color: colors.outlineVariant),
                    Expanded(
                      child: AnimatedBuilder(
                        animation:
                            widget.workspaceController ??
                            _NoopListenable.instance,
                        builder: (context, _) => _WorkspaceCanvas(
                          cards:
                              widget.workspaceController?.cards ??
                              widget.workspaceCards,
                          agentPanelOpen: _agentPanelOpen,
                          onOpenAgentPanel: () {
                            setState(() => _agentPanelOpen = true);
                          },
                        ),
                      ),
                    ),
                    AnimatedContainer(
                      duration: const Duration(milliseconds: 220),
                      curve: Curves.easeOutCubic,
                      width: _agentPanelOpen ? 368 : 0,
                      clipBehavior: Clip.hardEdge,
                      decoration: const BoxDecoration(),
                      child: _agentPanelOpen
                          ? _AgentPanel(
                              controller: widget.agentStudioController,
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
            selected: false,
            onTap: () {},
          ),
          const SizedBox(height: 14),
        ],
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
    required this.cards,
    required this.agentPanelOpen,
    required this.onOpenAgentPanel,
  });

  final List<WorkspaceCard> cards;
  final bool agentPanelOpen;
  final VoidCallback onOpenAgentPanel;

  @override
  Widget build(BuildContext context) {
    final colors = Theme.of(context).colorScheme;
    return LayoutBuilder(
      builder: (context, constraints) {
        final columnWidth = (constraints.maxWidth - 56) / 12;
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
                        onPressed: () {},
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
            for (final card in cards)
              Positioned(
                left: 28 + card.instance.placement.x * columnWidth,
                top: 80 + card.instance.placement.y * 80,
                width: card.instance.placement.width * columnWidth - 12,
                height: card.instance.placement.height * 80 - 12,
                child: _WorkspaceCardView(
                  key: ValueKey(card.instance.instanceId),
                  card: card,
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
  const _WorkspaceCardView({required this.card, super.key});

  final WorkspaceCard card;

  @override
  State<_WorkspaceCardView> createState() => _WorkspaceCardViewState();
}

class _WorkspaceCardViewState extends State<_WorkspaceCardView> {
  late NativeCardController _controller;

  @override
  void initState() {
    super.initState();
    _createController();
  }

  @override
  void didUpdateWidget(covariant _WorkspaceCardView oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.card.instance.versionId != widget.card.instance.versionId) {
      _controller.dispose();
      _createController();
    }
  }

  void _createController() {
    _controller = NativeCardController({
      ...widget.card.spec.initialState,
      ...widget.card.persistedState,
    });
  }

  @override
  void dispose() {
    _controller.dispose();
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
      child: NativeCardRenderer(
        spec: widget.card.spec,
        controller: _controller,
      ),
    );
  }
}

class _AgentPanel extends StatefulWidget {
  const _AgentPanel({required this.onCollapse, this.controller});

  final VoidCallback onCollapse;
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
