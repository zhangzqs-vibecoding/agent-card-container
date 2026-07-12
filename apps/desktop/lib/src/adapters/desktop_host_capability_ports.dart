import 'dart:io';

import 'package:flutter/services.dart';
import 'package:local_notifier/local_notifier.dart';
import 'package:url_launcher/url_launcher.dart';

import '../capabilities/host_capability_handlers.dart';

class FlutterClipboardCapabilityPort implements ClipboardCapabilityPort {
  @override
  Future<String?> readText() async {
    return (await Clipboard.getData(Clipboard.kTextPlain))?.text;
  }

  @override
  Future<void> writeText(String text) {
    return Clipboard.setData(ClipboardData(text: text));
  }
}

class UrlLauncherExternalUrlPort implements ExternalUrlCapabilityPort {
  @override
  Future<bool> open(Uri uri) {
    return launchUrl(uri, mode: LaunchMode.externalApplication);
  }
}

class LocalNotifierCapabilityPort implements NotificationCapabilityPort {
  var _initialized = false;

  Future<void> initialize() async {
    if (_initialized) {
      return;
    }
    await localNotifier.setup(
      appName: 'Agent Card',
      shortcutPolicy: ShortcutPolicy.requireCreate,
    );
    _initialized = true;
  }

  @override
  Future<void> show({required String title, required String body}) async {
    await initialize();
    await LocalNotification(title: title, body: body).show();
  }
}

class ProcessSystemMetricsPort implements SystemMetricsCapabilityPort {
  @override
  Future<Map<String, Object?>> read() async {
    return {
      'residentBytes': ProcessInfo.currentRss,
      'maxResidentBytes': ProcessInfo.maxRss,
      'processorCount': Platform.numberOfProcessors,
      'timestamp': DateTime.now().toUtc().toIso8601String(),
    };
  }
}
