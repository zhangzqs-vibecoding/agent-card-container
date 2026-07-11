import 'dart:convert';
import 'dart:typed_data';

class ZipTestEntry {
  const ZipTestEntry(this.path, this.bytes, {this.unixMode = 0x81A4});

  final String path;
  final List<int> bytes;
  final int unixMode;
}

Uint8List buildStoredZip(List<ZipTestEntry> entries) {
  final output = BytesBuilder(copy: false);
  final central = BytesBuilder(copy: false);
  var offset = 0;
  for (final entry in entries) {
    final nameBytes = utf8.encode(entry.path);
    final local = BytesBuilder(copy: false)
      ..add(_u32(0x04034b50))
      ..add(_u16(20))
      ..add(_u16(0x0800))
      ..add(_u16(0))
      ..add(_u16(0))
      ..add(_u16(0))
      ..add(_u32(0))
      ..add(_u32(entry.bytes.length))
      ..add(_u32(entry.bytes.length))
      ..add(_u16(nameBytes.length))
      ..add(_u16(0))
      ..add(nameBytes)
      ..add(entry.bytes);
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
      ..add(_u32(entry.bytes.length))
      ..add(_u32(entry.bytes.length))
      ..add(_u16(nameBytes.length))
      ..add(_u16(0))
      ..add(_u16(0))
      ..add(_u16(0))
      ..add(_u16(0))
      ..add(_u32(entry.unixMode << 16))
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
