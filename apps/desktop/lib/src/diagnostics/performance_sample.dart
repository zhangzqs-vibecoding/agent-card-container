import 'dart:convert';
import 'dart:math';

class PerformanceReport {
  PerformanceReport._(this._samples);

  factory PerformanceReport.fromSamples(Iterable<num> samples) {
    final sorted = samples.map((sample) => sample.toDouble()).toList()..sort();
    if (sorted.isEmpty ||
        sorted.any((sample) => !sample.isFinite || sample <= 0)) {
      throw const FormatException(
        'performance samples must be finite positive numbers',
      );
    }
    return PerformanceReport._(List.unmodifiable(sorted));
  }

  final List<double> _samples;

  int get sampleCount => _samples.length;
  double get p50 => _percentile(0.50);
  double get p95 => _percentile(0.95);
  double get maximum => _samples.last;
  List<double> get samples => _samples;

  bool withinBudget({double? p50, double? p95, double? maximum}) {
    return (p50 == null || this.p50 <= p50) &&
        (p95 == null || this.p95 <= p95) &&
        (maximum == null || this.maximum <= maximum);
  }

  double _percentile(double percentile) {
    final rank = max(1, (percentile * _samples.length).ceil());
    return _samples[rank - 1];
  }
}

class PerformanceBaseline {
  PerformanceBaseline._({
    required this.commit,
    required this.host,
    required this.scenarios,
  });

  factory PerformanceBaseline.fromJson(Map<String, Object?> json) {
    if (json['schemaVersion'] != 1 ||
        json['commit'] is! String ||
        (json['commit']! as String).isEmpty ||
        json['host'] is! Map<String, Object?> ||
        json['scenarios'] is! Map<String, Object?>) {
      throw const FormatException('performance baseline envelope is invalid');
    }
    final reports = <String, PerformanceReport>{};
    for (final entry in (json['scenarios']! as Map<String, Object?>).entries) {
      final value = entry.value;
      if (value is! List || value.any((sample) => sample is! num)) {
        throw FormatException('invalid performance scenario: ${entry.key}');
      }
      reports[entry.key] = PerformanceReport.fromSamples(value.cast<num>());
    }
    return PerformanceBaseline._(
      commit: json['commit']! as String,
      host: Map.unmodifiable(json['host']! as Map<String, Object?>),
      scenarios: Map.unmodifiable(reports),
    );
  }

  final String commit;
  final Map<String, Object?> host;
  final Map<String, PerformanceReport> scenarios;

  List<String> evaluate() {
    final failures = <String>[];
    _evaluate(
      failures,
      'coldStartupMs',
      (report) => report.withinBudget(p50: 3000, p95: 5000),
      failedMetric: 'p95',
    );
    _evaluate(
      failures,
      'nativeMountMs',
      (report) => report.withinBudget(p50: 300),
      failedMetric: 'p50',
    );
    _evaluate(
      failures,
      'codeMountMs',
      (report) => report.withinBudget(p50: 1500),
      failedMetric: 'p50',
    );
    _evaluate(
      failures,
      'steadyCpuPercent',
      (report) => report.p50 < 3,
      failedMetric: 'p50',
    );
    _evaluate(
      failures,
      'residentMemoryMiB',
      (report) => report.maximum < 1024,
      failedMetric: 'maximum',
    );
    _evaluate(
      failures,
      'frameTimeMs',
      (report) => report.p95 <= 16.7,
      failedMetric: 'p95',
    );
    return List.unmodifiable(failures);
  }

  String toJson() {
    return jsonEncode({
      'schemaVersion': 1,
      'commit': commit,
      'host': host,
      'scenarios': {
        for (final entry in scenarios.entries) entry.key: entry.value.samples,
      },
    });
  }

  void _evaluate(
    List<String> failures,
    String name,
    bool Function(PerformanceReport report) passes, {
    required String failedMetric,
  }) {
    final report = scenarios[name];
    if (report == null) {
      failures.add('$name:missing');
    } else if (!passes(report)) {
      failures.add('$name:$failedMetric');
    }
  }
}
