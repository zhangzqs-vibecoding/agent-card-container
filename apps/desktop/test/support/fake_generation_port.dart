import 'package:agent_card_desktop/src/agent_studio/agent_studio_controller.dart';
import 'package:agent_card_desktop/src/cloud/cloud_api_client.dart';

class FakeGenerationPort implements GenerationPort {
  FakeGenerationPort({this.failCreate = false});

  final bool failCreate;

  @override
  Future<GenerationSession> create({
    required String prompt,
    required GenerationTarget target,
    required String locale,
  }) async {
    if (failCreate) {
      throw const CloudApiException(
        statusCode: 500,
        code: 'INTERNAL',
        message: '服务不可用',
      );
    }
    return fakeGenerationSession(
      status: GenerationStatus.awaitingConfirmation,
      prompt: prompt,
    );
  }

  @override
  Future<GenerationSession> addMessage(String sessionId, String content) async {
    return fakeGenerationSession(
      status: GenerationStatus.awaitingConfirmation,
      prompt: content,
    );
  }

  @override
  Future<GenerationSession> confirm(String sessionId) async {
    return fakeGenerationSession(status: GenerationStatus.queued);
  }

  @override
  Future<GenerationSession> cancel(String sessionId) async {
    return fakeGenerationSession(status: GenerationStatus.cancelled);
  }

  @override
  Stream<GenerationEvent> events(
    String sessionId, {
    required int lastEventId,
  }) async* {
    yield GenerationEvent(
      eventId: 2,
      sessionId: sessionId,
      type: 'status.changed',
      stage: 'generating',
      message: '生成中',
      progress: .4,
      timestamp: DateTime.utc(2026, 7, 12),
    );
    yield GenerationEvent(
      eventId: 3,
      sessionId: sessionId,
      type: 'status.changed',
      stage: 'ready',
      message: '完成',
      progress: 1,
      versionId: 'ver_01',
      timestamp: DateTime.utc(2026, 7, 12),
    );
  }

  @override
  Future<GenerationSession> get(String sessionId) async {
    return fakeGenerationSession(
      status: GenerationStatus.ready,
      versionId: 'ver_01',
    );
  }
}

GenerationSession fakeGenerationSession({
  required GenerationStatus status,
  String prompt = '做一个离线番茄钟',
  String? versionId,
}) {
  return GenerationSession(
    id: 'gen_01',
    prompt: prompt,
    target: GenerationTarget.auto,
    locale: 'zh-CN',
    status: status,
    summary: RequirementSummary(
      goal: prompt,
      constraints: const ['可离线'],
      locale: 'zh-CN',
    ),
    versionId: versionId,
    createdAt: DateTime.utc(2026, 7, 12),
    updatedAt: DateTime.utc(2026, 7, 12),
  );
}
