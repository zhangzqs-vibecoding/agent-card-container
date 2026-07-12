import 'dart:async';
import 'dart:ui';

import 'package:agent_card_desktop/src/app/agent_card_app.dart';
import 'package:agent_card_desktop/src/app/desktop_bootstrap.dart';
import 'package:agent_card_desktop/src/adapters/desktop_main_window_lifecycle.dart';
import 'package:agent_card_desktop/src/cloud/cloud_settings_controller.dart';
import 'package:flutter/widgets.dart';

import 'src/surfaces/surface_window_launcher.dart';

late final DesktopRuntime desktopRuntime;
late final AppLifecycleListener desktopLifecycle;
DesktopMainWindowLifecycle? desktopMainWindowLifecycle;
late final CloudSettingsController cloudSettingsController;

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  if (await SurfaceWindowLauncher.tryLaunch()) {
    return;
  }
  desktopRuntime = await DesktopBootstrap.start();
  cloudSettingsController = CloudSettingsController(
    desktopRuntime.cloudSettingsService,
  );
  await cloudSettingsController.initialize();
  await desktopRuntime.initializePlatformSurfaces();
  final mainWindowLifecycle = DesktopMainWindowLifecycle(
    closeRuntime: desktopRuntime.close,
  );
  try {
    await mainWindowLifecycle.start();
    desktopMainWindowLifecycle = mainWindowLifecycle;
  } catch (_) {
    try {
      await mainWindowLifecycle.dispose();
    } catch (_) {
      // A missing tray integration must not prevent the ordinary window.
    }
  }
  desktopLifecycle = AppLifecycleListener(
    onResume: () => unawaited(desktopRuntime.environmentMonitor.refresh()),
    onExitRequested: () async {
      await desktopMainWindowLifecycle?.dispose();
      cloudSettingsController.dispose();
      await desktopRuntime.close();
      return AppExitResponse.exit;
    },
  );
  runApp(
    AgentCardApp(
      runtimePort: desktopRuntime.runtimeServer.port,
      workspaceCards: desktopRuntime.workspaceCards,
      workspaceController: desktopRuntime.workspaceController,
      agentStudioController: desktopRuntime.agentStudioController,
      cardCatalogController: desktopRuntime.cardCatalogController,
      cloudSettingsController: cloudSettingsController,
      onNativeCardStateChanged: desktopRuntime.database.replaceState,
      onDetachCard: desktopRuntime.surfaceCoordinator.detach,
      onMoveCardToOverlay: (instanceId, placement) =>
          desktopRuntime.surfaceCoordinator.moveToOverlay(
            instanceId,
            monitorId: 'primary',
            placement: placement,
          ),
      permissionRequests: desktopRuntime.permissionRequests,
      onExportDiagnostics: () async =>
          desktopRuntime.exportDiagnosticBundle().path,
    ),
  );
}
