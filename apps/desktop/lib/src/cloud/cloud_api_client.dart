import 'dart:async';
import 'dart:convert';
import 'dart:io';

enum GenerationTarget { auto, native, web }

enum GenerationStatus {
  draft,
  awaitingConfirmation,
  queued,
  generating,
  validating,
  ready,
  failed,
  cancelled,
}

class RequirementSummary {
  const RequirementSummary({
    required this.goal,
    required this.constraints,
    required this.locale,
  });

  factory RequirementSummary.fromJson(Map<String, Object?> json) {
    return RequirementSummary(
      goal: _string(json, 'goal'),
      constraints: _stringList(json, 'constraints'),
      locale: _string(json, 'locale'),
    );
  }

  final String goal;
  final List<String> constraints;
  final String locale;
}

class GenerationSession {
  const GenerationSession({
    required this.id,
    required this.prompt,
    required this.target,
    required this.locale,
    required this.status,
    required this.summary,
    required this.createdAt,
    required this.updatedAt,
    this.versionId,
    this.baseCardId,
    this.baseVersionId,
  });

  factory GenerationSession.fromJson(Map<String, Object?> json) {
    return GenerationSession(
      id: _string(json, 'id'),
      prompt: _string(json, 'prompt'),
      target: GenerationTarget.values.byName(_string(json, 'target')),
      locale: _string(json, 'locale'),
      status: _status(_string(json, 'status')),
      summary: RequirementSummary.fromJson(_map(json, 'summary')),
      versionId: json['versionId'] as String?,
      baseCardId: json['baseCardId'] as String?,
      baseVersionId: json['baseVersionId'] as String?,
      createdAt: DateTime.parse(_string(json, 'createdAt')).toUtc(),
      updatedAt: DateTime.parse(_string(json, 'updatedAt')).toUtc(),
    );
  }

  final String id;
  final String prompt;
  final GenerationTarget target;
  final String locale;
  final GenerationStatus status;
  final RequirementSummary summary;
  final String? versionId;
  final String? baseCardId;
  final String? baseVersionId;
  final DateTime createdAt;
  final DateTime updatedAt;
}

class GenerationEvent {
  const GenerationEvent({
    required this.eventId,
    required this.sessionId,
    required this.type,
    required this.stage,
    required this.message,
    required this.progress,
    required this.timestamp,
    this.versionId,
    this.errorCode,
  });

  factory GenerationEvent.fromJson(Map<String, Object?> json) {
    final eventId = json['eventId'];
    final progress = json['progress'];
    if (eventId is! int || progress is! num) {
      throw const FormatException('invalid generation event numbers');
    }
    return GenerationEvent(
      eventId: eventId,
      sessionId: _string(json, 'sessionId'),
      type: _string(json, 'type'),
      stage: _string(json, 'stage'),
      message: _string(json, 'message'),
      progress: progress.toDouble(),
      timestamp: DateTime.parse(_string(json, 'timestamp')).toUtc(),
      versionId: json['versionId'] as String?,
      errorCode: json['errorCode'] as String?,
    );
  }

  final int eventId;
  final String sessionId;
  final String type;
  final String stage;
  final String message;
  final double progress;
  final String? versionId;
  final String? errorCode;
  final DateTime timestamp;
}

class CloudCardVersion {
  const CloudCardVersion({
    required this.versionId,
    required this.cardId,
    required this.runtime,
    required this.displayVersion,
    required this.title,
    required this.description,
    required this.artifactSha256,
    required this.keyId,
    required this.preview,
    required this.createdAt,
  });

  factory CloudCardVersion.fromJson(Map<String, Object?> json) {
    return CloudCardVersion(
      versionId: _string(json, 'versionId'),
      cardId: _string(json, 'cardId'),
      runtime: _string(json, 'runtime'),
      displayVersion: _string(json, 'displayVersion'),
      title: _string(json, 'title'),
      description: _string(json, 'description'),
      artifactSha256: _string(json, 'artifactSha256'),
      keyId: _string(json, 'keyId'),
      preview: _map(json, 'preview'),
      createdAt: DateTime.parse(_string(json, 'createdAt')).toUtc(),
    );
  }

  final String versionId;
  final String cardId;
  final String runtime;
  final String displayVersion;
  final String title;
  final String description;
  final String artifactSha256;
  final String keyId;
  final Map<String, Object?> preview;
  final DateTime createdAt;
}

class CloudCardSummary {
  const CloudCardSummary({
    required this.cardId,
    required this.title,
    required this.description,
    required this.latestVersion,
  });

