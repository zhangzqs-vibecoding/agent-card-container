enum CardRuntimeKind { native, code }

abstract final class RuntimeActivityBudget {
  static const maxNativeCards = 20;
  static const maxCodeCards = 8;

  static Set<int> activeIndexes(Iterable<CardRuntimeKind> runtimes) {
    var nativeCount = 0;
    var codeCount = 0;
    final active = <int>{};
    var index = 0;
    for (final runtime in runtimes) {
      final allowed = switch (runtime) {
        CardRuntimeKind.native => nativeCount++ < maxNativeCards,
        CardRuntimeKind.code => codeCount++ < maxCodeCards,
      };
      if (allowed) {
        active.add(index);
      }
      index++;
    }
    return Set.unmodifiable(active);
  }
}
