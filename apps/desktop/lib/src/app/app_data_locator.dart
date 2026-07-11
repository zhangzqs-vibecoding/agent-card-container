import 'dart:io';

abstract final class AppDataLocator {
  static Directory resolve({
    String? operatingSystem,
    Map<String, String>? environment,
  }) {
    final os = operatingSystem ?? Platform.operatingSystem;
    final env = environment ?? Platform.environment;
    final String path;
    switch (os) {
      case 'windows':
        final base = env['LOCALAPPDATA'];
        if (base == null || base.isEmpty) {
          throw StateError('LOCALAPPDATA is unavailable');
        }
        path = _join(base, 'AgentCard');
      case 'macos':
        final home = env['HOME'];
        if (home == null || home.isEmpty) {
          throw StateError('HOME is unavailable');
        }
        path = _join(home, 'Library', 'Application Support', 'AgentCard');
      default:
        final xdg = env['XDG_DATA_HOME'];
        final home = env['HOME'];
        if (xdg != null && xdg.isNotEmpty) {
          path = _join(xdg, 'agent-card');
        } else if (home != null && home.isNotEmpty) {
          path = _join(home, '.local', 'share', 'agent-card');
        } else {
          throw StateError('no user data directory is available');
        }
    }
    return Directory(path);
  }
}

String _join(String first, String second, [String? third, String? fourth]) {
  return [
    first,
    second,
    third,
    fourth,
  ].whereType<String>().join(Platform.pathSeparator);
}
