import 'dart:convert';

import 'package:desktop_multi_window/desktop_multi_window.dart';

import 'multi_window_backend.dart';

class DesktopMultiWindowDriver implements MultiWindowDriver {
  final Map<String, WindowController> _windows = {};

  @override
  Future<String> create(SurfaceWindowConfiguration configuration) async {
    final owner = await WindowController.fromCurrentEngine();
    final arguments =
        jsonDecode(configuration.arguments) as Map<String, Object?>
          ..['ownerWindowId'] = owner.windowId;
    final controller = await WindowController.create(
      WindowConfiguration(
        arguments: jsonEncode(arguments),
        hiddenAtLaunch: configuration.hiddenAtLaunch,
      ),
    );
    _windows[controller.windowId] = controller;
    return controller.windowId;
  }

  @override
  Future<void> show(String windowId) => _controller(windowId).show();

  @override
  Future<void> updateInstances(
    String windowId,
    List<String> instanceIds,
  ) async {
    await _controller(
      windowId,
    ).invokeMethod<void>('surface.updateInstances', instanceIds);
  }

  @override
  Future<void> close(String windowId) async {
    final controller = _windows.remove(windowId);
    await controller?.invokeMethod<void>('surface.close');
  }

  WindowController _controller(String windowId) {
    final controller = _windows[windowId];
    if (controller == null) {
      throw StateError('surface window is not registered');
    }
    return controller;
  }
}
