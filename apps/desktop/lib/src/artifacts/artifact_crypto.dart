import 'dart:collection';
import 'dart:convert';
import 'dart:ffi';
import 'dart:io';
import 'dart:typed_data';

typedef _SodiumInitNative = Int32 Function();
typedef _SodiumInit = int Function();
typedef _SodiumMallocNative = Pointer<Void> Function(Size);
typedef _SodiumMalloc = Pointer<Void> Function(int);
typedef _SodiumFreeNative = Void Function(Pointer<Void>);
typedef _SodiumFree = void Function(Pointer<Void>);
typedef _Sha256Native = Int32 Function(Pointer<Uint8>, Pointer<Uint8>, Uint64);
typedef _Sha256 = int Function(Pointer<Uint8>, Pointer<Uint8>, int);
typedef _VerifyNative =
    Int32 Function(Pointer<Uint8>, Pointer<Uint8>, Uint64, Pointer<Uint8>);
typedef _Verify =
    int Function(Pointer<Uint8>, Pointer<Uint8>, int, Pointer<Uint8>);
typedef _SeedKeyPairNative =
    Int32 Function(Pointer<Uint8>, Pointer<Uint8>, Pointer<Uint8>);
typedef _SeedKeyPair =
    int Function(Pointer<Uint8>, Pointer<Uint8>, Pointer<Uint8>);
typedef _SignNative =
    Int32 Function(
      Pointer<Uint8>,
      Pointer<Uint64>,
      Pointer<Uint8>,
      Uint64,
      Pointer<Uint8>,
    );
typedef _Sign =
    int Function(
      Pointer<Uint8>,
      Pointer<Uint64>,
      Pointer<Uint8>,
      int,
      Pointer<Uint8>,
    );

class ArtifactKeyPair {
  const ArtifactKeyPair({required this.publicKey, required this.secretKey});

  final Uint8List publicKey;
  final Uint8List secretKey;
}

class ArtifactCrypto {
  ArtifactCrypto._(this._bindings);

  factory ArtifactCrypto.native() {
    final bindings = _SodiumBindings.load();
    if (bindings.initialize() < 0) {
      throw StateError('libsodium initialization failed');
    }
    return ArtifactCrypto._(bindings);
  }

  final _SodiumBindings _bindings;

  Uint8List sha256(Uint8List input) {
    final output = _bindings.allocate(32);
    final message = _bindings.copyIn(input);
    try {
      if (_bindings.hash(output, message, input.length) != 0) {
        throw StateError('SHA-256 failed');
      }
      return Uint8List.fromList(output.asTypedList(32));
    } finally {
      _bindings.free(output.cast());
      _bindings.free(message.cast());
    }
  }

  bool verify(Uint8List message, Uint8List signature, Uint8List publicKey) {
    if (signature.length != 64 || publicKey.length != 32) {
      return false;
    }
    final messagePointer = _bindings.copyIn(message);
    final signaturePointer = _bindings.copyIn(signature);
    final keyPointer = _bindings.copyIn(publicKey);
    try {
      return _bindings.verify(
            signaturePointer,
            messagePointer,
            message.length,
            keyPointer,
          ) ==
          0;
    } finally {
      _bindings.free(messagePointer.cast());
      _bindings.free(signaturePointer.cast());
      _bindings.free(keyPointer.cast());
    }
  }

  ArtifactKeyPair keyPairFromSeed(Uint8List seed) {
    if (seed.length != 32) {
      throw ArgumentError.value(seed.length, 'seed', 'must contain 32 bytes');
    }
    final publicKey = _bindings.allocate(32);
    final secretKey = _bindings.allocate(64);
    final seedPointer = _bindings.copyIn(seed);
    try {
      if (_bindings.seedKeyPair(publicKey, secretKey, seedPointer) != 0) {
        throw StateError('Ed25519 key generation failed');
      }
      return ArtifactKeyPair(
        publicKey: Uint8List.fromList(publicKey.asTypedList(32)),
        secretKey: Uint8List.fromList(secretKey.asTypedList(64)),
      );
    } finally {
      _bindings.free(publicKey.cast());
      _bindings.free(secretKey.cast());
      _bindings.free(seedPointer.cast());
    }
  }

