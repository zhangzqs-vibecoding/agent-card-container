import 'dart:convert';
import 'dart:io';

import '../artifacts/artifact_installer.dart';
import '../capabilities/capability.dart';
import '../capabilities/capability_broker.dart';
import '../cards/card_instance.dart';
import '../contracts/card_definition.dart';
import '../native_card/native_card_spec.dart';
import '../runtime/local_runtime_server.dart';
import '../runtime/runtime_capability_adapter.dart';
import '../storage/database_runtime_storage.dart';
import '../storage/local_database.dart';
import 'workspace_card.dart';

typedef CapabilityRuntimeFactory =
    ({CapabilityBroker broker, CardContext context}) Function(
      CardInstance instance,
      CardDefinition definition,
    );

class InstalledWorkspaceCardFactory {
  const InstalledWorkspaceCardFactory({
    required this.runtimeServer,
    required this.database,
    this.capabilityRuntimeFactory,
  });

  final LocalRuntimeServer runtimeServer;
  final LocalDatabase database;
  final CapabilityRuntimeFactory? capabilityRuntimeFactory;

  WorkspaceCard create(InstalledArtifact artifact, CardInstance instance) {
    if (artifact.definition.cardId != instance.cardId ||
        artifact.definition.versionId != instance.versionId) {
      throw const FormatException(
        'installed artifact does not match the card instance',
      );
    }
    final capabilityRuntime = capabilityRuntimeFactory?.call(
      instance,
      artifact.definition,
    );
    if (artifact.definition.runtime == CardRuntime.native) {
      final payload = _file(artifact, artifact.definition.entrypoint);
      final decoded =
          jsonDecode(payload.readAsStringSync()) as Map<String, Object?>;
      return WorkspaceCard(
        instance: instance,
        spec: NativeCardSpec.fromJson(decoded),
        persistedState: database.readState(instance.stateNamespace),
        capabilityBroker: capabilityRuntime?.broker,
        cardContext: capabilityRuntime?.context,
      );
    }
    final prefix = '/bundle/${artifact.contentHash}';
    final resources = <String, RuntimeResource>{};
    for (final entry in artifact.definition.files) {
      final file = _file(artifact, entry.path);
      resources['$prefix/${entry.path}'] = RuntimeResource(
        bytes: file.readAsBytesSync(),
        contentType: _contentType(entry.path),
      );
    }
    final session = runtimeServer.createSession(
      instanceId: instance.instanceId,
      cardId: instance.cardId,
      versionId: instance.versionId,
      resources: resources,
      declaredCapabilities: artifact.definition.capabilities.toSet(),
      storage: DatabaseRuntimeStorage(database, instance.stateNamespace),
      rpcHandler: capabilityRuntime == null
          ? null
          : RuntimeCapabilityAdapter(
              broker: capabilityRuntime.broker,
              cardContext: capabilityRuntime.context,
            ).handle,
    );
    return WorkspaceCard.code(
      instance: instance,
      codeCard: CodeCardDescriptor(
        session: session,
        entrypoint: '$prefix/${artifact.definition.entrypoint}',
        onLaunchFailure: () async =>
            database.recordLaunchFailure(instance.instanceId),
        onLaunchSuccess: () async =>
            database.clearLaunchFailures(instance.instanceId),
        onQuarantine: () async =>
            database.quarantineInstance(instance.instanceId),
      ),
      persistedState: database.readState(instance.stateNamespace),
      capabilityBroker: capabilityRuntime?.broker,
      cardContext: capabilityRuntime?.context,
    );
  }
}

File _file(InstalledArtifact artifact, String path) {
  final file = File(
    [artifact.directory.path, ...path.split('/')].join(Platform.pathSeparator),
  );
  if (!file.existsSync()) {
    throw FormatException('installed card file is missing: $path');
  }
  return file;
}

String _contentType(String path) {
  final extension = path.split('.').last.toLowerCase();
  return switch (extension) {
    'html' => 'text/html; charset=utf-8',
    'js' || 'mjs' => 'text/javascript; charset=utf-8',
    'css' => 'text/css; charset=utf-8',
    'json' => 'application/json; charset=utf-8',
    'svg' => 'image/svg+xml',
    'png' => 'image/png',
    'jpg' || 'jpeg' => 'image/jpeg',
    'gif' => 'image/gif',
    'webp' => 'image/webp',
    'woff' => 'font/woff',
    'woff2' => 'font/woff2',
    _ => 'application/octet-stream',
  };
}
