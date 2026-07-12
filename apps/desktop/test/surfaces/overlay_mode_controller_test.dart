import 'package:agent_card_desktop/src/surfaces/overlay_mode_controller.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test('global restore shortcut exits click-through display mode', () async {
    final shortcut = _FakeRestoreShortcut();
    final surfaces = _FakeOverlaySurfaces();
    final controller = OverlayModeController(
      shortcut: shortcut,
      surfaces: surfaces,
    );

    await controller.initialize();
    await controller.enterDisplayMode();
    expect(controller.editing, isFalse);
    expect(surfaces.editingChanges, [false]);

    await shortcut.trigger();

    expect(controller.editing, isTrue);
    expect(surfaces.editingChanges, [false, true]);
    await controller.dispose();
    expect(shortcut.unregistered, isTrue);
  });
}

class _FakeRestoreShortcut implements OverlayRestoreShortcut {
  Future<void> Function()? handler;
  var unregistered = false;

  Future<void> trigger() => handler!();

  @override
  Future<void> register(Future<void> Function() restoreEditing) async {
    handler = restoreEditing;
  }

  @override
  Future<void> unregister() async {
    unregistered = true;
  }
}

class _FakeOverlaySurfaces implements OverlaySurfaceModePort {
  final editingChanges = <bool>[];

  @override
  Future<void> setEditing(bool editing) async {
    editingChanges.add(editing);
  }
}
