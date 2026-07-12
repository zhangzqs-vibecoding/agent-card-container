import 'dart:io';

final _usesPattern = RegExp(r'^\s*-?\s*uses:\s*([^\s#]+)', multiLine: true);
final _immutableActionPattern = RegExp(r'^[^@]+@[0-9a-f]{40}$');

List<String> validateWorkflowText(String sourceText, {required String source}) {
  final errors = <String>[];
  for (final match in _usesPattern.allMatches(sourceText)) {
    final reference = match.group(1)!;
    if (!reference.startsWith('./') &&
        !_immutableActionPattern.hasMatch(reference)) {
      errors.add(
        '$source: action must use an immutable full commit SHA: $reference',
      );
    }
  }

  final pullRequest = RegExp(
    r'^\s{2}pull_request\s*:',
    multiLine: true,
  ).hasMatch(sourceText);
  if (pullRequest &&
      RegExp(
        r'^\s*[A-Za-z_-]+:\s*(write|write-all)\s*$',
        multiLine: true,
      ).hasMatch(sourceText)) {
    errors.add('$source: pull-request workflow cannot grant write permissions');
  }
  if (pullRequest && sourceText.contains(r'${{ secrets.')) {
    errors.add('$source: pull-request workflow cannot reference secrets');
  }

  for (final job in _jobBlocks(sourceText)) {
    final reusable = RegExp(
      r'^\s{4}uses:\s*\./',
      multiLine: true,
    ).hasMatch(job.text);
    if (!reusable &&
        !RegExp(
          r'^\s{4}timeout-minutes:\s*\d+\s*$',
          multiLine: true,
        ).hasMatch(job.text)) {
      errors.add('$source: job ${job.name} is missing timeout-minutes');
    }
  }

  final uploadsArtifact = sourceText.contains('actions/upload-artifact@');
  final buildsOnWindows = RegExp(r'runs-on:\s*windows-').hasMatch(sourceText);
  if (uploadsArtifact && buildsOnWindows) {
    const required = [
      'verify-release.ps1',
      'build-portable-package.ps1',
      'verify-portable-package.ps1',
    ];
    if (required.any((name) => !sourceText.contains(name))) {
      errors.add(
        '$source: Windows artifact upload must run release and portable verifiers',
      );
    }
  }
  return errors;
}

List<String> validateWorkflowSet(Map<String, String> workflows) {
  final errors = <String>[];
  final release = workflows['release.yml'] ?? '';
  if (!release.contains("'v*'") && !release.contains('"v*"')) {
    errors.add('release workflow must be limited to v* tags');
  }
  if (!release.contains('verify-portable-package.ps1') ||
      !release.contains('SHA256SUMS.txt') ||
      !release.contains('--prerelease') ||
      !RegExp(r'contents:\s*write').hasMatch(release)) {
    errors.add(
      'release workflow must verify archives and publish checksums as prerelease',
    );
  }

  final deepSeek = workflows['deepseek-live.yml'] ?? '';
  if (!deepSeek.contains('workflow_dispatch:') ||
      !deepSeek.contains('confirm_paid_test') ||
      deepSeek.contains('pull_request:') ||
      RegExp(r'^\s{2}push\s*:', multiLine: true).hasMatch(deepSeek)) {
    errors.add('DeepSeek workflow must be manual and paid-test confirmed');
  }
  if (!deepSeek.contains('environment: deepseek-live') ||
      !deepSeek.contains(r'secrets.AGENTCARD_MODEL_API_KEY') ||
      deepSeek.contains('actions/upload-artifact@')) {
    errors.add(
      'DeepSeek workflow must use the protected deepseek-live environment',
    );
  }
  return errors;
}

List<({String name, String text})> _jobBlocks(String workflow) {
  final lines = workflow.split('\n');
  final result = <({String name, String text})>[];
  var inJobs = false;
  String? name;
  var buffer = <String>[];
  void flush() {
    final current = name;
    if (current != null) {
      result.add((name: current, text: buffer.join('\n')));
    }
    name = null;
    buffer = <String>[];
  }

  for (final line in lines) {
    if (line == 'jobs:') {
      inJobs = true;
      continue;
    }
    if (!inJobs) continue;
    if (line.isNotEmpty && !line.startsWith(' ')) {
      flush();
      break;
    }
    final match = RegExp(r'^  ([A-Za-z0-9_-]+):\s*$').firstMatch(line);
    if (match != null) {
      flush();
      name = match.group(1)!;
    }
    if (name != null) buffer.add(line);
  }
  flush();
  return result;
}

void main(List<String> arguments) {
  if (arguments.isEmpty) {
    stderr.writeln('usage: dart validate-workflows.dart <workflow.yml> [...]');
    exitCode = 64;
    return;
  }
  final errors = <String>[];
  final workflows = <String, String>{};
  for (final path in arguments) {
    final file = File(path);
    if (!file.existsSync()) {
      errors.add('$path: workflow does not exist');
      continue;
    }
    final text = file.readAsStringSync();
    workflows[file.uri.pathSegments.last] = text;
    errors.addAll(validateWorkflowText(text, source: path));
  }
  errors.addAll(validateWorkflowSet(workflows));
  if (errors.isNotEmpty) {
    for (final error in errors) {
      stderr.writeln(error);
    }
    exitCode = 1;
    return;
  }
  stdout.writeln('workflow policy passed for ${arguments.length} file(s)');
}
