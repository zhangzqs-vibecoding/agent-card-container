import 'dart:async';

import 'package:flutter/foundation.dart';

import '../cloud/cloud_api_client.dart';

enum AgentStudioPhase {
  idle,
  submitting,
  awaitingConfirmation,
  queued,
  generating,
  validating,
  ready,
  failed,
  cancelled,
  error,
}

abstract interface class GenerationPort {
  Future<GenerationSession> create({
    required String prompt,
    required GenerationTarget target,
    required String locale,
  });

  Future<GenerationSession> addMessage(String sessionId, String content);

  Future<GenerationSession> confirm(String sessionId);

  Future<GenerationSession> cancel(String sessionId);

  Future<GenerationSession> get(String sessionId);

  Stream<GenerationEvent> events(String sessionId, {required int lastEventId});
}

class CloudGenerationPort implements GenerationPort {
  const CloudGenerationPort(this.client);

  final CloudApiClient client;

  @override
  Future<GenerationSession> create({
    required String prompt,
    required GenerationTarget target,
    required String locale,
  }) {
    return client.createGeneration(
      prompt: prompt,
      target: target,
      locale: locale,
    );
  }

  @override
  Future<GenerationSession> addMessage(String sessionId, String content) {
    return client.addMessage(sessionId, content);
  }

  @override
  Future<GenerationSession> cancel(String sessionId) {
    return client.cancelGeneration(sessionId);
  }

  @override
  Future<GenerationSession> confirm(String sessionId) {
    return client.confirmGeneration(sessionId);
  }

  @override
  Stream<GenerationEvent> events(String sessionId, {required int lastEventId}) {
    return client.connectGenerationEvents(sessionId, lastEventId: lastEventId);
  }

  @override
  Future<GenerationSession> get(String sessionId) {
    return client.generation(sessionId);
  }
}

class AgentStudioController extends ChangeNotifier {
  AgentStudioController({
    required this.port,
    this.retryDelay = const Duration(seconds: 1),
  });

  final GenerationPort port;
  final Duration retryDelay;

  AgentStudioPhase _phase = AgentStudioPhase.idle;
  GenerationSession? _session;
  final List<GenerationEvent> _events = [];
  String _errorMessage = '';
  bool _disposed = false;
  int _lastEventId = 0;

  AgentStudioPhase get phase => _phase;
  GenerationSession? get session => _session;
  List<GenerationEvent> get events => List.unmodifiable(_events);
  String get errorMessage => _errorMessage;

  bool get canSubmit =>
      _phase == AgentStudioPhase.idle ||
      _phase == AgentStudioPhase.awaitingConfirmation ||
      _phase == AgentStudioPhase.error;

  Future<void> submit(
    String prompt, {
    GenerationTarget target = GenerationTarget.auto,
    String locale = 'zh-CN',
  }) async {
    final normalized = prompt.trim();
    if (normalized.isEmpty || !canSubmit) {
      return;
    }
    _setPhase(AgentStudioPhase.submitting);
    try {
      final current = _session;
      _session =
          current == null ||
              current.status != GenerationStatus.awaitingConfirmation
          ? await port.create(
              prompt: normalized,
              target: target,
              locale: locale,
            )
          : await port.addMessage(current.id, normalized);
      _errorMessage = '';
      _setPhase(_phaseForStatus(_session!.status));
    } catch (error) {
      _captureError(error);
    }
  }

  Future<void> confirm() async {
    final current = _session;
    if (current == null ||
        current.status != GenerationStatus.awaitingConfirmation) {
      return;
    }
    try {
      _session = await port.confirm(current.id);
      _setPhase(_phaseForStatus(_session!.status));
      unawaited(_follow(current.id));
    } catch (error) {
      _captureError(error);
    }
  }

  Future<void> cancel() async {
    final current = _session;
    if (current == null || _terminal(current.status)) {
      return;
    }
    try {
      _session = await port.cancel(current.id);
      _setPhase(_phaseForStatus(_session!.status));
    } catch (error) {
      _captureError(error);
    }
  }

  Future<void> _follow(String sessionId) async {
    var consecutiveFailures = 0;
    while (!_disposed) {
      try {
        await for (final event in port.events(
          sessionId,
          lastEventId: _lastEventId,
        )) {
          if (_disposed || event.eventId <= _lastEventId) {
            continue;
          }
          _lastEventId = event.eventId;
          _events.add(event);
          _phase = _phaseForStage(event.stage);
          notifyListeners();
        }
        _session = await port.get(sessionId);
        _phase = _phaseForStatus(_session!.status);
        notifyListeners();
        if (_terminal(_session!.status)) {
          return;
        }
        consecutiveFailures = 0;
      } catch (error) {
        consecutiveFailures++;
        if (consecutiveFailures >= 3) {
          _captureError(error);
          return;
        }
      }
      await Future<void>.delayed(retryDelay);
    }
  }

  void reset() {
    _session = null;
    _events.clear();
    _lastEventId = 0;
    _errorMessage = '';
    _setPhase(AgentStudioPhase.idle);
  }

  void _captureError(Object error) {
    _errorMessage = error is CloudApiException ? error.message : '暂时无法连接生成服务';
    _setPhase(AgentStudioPhase.error);
  }

  void _setPhase(AgentStudioPhase value) {
    _phase = value;
    if (!_disposed) {
      notifyListeners();
    }
  }

  @override
  void dispose() {
    _disposed = true;
    super.dispose();
  }
}

AgentStudioPhase _phaseForStatus(GenerationStatus status) {
  return switch (status) {
    GenerationStatus.draft => AgentStudioPhase.submitting,
    GenerationStatus.awaitingConfirmation =>
      AgentStudioPhase.awaitingConfirmation,
    GenerationStatus.queued => AgentStudioPhase.queued,
    GenerationStatus.generating => AgentStudioPhase.generating,
    GenerationStatus.validating => AgentStudioPhase.validating,
    GenerationStatus.ready => AgentStudioPhase.ready,
    GenerationStatus.failed => AgentStudioPhase.failed,
    GenerationStatus.cancelled => AgentStudioPhase.cancelled,
  };
}

AgentStudioPhase _phaseForStage(String stage) {
  return switch (stage) {
    'queued' => AgentStudioPhase.queued,
    'generating' => AgentStudioPhase.generating,
    'validating' => AgentStudioPhase.validating,
    'ready' => AgentStudioPhase.ready,
    'failed' => AgentStudioPhase.failed,
    'cancelled' => AgentStudioPhase.cancelled,
    _ => AgentStudioPhase.submitting,
  };
}

bool _terminal(GenerationStatus status) {
  return status == GenerationStatus.ready ||
      status == GenerationStatus.failed ||
      status == GenerationStatus.cancelled;
}
