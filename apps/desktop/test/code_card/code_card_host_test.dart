import 'package:agent_card_desktop/src/code_card/code_card_host.dart';
import 'package:agent_card_desktop/src/runtime/local_runtime_server.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test('mounts only with required WebView isolation capabilities', () async {
    final port = _FakeWebViewPort(const WebViewCapabilities.secure());
    final host = CodeCardHost(
      session: _session(),
      entrypoint: '/bundle/hash/index.html',
      webView: port,
      diagnosticMode: false,
    );

    await host.mount();

    expect(host.state, CodeCardHostState.mounted);
    expect(port.initialUri.toString(), contains('/bundle/hash/index.html'));
    expect(port.configuration?.developerToolsEnabled, isFalse);
    expect(
      port.configuration?.policy.allowsNavigation(
        Uri.parse('https://evil.example'),
      ),
      isFalse,
    );
    expect(
      port.configuration?.policy.allowsNavigation(
        Uri.parse('${_session().origin}/bundle/hash/app.js'),
      ),
      isTrue,
    );
    expect(
      port.configuration?.policy.allowsPermission(WebPermission.camera),
      isFalse,
    );
    expect(port.configuration?.policy.allowDownloads, isFalse);
    expect(port.configuration?.policy.allowPopups, isFalse);
  });

  test('fails closed when the platform cannot enforce policy', () async {
    final port = _FakeWebViewPort(
      const WebViewCapabilities(
        embedded: true,
        requestInterception: false,
        isolatedOrigin: true,
      ),
    );
    final host = CodeCardHost(
      session: _session(),
      entrypoint: '/bundle/hash/index.html',
      webView: port,
    );

    await expectLater(
      host.mount(),
      throwsA(isA<CodeCardUnavailableException>()),
    );
    expect(host.state, CodeCardHostState.error);
    expect(port.initialUri, isNull);
  });

  test('suspends, resumes and disposes one isolated WebView', () async {
    final port = _FakeWebViewPort(const WebViewCapabilities.secure());
    final host = CodeCardHost(
      session: _session(),
      entrypoint: '/bundle/hash/index.html',
      webView: port,
    );

    await host.mount();
    await host.suspend();
    expect(host.state, CodeCardHostState.suspended);
    await host.resume();
    expect(host.state, CodeCardHostState.mounted);
    await host.dispose();
    expect(host.state, CodeCardHostState.disposed);
    expect(port.calls, ['mount', 'suspend', 'resume', 'dispose']);
  });

  test('quarantines an instance after three launch failures', () async {
    var quarantined = false;
    final host = CodeCardHost(
      session: _session(),
      entrypoint: '/bundle/hash/index.html',
      webView: _FakeWebViewPort(const WebViewCapabilities.secure()),
      onQuarantine: () async => quarantined = true,
    );

    await host.recordLaunchFailure();
    await host.recordLaunchFailure();
    expect(quarantined, isFalse);
    await host.recordLaunchFailure();

    expect(quarantined, isTrue);
    expect(host.state, CodeCardHostState.quarantined);
  });

  test(
    'persists automatic mount failures and clears them on success',
    () async {
      var failures = 2;
      var successes = 0;
      var quarantined = false;
      final failing = CodeCardHost(
        session: _session(),
        entrypoint: '/bundle/hash/index.html',
        webView: _FakeWebViewPort(
          const WebViewCapabilities.secure(),
          mountError: StateError('renderer crashed'),
        ),
        onLaunchFailure: () async => ++failures,
        onLaunchSuccess: () async => successes++,
        onQuarantine: () async => quarantined = true,
      );

      await expectLater(failing.mount(), throwsStateError);
      expect(failures, 3);
      expect(quarantined, isTrue);
      expect(failing.state, CodeCardHostState.quarantined);

      final successful = CodeCardHost(
        session: _session(),
        entrypoint: '/bundle/hash/index.html',
        webView: _FakeWebViewPort(const WebViewCapabilities.secure()),
        onLaunchFailure: () async => ++failures,
        onLaunchSuccess: () async => successes++,
      );
      await successful.mount();
      expect(successes, 1);
    },
  );
}

RuntimeSession _session() {
  return RuntimeSession(
    id: '0123456789abcdef0123456789abcdef',
    authority: '0123456789abcdef0123456789abcdef.localhost:43125',
    token: 'secret',
    instanceId: 'instance-1',
    cardId: 'card-1',
    versionId: 'version-1',
    resources: const {},
  );
}

class _FakeWebViewPort implements WebViewPort {
  _FakeWebViewPort(this.capabilities, {this.mountError});

  final WebViewCapabilities capabilities;
  final Object? mountError;
  final List<String> calls = [];
  WebViewConfiguration? configuration;
  Uri? initialUri;

  @override
  Future<WebViewCapabilities> inspectCapabilities() async => capabilities;

  @override
  Future<void> mount(WebViewConfiguration configuration, Uri initialUri) async {
    if (mountError case final error?) {
      throw error;
    }
    this.configuration = configuration;
    this.initialUri = initialUri;
    calls.add('mount');
  }

  @override
  Future<void> resume() async => calls.add('resume');

  @override
  Future<void> suspend() async => calls.add('suspend');

  @override
  Future<void> dispose() async => calls.add('dispose');
}