  Uint8List signForTesting(Uint8List message, Uint8List secretKey) {
    if (secretKey.length != 64) {
      throw ArgumentError.value(
        secretKey.length,
        'secretKey',
        'must contain 64 bytes',
      );
    }
    final signature = _bindings.allocate(64);
    final signatureLength = _bindings.allocateUint64();
    final messagePointer = _bindings.copyIn(message);
    final keyPointer = _bindings.copyIn(secretKey);
    try {
      final result = _bindings.sign(
        signature,
        signatureLength,
        messagePointer,
        message.length,
        keyPointer,
      );
      if (result != 0 || signatureLength.value != 64) {
        throw StateError('Ed25519 signing failed');
      }
      return Uint8List.fromList(signature.asTypedList(64));
    } finally {
      _bindings.free(signature.cast());
      _bindings.free(signatureLength.cast());
      _bindings.free(messagePointer.cast());
      _bindings.free(keyPointer.cast());
    }
  }
}

class _SodiumBindings {
  _SodiumBindings(this.library)
    : initialize = library.lookupFunction<_SodiumInitNative, _SodiumInit>(
        'sodium_init',
      ),
      malloc = library.lookupFunction<_SodiumMallocNative, _SodiumMalloc>(
        'sodium_malloc',
      ),
      free = library.lookupFunction<_SodiumFreeNative, _SodiumFree>(
        'sodium_free',
      ),
      hash = library.lookupFunction<_Sha256Native, _Sha256>(
        'crypto_hash_sha256',
      ),
      verify = library.lookupFunction<_VerifyNative, _Verify>(
        'crypto_sign_verify_detached',
      ),
      seedKeyPair = library.lookupFunction<_SeedKeyPairNative, _SeedKeyPair>(
        'crypto_sign_seed_keypair',
      ),
      sign = library.lookupFunction<_SignNative, _Sign>('crypto_sign_detached');

  factory _SodiumBindings.load() {
    final candidates = Platform.isWindows
        ? const ['libsodium.dll']
        : Platform.isMacOS
        ? const ['libsodium.dylib', '/opt/homebrew/lib/libsodium.dylib']
        : const ['libsodium.so.23', 'libsodium.so'];
    Object? lastError;
    for (final candidate in candidates) {
      try {
        return _SodiumBindings(DynamicLibrary.open(candidate));
      } catch (error) {
        lastError = error;
      }
    }
    throw UnsupportedError('libsodium is unavailable: $lastError');
  }

  final DynamicLibrary library;
  final _SodiumInit initialize;
  final _SodiumMalloc malloc;
  final _SodiumFree free;
  final _Sha256 hash;
  final _Verify verify;
  final _SeedKeyPair seedKeyPair;
  final _Sign sign;

  Pointer<Uint8> allocate(int length) {
    final pointer = malloc(length).cast<Uint8>();
    if (pointer == nullptr) {
      throw StateError('libsodium allocation failed');
    }
    return pointer;
  }

  Pointer<Uint64> allocateUint64() => allocate(8).cast<Uint64>();

  Pointer<Uint8> copyIn(Uint8List bytes) {
    final pointer = allocate(bytes.isEmpty ? 1 : bytes.length);
    if (bytes.isNotEmpty) {
      pointer.asTypedList(bytes.length).setAll(0, bytes);
    }
    return pointer;
  }
}

String canonicalJson(Object? value) {
  return jsonEncode(_canonicalValue(value));
}

Object? _canonicalValue(Object? value) {
  if (value is Map) {
    final sorted = SplayTreeMap<String, Object?>();
    for (final entry in value.entries) {
      if (entry.key is! String) {
        throw const FormatException(
          'canonical JSON object keys must be strings',
        );
      }
      sorted[entry.key as String] = _canonicalValue(entry.value);
    }
    return sorted;
  }
  if (value is List) {
    return value.map(_canonicalValue).toList(growable: false);
  }
  if (value == null || value is String || value is bool || value is int) {
    return value;
  }
  if (value is double && value.isFinite) {
    return value;
  }
  throw FormatException('unsupported canonical JSON value: $value');
}

String hexEncode(Uint8List bytes) {
  final output = StringBuffer();
  for (final byte in bytes) {
    output.write(byte.toRadixString(16).padLeft(2, '0'));
  }
  return output.toString();
}
