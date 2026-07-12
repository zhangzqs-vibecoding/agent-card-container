import '../cards/card_instance.dart';
import '../capabilities/capability.dart';
import '../capabilities/capability_broker.dart';
import '../native_card/native_card_spec.dart';
import '../runtime/runtime_session.dart';

class CodeCardDescriptor {
  const CodeCardDescriptor({required this.session, required this.entrypoint});

  final RuntimeSession session;
  final String entrypoint;
}

class WorkspaceCard {
  const WorkspaceCard({
    required this.instance,
    required NativeCardSpec spec,
    this.persistedState = const {},
    this.capabilityBroker,
    this.cardContext,
  }) : nativeSpec = spec,
       codeCard = null;

  const WorkspaceCard.code({
    required this.instance,
    required this.codeCard,
    this.persistedState = const {},
    this.capabilityBroker,
    this.cardContext,
  }) : nativeSpec = null;

  final CardInstance instance;
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
