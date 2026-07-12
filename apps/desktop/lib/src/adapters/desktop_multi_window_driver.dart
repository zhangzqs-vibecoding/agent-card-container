import 'dart:convert';

import 'package:desktop_multi_window/desktop_multi_window.dart';
import 'package:flutter/services.dart';

import 'multi_window_backend.dart';
import '../surfaces/surface_window.dart';

class DesktopMultiWindowDriver implements MultiWindowDriver {
  final Map<String, WindowController> _windows = {};

  @override
  Future<void> setBridgeHandler(
    Future<Object?> Function(Map<String, Object?> message) handler,
  ) async {
    final owner = await WindowController.fromCurrentEngine();
    await owner.setWindowMethodHandler((call) async {
      if (call.method != 'surface.bridge' || call.arguments is! Map) {
        throw MissingPluginException('unknown main window method');
      }
      return handler((call.arguments as Map).cast<String, Object?>());
    });
  }

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
  Future<void> updateCards(
    String windowId,
    List<SurfaceCardSnapshot> cards,
  ) async {
    await _controller(windowId).invokeMethod<void>(
      'surface.updateCards',
      cards.map((card) => card.toJson()).toList(),
    );
  }

  @override
  Future<void> setOverlayEditing(String windowId, bool editing) async {
    await _controller(
      windowId,
    ).invokeMethod<void>('surface.setOverlayEditing', editing);
  }

  @override
  Future<void> setAlwaysOnTop(String windowId, bool value) async {
    await _controller(
      windowId,
    ).invokeMethod<void>('surface.setAlwaysOnTop', value);
  }

  @override
  Future<void> requestAttention(String windowId) async {
    await _controller(windowId).invokeMethod<void>('surface.requestAttention');
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
