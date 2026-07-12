import 'dart:io';

import 'package:agent_card_desktop/src/recovery/startup_recovery.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test('detects an unclean previous run and clears on orderly shutdown', () {
    final root = Directory.systemTemp.createTempSync('startup-recovery-');
    addTearDown(() => root.deleteSync(recursive: true));
    final file = File('${root.path}/run.marker');

    final first = StartupRecovery.start(file);
    expect(first.previousRunUnclean, isFalse);
    expect(file.existsSync(), isTrue);

    final afterCrash = StartupRecovery.start(file);
    expect(afterCrash.previousRunUnclean, isTrue);
    afterCrash.markClean();
    expect(file.existsSync(), isFalse);

    final afterCleanExit = StartupRecovery.start(file);
    expect(afterCleanExit.previousRunUnclean, isFalse);
    afterCleanExit.markClean();
  });

  test('rejects a directory in place of the run marker', () {
    final root = Directory.systemTemp.createTempSync('startup-recovery-');
    addTearDown(() => root.deleteSync(recursive: true));
    final marker = Directory('${root.path}/run.marker')..createSync();

    expect(
      () => StartupRecovery.start(File(marker.path)),
      throwsA(isA<FileSystemException>()),
    );
  });
}
