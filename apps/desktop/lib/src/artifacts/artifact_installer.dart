import 'dart:convert';
import 'dart:io';
import 'dart:math';
import 'dart:typed_data';

import '../contracts/card_definition.dart';
import 'artifact_crypto.dart';
import 'zip_archive.dart';

class InstalledArtifact {
  const InstalledArtifact({
    required this.definition,
    required this.contentHash,
    required this.directory,
    required this.keyId,
  });

  final CardDefinition definition;
  final String contentHash;
  final Directory directory;
  final String keyId;
}

class ArtifactInstaller {
  ArtifactInstaller({
    required this.root,
    required this.crypto,
    required Map<String, Uint8List> trustedKeys,
  }) : _trustedKeys = Map.unmodifiable({
         for (final entry in trustedKeys.entries)
           entry.key: Uint8List.fromList(entry.value),
       });

  final Directory root;
  final ArtifactCrypto crypto;
  final Map<String, Uint8List> _trustedKeys;

  InstalledArtifact install(Uint8List artifactBytes) {
    final archive = SafeZipArchive.read(artifactBytes);
    final manifest = _readManifest(archive.file('manifest.json'));
    final keyId = _requiredString(manifest, 'keyId');
    final signatureText = _requiredString(manifest, 'signature');
    final publicKey = _trustedKeys[keyId];
    if (publicKey == null) {
      throw FormatException('untrusted artifact signing key: $keyId');
    }

    final unsigned = Map<String, Object?>.from(manifest)..remove('signature');
    final signature = _decodeSignature(signatureText);
    final signedBytes = Uint8List.fromList(
      utf8.encode(canonicalJson(unsigned)),
    );
    if (!crypto.verify(signedBytes, signature, publicKey)) {
      throw const FormatException('artifact signature is invalid');
    }

    final definitionJson = Map<String, Object?>.from(manifest)
      ..remove('keyId')
      ..remove('signature');
    final definition = CardDefinition.fromJson(definitionJson);
    final expectedPaths = {
      'manifest.json',
      for (final file in definition.files) file.path,
    };
    if (archive.paths.length != expectedPaths.length ||
        !archive.paths.containsAll(expectedPaths)) {
      throw const FormatException(
        'artifact files do not exactly match the manifest',
      );
    }
    for (final file in definition.files) {
      final bytes = archive.file(file.path);
      if (bytes.length != file.size ||
          hexEncode(crypto.sha256(bytes)) != file.sha256) {
        throw FormatException('artifact file hash mismatch: ${file.path}');
      }
    }

    final contentHash = hexEncode(crypto.sha256(artifactBytes));
    final cacheRoot = Directory(_joinParts([root.path, 'artifacts', 'sha256']))
      ..createSync(recursive: true);
    final destination = Directory(_joinParts([cacheRoot.path, contentHash]));
    if (!destination.existsSync()) {
      _extractAtomically(
        archive: archive,
        paths: expectedPaths,
        cacheRoot: cacheRoot,
        destination: destination,
      );
    }
    return InstalledArtifact(
      definition: definition,
      contentHash: contentHash,
      directory: destination,
      keyId: keyId,
    );
  }

  void _extractAtomically({
    required SafeZipArchive archive,
    required Set<String> paths,
    required Directory cacheRoot,
    required Directory destination,
  }) {
    final temporary = Directory(
      _joinParts([cacheRoot.path, '.tmp-${_randomHex(16)}']),
    );
    temporary.createSync();
    try {
      for (final path in paths) {
        final target = File(_joinParts([temporary.path, ...path.split('/')]));
        target.parent.createSync(recursive: true);
        target.writeAsBytesSync(archive.file(path), flush: true);
      }
      try {
        temporary.renameSync(destination.path);
      } on FileSystemException {
        if (!destination.existsSync()) {
          rethrow;
        }
      }
    } finally {
      if (temporary.existsSync()) {
        temporary.deleteSync(recursive: true);
      }
    }
  }
}

Map<String, Object?> _readManifest(Uint8List bytes) {
  final Object? decoded;
  try {
    decoded = jsonDecode(utf8.decode(bytes));
  } catch (_) {
    throw const FormatException('manifest.json must be valid UTF-8 JSON');
  }
  if (decoded is! Map<String, Object?>) {
    throw const FormatException('manifest.json must contain an object');
  }
  final manifest = decoded;
  final allowed = {...CardDefinition.jsonKeys, 'keyId', 'signature'};
  final unknown = manifest.keys.where((key) => !allowed.contains(key)).toList();
  if (unknown.isNotEmpty) {
    throw FormatException('unknown manifest fields: ${unknown.join(', ')}');
  }
  if (!manifest.keys.toSet().containsAll(allowed)) {
    final required = {...CardDefinition.jsonKeys, 'keyId', 'signature'}
      ..remove('catalogVersion');
    final missing = required
        .where((key) => !manifest.containsKey(key))
        .toList();
    if (missing.isNotEmpty) {
      throw FormatException('missing manifest fields: ${missing.join(', ')}');
    }
  }
  return manifest;
}

String _requiredString(Map<String, Object?> json, String key) {
  final value = json[key];
  if (value is! String || value.isEmpty) {
    throw FormatException('$key must be a non-empty string');
  }
  return value;
}

Uint8List _decodeSignature(String encoded) {
  try {
    final padding = (4 - encoded.length % 4) % 4;
    final bytes = base64Url.decode(encoded + '=' * padding);
    if (bytes.length != 64) {
      throw const FormatException('signature must contain 64 bytes');
    }
    return Uint8List.fromList(bytes);
  } on FormatException {
    rethrow;
  } catch (_) {
    throw const FormatException('signature must be Base64URL');
  }
}

String _joinParts(Iterable<String> parts) {
  return parts.join(Platform.pathSeparator);
}

String _randomHex(int bytes) {
  final random = Random.secure();
  return List.generate(
    bytes,
    (_) => random.nextInt(256).toRadixString(16).padLeft(2, '0'),
  ).join();
}
