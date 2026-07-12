abstract interface class MainWindowPort {
  Future<void> setPreventClose(bool value);

  Future<void> hide();

  Future<void> show();

  Future<void> focus();

  Future<void> destroy();
}

abstract interface class MainTrayPort {
  Future<void> initialize();

  Future<void> destroy();
}

class MainWindowLifecycleCoordinator {
  MainWindowLifecycleCoordinator({
    required this.window,
    required this.tray,
    required this.closeRuntime,
  });

  final MainWindowPort window;
  final MainTrayPort tray;
  final Future<void> Function() closeRuntime;
  bool _quitting = false;

  bool get quitting => _quitting;

  Future<void> start() async {
    await tray.initialize();
    await window.setPreventClose(true);
  }

  Future<void> handleWindowClose() async {
    if (_quitting) return;
    await window.hide();
  }

  Future<void> handleShow() async {
    if (_quitting) return;
    await window.show();
    await window.focus();
  }

  Future<void> handleQuit() async {
    if (_quitting) return;
    _quitting = true;
    await tray.destroy();
    await closeRuntime();
    await window.setPreventClose(false);
    await window.destroy();
  }
}
