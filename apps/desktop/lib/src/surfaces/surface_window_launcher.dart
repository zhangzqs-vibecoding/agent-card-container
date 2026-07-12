import 'dart:async';

import 'package:desktop_multi_window/desktop_multi_window.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:screen_retriever/screen_retriever.dart';
import 'package:window_manager/window_manager.dart';

import '../capabilities/capability.dart';
import 'surface_window.dart';
import 'surface_bridge.dart';

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
        case 'surface.setAlwaysOnTop':
          if (call.arguments is! bool) {
            throw const FormatException('invalid always-on-top command');
          }
          await windowManager.setAlwaysOnTop(call.arguments! as bool);
          return null;
        case 'surface.requestAttention':
          await windowManager.show();
          await windowManager.focus();
          return null;
        default:
          throw MissingPluginException('unknown surface window method');
      }
    });
    await windowManager.ensureInitialized();
    windowManager.addListener(closeListener);
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
        onCapabilityInvocation: (instanceId, method, params) async {
          final owner = WindowController.fromWindowId(arguments.ownerWindowId);
          final response = await owner.invokeMethod<Object?>(
            'surface.bridge',
            SurfaceBridgeMessage(
              type: SurfaceBridgeMessageType.invokeCapability,
              windowId: controller.windowId,
              instanceId: instanceId,
              payload: {'method': method, 'params': params},
            ).toJson(),
          );
          if (response is Map && response['ok'] == true) {
            return response['result'];
          }
          if (response is Map &&
              response['errorCode'] is String &&
              response['message'] is String) {
            final code = CapabilityErrorCode.values
                .where((candidate) => candidate.name == response['errorCode'])
                .firstOrNull;
            if (code != null) {
              throw CapabilityException(code, response['message']! as String);
            }
          }
          throw const CapabilityException(
            CapabilityErrorCode.capabilityUnavailable,
            'invalid capability bridge response',
          );
        },
        onStateChanged: (instanceId, state) async {
          final owner = WindowController.fromWindowId(arguments.ownerWindowId);
          await owner.invokeMethod<Object?>(
            'surface.bridge',
            SurfaceBridgeMessage(
              type: SurfaceBridgeMessageType.stateChanged,
              windowId: controller.windowId,
              instanceId: instanceId,
              payload: {'state': state},
            ).toJson(),
          );
        },
        onCodeCardLaunch: (instanceId, succeeded) async {
          final owner = WindowController.fromWindowId(arguments.ownerWindowId);
          await owner.invokeMethod<Object?>(
            'surface.bridge',
            SurfaceBridgeMessage(
              type: SurfaceBridgeMessageType.hostEvent,
              windowId: controller.windowId,
              instanceId: instanceId,
              payload: {
                'event': succeeded
                    ? 'codeCardLaunchSucceeded'
                    : 'codeCardLaunchFailed',
              },
            ).toJson(),
          );
        },
        onRuntimeVisibilityChanged: (instanceId, visible) async {
          final owner = WindowController.fromWindowId(arguments.ownerWindowId);
          await owner.invokeMethod<Object?>(
            'surface.bridge',
            SurfaceBridgeMessage(
              type: SurfaceBridgeMessageType.hostEvent,
              windowId: controller.windowId,
              instanceId: instanceId,
              payload: {
                'event': visible ? 'runtimeResumed' : 'runtimeSuspended',
              },
            ).toJson(),
          );
        },
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
    if (arguments.surfaceType == 'detached' && !_closing) {
      unawaited(_requestDockAndClose());
    }
  }

  @override
  void onWindowFocus() {
    unawaited(_sendFocus(focused: true));
  }

  @override
  void onWindowBlur() {
    unawaited(_sendFocus(focused: false));
  }

  @override
  void onWindowMoved() {
    unawaited(_sendPlacement());
  }

  @override
  void onWindowResized() {
    unawaited(_sendPlacement());
  }

  Future<void> _sendFocus({required bool focused}) async {
    await _send(
      buildSurfaceFocusMessages(
        arguments,
        controller.windowId,
        focused: focused,
      ),
    );
  }

  Future<void> _sendPlacement() async {
    final position = await windowManager.getPosition();
    final size = await windowManager.getSize();
    final bounds = position & size;
    await _send(
      buildSurfacePlacementMessages(
        arguments,
        controller.windowId,
        bounds,
        monitorId: await _monitorId(bounds),
      ),
    );
  }

  Future<String?> _monitorId(Rect windowBounds) async {
    final center = windowBounds.center;
    final displays = await screenRetriever.getAllDisplays();
    for (final display in displays) {
      final position = display.visiblePosition ?? Offset.zero;
      final size = display.visibleSize ?? display.size;
      if ((position & size).contains(center)) {
        return display.id;
      }
    }
    return null;
  }

  Future<void> _send(Iterable<SurfaceBridgeMessage> messages) async {
    final owner = WindowController.fromWindowId(arguments.ownerWindowId);
    for (final message in messages) {
      try {
        await owner.invokeMethod<Object?>('surface.bridge', message.toJson());
      } catch (_) {
        // Window telemetry is best effort; the main engine remains authoritative.
      }
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
