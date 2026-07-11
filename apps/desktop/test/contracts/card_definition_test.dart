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
  });
}

String _readFixture(String name) {
  return File('../../contracts/card/fixtures/$name').readAsStringSync();
}
