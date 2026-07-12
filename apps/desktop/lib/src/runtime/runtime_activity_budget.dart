enum CardRuntimeKind { native, code }

abstract final class RuntimeActivityBudget {
  static const maxNativeCards = 20;
  static const maxCodeCards = 8;

  static Set<int> activeIndexes(
    Iterable<CardRuntimeKind> runtimes, {
    Iterable<bool>? eligible,
  }) {
    final runtimeList = runtimes.toList(growable: false);
    final eligibility =
        eligible?.toList(growable: false) ??
        List.filled(runtimeList.length, true);
    if (eligibility.length != runtimeList.length) {
      throw ArgumentError('runtime eligibility length does not match cards');
    }
    var nativeCount = 0;
    var codeCount = 0;
    final active = <int>{};
    for (final (index, runtime) in runtimeList.indexed) {
      if (!eligibility[index]) continue;
      final allowed = switch (runtime) {
        CardRuntimeKind.native => nativeCount++ < maxNativeCards,
        CardRuntimeKind.code => codeCount++ < maxCodeCards,
      };
      if (allowed) {
        active.add(index);
      }
    }
    return Set.unmodifiable(active);
  }
}
