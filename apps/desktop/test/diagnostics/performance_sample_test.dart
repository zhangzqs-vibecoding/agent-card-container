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
        'coldStartupMs': [2000, 2500, 3000, 3500, 4500],
        'nativeMountMs': [100, 150, 180, 220, 280],
        'codeMountMs': [500, 700, 900, 1100, 1400],
        'steadyCpuPercent': [1.0, 1.5, 2.0],
        'residentMemoryMiB': [700, 750, 800],
        'frameTimeMs': [10, 12, 14, 15, 16],
      },
    });

    expect(passing.evaluate(), isEmpty);
    expect(jsonDecode(passing.toJson())['commit'], 'abc123');
  });

  test('fails closed for missing scenarios and exceeded budgets', () {
    final baseline = PerformanceBaseline.fromJson({
      'schemaVersion': 1,
      'commit': 'abc123',
      'host': {'os': 'linux'},
      'scenarios': {
        'coldStartupMs': [6000],
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
