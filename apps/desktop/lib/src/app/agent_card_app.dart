import 'package:agent_card_desktop/src/workspace/workspace_screen.dart';
import 'package:agent_card_desktop/src/workspace/workspace_card.dart';
import 'package:flutter/material.dart';

class AgentCardApp extends StatelessWidget {
  const AgentCardApp({
    super.key,
    this.runtimePort,
    this.workspaceCards = const [],
  });

  final int? runtimePort;
  final List<WorkspaceCard> workspaceCards;

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'Agent Card',
      debugShowCheckedModeBanner: false,
      themeMode: ThemeMode.dark,
      theme: _theme(Brightness.light),
      darkTheme: _theme(Brightness.dark),
      home: WorkspaceScreen(
        runtimePort: runtimePort,
        workspaceCards: workspaceCards,
      ),
    );
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
