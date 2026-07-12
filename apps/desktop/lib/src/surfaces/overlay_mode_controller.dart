abstract interface class OverlayRestoreShortcut {
  Future<void> register(Future<void> Function() restoreEditing);

  Future<void> unregister();
}

abstract interface class OverlaySurfaceModePort {
  Future<void> setEditing(bool editing);
}

class OverlayModeController {
  OverlayModeController({required this.shortcut, required this.surfaces});

  final OverlayRestoreShortcut shortcut;
  final OverlaySurfaceModePort surfaces;
  var _editing = true;
  var _initialized = false;

  bool get editing => _editing;

  Future<void> initialize() async {
    if (_initialized) {
      return;
    }
    _initialized = true;
    await shortcut.register(restoreEditing);
  }

  Future<void> enterDisplayMode() async {
    await surfaces.setEditing(false);
    _editing = false;
  }

  Future<void> restoreEditing() async {
    await surfaces.setEditing(true);
    _editing = true;
  }

  Future<void> dispose() async {
    if (!_initialized) {
      return;
    }
    _initialized = false;
    await shortcut.unregister();
  }
}
