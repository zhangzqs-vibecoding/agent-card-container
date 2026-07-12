import 'package:agent_card_desktop/src/adapters/application_restarter.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test(
    'closes current runtime only after replacement launch succeeds',
    () async {
      final events = <String>[];
      final restarter = ApplicationRestarter(
        executable: 'agent_card_desktop.exe',
        launch: (executable) async {
          expect(executable, 'agent_card_desktop.exe');
          events.add('launched');
        },
      );

      await restarter.restart(shutdown: () async => events.add('shutdown'));

      expect(events, ['launched', 'shutdown']);
    },
  );

  test('launch failure leaves current runtime active', () async {
    var shutdown = false;
    final restarter = ApplicationRestarter(
      executable: 'agent_card_desktop.exe',
      launch: (_) async => throw StateError('cannot launch'),
    );

    await expectLater(
      restarter.restart(shutdown: () async => shutdown = true),
      throwsStateError,
    );

    expect(shutdown, isFalse);
  });
}
