import 'package:agent_card_desktop/src/runtime/local_runtime_server.dart';
import 'package:agent_card_desktop/src/runtime/runtime_environment_monitor.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test('refreshes online context from an injected host probe', () async {
    final server = await LocalRuntimeServer.start(initialOnline: true);
    addTearDown(server.close);
    final session = server.createSession(
      instanceId: 'instance-1',
      cardId: 'card-1',
      versionId: 'version-1',
      resources: const {},
    );
    var online = false;
    final monitor = RuntimeEnvironmentMonitor(
      runtimeServer: server,
      onlineProbe: () async => online,
    );
    addTearDown(monitor.dispose);

    await monitor.refresh();
    expect(session.context.online, isFalse);
    online = true;
    await monitor.refresh();
    expect(session.context.online, isTrue);
  });

  test('normalizes host locale names for card context', () {
    expect(normalizeRuntimeLocale('zh_CN.UTF-8'), 'zh-CN');
    expect(normalizeRuntimeLocale('en_US'), 'en-US');
    expect(normalizeRuntimeLocale(''), 'en-US');
  });
}
