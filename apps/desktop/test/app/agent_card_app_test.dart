import 'dart:convert';
import 'dart:io';

import 'package:agent_card_desktop/src/adapters/in_app_webview_port.dart';
import 'package:agent_card_desktop/src/agent_studio/agent_studio_controller.dart';
import 'package:agent_card_desktop/src/app/agent_card_app.dart';
import 'package:agent_card_desktop/src/cards/card_instance.dart';
import 'package:agent_card_desktop/src/capabilities/capability.dart';
import 'package:agent_card_desktop/src/capabilities/capability_broker.dart';
import 'package:agent_card_desktop/src/cloud/card_catalog_controller.dart';
import 'package:agent_card_desktop/src/cloud/card_version_lifecycle.dart';
import 'package:agent_card_desktop/src/cloud/cloud_api_client.dart';
import 'package:agent_card_desktop/src/cloud/cloud_connection_settings.dart';
import 'package:agent_card_desktop/src/cloud/cloud_settings_controller.dart';
import 'package:agent_card_desktop/src/cloud/cloud_settings_repository.dart';
import 'package:agent_card_desktop/src/cloud/cloud_settings_service.dart';
import 'package:agent_card_desktop/src/cloud/secret_store.dart';
import 'package:agent_card_desktop/src/native_card/native_card_spec.dart';
import 'package:agent_card_desktop/src/runtime/runtime_session.dart';
import 'package:agent_card_desktop/src/surfaces/surface.dart';
import 'package:agent_card_desktop/src/workspace/workspace_card.dart';
import 'package:agent_card_desktop/src/workspace/workspace_controller.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

import '../support/fake_generation_port.dart';

