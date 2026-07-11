import 'package:agent_card_desktop/src/native_card/native_card_controller.dart';
import 'package:agent_card_desktop/src/native_card/native_card_spec.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  group('NativeCardController', () {
    test('applies local state actions', () {
      final controller = NativeCardController({
        'count': 1,
        'enabled': false,
        'items': [1],
        'title': 'Old',
      });
      addTearDown(controller.dispose);

      controller.applyActions([
        const NativeAction(
          type: NativeActionType.set,
          path: 'title',
          value: 'Focus',
        ),
        const NativeAction(
          type: NativeActionType.increment,
          path: 'count',
          value: 2,
        ),
        const NativeAction(type: NativeActionType.toggle, path: 'enabled'),
        const NativeAction(
          type: NativeActionType.append,
          path: 'items',
          value: 2,
        ),
        const NativeAction(
          type: NativeActionType.remove,
          path: 'items',
          value: 1,
        ),
      ]);

      expect(controller.state['title'], 'Focus');
      expect(controller.state['count'], 3);
      expect(controller.state['enabled'], isTrue);
      expect(controller.state['items'], [2]);
    });

    test('evaluates bound values before setting state', () {
      final controller = NativeCardController({'count': 2});
      addTearDown(controller.dispose);

      controller.applyAction(
        const NativeAction(
          type: NativeActionType.set,
          path: 'total',
          value: {
            'expr': {
              'op': 'multiply',
              'args': [
                {'path': 'state.count'},
                5,
              ],
            },
          },
        ),
      );

      expect(controller.state['total'], 10);
    });

    test('starts and stops a local timer', () async {
      final controller = NativeCardController({'remaining': 5});
      addTearDown(controller.dispose);

      controller.applyAction(
        const NativeAction(
          type: NativeActionType.startTimer,
          path: 'remaining',
          value: {'intervalMs': 10, 'delta': -1, 'stopAt': 0},
        ),
      );
      await Future<void>.delayed(const Duration(milliseconds: 35));
      final afterTicks = controller.state['remaining']! as num;
      expect(afterTicks, lessThan(5));

      controller.applyAction(
        const NativeAction(type: NativeActionType.stopTimer, path: 'remaining'),
      );
      final stoppedAt = controller.state['remaining'];
      await Future<void>.delayed(const Duration(milliseconds: 25));
      expect(controller.state['remaining'], stoppedAt);
    });

    test('rejects actions that do not match state types', () {
      final controller = NativeCardController({'count': 'one'});
      addTearDown(controller.dispose);

      expect(
        () => controller.applyAction(
          const NativeAction(
            type: NativeActionType.increment,
            path: 'count',
            value: 1,
          ),
        ),
        throwsA(isA<NativeCardActionException>()),
      );
    });
  });
}
