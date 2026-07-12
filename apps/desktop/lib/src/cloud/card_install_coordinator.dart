import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';

import '../artifacts/artifact_installer.dart';
import '../cards/card_instance.dart';
import '../capabilities/capability.dart';
import '../contracts/card_definition.dart';
import '../native_card/native_card_spec.dart';
import '../storage/local_database.dart';
import '../surfaces/surface.dart';
import '../workspace/workspace_card.dart';
import '../workspace/installed_workspace_card_factory.dart';
import 'cloud_api_client.dart';

class InstalledCardResult {
  const InstalledCardResult({
    required this.instance,
    required this.artifact,
    this.workspaceCard,
  });

  final CardInstance instance;
  final InstalledArtifact artifact;
  final WorkspaceCard? workspaceCard;
}

class CardInstallCoordinator {
  const CardInstallCoordinator({
    required this.client,
    required this.installer,
    required this.database,
    required this.newInstanceId,
    required this.newStateNamespace,
    required this.now,
    this.workspaceCardFactory,
  });

  final CloudApiClient client;
  final ArtifactInstaller installer;
  final LocalDatabase database;
  final String Function() newInstanceId;
  final String Function() newStateNamespace;
  final DateTime Function() now;
  final InstalledWorkspaceCardFactory? workspaceCardFactory;

  Future<InstalledCardResult> installVersion(String versionId) async {
    final cards = await client.listCards();
    CloudCardVersion? version;
    for (final card in cards) {
      if (card.latestVersion.versionId == versionId) {
        version = card.latestVersion;
        break;
      }
    }
    if (version == null) {
      throw const CloudApiException(
        statusCode: HttpStatus.notFound,
        code: 'NOT_FOUND',
        message: '待安装版本不存在',
      );
    }
    return _install(version);
  }

  Future<InstalledCardResult> installCardVersion(
    String cardId,
    String versionId,
  ) async {
    final card = await client.getCard(cardId);
    CloudCardVersion? version;
    for (final candidate in card.versions) {
      if (candidate.versionId == versionId) {
        version = candidate;
        break;
      }
    }
    if (version == null) {
      throw const CloudApiException(
        statusCode: HttpStatus.notFound,
        code: 'NOT_FOUND',
        message: '待安装版本不存在',
      );
    }
    return _install(version);
  }

  Future<InstalledCardResult> _install(CloudCardVersion version) async {
    final download = await client.artifactDownload(
      version.cardId,
      version.versionId,
    );
    if (download.expiresAt.isBefore(now().toUtc())) {
      throw const CloudApiException(
        statusCode: HttpStatus.gone,
        code: 'ARTIFACT_URL_EXPIRED',
        message: '制品下载地址已过期',
      );
    }
    if (download.sha256 != version.artifactSha256 ||
        download.keyId != version.keyId) {
      throw const FormatException(
        'artifact download metadata does not match card version',
      );
    }
    final bytes = await client.downloadArtifact(download.url);
    final installed = installer.install(Uint8List.fromList(bytes));
    if (installed.contentHash != download.sha256 ||
        installed.keyId != download.keyId ||
        installed.definition.cardId != version.cardId ||
        installed.definition.versionId != version.versionId ||
        installed.definition.runtime.name != version.runtime) {
      throw const FormatException(
        'installed artifact identity does not match cloud version',
      );
    }

    NativeCardSpec? nativeSpec;
    if (installed.definition.runtime == CardRuntime.native) {
      final payload = File(
        [
          installed.directory.path,
          ...installed.definition.entrypoint.split('/'),
        ].join(Platform.pathSeparator),
      );
      final decoded =
          jsonDecode(payload.readAsStringSync()) as Map<String, Object?>;
      nativeSpec = NativeCardSpec.fromJson(decoded);
    }
    final instance = CardInstance(
      instanceId: newInstanceId(),
      cardId: installed.definition.cardId,
      versionId: installed.definition.versionId,
      surfaceId: 'workspace-main',
      placement: const CardPlacement(x: 0, y: 0, width: 4, height: 3),
      stateNamespace: newStateNamespace(),
      status: CardInstanceStatus.active,
    );
    database.registerInstalledInstance(
      installation: StoredInstallation(
        installation: CardInstallation(
          cardId: installed.definition.cardId,
          versionId: installed.definition.versionId,
          contentHash: installed.contentHash,
          runtime: installed.definition.runtime,
          installedAt: now().toUtc(),
          verified: true,
        ),
        definition: installed.definition,
        keyId: installed.keyId,
      ),
      surface: const CardSurface(
        id: 'workspace-main',
        type: SurfaceType.workspace,
      ),
      instance: instance,
      grants: {
        for (final capability in const {'storage', 'window.manageSelf'})
          if (installed.definition.hasCapability(capability))
            PermissionGrant(
              instanceId: instance.instanceId,
              versionId: instance.versionId,
              capability: capability,
            ),
      },
    );

    WorkspaceCard? workspaceCard;
    if (workspaceCardFactory != null) {
      workspaceCard = workspaceCardFactory!.create(installed, instance);
    } else if (nativeSpec != null) {
      workspaceCard = WorkspaceCard(
        instance: instance,
        spec: nativeSpec,
        persistedState: database.readState(instance.stateNamespace),
      );
    }
    return InstalledCardResult(
      instance: instance,
      artifact: installed,
      workspaceCard: workspaceCard,
    );
  }
}
