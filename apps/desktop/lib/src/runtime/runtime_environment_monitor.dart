import 'dart:async';
import 'dart:io';

import 'local_runtime_server.dart';

typedef RuntimeOnlineProbe = Future<bool> Function();

class RuntimeEnvironmentMonitor {
  RuntimeEnvironmentMonitor({
    required this.runtimeServer,
    required this.onlineProbe,
    this.localeProvider,
    this.themeProvider,
    this.interval = const Duration(seconds: 15),
  });

  final LocalRuntimeServer runtimeServer;
  final RuntimeOnlineProbe onlineProbe;
  final String Function()? localeProvider;
  final String Function()? themeProvider;
  final Duration interval;
  Timer? _timer;
  Future<void>? _refreshing;

  Future<void> start() async {
    if (_timer != null) return;
    await refresh();
    _timer = Timer.periodic(interval, (_) => unawaited(refresh()));
  }

  Future<void> refresh() {
    final running = _refreshing;
    if (running != null) return running;
    final operation = _refresh();
    _refreshing = operation;
    return operation.whenComplete(() {
      if (identical(_refreshing, operation)) _refreshing = null;
    });
  }

  Future<void> _refresh() async {
    try {
      runtimeServer.updateEnvironment(
        locale: localeProvider?.call(),
        theme: themeProvider?.call(),
        online: await onlineProbe(),
      );
    } on SocketException {
      runtimeServer.updateEnvironment(online: false);
    }
  }

  void dispose() {
    _timer?.cancel();
    _timer = null;
  }
}

Future<bool> hasUsableNetworkInterface() async {
  try {
    final interfaces = await NetworkInterface.list(
      includeLoopback: false,
      includeLinkLocal: false,
    );
    return interfaces.any((interface) => interface.addresses.isNotEmpty);
  } on SocketException {
    return false;
  }
}

String normalizeRuntimeLocale(String locale) {
  final base = locale.split('.').first.split('@').first.trim();
  if (base.isEmpty) return 'en-US';
  return base.replaceAll('_', '-');
}
