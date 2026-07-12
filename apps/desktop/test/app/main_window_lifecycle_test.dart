import 'package:agent_card_desktop/src/app/main_window_lifecycle.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test('window close hides the main window without closing runtime', () async {
    final window = _FakeWindowPort();
    final tray = _FakeTrayPort();
    var runtimeCloses = 0;
    final lifecycle = MainWindowLifecycleCoordinator(
      window: window,
      tray: tray,
      closeRuntime: () async => runtimeCloses++,
    );

    await lifecycle.start();
    await lifecycle.handleWindowClose();

    expect(window.calls, ['prevent:true', 'hide']);
    expect(tray.calls, ['initialize']);
    expect(runtimeCloses, 0);
  });

  test('tray show restores focus and explicit quit cleans up once', () async {
    final window = _FakeWindowPort();
    final tray = _FakeTrayPort();
    var runtimeCloses = 0;
    final lifecycle = MainWindowLifecycleCoordinator(
      window: window,
      tray: tray,
      closeRuntime: () async => runtimeCloses++,
    );
    await lifecycle.start();

    await lifecycle.handleShow();
    await lifecycle.handleQuit();
    await lifecycle.handleQuit();

    expect(window.calls, [
      'prevent:true',
      'show',
      'focus',
      'prevent:false',
      'destroy',
    ]);
    expect(tray.calls, ['initialize', 'destroy']);
    expect(runtimeCloses, 1);
    expect(lifecycle.quitting, isTrue);
  });
}

class _FakeWindowPort implements MainWindowPort {
  final calls = <String>[];

  @override
  Future<void> destroy() async => calls.add('destroy');

  @override
  Future<void> focus() async => calls.add('focus');

  @override
  Future<void> hide() async => calls.add('hide');

  @override
  Future<void> setPreventClose(bool value) async => calls.add('prevent:$value');

  @override
  Future<void> show() async => calls.add('show');
}

class _FakeTrayPort implements MainTrayPort {
  final calls = <String>[];

  @override
  Future<void> destroy() async => calls.add('destroy');

  @override
  Future<void> initialize() async => calls.add('initialize');
}
