import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';

import 'package:agent_card_desktop/src/artifacts/artifact_crypto.dart';
import 'package:agent_card_desktop/src/artifacts/artifact_installer.dart';
import 'package:flutter_test/flutter_test.dart';

import '../support/zip_builder.dart';

void main() {
  group('ArtifactInstaller', () {
    late Directory root;
    late ArtifactCrypto crypto;
    late ArtifactKeyPair keys;
    late ArtifactInstaller installer;

    setUp(() {
      root = Directory.systemTemp.createTempSync('agent-card-artifacts-');
      crypto = ArtifactCrypto.native();
      keys = crypto.keyPairFromSeed(Uint8List(32)..[0] = 19);
      installer = ArtifactInstaller(
        root: root,
        crypto: crypto,
        trustedKeys: {'test-key': keys.publicKey},
      );
    });

    tearDown(() {
      if (root.existsSync()) {
        root.deleteSync(recursive: true);
      }
    });

    test('verifies and atomically installs a signed CodeCard', () {
      final artifact = _signedArtifact(crypto, keys.secretKey);

      final result = installer.install(artifact);
      final repeated = installer.install(artifact);

      expect(result.definition.runtime.name, 'web');
      expect(result.contentHash, hasLength(64));
      expect(result.directory.path, repeated.directory.path);
      expect(
        File(
          '${result.directory.path}/payload/web/index.html',
        ).readAsStringSync(),
        contains('offline card'),
      );
      expect(
        result.directory.path,
        endsWith('artifacts/sha256/${result.contentHash}'),
      );
    });

    test('rejects payload tampering and leaves no installed directory', () {
      final artifact = _signedArtifact(
        crypto,
        keys.secretKey,
        payload: utf8.encode('<script>tampered</script>'),
        signedPayload: utf8.encode('<script>offline card</script>'),
      );

      expect(() => installer.install(artifact), throwsFormatException);
      final cache = Directory('${root.path}/artifacts/sha256');
      expect(!cache.existsSync() || cache.listSync().isEmpty, isTrue);
    });

    test('rejects unknown signing keys and extra files', () {
      expect(
        () => installer.install(
          _signedArtifact(crypto, keys.secretKey, keyId: 'unknown'),
        ),
        throwsFormatException,
      );
      expect(
        () => installer.install(
          _signedArtifact(crypto, keys.secretKey, includeExtraFile: true),
        ),
        throwsFormatException,
      );
    });
  });
}

Uint8List _signedArtifact(
  ArtifactCrypto crypto,
  Uint8List secretKey, {
  String keyId = 'test-key',
  List<int>? payload,
  List<int>? signedPayload,
  bool includeExtraFile = false,
}) {
  final actualPayload = payload ?? utf8.encode('<script>offline card</script>');
  final hashedPayload = signedPayload ?? actualPayload;
  final fixture =
      jsonDecode(
            File(
              '../../contracts/card/fixtures/web-card.json',
            ).readAsStringSync(),
          )
          as Map<String, Object?>;
  final unsigned = Map<String, Object?>.from(fixture)
    ..['keyId'] = keyId
    ..['files'] = [
      {
        'path': 'payload/web/index.html',
        'sha256': hexEncode(crypto.sha256(Uint8List.fromList(hashedPayload))),
        'size': hashedPayload.length,
      },
    ];
  final signature = crypto.signForTesting(
    Uint8List.fromList(utf8.encode(canonicalJson(unsigned))),
    secretKey,
  );
  final manifest = Map<String, Object?>.from(unsigned)
    ..['signature'] = base64Url.encode(signature).replaceAll('=', '');
  return buildStoredZip([
    ZipTestEntry('manifest.json', utf8.encode(jsonEncode(manifest))),
    ZipTestEntry('payload/web/index.html', actualPayload),
    if (includeExtraFile)
      const ZipTestEntry('payload/web/hidden.js', [1, 2, 3]),
  ]);
}
