import 'dart:async';
import 'dart:io';

import 'package:tray_manager/tray_manager.dart';
import 'package:window_manager/window_manager.dart';

import '../app/main_window_lifecycle.dart';

class DesktopMainWindowLifecycle with WindowListener, TrayListener {
  DesktopMainWindowLifecycle({required Future<void> Function() closeRuntime})
    : _coordinator = MainWindowLifecycleCoordinator(
        window: const _DesktopMainWindowPort(),
        tray: const _DesktopTrayPort(),
        closeRuntime: closeRuntime,
      );

  final MainWindowLifecycleCoordinator _coordinator;
  var _disposed = false;

  bool get quitting => _coordinator.quitting;

  Future<void> start() async {
    await windowManager.ensureInitialized();
    windowManager.addListener(this);
    trayManager.addListener(this);
    try {
      await _coordinator.start();
    } catch (_) {
      windowManager.removeListener(this);
      trayManager.removeListener(this);
      rethrow;
    }
  }

  @override
  void onWindowClose() {
    unawaited(_coordinator.handleWindowClose());
  }

  @override
  void onTrayIconMouseDown() {
    unawaited(_coordinator.handleShow());
  }

  @override
  void onTrayIconRightMouseDown() {
    unawaited(trayManager.popUpContextMenu());
  }

  @override
  void onTrayMenuItemClick(MenuItem menuItem) {
    switch (menuItem.key) {
      case 'show_window':
        unawaited(_coordinator.handleShow());
      case 'quit_app':
        _removeListeners();
        unawaited(_coordinator.handleQuit());
    }
  }

  Future<void> dispose() async {
    if (_disposed) return;
    _removeListeners();
    await trayManager.destroy();
  }

  void _removeListeners() {
    if (_disposed) return;
    _disposed = true;
    windowManager.removeListener(this);
    trayManager.removeListener(this);
  }
}

class _DesktopMainWindowPort implements MainWindowPort {
  const _DesktopMainWindowPort();

  @override
  Future<void> destroy() => windowManager.destroy();

  @override
  Future<void> focus() => windowManager.focus();

  @override
  Future<void> hide() => windowManager.hide();

  @override
  Future<void> setPreventClose(bool value) =>
      windowManager.setPreventClose(value);

  @override
  Future<void> show() async {
    if (await windowManager.isMinimized()) await windowManager.restore();
    await windowManager.show();
  }
}

class _DesktopTrayPort implements MainTrayPort {
  const _DesktopTrayPort();

  @override
  Future<void> destroy() => trayManager.destroy();

  @override
  Future<void> initialize() async {
    await trayManager.setIcon(
      Platform.isWindows ? 'assets/tray_icon.ico' : 'assets/tray_icon.png',
      isTemplate: Platform.isMacOS,
    );
    await trayManager.setToolTip('Agent Card');
    await trayManager.setContextMenu(
      Menu(
        items: [
          MenuItem(key: 'show_window', label: '显示主窗口'),
          MenuItem.separator(),
          MenuItem(key: 'quit_app', label: '退出 Agent Card'),
        ],
      ),
    );
  }
}