  factory CloudCardSummary.fromJson(Map<String, Object?> json) {
    return CloudCardSummary(
      cardId: _string(json, 'cardId'),
      title: _string(json, 'title'),
      description: _string(json, 'description'),
      latestVersion: CloudCardVersion.fromJson(_map(json, 'latestVersion')),
    );
  }

  final String cardId;
  final String title;
  final String description;
  final CloudCardVersion latestVersion;
}

class CloudCardDetail {
  const CloudCardDetail({
    required this.cardId,
    required this.title,
    required this.description,
    required this.versions,
  });

  factory CloudCardDetail.fromJson(Map<String, Object?> json) {
    final versions = json['versions'];
    if (versions is! List) {
      throw const FormatException('versions must be an array');
    }
    return CloudCardDetail(
      cardId: _string(json, 'cardId'),
      title: _string(json, 'title'),
      description: _string(json, 'description'),
      versions: versions
          .map((version) {
            if (version is! Map<String, Object?>) {
              throw const FormatException('card version must be an object');
            }
            return CloudCardVersion.fromJson(version);
          })
          .toList(growable: false),
    );
  }

  final String cardId;
  final String title;
  final String description;
  final List<CloudCardVersion> versions;
}

class ArtifactDownload {
  const ArtifactDownload({
    required this.url,
    required this.sha256,
    required this.keyId,
    required this.expiresAt,
  });

  factory ArtifactDownload.fromJson(Map<String, Object?> json) {
    return ArtifactDownload(
      url: Uri.parse(_string(json, 'url')),
      sha256: _string(json, 'sha256'),
      keyId: _string(json, 'keyId'),
      expiresAt: DateTime.parse(_string(json, 'expiresAt')).toUtc(),
    );
  }

  final Uri url;
  final String sha256;
  final String keyId;
  final DateTime expiresAt;
}

class CloudApiException implements Exception {
  const CloudApiException({
    required this.statusCode,
    required this.code,
    required this.message,
    this.requestId,
  });

  final int statusCode;
  final String code;
  final String message;
  final String? requestId;

  @override
  String toString() => 'CloudApiException($code): $message';
}

class CloudApiClient {
  CloudApiClient({
    required this.baseUri,
    required this.tokenProvider,
    this.allowInsecureForDevelopment = false,
    HttpClient? httpClient,
  }) : _httpClient = httpClient ?? HttpClient() {
    if (!baseUri.hasAuthority ||
        (baseUri.scheme != 'https' && !allowInsecureForDevelopment)) {
      throw ArgumentError.value(baseUri, 'baseUri', 'must use HTTPS');
    }
    _httpClient.connectionTimeout = const Duration(seconds: 10);
  }

  final Uri baseUri;
  final Future<String> Function() tokenProvider;
  final bool allowInsecureForDevelopment;
  final HttpClient _httpClient;

  Future<void> testConnection() async {
    final uri = baseUri.resolve('/healthz');
    if (uri.scheme != baseUri.scheme ||
        uri.host != baseUri.host ||
        uri.port != baseUri.port) {
      throw StateError('health request escaped configured origin');
    }
    final request = await _httpClient.getUrl(uri);
    request
      ..followRedirects = false
      ..headers.set(HttpHeaders.acceptHeader, 'application/json');
    final response = await request.close();
    if (response.isRedirect) {
      throw const CloudApiException(
        statusCode: 0,
        code: 'REDIRECT_REJECTED',
        message: '云端健康检查禁止重定向',
      );
    }
    if (response.statusCode != HttpStatus.ok) {
      throw const CloudApiException(
        statusCode: 0,
        code: 'HEALTH_CHECK_FAILED',
        message: '云端服务健康检查失败',
      );
    }
    final bytes = await _readBounded(response, 64 * 1024);
    try {
      final decoded = jsonDecode(utf8.decode(bytes));
      if (decoded is! Map<String, Object?> ||
          decoded['status'] != 'ok' ||
          decoded['service'] != 'agent-card-cloud') {
        throw const FormatException('unexpected health response');
      }
    } catch (_) {
      throw const CloudApiException(
        statusCode: 0,
        code: 'INCOMPATIBLE_SERVICE',
        message: '目标地址不是兼容的 Agent Card Cloud',
      );
    }
    await listCards();
  }

