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
  final setErrors = validateWorkflowSet({
    'release.yml': '''
on:
  push:
    tags: ['v*']
jobs:
  release:
    timeout-minutes: 20
    permissions:
      contents: write
    steps:
      - run: pwsh packaging/windows/verify-portable-package.ps1
      - run: sha256sum --check SHA256SUMS.txt
      - run: gh release create --prerelease
''',
    'deepseek-live.yml': r'''
on:
  workflow_dispatch:
    inputs:
      confirm_paid_test:
jobs:
  live:
    timeout-minutes: 5
    if: inputs.confirm_paid_test
    environment: deepseek-live
    env:
      AGENTCARD_MODEL_API_KEY: ${{ secrets.AGENTCARD_MODEL_API_KEY }}
    steps:
      - run: go test ./internal/modelprovider
''',
    'codecard-builder.yml': r'''
on:
  workflow_dispatch:
  pull_request:
  push:
    branches: [develop]
permissions:
  contents: read
jobs:
  verify:
    timeout-minutes: 30
    steps:
      - uses: aquasecurity/trivy-action@ed142fd0673e97e23eac54620cfb913e5ce36c25
      - run: NODE_IMAGE=node@sha256:b04ce4ae4e95b522112c2e5c52f781471a5cbc3b594527bcddedee9bc48c03a0 sh tooling/security/run-sandbox-integration.sh
  publish:
    if: github.event_name != 'pull_request'
    timeout-minutes: 10
    permissions:
      contents: read
      packages: write
    steps:
      - run: docker push "$IMAGE"
      - run: docker image inspect "$IMAGE" --format '{{json .RepoDigests}}'
''',
  });
  if (setErrors.isNotEmpty) {
    throw StateError('valid workflow set rejected: ${setErrors.join('; ')}');
  }
  final unsafeSet = validateWorkflowSet({
    'release.yml': 'on: [push]\njobs: {}',
    'deepseek-live.yml': r'''on: [push]
secrets.AGENTCARD_MODEL_API_KEY''',
    'codecard-builder.yml': '''
on: [pull_request]
permissions: write-all
jobs:
  publish:
    steps:
      - run: docker push latest
''',
  });
  for (final expected in [
    'release workflow must be limited to v* tags',
    'release workflow must verify archives and publish checksums as prerelease',
    'DeepSeek workflow must be manual and paid-test confirmed',
    'DeepSeek workflow must use the protected deepseek-live environment',
    'CodeCard builder workflow must verify pull requests and develop',
    'CodeCard builder workflow must pin and scan its base image',
    'CodeCard builder publish job must exclude pull requests',
    'CodeCard builder workflow must publish and record an immutable digest',
  ]) {
    if (!unsafeSet.any((error) => error.contains(expected))) {
      throw StateError(
        'unsafe workflow set did not report "$expected": $unsafeSet',
      );
    }
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
  String uses = 'actions/checkout@93cb6efe18208431cddfb8368fd83d5badbf9bfd',
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
