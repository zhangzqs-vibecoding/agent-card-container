import 'package:agent_card_desktop/src/app/agent_card_app.dart';
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
}