void main() {
  testWidgets('renders the desktop workspace shell', (tester) async {
    await tester.binding.setSurfaceSize(const Size(1440, 900));
    addTearDown(() => tester.binding.setSurfaceSize(null));

    await tester.pumpWidget(const AgentCardApp());

    expect(find.text('AGENT CARD'), findsOneWidget);
    expect(find.text('工作区'), findsWidgets);
    expect(find.text('和 Agent 对话，生成你的第一张卡片'), findsOneWidget);
    expect(find.text('生成卡片'), findsOneWidget);
    expect(find.text('AGENT STUDIO'), findsOneWidget);
    expect(find.text('本地运行时'), findsOneWidget);
    expect(find.byIcon(Icons.add_rounded), findsOneWidget);
  });

  testWidgets('can collapse and reopen the agent panel', (tester) async {
    await tester.binding.setSurfaceSize(const Size(1440, 900));
    addTearDown(() => tester.binding.setSurfaceSize(null));

    await tester.pumpWidget(const AgentCardApp());

    await tester.tap(find.byKey(const Key('collapse-agent-panel')));
    await tester.pumpAndSettle();
    expect(find.text('AGENT STUDIO'), findsNothing);

    await tester.tap(find.byKey(const Key('open-agent-panel')));
    await tester.pumpAndSettle();
    expect(find.text('AGENT STUDIO'), findsOneWidget);
  });

  testWidgets('exports a redacted diagnostic bundle from settings', (
    tester,
  ) async {
    await tester.binding.setSurfaceSize(const Size(1440, 900));
    addTearDown(() => tester.binding.setSurfaceSize(null));
    var exports = 0;
    await tester.pumpWidget(
      AgentCardApp(
        onExportDiagnostics: () async {
          exports++;
          return '/safe/diagnostics/diagnostic.json';
        },
      ),
    );

    await tester.tap(find.byIcon(Icons.settings_outlined));
    await tester.pump();
    expect(find.text('诊断与支持'), findsOneWidget);
    await tester.tap(find.byKey(const Key('export-diagnostics')));
    await tester.pumpAndSettle();

    expect(exports, 1);
    expect(
      find.textContaining('/safe/diagnostics/diagnostic.json'),
      findsOneWidget,
    );
  });

  testWidgets('configures cloud service and marks restart required', (
    tester,
  ) async {
    await tester.binding.setSurfaceSize(const Size(1440, 900));
    addTearDown(() => tester.binding.setSurfaceSize(null));
    final secrets = _AppTestSecretStore();
    final settings = _AppTestSettingsStore();
    final controller = CloudSettingsController(
      CloudSettingsService(
        environment: const {},
        repository: settings,
        secretStore: secrets,
        connectionTester: (_, _) async {},
      ),
    );
    await controller.initialize();
    addTearDown(controller.dispose);

    await tester.pumpWidget(AgentCardApp(cloudSettingsController: controller));
    await tester.tap(find.byIcon(Icons.settings_outlined));
    await tester.pump();

    expect(find.text('云端服务'), findsOneWidget);
    await tester.enterText(
      find.byKey(const Key('cloud-base-url')),
      'http://127.0.0.1:8080',
    );
    await tester.enterText(
      find.byKey(const Key('cloud-access-token')),
      'ui-secret',
    );
    await tester.tap(find.byKey(const Key('cloud-allow-insecure')));
    await tester.tap(find.byKey(const Key('cloud-test-connection')));
    await tester.pump();
    await tester.pump();
    expect(find.textContaining('连接正常'), findsOneWidget);

    await tester.tap(find.byKey(const Key('cloud-save')));
    await tester.pump();
    await tester.pump();
    expect(find.textContaining('重启应用后生效'), findsOneWidget);
    expect(secrets.value, 'ui-secret');
  });

  testWidgets('matches the desktop workspace visual baseline', (tester) async {
    await tester.binding.setSurfaceSize(const Size(1440, 900));
    addTearDown(() => tester.binding.setSurfaceSize(null));

    await tester.pumpWidget(const AgentCardApp(runtimePort: 43125));
    await tester.pumpAndSettle();

    await expectLater(
      find.byType(MaterialApp),
      matchesGoldenFile('goldens/workspace_shell.png'),
    );
  }, skip: !Platform.isLinux);

  testWidgets('renders a persisted NativeCard instance in the grid', (
    tester,
  ) async {
    await tester.binding.setSurfaceSize(const Size(1440, 900));
    addTearDown(() => tester.binding.setSurfaceSize(null));
    final spec = NativeCardSpec.fromJson(
      jsonDecode(
            File(
              '../../contracts/card/fixtures/pomodoro-native.json',
            ).readAsStringSync(),
          )
          as Map<String, Object?>,
    );

    await tester.pumpWidget(
      AgentCardApp(
        workspaceCards: [
          WorkspaceCard(
            instance: const CardInstance(
              instanceId: 'instance-1',
              cardId: 'card-1',
              versionId: 'version-1',
              surfaceId: 'workspace-main',
              placement: CardPlacement(x: 0, y: 0, width: 4, height: 3),
              stateNamespace: 'state-1',
              status: CardInstanceStatus.active,
            ),
            spec: spec,
          ),
        ],
      ),
    );

    expect(find.text('专注时间'), findsOneWidget);
    expect(find.text('和 Agent 对话，生成你的第一张卡片'), findsNothing);
    expect(find.textContaining('1 个实例'), findsOneWidget);
  });

  testWidgets('persists NativeCard interactions with its state namespace', (
    tester,
  ) async {
    await tester.binding.setSurfaceSize(const Size(1440, 900));
    addTearDown(() => tester.binding.setSurfaceSize(null));
    final writes = <({String namespace, Map<String, Object?> state})>[];
    final spec = NativeCardSpec.fromJson(
      jsonDecode(
            File(
              '../../contracts/card/fixtures/pomodoro-native.json',
            ).readAsStringSync(),
          )
          as Map<String, Object?>,
    );

    await tester.pumpWidget(
      AgentCardApp(
        onNativeCardStateChanged: (namespace, state) {
          writes.add((namespace: namespace, state: state));
        },
        workspaceCards: [
          WorkspaceCard(
            instance: const CardInstance(
              instanceId: 'instance-1',
              cardId: 'card-1',
              versionId: 'version-1',
              surfaceId: 'workspace-main',
              placement: CardPlacement(x: 0, y: 0, width: 4, height: 3),
              stateNamespace: 'state-1',
              status: CardInstanceStatus.active,
            ),
            spec: spec,
          ),
        ],
      ),
    );

    await tester.tap(find.text('开始 / 暂停'));
    await tester.pump(const Duration(milliseconds: 301));

    expect(writes, hasLength(1));
    expect(writes.single.namespace, 'state-1');
    expect(writes.single.state['running'], isTrue);
  });

  testWidgets('offers explicit detached and overlay surface actions', (
    tester,
  ) async {
    await tester.binding.setSurfaceSize(const Size(1440, 900));
    addTearDown(() => tester.binding.setSurfaceSize(null));
    final actions = <String>[];
    final spec = NativeCardSpec.fromJson(
      jsonDecode(
            File(
              '../../contracts/card/fixtures/pomodoro-native.json',
            ).readAsStringSync(),
          )
          as Map<String, Object?>,
    );
    final card = WorkspaceCard(
      instance: const CardInstance(
        instanceId: 'instance-1',
        cardId: 'card-1',
        versionId: 'version-1',
        surfaceId: 'workspace-main',
        placement: CardPlacement(x: 0, y: 0, width: 4, height: 3),
        stateNamespace: 'state-1',
        status: CardInstanceStatus.active,
      ),
      spec: spec,
    );
    final workspace = WorkspaceController([card]);
    addTearDown(workspace.dispose);

    await tester.pumpWidget(
      AgentCardApp(
        workspaceController: workspace,
        onDetachCard: (instanceId, placement) async {
          actions.add('detach:$instanceId');
          workspace.moveInstance(
            instanceId,
            surfaceId: 'detached-1',
            placement: placement,
          );
        },
        onMoveCardToOverlay: (instanceId, _) async =>
            actions.add('overlay:$instanceId'),
      ),
    );

    await tester.tap(find.byKey(const Key('surface-menu-instance-1')));
    await tester.pumpAndSettle();
    await tester.tap(find.text('分离为独立窗口'));
    await tester.pumpAndSettle();

    expect(actions, ['detach:instance-1']);
    expect(find.text('专注时间'), findsNothing);
  });

  testWidgets('drags a workspace card on the grid and preserves identity', (
    tester,
  ) async {
    await tester.binding.setSurfaceSize(const Size(1440, 900));
    addTearDown(() => tester.binding.setSurfaceSize(null));
    final writes = <CardPlacement>[];
    final workspace = WorkspaceController([
      _workspaceTestCard(),
    ], persistPlacement: (_, _, placement) async => writes.add(placement));
    addTearDown(workspace.dispose);
    await tester.pumpWidget(AgentCardApp(workspaceController: workspace));

    await tester.drag(
      find.byKey(const Key('card-drag-instance-layout')),
      const Offset(100, 80),
    );
    await tester.pump(const Duration(milliseconds: 301));

    final instance = workspace.cards.single.instance;
    expect(instance.placement.x, 1);
    expect(instance.placement.y, 1);
    expect(instance.versionId, 'version-layout');
    expect(instance.stateNamespace, 'state-layout');
    expect(writes, hasLength(1));
  });

  testWidgets('resizes a workspace card with grid constraints', (tester) async {
    await tester.binding.setSurfaceSize(const Size(1440, 900));
    addTearDown(() => tester.binding.setSurfaceSize(null));
    final workspace = WorkspaceController([_workspaceTestCard()]);
    addTearDown(workspace.dispose);
    await tester.pumpWidget(AgentCardApp(workspaceController: workspace));

    await tester.drag(
      find.byKey(const Key('card-resize-instance-layout')),
      const Offset(-1000, -1000),
    );
    await tester.pump();
    expect(workspace.cards.single.instance.placement.width, 2);
    expect(workspace.cards.single.instance.placement.height, 2);

    await tester.drag(
      find.byKey(const Key('card-resize-instance-layout')),
      const Offset(100, 80),
    );
    await tester.pump();
    expect(workspace.cards.single.instance.placement.width, 3);
    expect(workspace.cards.single.instance.placement.height, 3);
  });

  testWidgets('moves a dragged card below a grid collision', (tester) async {
    await tester.binding.setSurfaceSize(const Size(1440, 900));
    addTearDown(() => tester.binding.setSurfaceSize(null));
    final workspace = WorkspaceController([
      _workspaceTestCard(),
      _workspaceTestCard(
        instanceId: 'instance-blocker',
        stateNamespace: 'state-blocker',
        placement: const CardPlacement(x: 4, y: 0, width: 4, height: 3),
      ),
    ]);
    addTearDown(workspace.dispose);
    await tester.pumpWidget(AgentCardApp(workspaceController: workspace));

    await tester.drag(
      find.byKey(const Key('card-drag-instance-layout')),
      const Offset(315, 0),
    );
    await tester.pump();

    final moved = workspace.cards.first.instance.placement;
    expect(moved.x, 4);
    expect(moved.y, 3);
  });

  testWidgets('restores layout and shows a redacted persistence error', (
    tester,
  ) async {
    await tester.binding.setSurfaceSize(const Size(1440, 900));
    addTearDown(() => tester.binding.setSurfaceSize(null));
    final workspace = WorkspaceController(
      [_workspaceTestCard()],
      persistPlacement: (_, _, _) async =>
          throw Exception('private database path'),
    );
    addTearDown(workspace.dispose);
    await tester.pumpWidget(AgentCardApp(workspaceController: workspace));

    await tester.drag(
      find.byKey(const Key('card-drag-instance-layout')),
      const Offset(100, 80),
    );
    await tester.pump(const Duration(milliseconds: 301));
    await tester.pump();

    expect(workspace.cards.single.instance.placement.x, 0);
    expect(find.text('布局保存失败，已恢复上次位置'), findsOneWidget);
    expect(find.textContaining('private database path'), findsNothing);
    await tester.tap(find.byKey(const Key('dismiss-layout-error')));
    await tester.pump();
    expect(find.text('布局保存失败，已恢复上次位置'), findsNothing);
  });

  testWidgets('opens and focuses Agent Studio from the empty workspace', (
    tester,
  ) async {
    await tester.binding.setSurfaceSize(const Size(1440, 900));
    addTearDown(() => tester.binding.setSurfaceSize(null));
    await tester.pumpWidget(const AgentCardApp());
    await tester.tap(find.byKey(const Key('collapse-agent-panel')));
    await tester.pumpAndSettle();

    await tester.tap(find.byKey(const Key('generate-card-empty-state')));
    await tester.pumpAndSettle();

    final field = tester.widget<TextField>(
      find.byKey(const Key('agent-prompt-field')),
    );
    expect(field.focusNode?.hasFocus, isTrue);
  });

  testWidgets('NativeCard invokes host abilities through its attached broker', (
    tester,
  ) async {
    await tester.binding.setSurfaceSize(const Size(1440, 900));
    addTearDown(() => tester.binding.setSurfaceSize(null));
    final broker = CapabilityBroker()
      ..register('system.metrics.get', (_, _) async => 'ready')
      ..replaceGrants({
        const PermissionGrant(
          instanceId: 'instance-1',
          versionId: 'version-1',
          capability: 'system.metrics.read',
        ),
      });
    final context = CardContext(
      instanceId: 'instance-1',
      cardId: 'card-1',
      versionId: 'version-1',
      declaredCapabilities: const {'system.metrics.read'},
    );
    final spec = NativeCardSpec.fromJson({
      'schemaVersion': 1,
      'initialState': {'result': 'pending'},
      'root': {
        'id': 'root',
        'type': 'Column',
        'children': [
          {
            'id': 'result',
            'type': 'Text',
            'props': {
              'text': {'path': 'state.result'},
            },
          },
          {
            'id': 'load',
            'type': 'Button',
            'props': {'label': '读取指标'},
            'events': {
              'onPressed': [
                {
                  'type': 'capability.invoke',
                  'method': 'system.metrics.get',
                  'path': 'result',
                },
              ],
            },
          },
        ],
      },
    });

    await tester.pumpWidget(
      AgentCardApp(
        workspaceCards: [
          WorkspaceCard(
            instance: const CardInstance(
              instanceId: 'instance-1',
              cardId: 'card-1',
              versionId: 'version-1',
              surfaceId: 'workspace-main',
              placement: CardPlacement(x: 0, y: 0, width: 4, height: 3),
              stateNamespace: 'state-1',
              status: CardInstanceStatus.active,
            ),
            spec: spec,
            capabilityBroker: broker,
            cardContext: context,
          ),
        ],
      ),
    );

    await tester.tap(find.text('读取指标'));
    await tester.pump();

    expect(find.text('ready'), findsOneWidget);
  });

  testWidgets('adds an installed card to the live workspace', (tester) async {
    await tester.binding.setSurfaceSize(const Size(1440, 900));
    addTearDown(() => tester.binding.setSurfaceSize(null));
    final workspace = WorkspaceController(const []);
    addTearDown(workspace.dispose);
    await tester.pumpWidget(AgentCardApp(workspaceController: workspace));
    expect(find.text('和 Agent 对话，生成你的第一张卡片'), findsOneWidget);

    final spec = NativeCardSpec.fromJson(
      jsonDecode(
            File(
              '../../contracts/card/fixtures/pomodoro-native.json',
            ).readAsStringSync(),
          )
          as Map<String, Object?>,
    );
    workspace.add(
      WorkspaceCard(
        instance: const CardInstance(
          instanceId: 'installed-1',
          cardId: 'card-1',
          versionId: 'version-1',
          surfaceId: 'workspace-main',
          placement: CardPlacement(x: 0, y: 0, width: 4, height: 3),
          stateNamespace: 'state-installed',
          status: CardInstanceStatus.active,
        ),
        spec: spec,
      ),
    );
    await tester.pump();

    expect(find.text('专注时间'), findsOneWidget);
    expect(find.text('和 Agent 对话，生成你的第一张卡片'), findsNothing);
  });

  testWidgets('browses cards and installs a selected historical version', (
    tester,
  ) async {
    await tester.binding.setSurfaceSize(const Size(1440, 900));
    addTearDown(() => tester.binding.setSurfaceSize(null));
    final installed = <String>[];
    final catalog = CardCatalogController(
      port: _CatalogFixture(),
      installVersion: (cardId, versionId) async {
        installed.add('$cardId/$versionId');
      },
    );
    addTearDown(catalog.dispose);
    await catalog.refresh();
    await tester.pumpWidget(AgentCardApp(cardCatalogController: catalog));

    await tester.tap(find.byIcon(Icons.widgets_outlined));
    await tester.pump();
    expect(find.text('我的卡片'), findsOneWidget);
    expect(
      find.descendant(of: find.byType(ListTile), matching: find.text('离线番茄钟')),
      findsOneWidget,
    );

    await tester.tap(find.text('查看版本'));
    await tester.pumpAndSettle();
    expect(find.text('版本历史'), findsOneWidget);
    expect(find.text('1.0.0'), findsOneWidget);

    await tester.tap(find.byKey(const Key('preview-ver_01')));
    await tester.pumpAndSettle();
    expect(find.text('版本预览'), findsOneWidget);
    expect(find.text('自动选择 NativeCard：只需声明式状态与计时器'), findsOneWidget);
    expect(find.text('预览不会下载或执行制品代码'), findsOneWidget);
    await tester.tap(find.text('关闭'));
    await tester.pumpAndSettle();

    await tester.tap(find.text('安装此版本'));
    await tester.pumpAndSettle();
    expect(installed, ['card_01/ver_01']);
  });

  testWidgets('confirms differences before upgrading and resetting state', (
    tester,
  ) async {
    await tester.binding.setSurfaceSize(const Size(1440, 900));
    addTearDown(() => tester.binding.setSurfaceSize(null));
    final instance = const CardInstance(
      instanceId: 'instance-1',
      cardId: 'card_01',
      versionId: 'ver_01',
      surfaceId: 'workspace-main',
      placement: CardPlacement(x: 0, y: 0, width: 4, height: 3),
      stateNamespace: 'state-1',
      status: CardInstanceStatus.active,
    );
    CardVersionDecision? appliedDecision;
    Set<PermissionGrant>? approved;
    final proposal = CatalogVersionChangeProposal(
      instance: instance,
      targetVersion: _upgradeCatalogVersion,
      difference: CardVersionDifference(
        addedCapabilities: const ['clipboard.read', 'network.fetch'],
        removedCapabilities: const ['storage'],
        addedDomains: const ['new.example.com'],
        removedDomains: const ['old.example.com'],
        currentStateSchemaVersion: 1,
        targetStateSchemaVersion: 2,
      ),
      apply: (decision, grants) async {
        appliedDecision = decision;
        approved = grants;
      },
    );
    final catalog = CardCatalogController(
      port: _UpgradeCatalogFixture(),
      activeInstanceForCard: (_) => instance,
      prepareVersionChange: (_, _, _) async => proposal,
    );
    addTearDown(catalog.dispose);
    await catalog.refresh();
    await catalog.selectCard('card_01');
    await tester.pumpWidget(AgentCardApp(cardCatalogController: catalog));

    await tester.tap(find.byIcon(Icons.history_rounded));
    await tester.pumpAndSettle();
    expect(find.text('升级到此版本'), findsOneWidget);
    await tester.tap(find.text('升级到此版本'));
    await tester.pumpAndSettle();

    expect(find.text('版本能力与状态确认'), findsOneWidget);
    expect(find.textContaining('clipboard.read'), findsOneWidget);
    expect(find.textContaining('new.example.com'), findsOneWidget);
    expect(find.textContaining('状态 schema 1 → 2'), findsOneWidget);
    expect(find.textContaining('迁移'), findsNothing);
    await tester.tap(find.text('安装并重置'));
    await tester.pumpAndSettle();

    expect(appliedDecision, CardVersionDecision.resetState);
    expect(approved?.map((grant) => grant.capability).toSet(), {
      'clipboard.read',
      'network.fetch',
    });
    expect(
      approved
          ?.singleWhere((grant) => grant.capability == 'network.fetch')
          .domains,
      {'new.example.com'},
    );
  });

  testWidgets('starts an Agent iteration from the selected card version', (
    tester,
  ) async {
    await tester.binding.setSurfaceSize(const Size(1440, 900));
    addTearDown(() => tester.binding.setSurfaceSize(null));
    final agent = AgentStudioController(port: FakeGenerationPort());
    addTearDown(agent.dispose);
    final workspace = WorkspaceController([
      WorkspaceCard(
        instance: const CardInstance(
          instanceId: 'instance-iterate',
          cardId: 'card_01',
          versionId: 'ver_01',
          surfaceId: 'workspace-main',
          placement: CardPlacement(x: 0, y: 0, width: 4, height: 3),
          stateNamespace: 'state-iterate',
          status: CardInstanceStatus.active,
        ),
        spec: NativeCardSpec.fromJson({
          'schemaVersion': 1,
          'initialState': <String, Object?>{},
          'root': {'id': 'root', 'type': 'Text'},
        }),
        displayVersion: '1.0.0',
      ),
    ]);
    addTearDown(workspace.dispose);
    await tester.pumpWidget(
      AgentCardApp(
        agentStudioController: agent,
        workspaceController: workspace,
      ),
    );

    await tester.tap(find.byKey(const Key('surface-menu-instance-iterate')));
    await tester.pumpAndSettle();
    await tester.tap(find.text('基于此版本修改'));
    await tester.pumpAndSettle();

    expect(agent.baseCardId, 'card_01');
    expect(agent.baseVersionId, 'ver_01');
    expect(find.text('基于版本 1.0.0 修改'), findsOneWidget);
    expect(find.text('描述一张新卡片…'), findsNothing);
    expect(find.text('描述你要修改的内容…'), findsOneWidget);
  });

  testWidgets('fails closed when CodeCard isolation is unavailable', (
    tester,
  ) async {
    await tester.binding.setSurfaceSize(const Size(1440, 900));
    addTearDown(() => tester.binding.setSurfaceSize(null));
    final session = RuntimeSession(
      id: '0123456789abcdef0123456789abcdef',
      authority: '0123456789abcdef0123456789abcdef.localhost:43125',
      token: 'secret',
      instanceId: 'code-instance',
      cardId: 'card-code',
      versionId: 'version-code',
      resources: const {},
    );

    await tester.pumpWidget(
      AgentCardApp(
        webViewPortFactory: () => InAppWebViewPort(platformSupported: false),
        workspaceCards: [
          WorkspaceCard.code(
            instance: const CardInstance(
              instanceId: 'code-instance',
              cardId: 'card-code',
              versionId: 'version-code',
              surfaceId: 'workspace-main',
              placement: CardPlacement(x: 0, y: 0, width: 4, height: 3),
              stateNamespace: 'code-state',
              status: CardInstanceStatus.active,
            ),
            codeCard: CodeCardDescriptor(
              session: session,
              entrypoint: '/bundle/hash/index.html',
            ),
          ),
        ],
      ),
    );
    await tester.pump();

    expect(find.text('当前平台无法满足 CodeCard 的安全隔离要求'), findsOneWidget);
  });

  testWidgets('keeps an offscreen CodeCard suspended', (tester) async {
    await tester.binding.setSurfaceSize(const Size(1440, 900));
    addTearDown(() => tester.binding.setSurfaceSize(null));
    final session = RuntimeSession(
      id: 'fedcba9876543210fedcba9876543210',
      authority: 'fedcba9876543210fedcba9876543210.localhost:43125',
      token: 'secret',
      instanceId: 'offscreen-code',
      cardId: 'card-code',
      versionId: 'version-code',
      resources: const {},
    );

    await tester.pumpWidget(
      AgentCardApp(
        workspaceCards: [
          WorkspaceCard.code(
            instance: const CardInstance(
              instanceId: 'offscreen-code',
              cardId: 'card-code',
              versionId: 'version-code',
              surfaceId: 'workspace-main',
              placement: CardPlacement(x: 100, y: 0, width: 4, height: 3),
              stateNamespace: 'offscreen-state',
              status: CardInstanceStatus.active,
            ),
            codeCard: CodeCardDescriptor(
              session: session,
              entrypoint: '/bundle/hash/index.html',
            ),
          ),
        ],
      ),
    );
    await tester.pump();

    expect(find.text('已暂停：卡片不可见或超过活动上限'), findsOneWidget);
    expect(find.text('当前平台无法满足 CodeCard 的安全隔离要求'), findsNothing);
  });
}

