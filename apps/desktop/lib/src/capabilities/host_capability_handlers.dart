import 'dart:convert';

import 'capability.dart';

abstract interface class ClipboardCapabilityPort {
  Future<void> writeText(String text);

  Future<String?> readText();
}

abstract interface class ExternalUrlCapabilityPort {
  Future<bool> open(Uri uri);
}

abstract interface class NotificationCapabilityPort {
  Future<void> show({required String title, required String body});
}

abstract interface class SystemMetricsCapabilityPort {
  Future<Map<String, Object?>> read();
}

class HostCapabilityHandlers {
  HostCapabilityHandlers({
    required this.clipboard,
    required this.externalUrls,
    required this.notifications,
    required this.metrics,
    DateTime Function()? now,
  }) : now = now ?? DateTime.now;

  final ClipboardCapabilityPort clipboard;
  final ExternalUrlCapabilityPort externalUrls;
  final NotificationCapabilityPort notifications;
  final SystemMetricsCapabilityPort metrics;
  final DateTime Function() now;
  final Map<String, List<DateTime>> _notificationTimes = {};

  Future<Object?> clipboardWrite(
    CardContext context,
    Map<String, Object?> params,
  ) async {
    final text = params['text'];
    if (params.length != 1 ||
        text is! String ||
        utf8.encode(text).length > 1024 * 1024) {
      throw const CapabilityException(
        CapabilityErrorCode.invalidParams,
        'clipboard.write requires text no larger than 1 MiB',
      );
    }
    await clipboard.writeText(text);
    return const {'written': true};
  }

  Future<Object?> clipboardRead(
    CardContext context,
    Map<String, Object?> params,
  ) async {
    if (params.isNotEmpty) {
      throw const CapabilityException(
        CapabilityErrorCode.invalidParams,
        'clipboard.read does not accept parameters',
      );
    }
    return {'text': await clipboard.readText()};
  }

  Future<Object?> openExternal(
    CardContext context,
    Map<String, Object?> params,
  ) async {
    final rawUrl = params['url'];
    final uri = rawUrl is String ? Uri.tryParse(rawUrl) : null;
    if (params.length != 1 ||
        uri == null ||
        !uri.hasAuthority ||
        (uri.scheme != 'http' && uri.scheme != 'https') ||
        uri.userInfo.isNotEmpty) {
      throw const CapabilityException(
        CapabilityErrorCode.invalidParams,
        'host.openExternal requires an absolute http or https URL',
      );
    }
    if (!await externalUrls.open(uri)) {
      throw const CapabilityException(
        CapabilityErrorCode.capabilityUnavailable,
        'system browser is unavailable',
      );
    }
    return const {'opened': true};
  }

  Future<Object?> notificationShow(
    CardContext context,
    Map<String, Object?> params,
  ) async {
    final body = params['body'];
    if (params.length != 1 ||
        body is! String ||
        body.isEmpty ||
        utf8.encode(body).length > 4096) {
      throw const CapabilityException(
        CapabilityErrorCode.invalidParams,
        'notification.show requires a body no larger than 4 KiB',
      );
    }
    final current = now().toUtc();
    final cutoff = current.subtract(const Duration(minutes: 1));
    final times = _notificationTimes.putIfAbsent(context.instanceId, () => []);
    times.removeWhere((value) => value.isBefore(cutoff));
    if (times.length >= 5) {
      throw const CapabilityException(
        CapabilityErrorCode.rateLimited,
        'notification rate limit exceeded',
      );
    }
    times.add(current);
    await notifications.show(
      title: 'Agent Card · ${context.cardId}',
      body: body,
    );
    return const {'shown': true};
  }

  Future<Object?> systemMetricsGet(
    CardContext context,
    Map<String, Object?> params,
  ) async {
    if (params.isNotEmpty) {
      throw const CapabilityException(
        CapabilityErrorCode.invalidParams,
        'system.metrics.get does not accept parameters',
      );
    }
    const allowed = {
      'residentBytes',
      'maxResidentBytes',
      'processorCount',
      'timestamp',
    };
    final values = await metrics.read();
    return {
      for (final entry in values.entries)
        if (allowed.contains(entry.key)) entry.key: entry.value,
    };
  }
}
