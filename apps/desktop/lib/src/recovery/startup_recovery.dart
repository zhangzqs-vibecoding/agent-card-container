import 'dart:io';

class StartupRecovery {
  StartupRecovery._(this._marker, {required this.previousRunUnclean});

  factory StartupRecovery.start(File marker) {
    if (FileSystemEntity.typeSync(marker.path) ==
        FileSystemEntityType.directory) {
      throw FileSystemException('run marker path is a directory', marker.path);
    }
    final previousRunUnclean = marker.existsSync();
    marker.parent.createSync(recursive: true);
    marker.writeAsStringSync('running\n', flush: true);
    return StartupRecovery._(marker, previousRunUnclean: previousRunUnclean);
  }

  final File _marker;
  final bool previousRunUnclean;
  var _clean = false;

  void markClean() {
    if (_clean) {
      return;
    }
    _clean = true;
    if (_marker.existsSync()) {
      _marker.deleteSync();
    }
  }
}
