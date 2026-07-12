import 'package:agent_card_desktop/src/agent_studio/agent_studio_controller.dart';
import 'package:agent_card_desktop/src/capabilities/permission_prompt_host.dart';
import 'package:agent_card_desktop/src/capabilities/permission_request_controller.dart';
import 'package:agent_card_desktop/src/cloud/card_catalog_controller.dart';
import 'package:agent_card_desktop/src/cloud/cloud_settings_controller.dart';
import 'package:agent_card_desktop/src/runtime/runtime_visibility.dart';
import 'package:agent_card_desktop/src/workspace/workspace_screen.dart';
import 'package:agent_card_desktop/src/workspace/workspace_card.dart';
import 'package:agent_card_desktop/src/workspace/workspace_controller.dart';
import 'package:flutter/material.dart';

import '../adapters/in_app_webview_port.dart';

class AgentCardApp extends StatelessWidget {
  const AgentCardApp({
    super.key,
    this.runtimePort,
    this.workspaceCards = const [],
    this.agentStudioController,
    this.workspaceController,
    this.cardCatalogController,
    this.cloudSettingsController,
    this.onNativeCardStateChanged,
    this.onDetachCard,
    this.onMoveCardToOverlay,
    this.permissionRequests,
    this.onExportDiagnostics,
    this.webViewPortFactory,
  });

  final int? runtimePort;
  final List<WorkspaceCard> workspaceCards;
  final AgentStudioController? agentStudioController;
  final WorkspaceController? workspaceController;
  final CardCatalogController? cardCatalogController;
  final CloudSettingsController? cloudSettingsController;
  final NativeCardStateChanged? onNativeCardStateChanged;
  final CardSurfaceAction? onDetachCard;
  final CardSurfaceAction? onMoveCardToOverlay;
  final PermissionRequestController? permissionRequests;
  final Future<String> Function()? onExportDiagnostics;
  final InAppWebViewPortFactory? webViewPortFactory;

  @override
  Widget build(BuildContext context) {
    return RuntimeVisibilityBuilder(
      builder: (context, runtimeVisible) => MaterialApp(
        title: 'Agent Card',
        debugShowCheckedModeBanner: false,
        themeMode: ThemeMode.dark,
        theme: _theme(Brightness.light),
        darkTheme: _theme(Brightness.dark),
        home: _home(runtimeVisible),
      ),
    );
  }

  Widget _home(bool runtimeVisible) {
    final workspace = WorkspaceScreen(
      runtimeVisible: runtimeVisible,
      runtimePort: runtimePort,
      workspaceCards: workspaceCards,
      agentStudioController: agentStudioController,
      workspaceController: workspaceController,
      cardCatalogController: cardCatalogController,
      cloudSettingsController: cloudSettingsController,
      onNativeCardStateChanged: onNativeCardStateChanged,
      onDetachCard: onDetachCard,
      onMoveCardToOverlay: onMoveCardToOverlay,
      onExportDiagnostics: onExportDiagnostics,
      webViewPortFactory: webViewPortFactory,
    );
    final permissions = permissionRequests;
    return permissions == null
        ? workspace
        : PermissionPromptHost(controller: permissions, child: workspace);
  }

  ThemeData _theme(Brightness brightness) {
    final isDark = brightness == Brightness.dark;
    final colorScheme = ColorScheme.fromSeed(
      seedColor: const Color(0xFFE8FF47),
      brightness: brightness,
      surface: isDark ? const Color(0xFF111516) : const Color(0xFFF5F3EC),
    );
    return ThemeData(
      useMaterial3: true,
      brightness: brightness,
      colorScheme: colorScheme,
      scaffoldBackgroundColor: isDark
          ? const Color(0xFF0B0E0F)
          : const Color(0xFFEAE8E1),
      fontFamily: 'Segoe UI Variable',
      textTheme: ThemeData(brightness: brightness).textTheme.apply(
        bodyColor: isDark ? const Color(0xFFE8ECE8) : const Color(0xFF171A19),
        displayColor: isDark
            ? const Color(0xFFF7FAF5)
            : const Color(0xFF111312),
      ),
      dividerColor: isDark ? const Color(0xFF2A3030) : const Color(0xFFD3D0C7),
    );
  }
}
