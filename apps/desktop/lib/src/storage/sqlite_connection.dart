import 'dart:convert' as convert;
import 'dart:ffi';
import 'dart:io';
import 'dart:typed_data';

final class _SqliteDatabase extends Opaque {}

final class _SqliteStatement extends Opaque {}

class SqliteException implements Exception {
  const SqliteException(this.message, this.code);

  final String message;
  final int code;

  @override
  String toString() => 'SqliteException($code): $message';
}

class SqliteConnection {
  SqliteConnection._(this._bindings, this._database);

  factory SqliteConnection.openInMemory() {
    return SqliteConnection.open(':memory:');
  }

  factory SqliteConnection.open(String path) {
    final bindings = _SqliteBindings.load();
    final databaseOut = bindings.allocatePointer<_SqliteDatabase>();
    final pathPointer = bindings.toUtf8(path);
    try {
      final result = bindings.openV2(
        pathPointer,
        databaseOut,
        _SqliteBindings.openReadWrite |
            _SqliteBindings.openCreate |
            _SqliteBindings.openFullMutex,
        nullptr,
      );
      final database = databaseOut.value;
      if (result != _SqliteBindings.ok || database == nullptr) {
        final message = database == nullptr
            ? 'unable to open SQLite database'
            : bindings.errorMessage(database);
        if (database != nullptr) {
          bindings.closeV2(database);
        }
        throw SqliteException(message, result);
      }
      return SqliteConnection._(bindings, database);
    } finally {
      bindings.free(pathPointer.cast());
      bindings.free(databaseOut.cast());
    }
  }

  final _SqliteBindings _bindings;
  Pointer<_SqliteDatabase> _database;
  bool _inTransaction = false;

  bool get isOpen => _database != nullptr;

  void execute(String sql, [List<Object?> parameters = const []]) {
    final statement = _prepare(sql);
    try {
      _bind(statement, parameters);
      while (true) {
        final result = _bindings.step(statement);
        if (result == _SqliteBindings.done) {
          return;
        }
        if (result != _SqliteBindings.row) {
          _throwDatabase(result);
        }
      }
    } finally {
      _bindings.finalize(statement);
    }
  }

  List<Map<String, Object?>> query(
    String sql, [
    List<Object?> parameters = const [],
  ]) {
    final statement = _prepare(sql);
    try {
      _bind(statement, parameters);
      final rows = <Map<String, Object?>>[];
      while (true) {
        final result = _bindings.step(statement);
        if (result == _SqliteBindings.done) {
          return rows;
        }
        if (result != _SqliteBindings.row) {
          _throwDatabase(result);
        }
        rows.add(_readRow(statement));
      }
    } finally {
      _bindings.finalize(statement);
    }
  }

  T transaction<T>(T Function() operation) {
    if (_inTransaction) {
      throw StateError('nested SQLite transactions are not supported');
    }
    execute('BEGIN IMMEDIATE');
    _inTransaction = true;
    try {
      final result = operation();
      execute('COMMIT');
      return result;
    } catch (_) {
      execute('ROLLBACK');
      rethrow;
    } finally {
      _inTransaction = false;
    }
  }

  void close() {
    if (_database == nullptr) {
      return;
    }
    final result = _bindings.closeV2(_database);
    if (result != _SqliteBindings.ok) {
      _throwDatabase(result);
    }
    _database = nullptr;
  }

  Pointer<_SqliteStatement> _prepare(String sql) {
    _ensureOpen();
    final sqlPointer = _bindings.toUtf8(sql);
    final statementOut = _bindings.allocatePointer<_SqliteStatement>();
    try {
      final result = _bindings.prepareV2(
        _database,
        sqlPointer,
        -1,
        statementOut,
        nullptr,
      );
      if (result != _SqliteBindings.ok) {
        _throwDatabase(result);
      }
      return statementOut.value;
    } finally {
      _bindings.free(sqlPointer.cast());
      _bindings.free(statementOut.cast());
    }
  }

