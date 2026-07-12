import 'dart:convert';

class DiagnosticRuntimeInfo {
  const DiagnosticRuntimeInfo({
    required this.appVersion,
    required this.platform,
    required this.operatingSystemVersion,
    required this.locale,
    required this.installedCardCount,
    required this.activeSurfaceCount,
  });

  final String appVersion;
  final String platform;
  final String operatingSystemVersion;
  final String locale;
  final int installedCardCount;
  final int activeSurfaceCount;

  Map<String, Object?> toJson() => {
    'appVersion': appVersion,
    'platform': platform,
    'operatingSystemVersion': operatingSystemVersion,
    'locale': locale,
    'installedCardCount': installedCardCount,
    'activeSurfaceCount': activeSurfaceCount,
  };
}

class DiagnosticError {
  const DiagnosticError({required this.category, required this.message})
    : assert(
        category == 'runtime' ||
            category == 'cloud' ||
            category == 'artifact' ||
            category == 'window' ||
            category == 'database',
        'diagnostic category is not allowlisted',
      );

  final String category;
  final String message;
}

class DiagnosticBundleBuilder {
  String build({
    required DiagnosticRuntimeInfo runtime,
    required List<DiagnosticError> errors,
    required DateTime generatedAt,
  }) {
    if (errors.length > 100) {
      throw ArgumentError.value(errors.length, 'errors', 'maximum is 100');
    }
    return jsonEncode({
      'schemaVersion': 1,
      'generatedAt': generatedAt.toUtc().toIso8601String(),
      'runtime': runtime.toJson(),
      'errors': [
        for (final error in errors)
          {
            'category': error.category,
            'message': redactDiagnosticText(
              error.message.length > 4096
                  ? error.message.substring(0, 4096)
                  : error.message,
            ),
          },
      ],
    });
  }
}

String redactDiagnosticText(String source) {
  var value = source;
  value = value.replaceAll(
    RegExp(
      r'\b(?:authorization|x-api-key|api-key)\s*[:=]\s*[^\s,;]+(?:\s+[^\s,;]+)?',
      caseSensitive: false,
    ),
    '[REDACTED]',
  );
  value = value.replaceAll(
    RegExp(r'\bbearer\s+[A-Za-z0-9._~+/=-]+', caseSensitive: false),
    '[REDACTED]',
  );
  value = value.replaceAll(
    RegExp(r'\b(?:sk|key|token)-[A-Za-z0-9_-]{16,}\b'),
    '[REDACTED]',
  );
  value = value.replaceAll(
    RegExp(r'[A-Za-z]:\\Users\\[^\\/\s]+', caseSensitive: false),
    r'%USERPROFILE%',
  );
  value = value.replaceAll(RegExp(r'/(?:home|Users)/[^/\s]+'), r'$HOME');
  return value;
}
