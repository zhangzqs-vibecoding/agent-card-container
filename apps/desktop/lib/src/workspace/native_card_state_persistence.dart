import 'dart:async';

import '../native_card/native_card_controller.dart';

typedef NativeCardStateWriter = void Function(Map<String, Object?> state);

class NativeCardStatePersistence {
  NativeCardStatePersistence({
    required this.controller,
    required this.write,
    this.debounce = const Duration(milliseconds: 300),
  }) {
    controller.addListener(_schedule);
  }

  final NativeCardController controller;
  final NativeCardStateWriter write;
  final Duration debounce;
  Timer? _timer;
  var _dirty = false;
  var _disposed = false;

  void _schedule() {
    if (_disposed) {
      return;
    }
    _dirty = true;
    _timer?.cancel();
    _timer = Timer(debounce, flush);
  }

  void flush() {
    _timer?.cancel();
    _timer = null;
    if (!_dirty || _disposed) {
      return;
    }
    _dirty = false;
    write(controller.state);
  }

  void dispose() {
    if (_disposed) {
      return;
    }
    flush();
    _disposed = true;
    controller.removeListener(_schedule);
  }
}
