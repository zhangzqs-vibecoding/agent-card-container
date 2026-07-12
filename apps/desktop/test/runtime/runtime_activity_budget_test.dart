import 'package:agent_card_desktop/src/runtime/runtime_activity_budget.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test('activates no more than twenty native and eight code cards', () {
    final runtimes = [
      ...List.filled(22, CardRuntimeKind.native),
      ...List.filled(10, CardRuntimeKind.code),
    ];

    final active = RuntimeActivityBudget.activeIndexes(runtimes);

    expect(active.where((index) => index < 22), hasLength(20));
    expect(active.where((index) => index >= 22), hasLength(8));
    expect(active, isNot(contains(20)));
    expect(active, isNot(contains(30)));
  });

  test('preserves input order independently for each runtime', () {
    final active = RuntimeActivityBudget.activeIndexes([
      CardRuntimeKind.code,
      CardRuntimeKind.native,
      CardRuntimeKind.code,
    ]);

    expect(active, {0, 1, 2});
  });

  test('does not allocate CodeCard slots to invisible cards', () {
    final active = RuntimeActivityBudget.activeIndexes(
      List.filled(10, CardRuntimeKind.code),
      eligible: [false, ...List.filled(9, true)],
    );

    expect(active, isNot(contains(0)));
    expect(active, containsAll(List.generate(8, (index) => index + 1)));
    expect(active, isNot(contains(9)));
  });
}
