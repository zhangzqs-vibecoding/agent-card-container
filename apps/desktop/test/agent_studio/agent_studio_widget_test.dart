import 'package:agent_card_desktop/src/agent_studio/agent_studio_controller.dart';
import 'package:agent_card_desktop/src/app/agent_card_app.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

import '../support/fake_generation_port.dart';

void main() {
  testWidgets('submits and confirms a card from Agent Studio', (tester) async {
    await tester.binding.setSurfaceSize(const Size(1440, 900));
    addTearDown(() => tester.binding.setSurfaceSize(null));
    final controller = AgentStudioController(port: FakeGenerationPort());
    addTearDown(controller.dispose);
    await tester.pumpWidget(AgentCardApp(agentStudioController: controller));

    await tester.enterText(
      find.byKey(const Key('agent-prompt-field')),
      '做一个离线番茄钟',
    );
    await tester.tap(find.byKey(const Key('agent-submit')));
    await tester.pumpAndSettle();

    expect(find.text('CONFIRM REQUIREMENTS'), findsOneWidget);
    expect(find.text('做一个离线番茄钟'), findsOneWidget);
    await tester.tap(find.byKey(const Key('confirm-generation')));
    await tester.pumpAndSettle();

    expect(find.text('READY'), findsOneWidget);
    expect(find.textContaining('ver_01'), findsOneWidget);
  });
}
