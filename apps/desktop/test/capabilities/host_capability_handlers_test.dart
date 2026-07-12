import 'package:agent_card_desktop/src/capabilities/capability.dart';
import 'package:agent_card_desktop/src/capabilities/host_capability_handlers.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  final context = CardContext(
    instanceId: 'instance-1',
    cardId: 'card-1',
    versionId: 'version-1',
    declaredCapabilities: const {
      'notification.show',
      'clipboard.write',
      'clipboard.read',
      'host.openExternal',
      'system.metrics.read',
      'window.manageSelf',
    },
  );

  test('validates clipboard payloads and never exceeds one MiB', () async {
    final clipboard = _Clipboard();
    final handlers = _handlers(clipboard: clipboard);

    await handlers.clipboardWrite(context, {'text': 'hello'});
    expect(clipboard.written, 'hello');
    await expectLater(
      handlers.clipboardWrite(context, {
        'text': List.filled(1024 * 1024 + 1, 'x').join(),
      }),
      throwsA(
        isA<CapabilityException>().having(
          (error) => error.code,
          'code',
          CapabilityErrorCode.invalidParams,
        ),
      ),
    );
  });

  test('opens only absolute http and https URLs', () async {
    final external = _External();
    final handlers = _handlers(external: external);

    await handlers.openExternal(context, {'url': 'https://example.com/path'});
    expect(external.opened, Uri.parse('https://example.com/path'));
    await expectLater(
      handlers.openExternal(context, {'url': 'file:///tmp/private'}),
      throwsA(isA<CapabilityException>()),
    );
  });

  test('attributes and rate limits notifications per card instance', () async {
    final notifications = _Notifications();
    final handlers = _handlers(notifications: notifications);

    for (var index = 0; index < 5; index++) {
      await handlers.notificationShow(context, {'body': 'Ready'});
    }
    expect(notifications.lastTitle, 'Agent Card · card-1');
    await expectLater(
      handlers.notificationShow(context, {'body': 'Too many'}),
      throwsA(
        isA<CapabilityException>().having(
          (error) => error.code,
          'code',
          CapabilityErrorCode.rateLimited,
        ),
      ),
    );
  });

  test('returns only the predefined aggregate system metrics', () async {
    final handlers = _handlers(
      metrics: _Metrics({
        'residentBytes': 1024,
        'processorCount': 8,
        'secretProcessList': ['forbidden'],
      }),
    );

    final result = await handlers.systemMetricsGet(context, const {});

    expect(result, {'residentBytes': 1024, 'processorCount': 8});
  });
}

HostCapabilityHandlers _handlers({
  ClipboardCapabilityPort? clipboard,
  ExternalUrlCapabilityPort? external,
  NotificationCapabilityPort? notifications,
  SystemMetricsCapabilityPort? metrics,
}) {
  return HostCapabilityHandlers(
    clipboard: clipboard ?? _Clipboard(),
    externalUrls: external ?? _External(),
    notifications: notifications ?? _Notifications(),
    metrics: metrics ?? _Metrics(const {}),
  );
}

class _Clipboard implements ClipboardCapabilityPort {
  String? written;

  @override
  Future<String?> readText() async => 'clipboard';

  @override
  Future<void> writeText(String text) async {
    written = text;
  }
}

class _External implements ExternalUrlCapabilityPort {
  Uri? opened;

  @override
  Future<bool> open(Uri uri) async {
    opened = uri;
    return true;
  }
}

class _Notifications implements NotificationCapabilityPort {
  String? lastTitle;

  @override
  Future<void> show({required String title, required String body}) async {
    lastTitle = title;
  }
}

class _Metrics implements SystemMetricsCapabilityPort {
  _Metrics(this.value);

  final Map<String, Object?> value;

  @override
  Future<Map<String, Object?>> read() async => value;
}
