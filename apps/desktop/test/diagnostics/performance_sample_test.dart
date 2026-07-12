import 'dart:convert';

import 'package:agent_card_desktop/src/diagnostics/performance_sample.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test('calculates deterministic nearest-rank p50 and p95', () {
    final report = PerformanceReport.fromSamples([500, 100, 400, 200, 300]);

    expect(report.sampleCount, 5);
    expect(report.p50, 300);
    expect(report.p95, 500);
    expect(report.withinBudget(p95: 450), isFalse);
    expect(report.withinBudget(p95: 500), isTrue);
  });

  test('evaluates the documented startup and mount budgets', () {
    final passing = PerformanceBaseline.fromJson({
      'schemaVersion': 1,
      'commit': 'abc123',
      'host': {'os': 'windows-11', 'logicalCores': 8, 'memoryGiB': 16},
      'scenarios': {
        'coldStartupMs': List.filled(20, 2500),
        'nativeMountMs': List.filled(30, 180),
        'codeMountMs': List.filled(20, 900),
        'steadyCpuPercent': [1.0, 1.5, 2.0],
        'residentMemoryMiB': [700, 750, 800],
        'frameTimeMs': List.filled(1000, 14),
      },
    });

    expect(passing.evaluate(), isEmpty);
    expect(jsonDecode(passing.toJson())['commit'], 'abc123');
  });

  test('fails closed when a scenario has too few samples', () {
    final baseline = PerformanceBaseline.fromJson({
      'schemaVersion': 1,
      'commit': 'abc123',
      'host': {'os': 'windows-11'},
      'scenarios': {
        'coldStartupMs': List.filled(19, 2000),
        'nativeMountMs': List.filled(29, 100),
        'codeMountMs': List.filled(19, 500),
        'steadyCpuPercent': [1],
        'residentMemoryMiB': [700],
        'frameTimeMs': List.filled(999, 12),
      },
    });

    expect(baseline.evaluate(), contains('coldStartupMs:samples'));
    expect(baseline.evaluate(), contains('nativeMountMs:samples'));
    expect(baseline.evaluate(), contains('codeMountMs:samples'));
    expect(baseline.evaluate(), contains('frameTimeMs:samples'));
  });

  test('fails closed for missing scenarios and exceeded budgets', () {
    final baseline = PerformanceBaseline.fromJson({
      'schemaVersion': 1,
      'commit': 'abc123',
      'host': {'os': 'linux'},
      'scenarios': {
        'coldStartupMs': List.filled(20, 6000),
      },
    });

    expect(baseline.evaluate(), contains('coldStartupMs:p95'));
    expect(baseline.evaluate(), contains('nativeMountMs:missing'));
    expect(baseline.evaluate(), contains('codeMountMs:missing'));
    expect(baseline.evaluate(), contains('steadyCpuPercent:missing'));
  });

  test('rejects empty non-finite or non-positive samples', () {
    expect(
      () => PerformanceReport.fromSamples(const []),
      throwsFormatException,
    );
    expect(
      () => PerformanceReport.fromSamples([double.nan]),
      throwsFormatException,
    );
    expect(() => PerformanceReport.fromSamples([0]), throwsFormatException);
  });
}
