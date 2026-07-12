import 'validate-workflows.dart';

void main() {
  _rejects(
    'mutable action',
    _workflow(uses: 'actions/checkout@v4'),
    'immutable full commit SHA',
  );
  _rejects(
    'write permission on pull request',
    _workflow(extra: 'permissions:\n  contents: write'),
    'pull-request workflow cannot grant write permissions',
  );
  _rejects(
    'missing timeout',
    _workflow(includeTimeout: false),
    'missing timeout-minutes',
  );
  _rejects(
    'PR secret reference',
    _workflow(
      extra: r'''env:
          TOKEN: ${{ secrets.PR_TOKEN }}''',
    ),
    'pull-request workflow cannot reference secrets',
  );
  _rejects(
    'unverified Windows upload',
    _workflow(
      runner: 'windows-2025',
      extra:
          '      - uses: actions/upload-artifact@65c4c4a1ddee5b72f698fdd19549f0f0fb45cf08',
    ),
    'Windows artifact upload must run release and portable verifiers',
  );
  final valid = _workflow(
    runner: 'windows-2025',
    extra: '''
      - run: pwsh packaging/windows/verify-release.ps1
      - run: pwsh packaging/windows/build-portable-package.ps1
      - run: pwsh packaging/windows/verify-portable-package.ps1
      - uses: actions/upload-artifact@65c4c4a1ddee5b72f698fdd19549f0f0fb45cf08''',
  );
  final errors = validateWorkflowText(valid, source: 'valid.yml');
  if (errors.isNotEmpty) {
    throw StateError('valid workflow rejected: ${errors.join('; ')}');
  }
  print('workflow policy tests passed');
}

void _rejects(String name, String workflow, String expected) {
  final errors = validateWorkflowText(workflow, source: '$name.yml');
  if (!errors.any((error) => error.contains(expected))) {
    throw StateError('$name did not report "$expected": $errors');
  }
}

String _workflow({
  String runner = 'ubuntu-24.04',
  String uses = 'actions/checkout@34e114876b0b11c390a56381ad16ebd13914f8d5',
  String extra = '',
  bool includeTimeout = true,
}) =>
    '''
name: fixture
on:
  pull_request:
permissions:
  contents: read
jobs:
  test:
    runs-on: $runner
${includeTimeout ? '    timeout-minutes: 10' : ''}
    steps:
      - uses: $uses
$extra
''';
