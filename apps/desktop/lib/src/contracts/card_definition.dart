enum CardRuntime { native, web }

class CardSize {
  const CardSize({required this.width, required this.height});

  factory CardSize.fromJson(Map<String, Object?> json) {
    _rejectUnknown(json, const {'width', 'height'});
    return CardSize(
      width: _number(json, 'width'),
      height: _number(json, 'height'),
    );
  }

  final double width;
  final double height;

  Map<String, Object?> toJson() => {'width': width, 'height': height};
}

class NetworkPolicy {
  const NetworkPolicy({required this.mode, required this.domains});

  factory NetworkPolicy.fromJson(Map<String, Object?> json) {
    _rejectUnknown(json, const {'mode', 'domains'});
    return NetworkPolicy(
      mode: _string(json, 'mode'),
      domains: _stringList(json, 'domains'),
    );
  }

  final String mode;
  final List<String> domains;

  Map<String, Object?> toJson() => {'mode': mode, 'domains': domains};
}

class CardFile {
  const CardFile({
    required this.path,
    required this.sha256,
    required this.size,
  });

  factory CardFile.fromJson(Map<String, Object?> json) {
    _rejectUnknown(json, const {'path', 'sha256', 'size'});
    return CardFile(
      path: _string(json, 'path'),
      sha256: _string(json, 'sha256'),
      size: _integer(json, 'size'),
    );
  }

  final String path;
  final String sha256;
  final int size;

  Map<String, Object?> toJson() => {
    'path': path,
    'sha256': sha256,
    'size': size,
  };
}

class CardDefinition {
  const CardDefinition({
    required this.formatVersion,
    required this.minHostVersion,
    required this.cardId,
    required this.versionId,
    required this.displayVersion,
    required this.runtime,
    required this.stateSchemaVersion,
    required this.title,
    required this.description,
    required this.entrypoint,
    required this.catalogVersion,
    required this.minSize,
    required this.preferredSize,
    required this.maxSize,
    required this.capabilities,
    required this.networkPolicy,
    required this.files,
    required this.createdAt,
  });

  factory CardDefinition.fromJson(Map<String, Object?> json) {
    _rejectUnknown(json, jsonKeys);
    final definition = CardDefinition(
      formatVersion: _integer(json, 'formatVersion'),
      minHostVersion: _string(json, 'minHostVersion'),
      cardId: _string(json, 'cardId'),
      versionId: _string(json, 'versionId'),
      displayVersion: _string(json, 'displayVersion'),
      runtime: _runtime(json['runtime']),
      stateSchemaVersion: _integer(json, 'stateSchemaVersion'),
      title: _string(json, 'title'),
      description: _string(json, 'description'),
      entrypoint: _string(json, 'entrypoint'),
      catalogVersion: json['catalogVersion'] as String?,
      minSize: CardSize.fromJson(_map(json, 'minSize')),
      preferredSize: CardSize.fromJson(_map(json, 'preferredSize')),
      maxSize: CardSize.fromJson(_map(json, 'maxSize')),
      capabilities: _stringList(json, 'capabilities'),
      networkPolicy: NetworkPolicy.fromJson(_map(json, 'networkPolicy')),
      files: _mapList(json, 'files').map(CardFile.fromJson).toList(),
      createdAt: DateTime.parse(_string(json, 'createdAt')).toUtc(),
    );
    definition._validate();
    return definition;
  }

  static const jsonKeys = {
    'formatVersion',
    'minHostVersion',
    'cardId',
    'versionId',
    'displayVersion',
    'runtime',
    'stateSchemaVersion',
    'title',
    'description',
    'entrypoint',
    'catalogVersion',
    'minSize',
    'preferredSize',
    'maxSize',
    'capabilities',
    'networkPolicy',
    'files',
    'createdAt',
  };

  final int formatVersion;
  final String minHostVersion;
  final String cardId;
  final String versionId;
  final String displayVersion;
  final CardRuntime runtime;
  final int stateSchemaVersion;
  final String title;
  final String description;
  final String entrypoint;
  final String? catalogVersion;
  final CardSize minSize;
  final CardSize preferredSize;
  final CardSize maxSize;
  final List<String> capabilities;
  final NetworkPolicy networkPolicy;
  final List<CardFile> files;
  final DateTime createdAt;

  bool hasCapability(String capability) => capabilities.contains(capability);

  Map<String, Object?> toJson() => {
    'formatVersion': formatVersion,
    'minHostVersion': minHostVersion,
    'cardId': cardId,
    'versionId': versionId,
    'displayVersion': displayVersion,
    'runtime': runtime.name,
    'stateSchemaVersion': stateSchemaVersion,
    'title': title,
    'description': description,
    'entrypoint': entrypoint,
    if (catalogVersion != null) 'catalogVersion': catalogVersion,
    'minSize': minSize.toJson(),
    'preferredSize': preferredSize.toJson(),
    'maxSize': maxSize.toJson(),
    'capabilities': capabilities,
    'networkPolicy': networkPolicy.toJson(),
    'files': files.map((file) => file.toJson()).toList(growable: false),
    'createdAt': createdAt.toUtc().toIso8601String(),
  };

