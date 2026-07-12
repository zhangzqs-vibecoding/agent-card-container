import 'package:agent_card_desktop/src/capabilities/capability.dart';
import 'package:agent_card_desktop/src/capabilities/permission_prompt_host.dart';
import 'package:agent_card_desktop/src/capabilities/permission_request_controller.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  testWidgets('shows and resolves a first-use capability prompt', (
    tester,
  ) async {
    final controller = PermissionRequestController();
    addTearDown(controller.dispose);
    await tester.pumpWidget(
      MaterialApp(
        home: PermissionPromptHost(
          controller: controller,
          child: const Text('workspace'),
        ),
      ),
    );
    final context = CardContext(
      instanceId: 'instance-1',
      cardId: 'weather-card',
      versionId: 'version-1',
      declaredCapabilities: const {'network.fetch'},
      networkDomains: const {'api.example.com'},
    );

    final result = controller.requestGrant(context, 'network.fetch', {
      'url': 'https://api.example.com/weather',
    });
    await tester.pump();

    expect(find.textContaining('weather-card'), findsOneWidget);
    expect(find.textContaining('api.example.com'), findsOneWidget);
    await tester.tap(find.byKey(const Key('approve-capability')));
    await tester.pump();

    expect((await result)?.capability, 'network.fetch');
    expect(find.byKey(const Key('approve-capability')), findsNothing);
  });
}
