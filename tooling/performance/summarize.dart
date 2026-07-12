import 'dart:convert';
import 'dart:io';

import '../../apps/desktop/lib/src/diagnostics/performance_sample.dart';

void main(List<String> arguments) {
  if (arguments.length != 1) {
    stderr.writeln(
      'usage: dart tooling/performance/summarize.dart <samples.json>',
    );
    exitCode = 64;
    return;
  }
  try {
    final decoded = jsonDecode(File(arguments.single).readAsStringSync());
    if (decoded is! Map<String, Object?>) {
      throw const FormatException('baseline must contain a JSON object');
    }
    final baseline = PerformanceBaseline.fromJson(decoded);
    final failures = baseline.evaluate();
    stdout.writeln(
      const JsonEncoder.withIndent('  ').convert({
        'commit': baseline.commit,
        'scenarios': {
          for (final entry in baseline.scenarios.entries)
            entry.key: {
              'samples': entry.value.sampleCount,
              'p50': entry.value.p50,
              'p95': entry.value.p95,
              'maximum': entry.value.maximum,
            },
        },
        'failures': failures,
        'passed': failures.isEmpty,
      }),
    );
    if (failures.isNotEmpty) {
      exitCode = 1;
    }
  } on Object catch (error) {
    stderr.writeln('invalid performance baseline: $error');
    exitCode = 65;
  }
}
