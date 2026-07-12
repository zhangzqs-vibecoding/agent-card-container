import 'dart:convert';

import 'package:agent_card_desktop/src/artifacts/zip_archive.dart';
import 'package:flutter_test/flutter_test.dart';

import '../support/zip_builder.dart';

void main() {
  group('SafeZipArchive', () {
    test('reads a bounded stored archive', () {
      final archive = SafeZipArchive.read(
        buildStoredZip([
          ZipTestEntry('manifest.json', utf8.encode('{}')),
          ZipTestEntry('payload/web/index.html', utf8.encode('<h1>ok</h1>')),
        ]),
      );

      expect(utf8.decode(archive.file('manifest.json')), '{}');
      expect(
        utf8.decode(archive.file('payload/web/index.html')),
        '<h1>ok</h1>',
      );
    });

    test('rejects duplicate and unsafe paths', () {
      expect(
        () => SafeZipArchive.read(
          buildStoredZip([
            const ZipTestEntry('same', [1]),
            const ZipTestEntry('same', [2]),
          ]),
        ),
        throwsFormatException,
      );
      expect(
        () => SafeZipArchive.read(
          buildStoredZip([
            const ZipTestEntry('../escape', [1]),
          ]),
        ),
        throwsFormatException,
      );
    });

    test('rejects symbolic links and oversized expanded contents', () {
      expect(
        () => SafeZipArchive.read(
          buildStoredZip([
            const ZipTestEntry('link', [1], unixMode: 0xA1FF),
          ]),
        ),
        throwsFormatException,
      );
      expect(
        () => SafeZipArchive.read(
          buildStoredZip([ZipTestEntry('large', List.filled(12, 0))]),
          limits: const ZipLimits(maxExpandedBytes: 10),
        ),
        throwsFormatException,
      );
    });

    test('stops a lying deflate stream before expanded allocation', () {
      expect(
        () => SafeZipArchive.read(
          buildStoredZip([
            ZipTestEntry(
              'bomb.txt',
              List.filled(1024 * 1024, 65),
              deflate: true,
              declaredExpandedSize: 1,
            ),
          ]),
          limits: const ZipLimits(maxFileBytes: 1024),
        ),
        throwsFormatException,
      );
    });
  });
}