  void _validate() {
    if (formatVersion != 1) {
      throw const FormatException('formatVersion must be 1');
    }
    if (!cardId.startsWith('card_')) {
      throw const FormatException('cardId must start with card_');
    }
    if (!versionId.startsWith('ver_')) {
      throw const FormatException('versionId must start with ver_');
    }
    if (stateSchemaVersion < 1) {
      throw const FormatException('stateSchemaVersion must be positive');
    }
    if (title.trim().isEmpty) {
      throw const FormatException('title is required');
    }
    if (minSize.width <= 0 || minSize.height <= 0) {
      throw const FormatException('minSize values must be positive');
    }
    if (preferredSize.width < minSize.width ||
        preferredSize.height < minSize.height) {
      throw const FormatException(
        'preferredSize must not be smaller than minSize',
      );
    }
    if (maxSize.width < preferredSize.width ||
        maxSize.height < preferredSize.height) {
      throw const FormatException(
        'maxSize must not be smaller than preferredSize',
      );
    }
    if (files.length > 512) {
      throw const FormatException('files must not contain more than 512 items');
    }
    if (capabilities.toSet().length != capabilities.length ||
        capabilities.any(
          (capability) => !_allowedCapabilities.contains(capability),
        )) {
      throw const FormatException(
        'capabilities contain duplicate or unknown values',
      );
    }
    if (networkPolicy.mode != 'none' && networkPolicy.mode != 'proxy') {
      throw const FormatException('networkPolicy.mode must be none or proxy');
    }
    if (networkPolicy.mode == 'none' && networkPolicy.domains.isNotEmpty) {
      throw const FormatException('networkPolicy none cannot declare domains');
    }
    final paths = <String>{};
    var totalSize = 0;
    for (final file in files) {
      _validateArtifactPath(file.path);
      if (!paths.add(file.path)) {
        throw FormatException('duplicate artifact path: ${file.path}');
      }
      if (!RegExp(r'^[a-f0-9]{64}$').hasMatch(file.sha256)) {
        throw FormatException('invalid sha256 for ${file.path}');
      }
      if (file.size < 0 || file.size > 8 * 1024 * 1024) {
        throw FormatException('invalid size for ${file.path}');
      }
      totalSize += file.size;
    }
    if (totalSize > 32 * 1024 * 1024) {
      throw const FormatException('artifact contents exceed 32 MiB');
    }
    if (!paths.contains(entrypoint)) {
      throw const FormatException('entrypoint must be listed in files');
    }
    switch (runtime) {
      case CardRuntime.native:
        if (entrypoint != 'payload/native.json') {
          throw const FormatException(
            'native entrypoint must be payload/native.json',
          );
        }
        if (catalogVersion == null || catalogVersion!.isEmpty) {
          throw const FormatException('native catalogVersion is required');
        }
      case CardRuntime.web:
        if (entrypoint != 'payload/web/index.html') {
          throw const FormatException(
            'web entrypoint must be payload/web/index.html',
          );
        }
    }
  }

  static const _allowedCapabilities = {
    'storage',
    'notification.show',
    'clipboard.write',
    'clipboard.read',
    'host.openExternal',
    'network.fetch',
    'system.metrics.read',
    'window.manageSelf',
  };
}

void _validateArtifactPath(String path) {
  if (path.isEmpty ||
      path.startsWith('/') ||
      path.contains(r'\') ||
      path.contains('//')) {
    throw FormatException('unsafe artifact path: $path');
  }
  final segments = path.split('/');
  if (segments.length > 8 ||
      segments.any(
        (segment) => segment.isEmpty || segment == '.' || segment == '..',
      )) {
    throw FormatException('unsafe artifact path: $path');
  }
}

CardRuntime _runtime(Object? value) {
  return switch (value) {
    'native' => CardRuntime.native,
    'web' => CardRuntime.web,
    _ => throw FormatException('invalid card runtime: $value'),
  };
}

void _rejectUnknown(Map<String, Object?> json, Set<String> allowed) {
  final unknown = json.keys.where((key) => !allowed.contains(key)).toList();
  if (unknown.isNotEmpty) {
    throw FormatException('unknown fields: ${unknown.join(', ')}');
  }
}

String _string(Map<String, Object?> json, String key) {
  final value = json[key];
  if (value is! String) {
    throw FormatException('$key must be a string');
  }
  return value;
}

int _integer(Map<String, Object?> json, String key) {
  final value = json[key];
  if (value is! int) {
    throw FormatException('$key must be an integer');
  }
  return value;
}

double _number(Map<String, Object?> json, String key) {
  final value = json[key];
  if (value is! num) {
    throw FormatException('$key must be a number');
  }
  return value.toDouble();
}

Map<String, Object?> _map(Map<String, Object?> json, String key) {
  final value = json[key];
  if (value is! Map<String, Object?>) {
    throw FormatException('$key must be an object');
  }
  return value;
}

List<Map<String, Object?>> _mapList(Map<String, Object?> json, String key) {
  final value = json[key];
  if (value is! List) {
    throw FormatException('$key must be an array');
  }
  return value.map((item) {
    if (item is! Map<String, Object?>) {
      throw FormatException('$key must contain objects');
    }
    return item;
  }).toList();
}

List<String> _stringList(Map<String, Object?> json, String key) {
  final value = json[key];
  if (value is! List) {
    throw FormatException('$key must be an array');
  }
  return value.map((item) {
    if (item is! String) {
      throw FormatException('$key must contain strings');
    }
    return item;
  }).toList();
}
