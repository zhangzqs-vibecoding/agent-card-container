import 'dart:convert';
import 'dart:io';
import 'dart:ui';

import 'package:agent_card_desktop/src/app/agent_card_app.dart';
import 'package:agent_card_desktop/src/cards/card_instance.dart';
import 'package:agent_card_desktop/src/native_card/native_card_spec.dart';
import 'package:agent_card_desktop/src/runtime/local_runtime_server.dart';
import 'package:agent_card_desktop/src/surfaces/surface.dart';
import 'package:agent_card_desktop/src/workspace/workspace_card.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:integration_test/integration_test.dart';

const _capture = bool.fromEnvironment('AGENT_CARD_PERFORMANCE_CAPTURE');
const _commit = String.fromEnvironment('GIT_COMMIT');
const _coldStartupSamplesBase64 = String.fromEnvironment(
  'COLD_STARTUP_SAMPLES_BASE64',
);

void main() {
  final binding = IntegrationTestWidgetsFlutterBinding.ensureInitialized();
  binding.framePolicy = LiveTestWidgetsFlutterBindingFramePolicy.fullyLive;

  testWidgets(
    'captures desktop card mount and frame samples',
    (tester) async {
      if (_commit.isEmpty) {
        fail('GIT_COMMIT must identify the build under test');
      }
      final coldStartupSamples = _decodeColdStartupSamples();
      final server = await LocalRuntimeServer.start();
      addTearDown(server.close);
      final nativeMounts = <double>[];
      final codeMounts = <double>[];
      final populations = <String, List<double>>{};

      for (var sample = 0; sample < 30; sample++) {
        nativeMounts.add(await _measureMount(tester, [_nativeCard(sample)]));
      }
      for (final count in const [1, 5, 20]) {
        populations['nativeCards$count'] = [
          await _measureMount(tester, [
            for (var index = 0; index < count; index++) _nativeCard(index),
          ]),
        ];
      }

      for (var sample = 0; sample < 20; sample++) {
        codeMounts.add(
          await _measureCodeMount(tester, server, [_codeCard(server, sample)]),
        );
      }
      for (final count in const [1, 3, 8]) {
        populations['codeCards$count'] = [
          await _measureCodeMount(tester, server, [
            for (var index = 0; index < count; index++)
              _codeCard(server, 1000 + count * 10 + index),
          ]),
        ];
      }

      final frameTimes = await _captureMovingFrameTimes(tester, binding);
      final steadyCards = [
        for (var index = 0; index < 10; index++) _nativeCard(2000 + index),
        for (var index = 0; index < 3; index++) _codeCard(server, 3000 + index),
      ];
      await _measureCodeMount(tester, server, steadyCards);
      final processSamples = await _captureWindowsProcessSamples();
      final report = <String, Object?>{
        'schemaVersion': 1,
        'commit': _commit,
        'host': {
          'os': Platform.operatingSystem,
          'osVersion': Platform.operatingSystemVersion,
          'logicalCores': Platform.numberOfProcessors,
          'locale': Platform.localeName,
          'residentMemoryMiB': ProcessInfo.currentRss / 1024 / 1024,
        },
        'scenarios': {
          'coldStartupMs': coldStartupSamples,
          'nativeMountMs': nativeMounts,
          'codeMountMs': codeMounts,
          'steadyCpuPercent': processSamples.cpuPercent,
          'residentMemoryMiB': processSamples.residentMemoryMiB,
          'frameTimeMs': frameTimes,
          ...populations,
        },
      };
      binding.reportData = {'agentCardPerformance': report};
      // The integration-test driver captures this exact JSON line as a raw
      // artifact before the Windows process-level sampler merges its metrics.
      // ignore: avoid_print
      print('AGENT_CARD_PERFORMANCE_JSON=${jsonEncode(report)}');
    },
    skip: !_capture,
    timeout: Timeout.none,
  );
}

List<double> _decodeColdStartupSamples() {
  if (_coldStartupSamplesBase64.isEmpty) {
    fail('COLD_STARTUP_SAMPLES_BASE64 is required');
  }
  final decoded = jsonDecode(
    utf8.decode(base64Decode(_coldStartupSamplesBase64)),
  );
  if (decoded is! List ||
      decoded.length < 20 ||
      decoded.any((v) => v is! num)) {
    fail('cold-startup input must contain at least 20 numeric samples');
  }
  return [for (final value in decoded) (value as num).toDouble()];
}

Future<double> _measureMount(
  WidgetTester tester,
  List<WorkspaceCard> cards,
) async {
  await tester.pumpWidget(const SizedBox.shrink());
  final stopwatch = Stopwatch()..start();
  await tester.pumpWidget(AgentCardApp(workspaceCards: cards));
  await tester.pump();
  stopwatch.stop();
  return _positiveMilliseconds(stopwatch);
}

Future<double> _measureCodeMount(
  WidgetTester tester,
  LocalRuntimeServer server,
  List<WorkspaceCard> cards,
) async {
  await tester.pumpWidget(const SizedBox.shrink());
  final stopwatch = Stopwatch()..start();
  await tester.pumpWidget(
    AgentCardApp(runtimePort: server.port, workspaceCards: cards),
  );
  final deadline = DateTime.now().add(const Duration(seconds: 30));
  while (find.byType(CircularProgressIndicator).evaluate().isNotEmpty) {
    if (DateTime.now().isAfter(deadline)) {
      fail('CodeCard did not reach first mounted frame within 30 seconds');
    }
    await tester.pump(const Duration(milliseconds: 50));
  }
  stopwatch.stop();
  return _positiveMilliseconds(stopwatch);
}