WorkspaceCard _workspaceTestCard({
  String instanceId = 'instance-layout',
  String stateNamespace = 'state-layout',
  CardPlacement placement = const CardPlacement(
    x: 0,
    y: 0,
    width: 4,
    height: 3,
  ),
}) {
  return WorkspaceCard(
    instance: CardInstance(
      instanceId: instanceId,
      cardId: 'card-layout',
      versionId: 'version-layout',
      surfaceId: 'workspace-main',
      placement: placement,
      stateNamespace: stateNamespace,
      status: CardInstanceStatus.active,
    ),
    spec: NativeCardSpec.fromJson({
      'schemaVersion': 1,
      'initialState': <String, Object?>{},
      'root': {
        'id': 'root',
        'type': 'Text',
        'props': {'text': '布局卡片'},
      },
    }),
  );
}

class _AppTestSecretStore implements SecretStore {
  String? value;

  @override
  Future<String?> readAccessToken() async => value;

  @override
  Future<void> writeAccessToken(String value) async {
    this.value = value;
  }

  @override
  Future<void> deleteAccessToken() async {
    value = null;
  }
}

class _AppTestSettingsStore implements CloudSettingsStore {
  CloudUserConfig? value;

  @override
  Future<CloudUserConfig?> read() async => value;

  @override
  Future<void> save(CloudUserConfig config) async {
    value = config;
  }

