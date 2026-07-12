import 'dart:convert';
import 'dart:io';

import 'package:agent_card_desktop/src/app/agent_card_app.dart';
import 'package:agent_card_desktop/src/cards/card_instance.dart';
import 'package:agent_card_desktop/src/native_card/native_card_spec.dart';
import 'package:agent_card_desktop/src/surfaces/surface.dart';
import 'package:agent_card_desktop/src/workspace/workspace_card.dart';
import 'package:agent_card_desktop/src/workspace/workspace_controller.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

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

  testWidgets('matches the desktop workspace visual baseline', (tester) async {
    await tester.binding.setSurfaceSize(const Size(1440, 900));
    addTearDown(() => tester.binding.setSurfaceSize(null));

    await tester.pumpWidget(const AgentCardApp(runtimePort: 43125));
    await tester.pumpAndSettle();

    await expectLater(
      find.byType(MaterialApp),
      matchesGoldenFile('goldens/workspace_shell.png'),
    );
  });

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

  testWidgets('adds an installed card to the live workspace', (tester) async {
    await tester.binding.setSurfaceSize(const Size(1440, 900));
    addTearDown(() => tester.binding.setSurfaceSize(null));
    final workspace = WorkspaceController();
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
}
