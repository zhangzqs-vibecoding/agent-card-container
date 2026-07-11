import 'package:agent_card_desktop/src/app/agent_card_app.dart';
import 'package:agent_card_desktop/src/runtime/local_runtime_server.dart';
import 'package:flutter/widgets.dart';

late final LocalRuntimeServer runtimeServer;

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  runtimeServer = await LocalRuntimeServer.start();
  runApp(AgentCardApp(runtimePort: runtimeServer.port));
}
