import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'capability.dart';

typedef NetworkResolver = Future<List<InternetAddress>> Function(String host);
typedef NetworkConnector =
    Future<ConnectionTask<Socket>> Function(
      Uri uri,
      List<InternetAddress> addresses,
    );

class SecureNetworkFetcher {
  SecureNetworkFetcher({
    NetworkResolver? resolver,
    NetworkConnector? connector,
    this.maxRequestBytes = 256 * 1024,
    this.maxResponseBytes = 5 * 1024 * 1024,
    this.timeout = const Duration(seconds: 10),
  }) : resolver = resolver ?? InternetAddress.lookup,
       connector = connector ?? _connect;

  final NetworkResolver resolver;
  final NetworkConnector connector;
  final int maxRequestBytes;
  final int maxResponseBytes;
  final Duration timeout;

  Future<Object?> handle(
    CardContext context,
    Map<String, Object?> params,
  ) async {
    final source = params['url'];
    final uri = source is String ? Uri.tryParse(source) : null;
    if (uri == null ||
        !uri.hasAuthority ||
        (uri.scheme != 'http' && uri.scheme != 'https') ||
        uri.userInfo.isNotEmpty ||
        !context.networkDomains.contains(uri.host.toLowerCase())) {
      throw const CapabilityException(
        CapabilityErrorCode.invalidParams,
        'network.fetch URL is invalid',
      );
    }
    final method = (params['method'] as String? ?? 'GET').toUpperCase();
    if (method != 'GET' && method != 'POST') {
      throw const CapabilityException(
        CapabilityErrorCode.invalidParams,
        'network.fetch method must be GET or POST',
      );
    }
    final body = params['body'];
    if (body != null && body is! String) {
      throw const CapabilityException(
        CapabilityErrorCode.invalidParams,
        'network.fetch body must be a string',
      );
    }
    final bodyBytes = body == null
        ? const <int>[]
        : utf8.encode(body as String);
    if (bodyBytes.length > maxRequestBytes) {
      throw const CapabilityException(
        CapabilityErrorCode.invalidParams,
        'network.fetch request is too large',
      );
    }

    try {
      final literal = InternetAddress.tryParse(uri.host);
      final addresses = literal == null
          ? await resolver(uri.host).timeout(timeout)
          : [literal];
      if (addresses.isEmpty ||
          addresses.any((address) => !_isPublic(address))) {
        throw const CapabilityException(
          CapabilityErrorCode.permissionDenied,
          'network.fetch resolved to a non-public address',
        );
      }
      final client = HttpClient();
      client.findProxy = (_) => 'DIRECT';
      client.connectionTimeout = timeout;
      client.connectionFactory = (target, proxyHost, proxyPort) {
        if (proxyHost != null ||
            target.host.toLowerCase() != uri.host.toLowerCase()) {
          throw const CapabilityException(
            CapabilityErrorCode.permissionDenied,
            'network.fetch connection target changed',
          );
        }
        return connector(target, addresses);
      };
      try {
        final request = await client.openUrl(method, uri).timeout(timeout);
        request
          ..followRedirects = false
          ..maxRedirects = 0
          ..headers.set(
            HttpHeaders.acceptHeader,
            'application/json, text/plain;q=0.9',
          );
        if (bodyBytes.isNotEmpty) {
          request.headers.contentType = ContentType.json;
          request.add(bodyBytes);
        }
        final response = await request.close().timeout(timeout);
        if (response.isRedirect ||
            (response.statusCode >= 300 && response.statusCode < 400)) {
          await response.drain<void>();
          throw const CapabilityException(
            CapabilityErrorCode.permissionDenied,
            'network.fetch redirects are disabled',
          );
        }
        final bytes = <int>[];
        await for (final chunk in response.timeout(timeout)) {
          bytes.addAll(chunk);
          if (bytes.length > maxResponseBytes) {
            throw const CapabilityException(
              CapabilityErrorCode.internal,
              'network.fetch response is too large',
            );
          }
        }
        return {
          'status': response.statusCode,
          'contentType': response.headers.contentType?.toString(),
          'body': utf8.decode(bytes, allowMalformed: true),
        };
      } finally {
        client.close(force: true);
      }
    } on CapabilityException {
      rethrow;
    } on TimeoutException {
      throw const CapabilityException(
        CapabilityErrorCode.timeout,
        'network.fetch timed out',
      );
    } on SocketException {
      throw const CapabilityException(
        CapabilityErrorCode.offline,
        'network.fetch could not connect',
      );
    } catch (_) {
      throw const CapabilityException(
        CapabilityErrorCode.internal,
        'network.fetch failed',
      );
    }
  }

  void close() {}
}

Future<ConnectionTask<Socket>> _connect(
  Uri uri,
  List<InternetAddress> addresses,
) {
  final port = uri.hasPort ? uri.port : (uri.scheme == 'https' ? 443 : 80);
  return Socket.startConnect(addresses.first, port);
}

bool _isPublic(InternetAddress address) {
  if (address.isLoopback || address.isLinkLocal || address.isMulticast) {
    return false;
  }
  final bytes = address.rawAddress;
  if (address.type == InternetAddressType.IPv4) {
    final first = bytes[0];
    final second = bytes[1];
    return first != 0 &&
        first != 10 &&
        first != 127 &&
        !(first == 100 && second >= 64 && second <= 127) &&
        !(first == 169 && second == 254) &&
        !(first == 172 && second >= 16 && second <= 31) &&
        !(first == 192 && second == 168) &&
        !(first == 198 && (second == 18 || second == 19)) &&
        first < 224;
  }
  if (bytes.every((byte) => byte == 0) ||
      (bytes[0] & 0xfe) == 0xfc ||
      (bytes[0] == 0xfe && (bytes[1] & 0xc0) == 0x80)) {
    return false;
  }
  final ipv4Mapped =
      bytes.take(10).every((byte) => byte == 0) &&
      bytes[10] == 0xff &&
      bytes[11] == 0xff;
  if (ipv4Mapped) {
    return _isPublic(InternetAddress.fromRawAddress(bytes.sublist(12)));
  }
  return true;
}
