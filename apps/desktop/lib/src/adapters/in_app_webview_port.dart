import 'dart:io';
import 'dart:typed_data';

import 'package:flutter/material.dart';
import 'package:flutter_inappwebview/flutter_inappwebview.dart';

import '../code_card/code_card_host.dart';

class InAppWebViewSnapshot {
  const InAppWebViewSnapshot({
    required this.configuration,
    required this.initialUri,
    required this.active,
  });

  final WebViewConfiguration configuration;
  final Uri initialUri;
  final bool active;
}

class InAppWebViewPort implements WebViewPort {
  InAppWebViewPort({bool? platformSupported})
    : platformSupported = platformSupported ?? Platform.isWindows;

  final bool platformSupported;
  InAppWebViewSnapshot? _snapshot;
  InAppWebViewController? _controller;
  final ValueNotifier<int> changes = ValueNotifier(0);

  InAppWebViewSnapshot? get snapshot => _snapshot;

  @override
  Future<WebViewCapabilities> inspectCapabilities() async {
    return WebViewCapabilities(
      embedded: platformSupported,
      requestInterception: platformSupported,
      isolatedOrigin: platformSupported,
    );
  }

  @override
  Future<void> mount(WebViewConfiguration configuration, Uri initialUri) async {
    _snapshot = InAppWebViewSnapshot(
      configuration: configuration,
      initialUri: initialUri,
      active: true,
    );
    _notify();
  }

  @override
  Future<void> suspend() async {
    final current = _snapshot;
    if (current == null) {
      return;
    }
    _snapshot = InAppWebViewSnapshot(
      configuration: current.configuration,
      initialUri: current.initialUri,
      active: false,
    );
    await _controller?.pauseTimers();
    await _controller?.pause();
    _notify();
  }

  @override
  Future<void> resume() async {
    final current = _snapshot;
    if (current == null) {
      return;
    }
    await _controller?.resume();
    await _controller?.resumeTimers();
    _snapshot = InAppWebViewSnapshot(
      configuration: current.configuration,
      initialUri: current.initialUri,
      active: true,
    );
    _notify();
  }

  @override
  Future<void> dispose() async {
    await _controller?.stopLoading();
    _controller = null;
    _snapshot = null;
    _notify();
    changes.dispose();
  }

  void attach(InAppWebViewController controller) {
    _controller = controller;
  }

  void _notify() {
    changes.value++;
  }
}

class CodeCardWebView extends StatelessWidget {
  const CodeCardWebView({required this.port, super.key});

  final InAppWebViewPort port;

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: port.changes,
      builder: (context, _) {
        final snapshot = port.snapshot;
        if (snapshot == null) {
          return const SizedBox.shrink();
        }
        final policy = snapshot.configuration.policy;
        return IgnorePointer(
          ignoring: !snapshot.active,
          child: InAppWebView(
            initialUrlRequest: URLRequest(
              url: WebUri(snapshot.initialUri.toString()),
            ),
            initialSettings: InAppWebViewSettings(
              useShouldOverrideUrlLoading: true,
              useShouldInterceptRequest: true,
              javaScriptCanOpenWindowsAutomatically: false,
              supportMultipleWindows: false,
              allowFileAccess: false,
              allowContentAccess: false,
              allowFileAccessFromFileURLs: false,
              allowUniversalAccessFromFileURLs: false,
              geolocationEnabled: false,
              thirdPartyCookiesEnabled: false,
              isInspectable: snapshot.configuration.developerToolsEnabled,
              cacheEnabled: true,
              domStorageEnabled: true,
            ),
            onWebViewCreated: port.attach,
            shouldOverrideUrlLoading: (_, navigation) async {
              final url = navigation.request.url;
              return url != null && policy.allowsNavigation(Uri.parse('$url'))
                  ? NavigationActionPolicy.ALLOW
                  : NavigationActionPolicy.CANCEL;
            },
            shouldInterceptRequest: (_, request) async {
              final url = request.url;
              if (policy.allowsResource(Uri.parse('$url'))) {
                return null;
              }
              return WebResourceResponse(
                data: Uint8List(0),
                statusCode: HttpStatus.forbidden,
                reasonPhrase: 'Forbidden',
                contentType: 'text/plain',
              );
            },
            onPermissionRequest: (_, request) async => PermissionResponse(
              resources: request.resources,
              action: PermissionResponseAction.DENY,
            ),
            onCreateWindow: (_, _) async => false,
            onDownloadStartRequest: (_, _) async {},
          ),
        );
      },
    );
  }
}
