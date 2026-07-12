import 'dart:convert';
import 'dart:io';

const _required = {
  'windows': {
    'webView2Embedded',
    'randomLocalhostOrigin',
    'originStorageIsolation',
    'offlineRestart',
    'chineseIme',
    'mixedDpi150',
    'overlayTransparency',
    'clickThroughRecovery',
    'detachedWindow',
    'multiMonitorUnplug',
    'sleepAndLock',
    'oneThreeTenWindows',
    'oneFiveTwentyCards',
    'tamperedArtifactRejected',
    'privateNetworkRejected',
  },
  'macos': {
    'wkWebViewEmbedded',
    'loopbackRuntime',
    'originStorageIsolation',
    'ordinaryWindow',
    'detachedWindow',
    'appSignature',
    'sandboxEntitlements',
  },
  'linux': {'ordinaryWindowBuild', 'unitAndWidgetTests'},
};

void main(List<String> arguments) {
  final options = _options(arguments);
  final platform = options['platform'];
  final expectedCommit = options['commit'];
  final evidencePath = options['evidence'];
  final required = _required[platform];
  if (required == null || expectedCommit == null || evidencePath == null) {
    stderr.writeln('invalid evidence validator arguments');
    exitCode = 64;
    return;
  }
  try {
    final decoded = jsonDecode(File(evidencePath).readAsStringSync());
    if (decoded is! Map<String, Object?> ||
        decoded['schemaVersion'] != 1 ||
        decoded['platform'] != platform ||
        decoded['commit'] != expectedCommit ||
        decoded['host'] is! Map<String, Object?> ||
        decoded['scenarios'] is! Map<String, Object?>) {
      throw const FormatException('device evidence envelope is invalid');
    }
    final scenarios = decoded['scenarios']! as Map<String, Object?>;
    if (platform == 'linux') {
      _validateLinuxEnvelope(decoded);
    }
    final failed = required
        .where((scenario) => scenarios[scenario] != 'PASS')
        .toList();
    if (failed.isNotEmpty) {
      throw FormatException(
        'mandatory scenarios are missing or not PASS: ${failed.join(', ')}',
      );
    }
    stdout.writeln('validated $platform device evidence for $expectedCommit');
  } on Object catch (error) {
    stderr.writeln('device evidence rejected: $error');
    exitCode = 1;
  }
}

void _validateLinuxEnvelope(Map<String, Object?> evidence) {
  final host = evidence['host']! as Map<String, Object?>;
  final runtime = evidence['runtime'];
  final artifacts = evidence['artifacts'];
  final duration = evidence['durationSeconds'];
  if (host['os'] is! String ||
      host['cpuLogicalCores'] is! int ||
      (host['cpuLogicalCores']! as int) < 1 ||
      host['memoryGiB'] is! num ||
      (host['memoryGiB']! as num) <= 0 ||
      host['gpu'] is! String ||
      host['displayScalePercent'] is! num ||
      runtime is! Map<String, Object?> ||
      runtime['flutter'] is! Map<String, Object?> ||
      duration is! int ||
      duration < 1 ||
      artifacts is! Map<String, Object?> ||
      !_isSha256(artifacts['linuxBundleSha256'])) {
    throw const FormatException('Linux evidence metadata is invalid');
  }
  final scenarios = evidence['scenarios']! as Map<String, Object?>;
  if (scenarios['waylandOverlay'] != 'OUT_OF_SCOPE') {
    throw const FormatException('Wayland overlay must remain OUT_OF_SCOPE');
  }
}

bool _isSha256(Object? value) =>
    value is String && RegExp(r'^[0-9a-f]{64}$').hasMatch(value);

Map<String, String> _options(List<String> arguments) {
  if (arguments.length.isOdd) {
    return const {};
  }
  final result = <String, String>{};
  for (var index = 0; index < arguments.length; index += 2) {
    final name = arguments[index];
    if (!name.startsWith('--') || arguments[index + 1].isEmpty) {
      return const {};
    }
    result[name.substring(2)] = arguments[index + 1];
  }
  return result;
}