  void _bind(Pointer<_SqliteStatement> statement, List<Object?> values) {
    for (var index = 0; index < values.length; index++) {
      final position = index + 1;
      final value = values[index];
      final int result;
      if (value == null) {
        result = _bindings.bindNull(statement, position);
      } else if (value is bool) {
        result = _bindings.bindInt64(statement, position, value ? 1 : 0);
      } else if (value is int) {
        result = _bindings.bindInt64(statement, position, value);
      } else if (value is double) {
        result = _bindings.bindDouble(statement, position, value);
      } else if (value is String) {
        result = _bindText(statement, position, value);
      } else if (value is Uint8List) {
        result = _bindBlob(statement, position, value);
      } else {
        throw ArgumentError.value(
          value,
          'values',
          'unsupported SQLite parameter',
        );
      }
      if (result != _SqliteBindings.ok) {
        _throwDatabase(result);
      }
    }
  }

  int _bindText(
    Pointer<_SqliteStatement> statement,
    int position,
    String value,
  ) {
    final pointer = _bindings.toUtf8(value);
    try {
      return _bindings.bindText(
        statement,
        position,
        pointer,
        -1,
        _SqliteBindings.transientDestructor,
      );
    } finally {
      _bindings.free(pointer.cast());
    }
  }

  int _bindBlob(
    Pointer<_SqliteStatement> statement,
    int position,
    Uint8List value,
  ) {
    final pointer = _bindings.allocateBytes(value.length);
    pointer.asTypedList(value.length).setAll(0, value);
    try {
      return _bindings.bindBlob(
        statement,
        position,
        pointer.cast(),
        value.length,
        _SqliteBindings.transientDestructor,
      );
    } finally {
      _bindings.free(pointer.cast());
    }
  }

  Map<String, Object?> _readRow(Pointer<_SqliteStatement> statement) {
    final row = <String, Object?>{};
    final count = _bindings.columnCount(statement);
    for (var index = 0; index < count; index++) {
      final name = _bindings.readUtf8(_bindings.columnName(statement, index));
      row[name] = switch (_bindings.columnType(statement, index)) {
        _SqliteBindings.integer => _bindings.columnInt64(statement, index),
        _SqliteBindings.float => _bindings.columnDouble(statement, index),
        _SqliteBindings.text => _bindings.readUtf8(
          _bindings.columnText(statement, index),
        ),
        _SqliteBindings.blob => _readBlob(statement, index),
        _SqliteBindings.nullValue => null,
        _ => throw const SqliteException('unknown SQLite column type', -1),
      };
    }
    return row;
  }

  Uint8List _readBlob(Pointer<_SqliteStatement> statement, int index) {
    final length = _bindings.columnBytes(statement, index);
    if (length == 0) {
      return Uint8List(0);
    }
    final pointer = _bindings.columnBlob(statement, index).cast<Uint8>();
    return Uint8List.fromList(pointer.asTypedList(length));
  }

  Never _throwDatabase(int code) {
    throw SqliteException(_bindings.errorMessage(_database), code);
  }

  void _ensureOpen() {
    if (_database == nullptr) {
      throw StateError('SQLite connection is closed');
    }
  }
}

