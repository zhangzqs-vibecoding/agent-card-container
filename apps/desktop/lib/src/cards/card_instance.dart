import '../contracts/card_definition.dart';
import '../surfaces/surface.dart';

enum CardInstanceStatus { active, suspended, error, quarantined }

class CardInstallation {
  const CardInstallation({
    required this.cardId,
    required this.versionId,
    required this.contentHash,
    required this.runtime,
    required this.installedAt,
    required this.verified,
  });

  final String cardId;
  final String versionId;
  final String contentHash;
  final CardRuntime runtime;
  final DateTime installedAt;
  final bool verified;
}

class StoredInstallation {
  const StoredInstallation({
    required this.installation,
    required this.definition,
    required this.keyId,
  });

  final CardInstallation installation;
  final CardDefinition definition;
  final String keyId;
}

class CardInstance {
  const CardInstance({
    required this.instanceId,
    required this.cardId,
    required this.versionId,
    required this.surfaceId,
    required this.placement,
    required this.stateNamespace,
    required this.status,
  });

  final String instanceId;
  final String cardId;
  final String versionId;
  final String surfaceId;
  final CardPlacement placement;
  final String stateNamespace;
  final CardInstanceStatus status;
}