  Future<GenerationSession> createGeneration({
    required String prompt,
    GenerationTarget target = GenerationTarget.auto,
    String locale = 'zh-CN',
    String? baseCardId,
    String? baseVersionId,
  }) async {
    final normalizedCardId = baseCardId?.trim();
    final normalizedVersionId = baseVersionId?.trim();
    final hasCard = normalizedCardId != null && normalizedCardId.isNotEmpty;
    final hasVersion =
        normalizedVersionId != null && normalizedVersionId.isNotEmpty;
    if (hasCard != hasVersion) {
      throw ArgumentError(
        'baseCardId and baseVersionId must be provided together',
      );
    }
    return GenerationSession.fromJson(
      await _json(
        'POST',
        '/v1/generations',
        body: {
          'prompt': prompt,
          'target': target.name,
          'locale': locale,
          if (hasCard) 'baseCardId': normalizedCardId,
          if (hasVersion) 'baseVersionId': normalizedVersionId,
        },
        expectedStatus: HttpStatus.created,
      ),
    );
  }

  Future<GenerationSession> addMessage(String sessionId, String content) async {
    return GenerationSession.fromJson(
      await _json(
        'POST',
        '/v1/generations/${Uri.encodeComponent(sessionId)}/messages',
        body: {'content': content},
      ),
    );
  }

  Future<GenerationSession> confirmGeneration(String sessionId) async {
    return GenerationSession.fromJson(
      await _json(
        'POST',
        '/v1/generations/${Uri.encodeComponent(sessionId)}/confirm',
      ),
    );
  }

  Future<GenerationSession> cancelGeneration(String sessionId) async {
    return GenerationSession.fromJson(
      await _json(
        'POST',
        '/v1/generations/${Uri.encodeComponent(sessionId)}/cancel',
      ),
    );
  }

  Future<GenerationSession> generation(String sessionId) async {
    return GenerationSession.fromJson(
      await _json('GET', '/v1/generations/${Uri.encodeComponent(sessionId)}'),
    );
  }

  Future<List<CloudCardSummary>> listCards() async {
    final response = await _json('GET', '/v1/cards');
    final cards = response['cards'];
    if (cards is! List) {
      throw const FormatException('cards must be an array');
    }
    return cards
        .map((card) {
          if (card is! Map<String, Object?>) {
            throw const FormatException('card summary must be an object');
          }
          return CloudCardSummary.fromJson(card);
        })
        .toList(growable: false);
  }

  Future<CloudCardDetail> getCard(String cardId) async {
    final response = await _json(
      'GET',
      '/v1/cards/${Uri.encodeComponent(cardId)}',
    );
    return CloudCardDetail.fromJson(response);
  }

  Future<ArtifactDownload> artifactDownload(
    String cardId,
    String versionId,
  ) async {
    final response = await _json(
      'GET',
      '/v1/cards/${Uri.encodeComponent(cardId)}'
          '/versions/${Uri.encodeComponent(versionId)}/artifact',
    );
    return ArtifactDownload.fromJson(response);
  }

  Future<List<int>> downloadArtifact(Uri uri) async {
    final secure = uri.scheme == 'https';
    final developmentLoopback =
        allowInsecureForDevelopment &&
        uri.scheme == 'http' &&
        (uri.host == '127.0.0.1' ||
            uri.host == '::1' ||
            uri.host == 'localhost');
    if (!uri.hasAuthority || (!secure && !developmentLoopback)) {
      throw ArgumentError.value(uri, 'uri', 'artifact download must use HTTPS');
    }
    final request = await _httpClient.getUrl(uri);
    request
      ..followRedirects = false
      ..headers.set(HttpHeaders.acceptHeader, 'application/octet-stream');
    final response = await request.close();
    if (response.isRedirect) {
      throw const CloudApiException(
        statusCode: 0,
        code: 'REDIRECT_REJECTED',
        message: '制品下载禁止重定向',
      );
    }
    if (response.statusCode != HttpStatus.ok) {
      throw CloudApiException(
        statusCode: response.statusCode,
        code: 'ARTIFACT_DOWNLOAD_FAILED',
        message: '制品下载失败',
      );
    }
    return _readBounded(response, 8 * 1024 * 1024);
  }

  Stream<GenerationEvent> connectGenerationEvents(
    String sessionId, {
    int lastEventId = 0,
  }) async* {
    final request = await _authorizedRequest(
      'GET',
      '/v1/generations/${Uri.encodeComponent(sessionId)}/events',
    );
    request.headers.set(HttpHeaders.acceptHeader, 'text/event-stream');
    if (lastEventId > 0) {
      request.headers.set('Last-Event-ID', lastEventId.toString());
    }
    final response = await request.close();
    if (response.isRedirect) {
      throw const CloudApiException(
        statusCode: 0,
        code: 'REDIRECT_REJECTED',
        message: '云端事件流禁止重定向',
      );
    }
    if (response.statusCode != HttpStatus.ok) {
      await _throwResponse(response);
    }
    var data = StringBuffer();
    await for (final line
        in response.transform(utf8.decoder).transform(const LineSplitter())) {
      if (line.isEmpty) {
        if (data.isNotEmpty) {
          final decoded = jsonDecode(data.toString());
          if (decoded is! Map<String, Object?>) {
            throw const FormatException('SSE data must be an object');
          }
          yield GenerationEvent.fromJson(decoded);
          data = StringBuffer();
        }
        continue;
      }
      if (line.startsWith('data:')) {
        if (data.isNotEmpty) {
          data.write('\n');
        }
        data.write(line.substring(5).trimLeft());
      }
    }
    if (data.isNotEmpty) {
      final decoded = jsonDecode(data.toString());
      if (decoded is Map<String, Object?>) {
        yield GenerationEvent.fromJson(decoded);
      }
    }
  }

