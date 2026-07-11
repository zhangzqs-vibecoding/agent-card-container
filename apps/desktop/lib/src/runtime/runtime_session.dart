import 'dart:convert';
import 'dart:typed_data';

class RuntimeResource {
  RuntimeResource({required this.bytes, required this.contentType});

  factory RuntimeResource.html(String source) {
    return RuntimeResource(
      bytes: Uint8List.fromList(utf8.encode(source)),
      contentType: 'text/html; charset=utf-8',
    );
  }

  final Uint8List bytes;
  final String contentType;
}

class RuntimeSession {
  RuntimeSession({
    required this.id,
    required this.authority,
    required this.token,
    required this.instanceId,
    required this.cardId,
    required this.versionId,
    required Map<String, RuntimeResource> resources,
  }) : resources = Map.unmodifiable(resources);

  final String id;
  final String authority;
  final String token;
  final String instanceId;
  final String cardId;
  final String versionId;
  final Map<String, RuntimeResource> resources;
  final Map<String, Object?> storage = {};

  String get origin => 'http://$authority';
}
