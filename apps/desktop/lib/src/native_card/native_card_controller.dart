import 'dart:async';

import 'package:flutter/foundation.dart';

import '../capabilities/capability.dart';
import '../capabilities/capability_broker.dart';
import 'expression.dart';
import 'native_card_spec.dart';

typedef NativeCapabilityInvocation =
    Future<Object?> Function(String method, Map<String, Object?> params);

class NativeCardActionException implements Exception {
  const NativeCardActionException(this.message);

  final String message;

  @override
  String toString() => 'NativeCardActionException: $message';
}

class NativeCardController extends ChangeNotifier {
  NativeCardController(
    Map<String, Object?> initialState, {
    NativeExpressionEvaluator evaluator = const NativeExpressionEvaluator(),
    CapabilityBroker? capabilityBroker,
    CardContext? cardContext,
    NativeCapabilityInvocation? capabilityInvocation,
  }) : _state = _cloneMap(initialState),
       _evaluator = evaluator,
       _capabilityBroker = capabilityBroker,
       _cardContext = cardContext,
       _capabilityInvocation = capabilityInvocation;

  final Map<String, Object?> _state;
  final NativeExpressionEvaluator _evaluator;
  final CapabilityBroker? _capabilityBroker;
  final CardContext? _cardContext;
  final NativeCapabilityInvocation? _capabilityInvocation;
  final Map<String, Timer> _timers = {};

  Map<String, Object?> get state => Map.unmodifiable(_cloneMap(_state));

  void applyActions(Iterable<NativeAction> actions) {
    for (final action in actions) {
      applyAction(action);
    }
  }

  Future<void> applyActionsAsync(Iterable<NativeAction> actions) async {
    for (final action in actions) {
      if (action.type == NativeActionType.capabilityInvoke) {
        await _applyCapability(action);
      } else {
        applyAction(action);
      }
    }
  }

  void applyAction(NativeAction action) {
    final path = action.path;
    switch (action.type) {
      case NativeActionType.set:
        _write(path, _evaluateValue(action.value));
      case NativeActionType.increment:
        final current = _read(path);
        final increment = _evaluateValue(action.value ?? 1);
        if (current is! num || increment is! num) {
          throw const NativeCardActionException(
            'increment requires numeric state and value',
          );
        }
        _write(path, current + increment);
      case NativeActionType.toggle:
        final current = _read(path);
        if (current is! bool) {
          throw const NativeCardActionException(
            'toggle requires boolean state',
          );
        }
        _write(path, !current);
      case NativeActionType.append:
        final current = _read(path);
        if (current is! List) {
          throw const NativeCardActionException('append requires list state');
        }
        _write(path, [...current, _evaluateValue(action.value)]);
      case NativeActionType.remove:
        final current = _read(path);
        if (current is! List) {
          throw const NativeCardActionException('remove requires list state');
        }
        final value = _evaluateValue(action.value);
        _write(path, [...current]..remove(value));
      case NativeActionType.startTimer:
        _startTimer(path, action.value);
      case NativeActionType.stopTimer:
        _stopTimer(path);
      case NativeActionType.capabilityInvoke:
        throw const NativeCardActionException(
          'capability broker is not attached',
        );
    }
  }

  Object? resolve(Object? binding) => _evaluator.evaluate(binding, _state);

  Future<void> _applyCapability(NativeAction action) async {
    final broker = _capabilityBroker;
    final context = _cardContext;
    final method = action.method;
    final invocation = _capabilityInvocation;
    if ((invocation == null && (broker == null || context == null)) ||
        method == null) {
      _setCapabilityError(
        CapabilityErrorCode.capabilityUnavailable,
        'capability broker is not attached',
      );
      return;
    }

    try {
      final result = invocation != null
          ? await invocation(method, action.params)
          : await broker!.invoke(
              context!.withUserGesture(true),
              method,
              action.params,
            );
      _state.remove('_capabilityError');
      if (action.path case final String path when path.isNotEmpty) {
        _write(path, result);
      } else {
        notifyListeners();
      }
    } on CapabilityException catch (error) {
      _setCapabilityError(error.code, error.message);
    } catch (_) {
      _setCapabilityError(
        CapabilityErrorCode.capabilityUnavailable,
        'capability bridge is unavailable',
      );
    }
  }

  void _setCapabilityError(CapabilityErrorCode code, String message) {
    _state['_capabilityError'] = {'code': code.name, 'message': message};
    notifyListeners();
  }

  Object? _evaluateValue(Object? value) {
    return _evaluator.evaluate(value, _state);
  }

  Object? _read(String? path) {
    final segments = _segments(path);
    Object? current = _state;
    for (final segment in segments) {
      if (current is! Map<String, Object?> || !current.containsKey(segment)) {
        throw NativeCardActionException(
          'state path not found: ${segments.join('.')}',
        );
      }
      current = current[segment];
    }
    return current;
  }

  void _write(String? path, Object? value) {
    final segments = _segments(path);
    Map<String, Object?> current = _state;
    for (final segment in segments.take(segments.length - 1)) {
      final next = current[segment];
      if (next is! Map<String, Object?>) {
        throw NativeCardActionException(
          'state path not found: ${segments.join('.')}',
        );
      }
      current = next;
    }
    current[segments.last] = _cloneValue(value);
    notifyListeners();
  }

  List<String> _segments(String? path) {
    if (path == null || path.isEmpty) {
      throw const NativeCardActionException('action path is required');
    }
    final segments = path.split('.');
    if (segments.any((segment) => segment.isEmpty)) {
      throw NativeCardActionException('invalid action path: $path');
    }
    return segments;
  }

  void _startTimer(String? path, Object? configuration) {
    final timerPath = _segments(path).join('.');
    if (configuration is! Map<String, Object?>) {
      throw const NativeCardActionException(
        'startTimer requires configuration',
      );
    }
    final intervalMs = configuration['intervalMs'];
    final delta = configuration['delta'];
    final stopAt = configuration['stopAt'];
    if (intervalMs is! int || intervalMs <= 0 || delta is! num) {
      throw const NativeCardActionException(
        'startTimer requires positive intervalMs and numeric delta',
      );
    }
    if (stopAt != null && stopAt is! num) {
      throw const NativeCardActionException(
        'startTimer stopAt must be numeric',
      );
    }
    if (_read(timerPath) is! num) {
      throw const NativeCardActionException(
        'startTimer requires numeric state',
      );
    }

    _timers.remove(timerPath)?.cancel();
    _timers[timerPath] = Timer.periodic(Duration(milliseconds: intervalMs), (
      timer,
    ) {
      final current = _read(timerPath);
      if (current is! num) {
        timer.cancel();
        _timers.remove(timerPath);
        return;
      }
      var next = current + delta;
      if (stopAt is num &&
          ((delta < 0 && next <= stopAt) || (delta > 0 && next >= stopAt))) {
        next = stopAt;
        timer.cancel();
        _timers.remove(timerPath);
      }
      _write(timerPath, next);
    });
  }

  void _stopTimer(String? path) {
    final timerPath = _segments(path).join('.');
    _timers.remove(timerPath)?.cancel();
  }

  @override
  void dispose() {
    for (final timer in _timers.values) {
      timer.cancel();
    }
    _timers.clear();
    super.dispose();
  }
}

Map<String, Object?> _cloneMap(Map<String, Object?> source) {
  return source.map((key, value) => MapEntry(key, _cloneValue(value)));
}

Object? _cloneValue(Object? value) {
  if (value is Map<String, Object?>) {
    return _cloneMap(value);
  }
  if (value is List) {
    return value.map(_cloneValue).toList();
  }
  return value;
}