  Future<Map<String, Object?>> _json(
    String method,
    String path, {
    Map<String, Object?>? body,
    int expectedStatus = HttpStatus.ok,
  }) async {
    final request = await _authorizedRequest(method, path);
    request.headers
      ..contentType = ContentType.json
      ..set(HttpHeaders.acceptHeader, 'application/json');
    if (body != null) {
      request.add(utf8.encode(jsonEncode(body)));
    }
    final response = await request.close();
    if (response.isRedirect) {
      throw const CloudApiException(
        statusCode: 0,
        code: 'REDIRECT_REJECTED',
        message: '云端 API 禁止携带凭据重定向',
      );
    }
    if (response.statusCode != expectedStatus) {
      await _throwResponse(response);
    }
    final bytes = await _readBounded(response, 1024 * 1024);
    final decoded = jsonDecode(utf8.decode(bytes));
    if (decoded is! Map<String, Object?>) {
      throw const FormatException('cloud response must be an object');
    }
    return decoded;
  }

  Future<HttpClientRequest> _authorizedRequest(
    String method,
    String path,
  ) async {
    final token = await tokenProvider();
    if (token.isEmpty) {
      throw const CloudApiException(
        statusCode: HttpStatus.unauthorized,
        code: 'UNAUTHENTICATED',
        message: '访问令牌不可用',
      );
    }
    final uri = baseUri.resolve(path);
    if (uri.scheme != baseUri.scheme ||
        uri.host != baseUri.host ||
        uri.port != baseUri.port) {
      throw StateError('cloud request escaped configured origin');
    }
    final request = await _httpClient.openUrl(method, uri);
    request
      ..followRedirects = false
      ..headers.set(HttpHeaders.authorizationHeader, 'Bearer $token');
    return request;
  }

  Future<Never> _throwResponse(HttpClientResponse response) async {
    final bytes = await _readBounded(response, 256 * 1024);
    try {
      final decoded = jsonDecode(utf8.decode(bytes)) as Map<String, Object?>;
      final error = _map(decoded, 'error');
      throw CloudApiException(
        statusCode: response.statusCode,
        code: _string(error, 'code'),
        message: _string(error, 'message'),
        requestId: error['requestId'] as String?,
      );
    } on CloudApiException {
      rethrow;
    } catch (_) {
      throw CloudApiException(
        statusCode: response.statusCode,
        code: 'HTTP_ERROR',
        message: '云端请求失败',
      );
    }
  }

  void close() {
    _httpClient.close(force: true);
  }
}

Future<List<int>> _readBounded(Stream<List<int>> stream, int maximum) async {
  final bytes = <int>[];
  await for (final chunk in stream) {
    bytes.addAll(chunk);
    if (bytes.length > maximum) {
      throw const FormatException('cloud response exceeds size limit');
    }
  }
  return bytes;
}

GenerationStatus _status(String value) {
  return switch (value) {
    'draft' => GenerationStatus.draft,
    'awaiting_confirmation' => GenerationStatus.awaitingConfirmation,
    'queued' => GenerationStatus.queued,
    'generating' => GenerationStatus.generating,
    'validating' => GenerationStatus.validating,
    'ready' => GenerationStatus.ready,
    'failed' => GenerationStatus.failed,
    'cancelled' => GenerationStatus.cancelled,
    _ => throw FormatException('unknown generation status: $value'),
  };
}

String _string(Map<String, Object?> json, String key) {
  final value = json[key];
  if (value is! String) {
    throw FormatException('$key must be a string');
  }
  return value;
}

Map<String, Object?> _map(Map<String, Object?> json, String key) {
  final value = json[key];
  if (value is! Map<String, Object?>) {
    throw FormatException('$key must be an object');
  }
  return value;
}

List<String> _stringList(Map<String, Object?> json, String key) {
  final value = json[key];
  if (value is! List || value.any((item) => item is! String)) {
    throw FormatException('$key must contain strings');
  }
  return value.cast<String>();
}
