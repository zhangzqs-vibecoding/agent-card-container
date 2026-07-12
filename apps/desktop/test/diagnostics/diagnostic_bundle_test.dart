import 'dart:convert';

import 'package:agent_card_desktop/src/diagnostics/diagnostic_bundle.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test('exports only allowlisted runtime metadata', () {
    final encoded = DiagnosticBundleBuilder().build(
      runtime: const DiagnosticRuntimeInfo(
        appVersion: '1.0.0',
        platform: 'windows',
        operatingSystemVersion: 'Windows 11 24H2',
        locale: 'zh-CN',
        installedCardCount: 3,
        activeSurfaceCount: 2,
      ),
      errors: const [],
      generatedAt: DateTime.utc(2026, 7, 12),
    );

    final bundle = jsonDecode(encoded) as Map<String, Object?>;
    expect(bundle['schemaVersion'], 1);
    expect((bundle['runtime']! as Map)['appVersion'], '1.0.0');
    expect(encoded, isNot(contains('state')));
    expect(encoded, isNot(contains('permission')));
    expect(encoded, isNot(contains('token')));
  });

  test('redacts credentials and absolute user paths from errors', () {
    final encoded = DiagnosticBundleBuilder().build(
      runtime: const DiagnosticRuntimeInfo(
        appVersion: '1.0.0',
        platform: 'windows',
        operatingSystemVersion: 'Windows 11',
        locale: 'zh-CN',
        installedCardCount: 1,
        activeSurfaceCount: 1,
      ),
      errors: const [
        DiagnosticError(
          category: 'runtime',
          message: r'C:\Users\alice\cards failed Authorization: Bearer abc.def',
        ),
        DiagnosticError(
          category: 'cloud',
          message: 'request /home/alice/app failed with sk-abcdefghijklmnop',
        ),
      ],
      generatedAt: DateTime.utc(2026, 7, 12),
    );

    expect(encoded, isNot(contains('alice')));
    expect(encoded, isNot(contains('abc.def')));
    expect(encoded, isNot(contains('sk-abcdefghijklmnop')));
    expect(encoded, contains('[REDACTED]'));
    expect(encoded, contains(r'%USERPROFILE%'));
    expect(encoded, contains(r'$HOME'));
  });

  test('rejects invalid diagnostic categories and excessive messages', () {
    expect(
      () => DiagnosticError(category: 'requestBody', message: 'x'),
      throwsA(anyOf(isA<ArgumentError>(), isA<AssertionError>())),
    );
    expect(
      () => DiagnosticBundleBuilder().build(
        runtime: const DiagnosticRuntimeInfo(
          appVersion: '1',
          platform: 'linux',
          operatingSystemVersion: 'linux',
          locale: 'en',
          installedCardCount: 0,
          activeSurfaceCount: 0,
        ),
        errors: List.generate(
          101,
          (_) => const DiagnosticError(category: 'runtime', message: 'error'),
        ),
        generatedAt: DateTime.utc(2026),
      ),
      throwsArgumentError,
    );
  });
}
