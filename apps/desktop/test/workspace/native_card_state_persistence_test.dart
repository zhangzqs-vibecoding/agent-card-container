import 'package:agent_card_desktop/src/native_card/native_card_controller.dart';
import 'package:agent_card_desktop/src/native_card/native_card_spec.dart';
import 'package:agent_card_desktop/src/workspace/native_card_state_persistence.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test(
    'debounces controller changes into one complete state snapshot',
    () async {
      final controller = NativeCardController({'count': 0});
      final writes = <Map<String, Object?>>[];
      final persistence = NativeCardStatePersistence(
        controller: controller,
        debounce: const Duration(milliseconds: 20),
        write: (state) => writes.add(state),
      );
      addTearDown(controller.dispose);
      addTearDown(persistence.dispose);

      controller.applyAction(
        const NativeAction(
          type: NativeActionType.increment,
          path: 'count',
          value: 1,
        ),
      );
      controller.applyAction(
        const NativeAction(
          type: NativeActionType.increment,
          path: 'count',
          value: 1,
        ),
      );
      await Future<void>.delayed(const Duration(milliseconds: 40));

      expect(writes, [
        {'count': 2},
      ]);
    },
  );

  test('flush writes pending state before a card is disposed', () {
    final controller = NativeCardController({'enabled': false});
    final writes = <Map<String, Object?>>[];
    final persistence = NativeCardStatePersistence(
      controller: controller,
      write: (state) => writes.add(state),
    );
    addTearDown(controller.dispose);
    addTearDown(persistence.dispose);
    controller.applyAction(
      const NativeAction(type: NativeActionType.toggle, path: 'enabled'),
    );

    persistence.flush();

    expect(writes, [
      {'enabled': true},
    ]);
  });
}
