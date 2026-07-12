import 'dart:convert';
import 'dart:io';

import '../agent_studio/agent_studio_controller.dart';
import '../cloud/cloud_api_client.dart';
import '../contracts/card_definition.dart';
import '../native_card/native_card_spec.dart';
import '../runtime/local_runtime_server.dart';
import '../storage/local_database.dart';
import '../workspace/workspace_card.dart';
import 'app_data_locator.dart';

class RecoveryError {
  const RecoveryError({required this.instanceId, required this.message});

  final String instanceId;
  final String message;
}

class DesktopRuntime {
  DesktopRuntime({
    required this.database,
    required this.runtimeServer,
    required this.workspaceCards,
    required this.recoveryErrors,
    this.cloudClient,
    this.agentStudioController,
  });

  final LocalDatabase database;
  final LocalRuntimeServer runtimeServer;
  final List<WorkspaceCard> workspaceCards;
  final List<RecoveryError> recoveryErrors;
  final CloudApiClient? cloudClient;
  final AgentStudioController? agentStudioController;
  var _closed = false;

  Future<void> close() async {
    if (_closed) {
      return;
    }
    _closed = true;
    agentStudioController?.dispose();
    cloudClient?.close();
    await runtimeServer.close();
    database.close();
  }
}

abstract final class DesktopBootstrap {
  static Future<DesktopRuntime> start({
    Directory? appDataDirectory,
    Map<String, String>? environment,
  }) async {
    final processEnvironment = environment ?? Platform.environment;
    final root = appDataDirectory ?? AppDataLocator.resolve();
    root.createSync(recursive: true);
    final database = LocalDatabase.open(_join(root.path, 'agent-card.sqlite3'));
    try {
      final installations = {
        for (final record in database.listInstallations())
          record.installation.versionId: record,
      };
      final cards = <WorkspaceCard>[];
      final errors = <RecoveryError>[];
      for (final instance in database.listInstances()) {
        if (instance.surfaceId != 'workspace-main' ||
            instance.status.name == 'quarantined') {
          continue;
        }
        final installation = installations[instance.versionId];
        if (installation == null) {
          errors.add(
            RecoveryError(
              instanceId: instance.instanceId,
              message: 'installed card definition is unavailable',
            ),
          );
          continue;
        }
        if (installation.definition.runtime != CardRuntime.native) {
          continue;
        }
        try {
          final payload = File(
            _artifactPath(
              root,
              installation.installation.contentHash,
              installation.definition.entrypoint,
            ),
          );
          final decoded =
              jsonDecode(payload.readAsStringSync()) as Map<String, Object?>;
          cards.add(
            WorkspaceCard(
              instance: instance,
              spec: NativeCardSpec.fromJson(decoded),
            ),
          );
        } catch (_) {
          errors.add(
            RecoveryError(
              instanceId: instance.instanceId,
              message: 'card payload could not be restored',
            ),
          );
        }
      }
      final cloud = _cloudConfiguration(processEnvironment);
      final runtimeServer = await LocalRuntimeServer.start();
      return DesktopRuntime(
        database: database,
        runtimeServer: runtimeServer,
        workspaceCards: List.unmodifiable(cards),
        recoveryErrors: List.unmodifiable(errors),
        cloudClient: cloud?.client,
        agentStudioController: cloud?.controller,
      );
    } catch (_) {
      database.close();
      rethrow;
    }
  }
}

({CloudApiClient client, AgentStudioController controller})?
_cloudConfiguration(Map<String, String> environment) {
  final url = environment['AGENTCARD_CLOUD_URL'];
  final token = environment['AGENTCARD_ACCESS_TOKEN'];
  if (url == null || url.isEmpty || token == null || token.isEmpty) {
    return null;
  }
  final client = CloudApiClient(
    baseUri: Uri.parse(url),
    tokenProvider: () async => token,
    allowInsecureForDevelopment:
        environment['AGENTCARD_ALLOW_INSECURE_CLOUD'] == 'true',
  );
  return (
    client: client,
    controller: AgentStudioController(port: CloudGenerationPort(client)),
  );
}

String _artifactPath(Directory root, String contentHash, String entrypoint) {
  return [
    root.path,
    'artifacts',
    'sha256',
    contentHash,
    ...entrypoint.split('/'),
  ].join(Platform.pathSeparator);
}

String _join(String first, String second) {
  return [first, second].join(Platform.pathSeparator);
}
