import '../artifacts/artifact_installer.dart';
import '../capabilities/capability.dart';
import '../cards/card_instance.dart';
import '../storage/local_database.dart';
import '../workspace/workspace_card.dart';

typedef WorkspaceCardBuilder =
    WorkspaceCard Function(InstalledArtifact artifact, CardInstance instance);
typedef WorkspaceCardReplacer = void Function(WorkspaceCard card);

class CardVersionLifecycleException implements Exception {
  const CardVersionLifecycleException(this.code, this.message);

  final String code;
  final String message;

  @override
  String toString() => 'CardVersionLifecycleException($code): $message';
}

class CardVersionDifference {
  CardVersionDifference({
    required Iterable<String> addedCapabilities,
    required Iterable<String> removedCapabilities,
    required Iterable<String> addedDomains,
    required Iterable<String> removedDomains,
    required this.currentStateSchemaVersion,
    required this.targetStateSchemaVersion,
  }) : addedCapabilities = _sorted(addedCapabilities),
       removedCapabilities = _sorted(removedCapabilities),
       addedDomains = _sorted(addedDomains),
       removedDomains = _sorted(removedDomains);

  final List<String> addedCapabilities;
  final List<String> removedCapabilities;
  final List<String> addedDomains;
  final List<String> removedDomains;
  final int currentStateSchemaVersion;
  final int targetStateSchemaVersion;

  bool get stateCompatible =>
      currentStateSchemaVersion == targetStateSchemaVersion;
}

class PreparedCardVersionChange {
  const PreparedCardVersionChange({
    required this.instance,
    required this.currentInstallation,
    required this.targetInstallation,
    required this.targetArtifact,
    required this.difference,
  });

  final CardInstance instance;
  final StoredInstallation currentInstallation;
  final StoredInstallation targetInstallation;
  final InstalledArtifact targetArtifact;
  final CardVersionDifference difference;
}

enum CardVersionDecision { reuseState, resetState, restoreState, cancel }

class CardVersionLifecycle {
  const CardVersionLifecycle({
    required this.database,
    required this.newBackupId,
    required this.newStateNamespace,
    required this.now,
    this.buildWorkspaceCard,
    this.replaceWorkspaceCard,
  });

  final LocalDatabase database;
  final String Function() newBackupId;
  final String Function() newStateNamespace;
  final DateTime Function() now;
  final WorkspaceCardBuilder? buildWorkspaceCard;
  final WorkspaceCardReplacer? replaceWorkspaceCard;

  PreparedCardVersionChange prepare(
    String instanceId,
    InstalledArtifact targetArtifact,
  ) {
    final instances = database.listInstances().where(
      (candidate) => candidate.instanceId == instanceId,
    );
    if (instances.isEmpty) {
      throw const CardVersionLifecycleException(
        'INSTANCE_NOT_FOUND',
        'card instance does not exist',
      );
    }
    final instance = instances.single;
    final current = database.installation(instance.versionId);
    final target = database.installation(targetArtifact.definition.versionId);
    if (current == null || target == null) {
      throw const CardVersionLifecycleException(
        'INSTALLATION_NOT_FOUND',
        'card version is not installed',
      );
    }
    if (!target.installation.verified ||
        target.installation.cardId != targetArtifact.definition.cardId ||
        target.installation.versionId != targetArtifact.definition.versionId ||
        target.installation.contentHash != targetArtifact.contentHash ||
        target.keyId != targetArtifact.keyId) {
      throw const CardVersionLifecycleException(
        'ARTIFACT_NOT_VERIFIED',
        'target artifact does not match the verified installation',
      );
    }
    if (instance.cardId != targetArtifact.definition.cardId ||
        current.definition.cardId != targetArtifact.definition.cardId) {
      throw const CardVersionLifecycleException(
        'CARD_VERSION_MISMATCH',
        'target version belongs to a different card',
      );
    }
    if (instance.versionId == targetArtifact.definition.versionId) {
      throw const CardVersionLifecycleException(
        'VERSION_ALREADY_ACTIVE',
        'target version is already active',
      );
    }

    final currentCapabilities = current.definition.capabilities.toSet();
    final targetCapabilities = target.definition.capabilities.toSet();
    final currentDomains = current.definition.networkPolicy.domains.toSet();
    final targetDomains = target.definition.networkPolicy.domains.toSet();
    return PreparedCardVersionChange(
      instance: instance,
      currentInstallation: current,
      targetInstallation: target,
      targetArtifact: targetArtifact,
      difference: CardVersionDifference(
        addedCapabilities: targetCapabilities.difference(currentCapabilities),
        removedCapabilities: currentCapabilities.difference(targetCapabilities),
        addedDomains: targetDomains.difference(currentDomains),
        removedDomains: currentDomains.difference(targetDomains),
        currentStateSchemaVersion: current.definition.stateSchemaVersion,
        targetStateSchemaVersion: target.definition.stateSchemaVersion,
      ),
    );
  }

