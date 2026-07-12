import '../surfaces/surface_window.dart';
import 'workspace_card.dart';

typedef WorkspaceCardsReader = Iterable<WorkspaceCard> Function();
typedef WorkspaceStateReader = Map<String, Object?> Function(String);

class WorkspaceSurfaceSnapshotProvider {
  const WorkspaceSurfaceSnapshotProvider({
    required this.cards,
    required this.readState,
  });

  final WorkspaceCardsReader cards;
  final WorkspaceStateReader readState;

  SurfaceCardSnapshot call(String instanceId) {
    final card = cards().where(
      (candidate) => candidate.instance.instanceId == instanceId,
    );
    if (card.length != 1) {
      throw StateError('surface card instance is unavailable');
    }
    final value = card.single;
    final nativeSpec = value.nativeSpec;
    if (nativeSpec != null) {
      return SurfaceCardSnapshot.fromJson({
        'instanceId': instanceId,
        'runtime': 'native',
        'spec': nativeSpec.toJson(),
        'state': readState(value.instance.stateNamespace),
      });
    }
    final descriptor = value.codeCard!;
    return SurfaceCardSnapshot.fromJson({
      'instanceId': instanceId,
      'runtime': 'code',
      'sessionId': descriptor.session.id,
      'origin': descriptor.session.origin,
      'entrypoint': descriptor.entrypoint,
    });
  }
}