  @override
  Future<void> clear() async {
    value = null;
  }
}

class _CatalogFixture implements CloudCatalogPort {
  @override
  Future<List<CloudCardSummary>> listCards() async => [
    CloudCardSummary(
      cardId: 'card_01',
      title: '离线番茄钟',
      description: '无需网络即可计时',
      latestVersion: _catalogVersion,
    ),
  ];

  @override
  Future<CloudCardDetail> getCard(String cardId) async => CloudCardDetail(
    cardId: cardId,
    title: '离线番茄钟',
    description: '无需网络即可计时',
    versions: [_catalogVersion],
  );
}

class _UpgradeCatalogFixture implements CloudCatalogPort {
  @override
  Future<List<CloudCardSummary>> listCards() async => [
    CloudCardSummary(
      cardId: 'card_01',
      title: '离线番茄钟',
      description: '无需网络即可计时',
      latestVersion: _upgradeCatalogVersion,
    ),
  ];

  @override
  Future<CloudCardDetail> getCard(String cardId) async => CloudCardDetail(
    cardId: cardId,
    title: '离线番茄钟',
    description: '无需网络即可计时',
    versions: [_upgradeCatalogVersion, _catalogVersion],
  );
}

final _catalogVersion = CloudCardVersion(
  versionId: 'ver_01',
  cardId: 'card_01',
  runtime: 'native',
  displayVersion: '1.0.0',
  title: '离线番茄钟',
  description: '无需网络即可计时',
  artifactSha256: List.filled(64, 'a').join(),
  keyId: 'key-1',
  preview: const {
    'title': '离线番茄钟',
    'runtime': 'native',
    'reason': '自动选择 NativeCard：只需声明式状态与计时器',
  },
  createdAt: DateTime.utc(2026, 7, 12),
);

final _upgradeCatalogVersion = CloudCardVersion(
  versionId: 'ver_02',
  cardId: 'card_01',
  runtime: 'native',
  displayVersion: '1.0.1',
  title: '离线番茄钟',
  description: '增加剪贴板能力',
  artifactSha256: List.filled(64, 'b').join(),
  keyId: 'key-1',
  preview: const {},
  createdAt: DateTime.utc(2026, 7, 13),
);