class _SqliteBindings {
  _SqliteBindings(this.library)
    : malloc64 = library
          .lookupFunction<
            Pointer<Void> Function(Uint64),
            Pointer<Void> Function(int)
          >('sqlite3_malloc64'),
      free = library
          .lookupFunction<
            Void Function(Pointer<Void>),
            void Function(Pointer<Void>)
          >('sqlite3_free'),
      openV2 = library
          .lookupFunction<
            Int32 Function(
              Pointer<Uint8>,
              Pointer<Pointer<_SqliteDatabase>>,
              Int32,
              Pointer<Uint8>,
            ),
            int Function(
              Pointer<Uint8>,
              Pointer<Pointer<_SqliteDatabase>>,
              int,
              Pointer<Uint8>,
            )
          >('sqlite3_open_v2'),
      closeV2 = library
          .lookupFunction<
            Int32 Function(Pointer<_SqliteDatabase>),
            int Function(Pointer<_SqliteDatabase>)
          >('sqlite3_close_v2'),
      errorMessagePointer = library
          .lookupFunction<
            Pointer<Uint8> Function(Pointer<_SqliteDatabase>),
            Pointer<Uint8> Function(Pointer<_SqliteDatabase>)
          >('sqlite3_errmsg'),
      prepareV2 = library
          .lookupFunction<
            Int32 Function(
              Pointer<_SqliteDatabase>,
              Pointer<Uint8>,
              Int32,
              Pointer<Pointer<_SqliteStatement>>,
              Pointer<Pointer<Uint8>>,
            ),
            int Function(
              Pointer<_SqliteDatabase>,
              Pointer<Uint8>,
              int,
              Pointer<Pointer<_SqliteStatement>>,
              Pointer<Pointer<Uint8>>,
            )
          >('sqlite3_prepare_v2'),
      step = library
          .lookupFunction<
            Int32 Function(Pointer<_SqliteStatement>),
            int Function(Pointer<_SqliteStatement>)
          >('sqlite3_step'),
      finalize = library
          .lookupFunction<
            Int32 Function(Pointer<_SqliteStatement>),
            int Function(Pointer<_SqliteStatement>)
          >('sqlite3_finalize'),
      bindNull = library
          .lookupFunction<
            Int32 Function(Pointer<_SqliteStatement>, Int32),
            int Function(Pointer<_SqliteStatement>, int)
          >('sqlite3_bind_null'),
      bindInt64 = library
          .lookupFunction<
            Int32 Function(Pointer<_SqliteStatement>, Int32, Int64),
            int Function(Pointer<_SqliteStatement>, int, int)
          >('sqlite3_bind_int64'),
      bindDouble = library
          .lookupFunction<
            Int32 Function(Pointer<_SqliteStatement>, Int32, Double),
            int Function(Pointer<_SqliteStatement>, int, double)
          >('sqlite3_bind_double'),
      bindText = library
          .lookupFunction<
            Int32 Function(
              Pointer<_SqliteStatement>,
              Int32,
              Pointer<Uint8>,
              Int32,
              Pointer<NativeFunction<Void Function(Pointer<Void>)>>,
            ),
            int Function(
              Pointer<_SqliteStatement>,
              int,
              Pointer<Uint8>,
              int,
              Pointer<NativeFunction<Void Function(Pointer<Void>)>>,
            )
          >('sqlite3_bind_text'),
      bindBlob = library
          .lookupFunction<
            Int32 Function(
              Pointer<_SqliteStatement>,
              Int32,
              Pointer<Void>,
              Int32,
              Pointer<NativeFunction<Void Function(Pointer<Void>)>>,
            ),
            int Function(
              Pointer<_SqliteStatement>,
              int,
              Pointer<Void>,
              int,
              Pointer<NativeFunction<Void Function(Pointer<Void>)>>,
            )
          >('sqlite3_bind_blob'),
      columnCount = library
          .lookupFunction<
            Int32 Function(Pointer<_SqliteStatement>),
            int Function(Pointer<_SqliteStatement>)
          >('sqlite3_column_count'),
      columnName = library
          .lookupFunction<
            Pointer<Uint8> Function(Pointer<_SqliteStatement>, Int32),
            Pointer<Uint8> Function(Pointer<_SqliteStatement>, int)
          >('sqlite3_column_name'),
      columnType = library
          .lookupFunction<
            Int32 Function(Pointer<_SqliteStatement>, Int32),
            int Function(Pointer<_SqliteStatement>, int)
          >('sqlite3_column_type'),
      columnInt64 = library
          .lookupFunction<
            Int64 Function(Pointer<_SqliteStatement>, Int32),
            int Function(Pointer<_SqliteStatement>, int)
          >('sqlite3_column_int64'),
      columnDouble = library
          .lookupFunction<
            Double Function(Pointer<_SqliteStatement>, Int32),
            double Function(Pointer<_SqliteStatement>, int)
          >('sqlite3_column_double'),
      columnText = library
          .lookupFunction<
            Pointer<Uint8> Function(Pointer<_SqliteStatement>, Int32),
            Pointer<Uint8> Function(Pointer<_SqliteStatement>, int)
          >('sqlite3_column_text'),
      columnBlob = library
          .lookupFunction<
            Pointer<Void> Function(Pointer<_SqliteStatement>, Int32),
            Pointer<Void> Function(Pointer<_SqliteStatement>, int)
          >('sqlite3_column_blob'),
      columnBytes = library
          .lookupFunction<
            Int32 Function(Pointer<_SqliteStatement>, Int32),
            int Function(Pointer<_SqliteStatement>, int)
          >('sqlite3_column_bytes');

  static const ok = 0;
  static const row = 100;
  static const done = 101;
  static const integer = 1;
  static const float = 2;
  static const text = 3;
  static const blob = 4;
  static const nullValue = 5;
  static const openReadWrite = 0x00000002;
  static const openCreate = 0x00000004;
  static const openFullMutex = 0x00010000;
  static final transientDestructor =
      Pointer<NativeFunction<Void Function(Pointer<Void>)>>.fromAddress(-1);

