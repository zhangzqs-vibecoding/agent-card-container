import 'dart:async';
import 'dart:ui';

import 'package:agent_card_desktop/src/app/agent_card_app.dart';
import 'package:agent_card_desktop/src/app/desktop_bootstrap.dart';
import 'package:flutter/widgets.dart';

import 'src/surfaces/surface_window_launcher.dart';

late final DesktopRuntime desktopRuntime;
late final AppLifecycleListener desktopLifecycle;

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  if (await SurfaceWindowLauncher.tryLaunch()) {
    return;
  }
  desktopRuntime = await DesktopBootstrap.start();
  desktopLifecycle = AppLifecycleListener(
    onResume: () => unawaited(desktopRuntime.environmentMonitor.refresh()),
    onExitRequested: () async {
      await desktopRuntime.close();
      return AppExitResponse.exit;
    },
  );
  await desktopRuntime.initializePlatformSurfaces();
  runApp(
    AgentCardApp(
      runtimePort: desktopRuntime.runtimeServer.port,
      workspaceCards: desktopRuntime.workspaceCards,
      workspaceController: desktopRuntime.workspaceController,
      agentStudioController: desktopRuntime.agentStudioController,
      cardCatalogController: desktopRuntime.cardCatalogController,
      onNativeCardStateChanged: desktopRuntime.database.replaceState,
      onDetachCard: desktopRuntime.surfaceCoordinator.detach,
      onMoveCardToOverlay: (instanceId, placement) =>
          desktopRuntime.surfaceCoordinator.moveToOverlay(
            instanceId,
            monitorId: 'primary',
            placement: placement,
          ),
      permissionRequests: desktopRuntime.permissionRequests,
    ),
  );
}
