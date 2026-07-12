import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';

import 'package:agent_card_desktop/src/artifacts/artifact_crypto.dart';

import 'zip_builder.dart';

class SignedArtifactFixture {
  const SignedArtifactFixture({
    required this.bytes,
    required this.publicKey,
    required this.keyId,
    required this.sha256,
  });

  final Uint8List bytes;
  final Uint8List publicKey;
  final String keyId;
  final String sha256;
}

SignedArtifactFixture buildSignedNativeArtifact() {
  const keyId = 'fixture-key';
  final crypto = ArtifactCrypto.native();
  final keys = crypto.keyPairFromSeed(Uint8List(32)..[0] = 41);
  final payload = File(
    '../../contracts/card/fixtures/pomodoro-native.json',
  ).readAsBytesSync();
  final definition =
      jsonDecode(
            File(
              '../../contracts/card/fixtures/native-card.json',
            ).readAsStringSync(),
          )
          as Map<String, Object?>;
  final unsigned = Map<String, Object?>.from(definition)
    ..['keyId'] = keyId
    ..['files'] = [
      {
        'path': 'payload/native.json',
        'sha256': hexEncode(crypto.sha256(payload)),
        'size': payload.length,
      },
    ];
  final signature = crypto.signForTesting(
    Uint8List.fromList(utf8.encode(canonicalJson(unsigned))),
    keys.secretKey,
  );
  final manifest = Map<String, Object?>.from(unsigned)
    ..['signature'] = base64Url.encode(signature).replaceAll('=', '');
  final archive = buildStoredZip([
    ZipTestEntry('manifest.json', utf8.encode(jsonEncode(manifest))),
    ZipTestEntry('payload/native.json', payload),
  ]);
  return SignedArtifactFixture(
    bytes: archive,
    publicKey: keys.publicKey,
    keyId: keyId,
    sha256: hexEncode(crypto.sha256(archive)),
  );
}