  final DynamicLibrary library;
  final Pointer<Void> Function(int) malloc64;
  final void Function(Pointer<Void>) free;
  final int Function(
    Pointer<Uint8>,
    Pointer<Pointer<_SqliteDatabase>>,
    int,
    Pointer<Uint8>,
  )
  openV2;
  final int Function(Pointer<_SqliteDatabase>) closeV2;
  final Pointer<Uint8> Function(Pointer<_SqliteDatabase>) errorMessagePointer;
  final int Function(
    Pointer<_SqliteDatabase>,
    Pointer<Uint8>,
    int,
    Pointer<Pointer<_SqliteStatement>>,
    Pointer<Pointer<Uint8>>,
  )
  prepareV2;
  final int Function(Pointer<_SqliteStatement>) step;
  final int Function(Pointer<_SqliteStatement>) finalize;
  final int Function(Pointer<_SqliteStatement>, int) bindNull;
  final int Function(Pointer<_SqliteStatement>, int, int) bindInt64;
  final int Function(Pointer<_SqliteStatement>, int, double) bindDouble;
  final int Function(
    Pointer<_SqliteStatement>,
    int,
    Pointer<Uint8>,
    int,
    Pointer<NativeFunction<Void Function(Pointer<Void>)>>,
  )
  bindText;
  final int Function(
    Pointer<_SqliteStatement>,
    int,
    Pointer<Void>,
    int,
    Pointer<NativeFunction<Void Function(Pointer<Void>)>>,
  )
  bindBlob;
  final int Function(Pointer<_SqliteStatement>) columnCount;
  final Pointer<Uint8> Function(Pointer<_SqliteStatement>, int) columnName;
  final int Function(Pointer<_SqliteStatement>, int) columnType;
  final int Function(Pointer<_SqliteStatement>, int) columnInt64;
  final double Function(Pointer<_SqliteStatement>, int) columnDouble;
  final Pointer<Uint8> Function(Pointer<_SqliteStatement>, int) columnText;
  final Pointer<Void> Function(Pointer<_SqliteStatement>, int) columnBlob;
  final int Function(Pointer<_SqliteStatement>, int) columnBytes;

  static _SqliteBindings load() {
    if (Platform.isWindows) {
      return _SqliteBindings(DynamicLibrary.open('sqlite3.dll'));
    }
    if (Platform.isMacOS) {
      return _SqliteBindings(DynamicLibrary.open('/usr/lib/libsqlite3.dylib'));
    }
    if (Platform.isLinux) {
      return _SqliteBindings(DynamicLibrary.open('libsqlite3.so.0'));
    }
    throw UnsupportedError('SQLite is not supported on this platform');
  }

  Pointer<Pointer<T>> allocatePointer<T extends NativeType>() {
    final pointer = malloc64(sizeOf<IntPtr>());
    if (pointer == nullptr) {
      throw const SqliteException('SQLite memory allocation failed', 7);
    }
    return pointer.cast<Pointer<T>>();
  }

  Pointer<Uint8> allocateBytes(int count) {
    final pointer = malloc64(count);
    if (pointer == nullptr) {
      throw const SqliteException('SQLite memory allocation failed', 7);
    }
    return pointer.cast<Uint8>();
  }

  Pointer<Uint8> toUtf8(String value) {
    final bytes = utf8Encode(value);
    final pointer = allocateBytes(bytes.length + 1);
    final buffer = pointer.asTypedList(bytes.length + 1);
    buffer.setAll(0, bytes);
    buffer[bytes.length] = 0;
    return pointer;
  }

  List<int> utf8Encode(String value) => convert.utf8.encoder.convert(value);

  String readUtf8(Pointer<Uint8> pointer) {
    if (pointer == nullptr) {
      return '';
    }
    var length = 0;
    while (pointer[length] != 0) {
      length++;
      if (length > 16 * 1024 * 1024) {
        throw const SqliteException('invalid SQLite UTF-8 value', -1);
      }
    }
    return convert.utf8.decode(pointer.asTypedList(length));
  }

  String errorMessage(Pointer<_SqliteDatabase> database) {
    return readUtf8(errorMessagePointer(database));
  }
}
