import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';

class ZipLimits {
  const ZipLimits({
    this.maxArchiveBytes = 8 * 1024 * 1024,
    this.maxExpandedBytes = 32 * 1024 * 1024,
    this.maxFileBytes = 8 * 1024 * 1024,
    this.maxFiles = 512,
    this.maxPathDepth = 8,
  });

  final int maxArchiveBytes;
  final int maxExpandedBytes;
  final int maxFileBytes;
  final int maxFiles;
  final int maxPathDepth;
}

class SafeZipArchive {
  SafeZipArchive._(this._files);

  factory SafeZipArchive.read(
    Uint8List bytes, {
    ZipLimits limits = const ZipLimits(),
  }) {
    if (bytes.length > limits.maxArchiveBytes) {
      throw const FormatException('ZIP archive exceeds compressed size limit');
    }
    final view = ByteData.sublistView(bytes);
    final eocd = _findEndOfCentralDirectory(view);
    final entryCount = _u16(view, eocd + 10);
    final centralSize = _u32(view, eocd + 12);
    final centralOffset = _u32(view, eocd + 16);
    if (entryCount > limits.maxFiles ||
        centralOffset + centralSize > bytes.length) {
      throw const FormatException('invalid ZIP central directory');
    }

    var cursor = centralOffset;
    var expandedBytes = 0;
    final files = <String, Uint8List>{};
    for (var index = 0; index < entryCount; index++) {
      _require(view, cursor, 46);
      if (_u32(view, cursor) != 0x02014b50) {
        throw const FormatException('invalid ZIP central entry');
      }
      final flags = _u16(view, cursor + 8);
      final method = _u16(view, cursor + 10);
      final compressedSize = _u32(view, cursor + 20);
      final expandedSize = _u32(view, cursor + 24);
      final nameLength = _u16(view, cursor + 28);
      final extraLength = _u16(view, cursor + 30);
      final commentLength = _u16(view, cursor + 32);
      final externalAttributes = _u32(view, cursor + 38);
      final localOffset = _u32(view, cursor + 42);
      _require(view, cursor + 46, nameLength + extraLength + commentLength);
      final name = _decodeName(
        bytes.sublist(cursor + 46, cursor + 46 + nameLength),
        flags,
      );
      cursor += 46 + nameLength + extraLength + commentLength;

      _validatePath(name, limits.maxPathDepth);
      final unixMode = externalAttributes >> 16;
      if ((unixMode & 0xF000) == 0xA000) {
        throw const FormatException('ZIP symbolic links are forbidden');
      }
      if (name.endsWith('/')) {
        continue;
      }
      if (files.containsKey(name)) {
        throw FormatException('duplicate ZIP path: $name');
      }
      if ((flags & 0x1) != 0 || (method != 0 && method != 8)) {
        throw const FormatException(
          'unsupported ZIP encryption or compression',
        );
      }
      if (expandedSize > limits.maxFileBytes) {
        throw FormatException('ZIP file exceeds size limit: $name');
      }
      expandedBytes += expandedSize;
      if (expandedBytes > limits.maxExpandedBytes) {
        throw const FormatException('ZIP expanded contents exceed size limit');
      }
      files[name] = _readPayload(
        bytes,
        view,
        localOffset,
        compressedSize,
        expandedSize,
        method,
      );
    }
    return SafeZipArchive._(Map.unmodifiable(files));
  }

  final Map<String, Uint8List> _files;

  Set<String> get paths => _files.keys.toSet();

  Uint8List file(String path) {
    final bytes = _files[path];
    if (bytes == null) {
      throw FormatException('ZIP file is missing: $path');
    }
    return Uint8List.fromList(bytes);
  }

  static Uint8List _readPayload(
    Uint8List bytes,
    ByteData view,
    int localOffset,
    int compressedSize,
    int expandedSize,
    int method,
  ) {
    _require(view, localOffset, 30);
    if (_u32(view, localOffset) != 0x04034b50) {
      throw const FormatException('invalid ZIP local entry');
    }
    final nameLength = _u16(view, localOffset + 26);
    final extraLength = _u16(view, localOffset + 28);
    final start = localOffset + 30 + nameLength + extraLength;
    _require(view, start, compressedSize);
    final compressed = bytes.sublist(start, start + compressedSize);
    final List<int> expanded;
    try {
      expanded = method == 0
          ? compressed
          : ZLibDecoder(raw: true).convert(compressed);
    } on FormatException {
      rethrow;
    } catch (_) {
      throw const FormatException('invalid ZIP compressed payload');
    }
    if (expanded.length != expandedSize) {
      throw const FormatException('ZIP expanded size does not match metadata');
    }
    return Uint8List.fromList(expanded);
  }
}

int _findEndOfCentralDirectory(ByteData view) {
  final first = view.lengthInBytes - 22;
  final minimum = view.lengthInBytes - 22 - 0xFFFF;
  for (var offset = first; offset >= 0 && offset >= minimum; offset--) {
    if (_u32(view, offset) == 0x06054b50) {
      _require(view, offset, 22);
      final commentLength = _u16(view, offset + 20);
      if (offset + 22 + commentLength == view.lengthInBytes) {
        return offset;
      }
    }
  }
  throw const FormatException('ZIP end record is missing');
}

String _decodeName(List<int> bytes, int flags) {
  try {
    return (flags & 0x0800) != 0 ? utf8.decode(bytes) : latin1.decode(bytes);
  } catch (_) {
    throw const FormatException('invalid ZIP file name encoding');
  }
}

void _validatePath(String path, int maxDepth) {
  if (path.isEmpty ||
      path.startsWith('/') ||
      path.contains(r'\') ||
      path.contains('//')) {
    throw FormatException('unsafe ZIP path: $path');
  }
  final normalized = path.endsWith('/')
      ? path.substring(0, path.length - 1)
      : path;
  final segments = normalized.split('/');
  if (segments.length > maxDepth ||
      segments.any((part) => part.isEmpty || part == '.' || part == '..')) {
    throw FormatException('unsafe ZIP path: $path');
  }
}

int _u16(ByteData view, int offset) {
  _require(view, offset, 2);
  return view.getUint16(offset, Endian.little);
}

int _u32(ByteData view, int offset) {
  _require(view, offset, 4);
  return view.getUint32(offset, Endian.little);
}

void _require(ByteData view, int offset, int length) {
  if (offset < 0 || length < 0 || offset + length > view.lengthInBytes) {
    throw const FormatException('truncated ZIP archive');
  }
}