Future<List<double>> _captureMovingFrameTimes(
  WidgetTester tester,
  IntegrationTestWidgetsFlutterBinding binding,
) async {
  final timings = <FrameTiming>[];
  void collectTimings(List<FrameTiming> values) => timings.addAll(values);
  await tester.pumpWidget(const MaterialApp(home: _MovingCardGrid()));
  await tester.pump();
  binding.addTimingsCallback(collectTimings);
  for (var frame = 0; frame < 1000; frame++) {
    await tester.pump(const Duration(microseconds: 16667));
  }
  await Future<void>.delayed(const Duration(seconds: 2));
  binding.removeTimingsCallback(collectTimings);
  if (timings.length < 1000) {
    fail('captured only ${timings.length} of 1000 required frame timings');
  }
  return [
    for (final timing in timings.take(1000))
      timing.totalSpan.inMicroseconds / 1000,
  ];
}

Future<({List<double> cpuPercent, List<double> residentMemoryMiB})>
_captureWindowsProcessSamples() async {
  if (!Platform.isWindows) {
    fail('process performance capture is Windows-only');
  }
  final cpu = <double>[];
  final memory = <double>[];
  for (var second = 0; second < 60; second++) {
    final before = await _readWindowsProcess();
    await Future<void>.delayed(const Duration(seconds: 1));
    final after = await _readWindowsProcess();
    final cpuPercent =
        (after.cpuSeconds - before.cpuSeconds) /
        Platform.numberOfProcessors *
        100;
    cpu.add(cpuPercent.clamp(0.001, 100).toDouble());
    memory.add(after.workingSetBytes / 1024 / 1024);
  }
  return (cpuPercent: cpu, residentMemoryMiB: memory);
}

Future<({double cpuSeconds, int workingSetBytes})> _readWindowsProcess() async {
  final result = await Process.run('powershell.exe', [
    '-NoProfile',
    '-NonInteractive',
    '-Command',
    'Get-Process -Id $pid | Select-Object CPU,WorkingSet64 | ConvertTo-Json -Compress',
  ]);
  if (result.exitCode != 0) {
    fail('unable to sample desktop process metrics');
  }
  final decoded = jsonDecode(result.stdout as String);
  if (decoded is! Map<String, Object?> ||
      decoded['CPU'] is! num ||
      decoded['WorkingSet64'] is! num) {
    fail('desktop process metric output is invalid');
  }
  return (
    cpuSeconds: (decoded['CPU']! as num).toDouble(),
    workingSetBytes: (decoded['WorkingSet64']! as num).toInt(),
  );
}

double _positiveMilliseconds(Stopwatch stopwatch) =>
    stopwatch.elapsedMicroseconds.clamp(1, 1 << 62) / 1000;

WorkspaceCard _nativeCard(int index) {
  return WorkspaceCard(
    instance: _instance('native-$index', index),
    spec: NativeCardSpec.fromJson({
      'schemaVersion': 1,
      'initialState': {'value': index},
      'root': {
        'id': 'value',
        'type': 'Text',
        'props': {
          'text': {'path': 'state.value'},
        },
      },
    }),
  );
}

WorkspaceCard _codeCard(LocalRuntimeServer server, int index) {
  const entrypoint = '/bundle/performance/index.html';
  final session = server.createSession(
    instanceId: 'code-$index',
    cardId: 'performance-code',
    versionId: 'performance-v1',
    resources: {
      entrypoint: RuntimeResource.html(
        '<script src="/runtime/bootstrap.js"></script><p>ready</p>',
      ),
    },
  );
  return WorkspaceCard.code(
    instance: _instance('code-$index', index),
    codeCard: CodeCardDescriptor(session: session, entrypoint: entrypoint),
  );
}

CardInstance _instance(String id, int index) {
  return CardInstance(
    instanceId: id,
    cardId: 'performance-card',
    versionId: 'performance-v1',
    surfaceId: 'workspace-main',
    placement: CardPlacement(
      x: (index % 4) * 3,
      y: (index ~/ 4) * 3,
      width: 3,
      height: 2,
    ),
    stateNamespace: 'state-$id',
    status: CardInstanceStatus.active,
  );
}

class _MovingCardGrid extends StatefulWidget {
  const _MovingCardGrid();

  @override
  State<_MovingCardGrid> createState() => _MovingCardGridState();
}

class _MovingCardGridState extends State<_MovingCardGrid>
    with SingleTickerProviderStateMixin {
  late final AnimationController _controller = AnimationController(
    vsync: this,
    duration: const Duration(seconds: 20),
  )..repeat(reverse: true);

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: _controller,
      builder: (context, _) {
        final progress = _controller.value;
        return Stack(
          children: [
            for (var index = 0; index < 20; index++)
              Positioned(
                left: 20 + (index % 5) * (120 + progress * 8),
                top: 20 + (index ~/ 5) * (90 + progress * 6),
                width: 100 + progress * 20,
                height: 70 + progress * 12,
                child: const ColoredBox(color: Color(0xFF394142)),
              ),
          ],
        );
      },
    );
  }
}