  CardInstance apply(
    PreparedCardVersionChange prepared, {
    required CardVersionDecision decision,
    Set<PermissionGrant> approvedGrants = const {},
  }) {
    final liveInstances = database.listInstances().where(
      (candidate) => candidate.instanceId == prepared.instance.instanceId,
    );
    if (liveInstances.isEmpty) {
      throw const CardVersionLifecycleException(
        'INSTANCE_NOT_FOUND',
        'card instance does not exist',
      );
    }
    final live = liveInstances.single;
    if (decision == CardVersionDecision.cancel) {
      return live;
    }
    if (live.versionId != prepared.instance.versionId ||
        live.stateNamespace != prepared.instance.stateNamespace) {
      throw const CardVersionLifecycleException(
        'VERSION_CHANGE_STALE',
        'card instance changed after confirmation was prepared',
      );
    }

    final compatible = prepared.difference.stateCompatible;
    if (compatible && decision != CardVersionDecision.reuseState) {
      throw const CardVersionLifecycleException(
        'INVALID_STATE_DECISION',
        'compatible card versions must reuse state',
      );
    }
    if (!compatible && decision == CardVersionDecision.reuseState) {
      throw const CardVersionLifecycleException(
        'INVALID_STATE_DECISION',
        'incompatible card versions cannot reuse state',
      );
    }

    final targetDefinition = prepared.targetInstallation.definition;
    final targetCapabilities = targetDefinition.capabilities.toSet();
    final targetDomains = targetDefinition.networkPolicy.domains.toSet();
    for (final grant in approvedGrants) {
      if (grant.instanceId != live.instanceId ||
          grant.versionId != targetDefinition.versionId ||
          !targetCapabilities.contains(grant.capability) ||
          !targetDomains.containsAll(grant.domains) ||
          (grant.capability != 'network.fetch' && grant.domains.isNotEmpty)) {
        throw const CardVersionLifecycleException(
          'PERMISSION_APPROVAL_INVALID',
          'approved grant exceeds the target card manifest',
        );
      }
    }
    final approvedCapabilities = approvedGrants
        .map((grant) => grant.capability)
        .toSet();
    final approvedDomains = approvedGrants
        .where((grant) => grant.capability == 'network.fetch')
        .expand((grant) => grant.domains)
        .toSet();
    if (!approvedCapabilities.containsAll(
          prepared.difference.addedCapabilities,
        ) ||
        !approvedDomains.containsAll(prepared.difference.addedDomains)) {
      throw const CardVersionLifecycleException(
        'PERMISSION_APPROVAL_REQUIRED',
        'new card capabilities and domains require explicit approval',
      );
    }

    final replacement = <String, Set<String>>{};
    for (final grant in database.grantsForInstance(live.instanceId)) {
      if (!targetCapabilities.contains(grant.capability)) continue;
      replacement
          .putIfAbsent(grant.capability, () => {})
          .addAll(grant.domains.intersection(targetDomains));
    }
    for (final grant in approvedGrants) {
      replacement.putIfAbsent(grant.capability, () => {}).addAll(grant.domains);
    }
    final replacementGrants = {
      for (final entry in replacement.entries)
        PermissionGrant(
          instanceId: live.instanceId,
          versionId: targetDefinition.versionId,
          capability: entry.key,
          domains: Set.unmodifiable(entry.value),
        ),
    };

    CardStateBackup? restoreBackup;
    var statePolicy = VersionStatePolicy.reuse;
    if (!compatible) {
      if (decision == CardVersionDecision.restoreState) {
        final backups = database.stateBackups(
          instanceId: live.instanceId,
          versionId: targetDefinition.versionId,
        );
        if (backups.isNotEmpty &&
            backups.first.stateSchemaVersion ==
                targetDefinition.stateSchemaVersion) {
          restoreBackup = backups.first;
          statePolicy = VersionStatePolicy.restore;
        } else {
          statePolicy = VersionStatePolicy.reset;
        }
      } else {
        statePolicy = VersionStatePolicy.reset;
      }
    }

    return database.switchInstalledInstanceVersion(
      instanceId: live.instanceId,
      targetVersionId: targetDefinition.versionId,
      statePolicy: statePolicy,
      currentStateSchemaVersion:
          prepared.currentInstallation.definition.stateSchemaVersion,
      targetStateSchemaVersion: targetDefinition.stateSchemaVersion,
      replacementGrants: replacementGrants,
      backupId: compatible ? '' : newBackupId(),
      newStateNamespace: compatible ? '' : newStateNamespace(),
      createdAt: now().toUtc(),
      restoreBackup: restoreBackup,
    );
  }

