import 'package:agent_card_desktop/src/native_card/expression.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  group('NativeExpressionEvaluator', () {
    const evaluator = NativeExpressionEvaluator();
    const state = {
      'count': 3,
      'enabled': true,
      'title': 'Focus',
      'nested': {'value': 8},
    };

    test('resolves state paths', () {
      expect(evaluator.evaluate({'path': 'state.nested.value'}, state), 8);
    });

    test('evaluates arithmetic and comparisons', () {
      expect(
        evaluator.evaluate({
          'op': 'multiply',
          'args': [
            {
              'op': 'add',
              'args': [
                {'path': 'state.count'},
                2,
              ],
            },
            4,
          ],
        }, state),
        20,
      );
      expect(
        evaluator.evaluate({
          'op': 'gt',
          'args': [
            {'path': 'state.nested.value'},
            5,
          ],
        }, state),
        isTrue,
      );
    });

    test('evaluates boolean and string expressions', () {
      expect(
        evaluator.evaluate({
          'op': 'and',
          'args': [
            {'path': 'state.enabled'},
            {
              'op': 'eq',
              'args': [
                {'path': 'state.title'},
                'Focus',
              ],
            },
          ],
        }, state),
        isTrue,
      );
      expect(
        evaluator.evaluate({
          'op': 'concat',
          'args': [
            {'path': 'state.title'},
            ' #',
            {'path': 'state.count'},
          ],
        }, state),
        'Focus #3',
      );
      expect(
        evaluator.evaluate({
          'op': 'formatDuration',
          'args': [125],
        }, state),
        '02:05',
      );
    });

    test('rejects unknown operations and wrong operand types', () {
      expect(
        () => evaluator.evaluate({'op': 'shell', 'args': const []}, state),
        throwsA(isA<NativeCardEvaluationException>()),
      );
      expect(
        () => evaluator.evaluate({
          'op': 'add',
          'args': ['one', 2],
        }, state),
        throwsA(isA<NativeCardEvaluationException>()),
      );
    });

    test('treats maps with extra expression fields as ordinary values', () {
      final value = <String, Object?>{
        'op': 'add',
        'args': [1],
        'extra': true,
      };

      expect(evaluator.evaluate(value, state), same(value));
    });

    test('rejects expression recursion beyond 32', () {
      Object? expression = 1;
      for (var index = 0; index < 33; index++) {
        expression = {
          'op': 'add',
          'args': [expression, 1],
        };
      }

      expect(
        () => evaluator.evaluate(expression, state),
        throwsA(isA<NativeCardEvaluationException>()),
      );
    });
  });
}
