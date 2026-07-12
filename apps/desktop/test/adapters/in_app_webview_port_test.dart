import 'package:agent_card_desktop/src/adapters/in_app_webview_port.dart';
import 'package:agent_card_desktop/src/code_card/code_card_host.dart';
import 'package:agent_card_desktop/src/runtime/runtime_session.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test('exposes secure capabilities only on an approved platform', () async {
    final supported = InAppWebViewPort(platformSupported: true);
    final unsupported = InAppWebViewPort(platformSupported: false);

    expect(
      (await supported.inspectCapabilities()).canEnforceCodeCardPolicy,
      isTrue,
    );
    expect(
      (await unsupported.inspectCapabilities()).canEnforceCodeCardPolicy,
      isFalse,
    );
  });

  test('receives the host policy and follows lifecycle changes', () async {
    final port = InAppWebViewPort(platformSupported: true);
    final host = CodeCardHost(
      session: _session,
      entrypoint: '/bundle/hash/index.html',
      webView: port,
    );

    await host.mount();
    expect(port.snapshot?.initialUri.host, _session.authority.split(':').first);
    expect(port.snapshot?.active, isTrue);
    await host.suspend();
    expect(port.snapshot?.active, isFalse);
    await host.resume();
    expect(port.snapshot?.active, isTrue);
    await host.dispose();
    expect(port.snapshot, isNull);
  });
}

final _session = RuntimeSession(
  id: '0123456789abcdef0123456789abcdef',
  authority: '0123456789abcdef0123456789abcdef.localhost:43125',
  token: 'secret',
  instanceId: 'instance-1',
  cardId: 'card-1',
  versionId: 'version-1',
  resources: const {},
);
