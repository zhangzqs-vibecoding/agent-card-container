import 'package:agent_card_desktop/src/agent_studio/agent_studio_controller.dart';
import 'package:flutter_test/flutter_test.dart';

import '../support/fake_generation_port.dart';

void main() {
  test('submits, confirms and follows generation to ready', () async {
    final port = FakeGenerationPort();
    final controller = AgentStudioController(port: port);
    addTearDown(controller.dispose);

    await controller.submit('做一个离线番茄钟');

    expect(controller.phase, AgentStudioPhase.awaitingConfirmation);
    expect(controller.session?.summary.goal, '做一个离线番茄钟');

    await controller.confirm();
    await Future<void>.delayed(Duration.zero);

    expect(controller.phase, AgentStudioPhase.ready);
    expect(controller.session?.versionId, 'ver_01');
    expect(controller.events.map((event) => event.stage), [
      'generating',
      'ready',
    ]);
  });

  test('maps cloud failures into a recoverable UI error', () async {
    final controller = AgentStudioController(
      port: FakeGenerationPort(failCreate: true),
    );
    addTearDown(controller.dispose);

    await controller.submit('失败请求');

    expect(controller.phase, AgentStudioPhase.error);
    expect(controller.errorMessage, isNotEmpty);
    controller.reset();
    expect(controller.phase, AgentStudioPhase.idle);
  });

  test('submits an iteration bound to its base version', () async {
    final port = FakeGenerationPort();
    final controller = AgentStudioController(port: port);
    addTearDown(controller.dispose);

    await controller.submit(
      '增加暂停按钮',
      baseCardId: 'card_01',
      baseVersionId: 'ver_01',
    );

    expect(port.createdBaseCardId, 'card_01');
    expect(port.createdBaseVersionId, 'ver_01');
    expect(controller.session?.baseCardId, 'card_01');
    expect(controller.session?.baseVersionId, 'ver_01');
  });

  test('invokes ready installation callback once per version', () async {
    var installs = 0;
    final controller = AgentStudioController(
      port: FakeGenerationPort(),
      onReady: (session) async {
        expect(session.versionId, 'ver_01');
        installs++;
      },
    );
    addTearDown(controller.dispose);

    await controller.submit('离线卡片');
    await controller.confirm();
    await Future<void>.delayed(Duration.zero);

    expect(installs, 1);
  });
}
