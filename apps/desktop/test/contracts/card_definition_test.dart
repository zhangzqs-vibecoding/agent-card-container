import 'dart:convert';
import 'dart:io';

import 'package:agent_card_desktop/src/contracts/card_definition.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  group('CardDefinition', () {
    for (final testCase
        in <
          ({
            String fixture,
            CardRuntime runtime,
            String entrypoint,
            String capability,
          })
        >[
          (
            fixture: 'native-card.json',
            runtime: CardRuntime.native,
            entrypoint: 'payload/native.json',
            capability: 'notification.show',
          ),
          (
            fixture: 'web-card.json',
            runtime: CardRuntime.web,
            entrypoint: 'payload/web/index.html',
            capability: 'storage',
          ),
        ]) {
      test('parses ${testCase.runtime.name} fixture', () {
        final definition = CardDefinition.fromJson(
          jsonDecode(_readFixture(testCase.fixture)) as Map<String, Object?>,
        );

        expect(definition.runtime, testCase.runtime);
        expect(definition.entrypoint, testCase.entrypoint);
        expect(
          definition.preferredSize.width,
          greaterThan(definition.minSize.width),
        );
        expect(definition.hasCapability(testCase.capability), isTrue);
      });
    }

    test('rejects an unknown runtime', () {
      final source = _readFixture(
        'web-card.json',
      ).replaceFirst('"runtime": "web"', '"runtime": "script"');

      expect(
        () =>
            CardDefinition.fromJson(jsonDecode(source) as Map<String, Object?>),
        throwsFormatException,
      );
    });

    test('rejects unsafe or duplicate artifact paths', () {
      final fixture =
          jsonDecode(_readFixture('web-card.json')) as Map<String, Object?>;
      final file = Map<String, Object?>.from(
        (fixture['files']! as List).single as Map<String, Object?>,
      );

      for (final path in [
        '../escape.html',
        '/absolute.html',
        r'C:\escape.html',
        'payload//index.html',
        'a/b/c/d/e/f/g/h/i.html',
      ]) {
        final candidate = Map<String, Object?>.from(fixture)
          ..['files'] = [Map<String, Object?>.from(file)..['path'] = path];
        expect(() => CardDefinition.fromJson(candidate), throwsFormatException);
      }

      final duplicate = Map<String, Object?>.from(fixture)
        ..['files'] = [file, Map<String, Object?>.from(file)];
      expect(() => CardDefinition.fromJson(duplicate), throwsFormatException);
    });

    test(
      'rejects invalid hashes, oversized files and unknown capabilities',
      () {
        final fixture =
            jsonDecode(_readFixture('web-card.json')) as Map<String, Object?>;
        final original =
            (fixture['files']! as List).single as Map<String, Object?>;

        for (final mutation in [
          Map<String, Object?>.from(original)..['sha256'] = 'not-a-hash',
          Map<String, Object?>.from(original)..['size'] = 8388609,
        ]) {
          final candidate = Map<String, Object?>.from(fixture)
            ..['files'] = [mutation];
          expect(
            () => CardDefinition.fromJson(candidate),
            throwsFormatException,
          );
        }

        final unknownCapability = Map<String, Object?>.from(fixture)
          ..['capabilities'] = ['shell.execute'];
        expect(
          () => CardDefinition.fromJson(unknownCapability),
          throwsFormatException,
        );
      },
    );
  });
}

String _readFixture(String name) {
  return File('../../contracts/card/fixtures/$name').readAsStringSync();
}
