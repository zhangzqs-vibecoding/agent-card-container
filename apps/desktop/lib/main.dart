import 'package:agent_card_desktop/src/app/agent_card_app.dart';
import 'package:agent_card_desktop/src/app/desktop_bootstrap.dart';
import 'package:flutter/widgets.dart';

late final DesktopRuntime desktopRuntime;

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  desktopRuntime = await DesktopBootstrap.start();
  runApp(
    AgentCardApp(
      runtimePort: desktopRuntime.runtimeServer.port,
      workspaceCards: desktopRuntime.workspaceCards,
    ),
  );
}
