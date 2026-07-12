import 'dart:async';

import 'package:flutter/services.dart';
import 'package:hotkey_manager/hotkey_manager.dart';

import '../surfaces/overlay_mode_controller.dart';

class HotKeyOverlayRestoreShortcut implements OverlayRestoreShortcut {
  HotKeyOverlayRestoreShortcut()
    : _hotKey = HotKey(
        key: PhysicalKeyboardKey.f12,
        modifiers: const [HotKeyModifier.control, HotKeyModifier.shift],
        scope: HotKeyScope.system,
      );

  final HotKey _hotKey;
  var _registered = false;

  @override
  Future<void> register(Future<void> Function() restoreEditing) async {
    if (_registered) {
      return;
    }
    await hotKeyManager.register(
      _hotKey,
      keyDownHandler: (_) => unawaited(restoreEditing()),
    );
    _registered = true;
  }

  @override
  Future<void> unregister() async {
    if (!_registered) {
      return;
    }
    await hotKeyManager.unregister(_hotKey);
    _registered = false;
  }
}
