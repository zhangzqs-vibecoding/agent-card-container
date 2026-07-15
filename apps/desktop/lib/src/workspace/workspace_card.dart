import '../cards/card_instance.dart';
import '../capabilities/capability.dart';
import '../capabilities/capability_broker.dart';
import '../native_card/native_card_spec.dart';
import '../runtime/runtime_session.dart';

class CodeCardDescriptor {
  const CodeCardDescriptor({
    required this.session,
    required this.entrypoint,
    this.onLaunchFailure,
    this.onLaunchSuccess,
    this.onQuarantine,
    this.onSuspend,
    this.onResume,
  });

  final RuntimeSession session;
  final String entrypoint;
  final Future<int> Function()? onLaunchFailure;
  final Future<void> Function()? onLaunchSuccess;
  final Future<void> Function()? onQuarantine;
  final Future<void> Function()? onSuspend;
  final Future<void> Function()? onResume;
}

class WorkspaceCard {
  const WorkspaceCard({
    required this.instance,
    required NativeCardSpec spec,
    this.displayVersion,
    this.persistedState = const {},
    this.capabilityBroker,
    this.cardContext,
  }) : nativeSpec = spec,
       codeCard = null;

  const WorkspaceCard.code({
    required this.instance,
    required this.codeCard,
    this.displayVersion,
    this.persistedState = const {},
    this.capabilityBroker,
    this.cardContext,
  }) : nativeSpec = null;

  final CardInstance instance;
  final String? displayVersion;
  final NativeCardSpec? nativeSpec;
  final CodeCardDescriptor? codeCard;
  final Map<String, Object?> persistedState;
  final CapabilityBroker? capabilityBroker;
  final CardContext? cardContext;

  NativeCardSpec get spec {
    final value = nativeSpec;
    if (value == null) {
      throw StateError('CodeCard does not contain a NativeCard spec');
    }
    return value;
  }
}
