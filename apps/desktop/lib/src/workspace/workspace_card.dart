import '../cards/card_instance.dart';
import '../native_card/native_card_spec.dart';

class WorkspaceCard {
  const WorkspaceCard({
    required this.instance,
    required this.spec,
    this.persistedState = const {},
  });

  final CardInstance instance;
  final NativeCardSpec spec;
  final Map<String, Object?> persistedState;
}
