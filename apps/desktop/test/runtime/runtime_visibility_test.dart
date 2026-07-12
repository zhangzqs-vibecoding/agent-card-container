import 'package:agent_card_desktop/src/runtime/runtime_visibility.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  testWidgets('tracks desktop window lifecycle visibility', (tester) async {
    addTearDown(() async {
      tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.resumed);
    });
    await tester.pumpWidget(
      MaterialApp(
        home: RuntimeVisibilityBuilder(
          builder: (context, visible) => Text(visible ? 'visible' : 'hidden'),
        ),
      ),
    );
    expect(find.text('visible'), findsOneWidget);

    tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.hidden);
    await tester.pump();
    expect(find.text('hidden'), findsOneWidget);

    tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.resumed);
    await tester.pump();
    expect(find.text('visible'), findsOneWidget);
  });
}
