import 'dart:io';

typedef ReplacementLauncher = Future<void> Function(String executable);

class ApplicationRestarter {
  ApplicationRestarter({String? executable, ReplacementLauncher? launch})
    : executable = executable ?? Platform.resolvedExecutable,
      _launch = launch ?? _launchDetached;

  final String executable;
  final ReplacementLauncher _launch;

  Future<void> restart({required Future<void> Function() shutdown}) async {
    await _launch(executable);
    await shutdown();
  }
}

Future<void> _launchDetached(String executable) async {
  await Process.start(executable, const [], mode: ProcessStartMode.detached);
}
