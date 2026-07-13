import 'native_card_catalog_generated.dart';

class NativeCardEvaluationException implements Exception {
  const NativeCardEvaluationException(this.message);

  final String message;

  @override
  String toString() => 'NativeCardEvaluationException: $message';
}

class NativeExpressionEvaluator {
  const NativeExpressionEvaluator();

  Object? evaluate(Object? expression, Map<String, Object?> state) {
    return _evaluate(expression, state, 0);
  }

  Object? _evaluate(Object? expression, Map<String, Object?> state, int depth) {
    if (depth > 32) {
      throw const NativeCardEvaluationException('expression exceeds depth 32');
    }
    if (expression is! Map<String, Object?>) {
      return expression;
    }
    if (expression.length == 1 && expression['path'] is String) {
      return _resolvePath(expression['path']! as String, state);
    }
    if (expression.length == 1 && expression['expr'] is Map<String, Object?>) {
      return _evaluate(expression['expr'], state, depth + 1);
    }

    if (expression.length != 2 ||
        !expression.containsKey('op') ||
        !expression.containsKey('args')) {
      return expression;
    }
    final operation = expression['op'];
    final rawArguments = expression['args'];
    if (operation is! String || rawArguments is! List) {
      return expression;
    }
    final arity = nativeCatalogExpressionArity[operation];
    if (arity == null) {
      throw NativeCardEvaluationException(
        'unknown expression operation: $operation',
      );
    }
    if (rawArguments.length < arity.min ||
        (arity.max != null && rawArguments.length > arity.max!)) {
      throw NativeCardEvaluationException(
        '$operation has invalid operand count',
      );
    }
    final arguments = rawArguments
        .map((item) => _evaluate(item, state, depth + 1))
        .toList();

    switch (operation) {
      case 'add':
        return _numbers(
          operation,
          arguments,
        ).fold<double>(0, (sum, value) => sum + value);
      case 'subtract':
        final values = _numbers(operation, arguments, exactLength: 2);
        return values[0] - values[1];
      case 'multiply':
        return _numbers(
          operation,
          arguments,
        ).fold<double>(1, (product, value) => product * value);
      case 'divide':
        final values = _numbers(operation, arguments, exactLength: 2);
        if (values[1] == 0) {
          throw const NativeCardEvaluationException('division by zero');
        }
        return values[0] / values[1];
      case 'eq':
        _requireLength(operation, arguments, 2);
        return arguments[0] == arguments[1];
      case 'gt':
        final values = _numbers(operation, arguments, exactLength: 2);
        return values[0] > values[1];
      case 'gte':
        final values = _numbers(operation, arguments, exactLength: 2);
        return values[0] >= values[1];
      case 'lt':
        final values = _numbers(operation, arguments, exactLength: 2);
        return values[0] < values[1];
      case 'lte':
        final values = _numbers(operation, arguments, exactLength: 2);
        return values[0] <= values[1];
      case 'and':
        return _booleans(operation, arguments).every((value) => value);
      case 'or':
        return _booleans(operation, arguments).any((value) => value);
      case 'not':
        final values = _booleans(operation, arguments, exactLength: 1);
        return !values.single;
      case 'concat':
        return arguments.map((value) => value?.toString() ?? '').join();
      case 'formatDuration':
        final values = _numbers(operation, arguments, exactLength: 1);
        final seconds = values.single.floor();
        if (seconds < 0) {
          throw const NativeCardEvaluationException(
            'formatDuration requires a non-negative value',
          );
        }
        final minutesPart = (seconds ~/ 60).toString().padLeft(2, '0');
        final secondsPart = (seconds % 60).toString().padLeft(2, '0');
        return '$minutesPart:$secondsPart';
      default:
        throw NativeCardEvaluationException(
          'unknown expression operation: $operation',
        );
    }
  }

  Object? _resolvePath(String path, Map<String, Object?> state) {
    final segments = path.split('.');
    if (segments.length < 2 || segments.first != 'state') {
      throw NativeCardEvaluationException('invalid state path: $path');
    }

    Object? current = state;
    for (final segment in segments.skip(1)) {
      if (current is! Map<String, Object?> || !current.containsKey(segment)) {
        throw NativeCardEvaluationException('state path not found: $path');
      }
      current = current[segment];
    }
    return current;
  }

  List<double> _numbers(
    String operation,
    List<Object?> arguments, {
    int? exactLength,
  }) {
    if (exactLength != null) {
      _requireLength(operation, arguments, exactLength);
    }
    if (arguments.isEmpty) {
      throw NativeCardEvaluationException('$operation requires operands');
    }
    return arguments.map((value) {
      if (value is! num) {
        throw NativeCardEvaluationException(
          '$operation requires numeric operands',
        );
      }
      return value.toDouble();
    }).toList();
  }

  List<bool> _booleans(
    String operation,
    List<Object?> arguments, {
    int? exactLength,
  }) {
    if (exactLength != null) {
      _requireLength(operation, arguments, exactLength);
    }
    if (arguments.isEmpty) {
      throw NativeCardEvaluationException('$operation requires operands');
    }
    return arguments.map((value) {
      if (value is! bool) {
        throw NativeCardEvaluationException(
          '$operation requires boolean operands',
        );
      }
      return value;
    }).toList();
  }

  void _requireLength(String operation, List<Object?> arguments, int length) {
    if (arguments.length != length) {
      throw NativeCardEvaluationException(
        '$operation requires $length operands',
      );
    }
  }
}
