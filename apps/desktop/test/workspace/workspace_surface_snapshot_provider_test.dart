import 'dart:convert';
import 'dart:io';

import 'package:agent_card_desktop/src/cards/card_instance.dart';
import 'package:agent_card_desktop/src/native_card/native_card_spec.dart';
import 'package:agent_card_desktop/src/runtime/runtime_session.dart';
import 'package:agent_card_desktop/src/surfaces/surface.dart';
import 'package:agent_card_desktop/src/workspace/workspace_card.dart';
import 'package:agent_card_desktop/src/workspace/workspace_surface_snapshot_provider.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test('uses the latest main-engine state for a NativeCard snapshot', () {
    final card = WorkspaceCard(
      instance: _instance('instance-1'),
      spec: NativeCardSpec.fromJson(
        jsonDecode(
              File(
                '../../contracts/card/fixtures/pomodoro-native.json',
              ).readAsStringSync(),
            )
            as Map<String, Object?>,
      ),
    );
    final provider = WorkspaceSurfaceSnapshotProvider(
      cards: () => [card],
      readState: (_) => {'remaining': 42},
    );

    final snapshot = provider('instance-1');

    expect(snapshot.state['remaining'], 42);
    expect(snapshot.nativeSpec, isNotNull);
  });

  test('does not copy a CodeCard session token into its snapshot', () {
    final session = RuntimeSession(
      id: 'session-1',
      authority: '127.0.0.1:43125',
      token: 'must-not-cross-engines',
      instanceId: 'instance-1',
      cardId: 'card-1',
      versionId: 'version-1',
      resources: const {},
    );
    final card = WorkspaceCard.code(
      instance: _instance('instance-1'),
      codeCard: CodeCardDescriptor(
        session: session,
        entrypoint: '/bundle/hash/index.html',
      ),
    );
    final provider = WorkspaceSurfaceSnapshotProvider(
      cards: () => [card],
      readState: (_) => const {},
    );

    final encoded = jsonEncode(provider('instance-1').toJson());

    expect(encoded, contains('127.0.0.1:43125'));
    expect(encoded, isNot(contains('must-not-cross-engines')));
  });
}

CardInstance _instance(String id) {
  return CardInstance(
    instanceId: id,
    cardId: 'card-1',
    versionId: 'version-1',
    surfaceId: 'workspace-main',
    placement: const CardPlacement(x: 0, y: 0, width: 4, height: 3),
    stateNamespace: 'state-1',
    status: CardInstanceStatus.active,
  );
}
