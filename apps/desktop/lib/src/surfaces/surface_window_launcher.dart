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
          await windowManager.setPreventClose(false);
          await windowManager.close();
          return null;
        default:
          throw MissingPluginException('unknown surface window method');
      }
    });
    await windowManager.ensureInitialized();
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
    runApp(SurfaceWindowApp(model: model));
    return true;
  }
}