  CardInstance applyAndActivate(
    PreparedCardVersionChange prepared, {
    required CardVersionDecision decision,
    Set<PermissionGrant> approvedGrants = const {},
  }) {
    final builder = buildWorkspaceCard;
    final replacer = replaceWorkspaceCard;
    if (builder == null || replacer == null) {
      throw const CardVersionLifecycleException(
        'VERSION_RUNTIME_UNAVAILABLE',
        'workspace runtime activation is unavailable',
      );
    }
    if (decision == CardVersionDecision.cancel) {
      return apply(prepared, decision: decision);
    }
    final previousGrants = database.grantsForInstance(
      prepared.instance.instanceId,
    );
    final switched = apply(
      prepared,
      decision: decision,
      approvedGrants: approvedGrants,
    );
    try {
      final card = builder(prepared.targetArtifact, switched);
      replacer(card);
      return switched;
    } catch (_) {
      try {
        _restoreAfterActivationFailure(prepared, switched, previousGrants);
      } catch (_) {
        throw const CardVersionLifecycleException(
          'VERSION_SWITCH_RECOVERY_FAILED',
          'target runtime failed and the previous version could not be restored',
        );
      }
      throw const CardVersionLifecycleException(
        'VERSION_RUNTIME_BUILD_FAILED',
        'target runtime could not be constructed; the previous version was restored',
      );
    }
  }

  void _restoreAfterActivationFailure(
    PreparedCardVersionChange prepared,
    CardInstance switched,
    Set<PermissionGrant> previousGrants,
  ) {
    final previousSchema =
        prepared.currentInstallation.definition.stateSchemaVersion;
    final switchedSchema =
        prepared.targetInstallation.definition.stateSchemaVersion;
    final compatible = previousSchema == switchedSchema;
    CardStateBackup? restoreBackup;
    if (!compatible) {
      final backups = database.stateBackups(
        instanceId: prepared.instance.instanceId,
        versionId: prepared.instance.versionId,
      );
      if (backups.isEmpty ||
          backups.first.stateSchemaVersion != previousSchema) {
        throw StateError('previous card state backup is unavailable');
      }
      restoreBackup = backups.first;
    }
    database.switchInstalledInstanceVersion(
      instanceId: switched.instanceId,
      targetVersionId: prepared.instance.versionId,
      statePolicy: compatible
          ? VersionStatePolicy.reuse
          : VersionStatePolicy.restore,
      currentStateSchemaVersion: switchedSchema,
      targetStateSchemaVersion: previousSchema,
      replacementGrants: previousGrants,
      backupId: compatible ? '' : newBackupId(),
      newStateNamespace: compatible ? '' : prepared.instance.stateNamespace,
      createdAt: now().toUtc(),
      restoreBackup: restoreBackup,
    );
  }
}

List<String> _sorted(Iterable<String> values) {
  final result = values.toSet().toList()..sort();
  return List.unmodifiable(result);
}
