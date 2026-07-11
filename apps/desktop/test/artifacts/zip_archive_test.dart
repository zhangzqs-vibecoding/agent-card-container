import 'dart:convert';
import 'dart:typed_data';

import 'package:agent_card_desktop/src/artifacts/zip_archive.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  group('SafeZipArchive', () {
    test('reads a bounded stored archive', () {
      final archive = SafeZipArchive.read(
        _zip({
          'manifest.json': utf8.encode('{}'),
          'payload/web/index.html': utf8.encode('<h1>ok</h1>'),
        }),
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
          _zipEntries([
            ('same', [1]),
            ('same', [2]),
          ]),
        ),
        throwsFormatException,
      );
      expect(
        () => SafeZipArchive.read(
          _zip({
            '../escape': [1],
          }),
        ),
        throwsFormatException,
      );
    });

    test('rejects symbolic links and oversized expanded contents', () {
      expect(
        () => SafeZipArchive.read(
          _zip(
            {
              'link': [1],
            },
            unixModes: {'link': 0xA1FF},
          ),
        ),
        throwsFormatException,
      );
      expect(
        () => SafeZipArchive.read(
          _zip({'large': List.filled(12, 0)}),
          limits: const ZipLimits(maxExpandedBytes: 10),
        ),
        throwsFormatException,
      );
    });
  });
}

Uint8List _zip(
  Map<String, List<int>> files, {
  Map<String, int> unixModes = const {},
}) {
  return _zipEntries([
    for (final entry in files.entries)
      (entry.key, entry.value, unixModes[entry.key] ?? 0x81A4),
  ]);
}

Uint8List _zipEntries(List<dynamic> entries) {
  final output = BytesBuilder(copy: false);
  final central = BytesBuilder(copy: false);
  var offset = 0;
  for (final dynamic raw in entries) {
    final String name = raw.$1 as String;
    final List<int> bytes = raw.$2 as List<int>;
    final int mode = raw is (String, List<int>, int) ? raw.$3 : 0x81A4;
    final nameBytes = utf8.encode(name);
    final local = BytesBuilder(copy: false)
      ..add(_u32(0x04034b50))
      ..add(_u16(20))
      ..add(_u16(0x0800))
      ..add(_u16(0))
      ..add(_u16(0))
      ..add(_u16(0))
      ..add(_u32(0))
      ..add(_u32(bytes.length))
      ..add(_u32(bytes.length))
      ..add(_u16(nameBytes.length))
      ..add(_u16(0))
      ..add(nameBytes)
      ..add(bytes);
    final localBytes = local.takeBytes();
    output.add(localBytes);

    central
      ..add(_u32(0x02014b50))
      ..add(_u16(0x031E))
      ..add(_u16(20))
      ..add(_u16(0x0800))
      ..add(_u16(0))
      ..add(_u16(0))
      ..add(_u16(0))
      ..add(_u32(0))
      ..add(_u32(bytes.length))
      ..add(_u32(bytes.length))
      ..add(_u16(nameBytes.length))
      ..add(_u16(0))
      ..add(_u16(0))
      ..add(_u16(0))
      ..add(_u16(0))
      ..add(_u32(mode << 16))
      ..add(_u32(offset))
      ..add(nameBytes);
    offset += localBytes.length;
  }
  final centralBytes = central.takeBytes();
  output
    ..add(centralBytes)
    ..add(_u32(0x06054b50))
    ..add(_u16(0))
    ..add(_u16(0))
    ..add(_u16(entries.length))
    ..add(_u16(entries.length))
    ..add(_u32(centralBytes.length))
    ..add(_u32(offset))
    ..add(_u16(0));
  return output.takeBytes();
}

Uint8List _u16(int value) =>
    Uint8List(2)..buffer.asByteData().setUint16(0, value, Endian.little);

Uint8List _u32(int value) =>
    Uint8List(4)..buffer.asByteData().setUint32(0, value, Endian.little);
