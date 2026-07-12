import 'dart:async';

import 'package:desktop_multi_window/desktop_multi_window.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:window_manager/window_manager.dart';

import 'surface_window.dart';

abstract final class SurfaceWindowLauncher {
  static Future<bool> tryLaunch() async {
    final controller = await WindowController.fromCurrentEngine();
    final arguments = SurfaceWindowArguments.tryParse(controller.arguments);
    if (arguments == null) {
      return false;
    }
    final model = SurfaceWindowModel(arguments);
    final closeListener = _SurfaceCloseListener(controller, arguments);
    await controller.setWindowMethodHandler((call) async {
      switch (call.method) {
        case 'surface.updateCards':
          final cards = (call.arguments as List)
              .map(
                (value) => SurfaceCardSnapshot.fromJson(
                  (value as Map).cast<String, Object?>(),
                ),
              )
              .toList(growable: false);
          model.updateCards(cards);
          return null;
        case 'surface.close':
          await closeListener.closeFromHost();
          return null;
        case 'surface.setOverlayEditing':
          if (arguments.surfaceType != 'overlay' || call.arguments is! bool) {
            throw const FormatException('invalid overlay mode command');
          }
          await windowManager.setIgnoreMouseEvents(
            !(call.arguments! as bool),
            forward: true,
          );
          return null;
        default:
          throw MissingPluginException('unknown surface window method');
      }
    });
    await windowManager.ensureInitialized();
    if (arguments.surfaceType == 'detached') {
      windowManager.addListener(closeListener);
    }
    final overlay = arguments.surfaceType == 'overlay';
    final bounds = arguments.bounds;
    await windowManager.waitUntilReadyToShow(
      WindowOptions(
        size: bounds?.size ?? const Size(480, 320),
        center: bounds == null,
        alwaysOnTop: arguments.alwaysOnTop,
        backgroundColor: overlay ? Colors.transparent : const Color(0xFF0B0E0F),
        skipTaskbar: overlay,
        title: 'Agent Card',
        titleBarStyle: overlay ? TitleBarStyle.hidden : TitleBarStyle.normal,
        windowButtonVisibility: !overlay,
      ),
      () async {
        if (bounds != null) {
          await windowManager.setPosition(bounds.topLeft);
        }
        await windowManager.setPreventClose(true);
        await windowManager.show();
        await windowManager.focus();
      },
    );
    runApp(
      SurfaceWindowApp(
        model: model,
        onEnterOverlayDisplayMode: arguments.surfaceType == 'overlay'
            ? () async {
                final owner = WindowController.fromWindowId(
                  arguments.ownerWindowId,
                );
                await owner.invokeMethod<Object?>(
                  'surface.bridge',
                  buildOverlayDisplayRequest(
                    arguments,
                    controller.windowId,
                  ).toJson(),
                );
              }
            : null,
      ),
    );
    return true;
  }
}

class _SurfaceCloseListener with WindowListener {
  _SurfaceCloseListener(this.controller, this.arguments);

  final WindowController controller;
  final SurfaceWindowArguments arguments;
  var _closing = false;

  @override
  void onWindowClose() {
    if (!_closing) {
      unawaited(_requestDockAndClose());
    }
  }

  Future<void> _requestDockAndClose() async {
    _closing = true;
    try {
      final owner = WindowController.fromWindowId(arguments.ownerWindowId);
      final response = await owner.invokeMethod<Object?>(
        'surface.bridge',
        buildSurfaceCloseRequest(arguments, controller.windowId).toJson(),
      );
      if (response is Map && response['allowClose'] == true) {
        await closeFromHost();
      } else {
        _closing = false;
      }
    } catch (_) {
      _closing = false;
    }
  }

  Future<void> closeFromHost() async {
    _closing = true;
    await windowManager.setPreventClose(false);
    await windowManager.close();
  }
}
