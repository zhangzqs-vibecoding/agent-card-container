import 'dart:convert';
import 'dart:io';
import 'dart:math';
import 'dart:typed_data';

import '../agent_studio/agent_studio_controller.dart';
import '../adapters/desktop_multi_window_driver.dart';
import '../adapters/desktop_host_capability_ports.dart';
import '../adapters/hotkey_overlay_restore_shortcut.dart';
import '../adapters/multi_window_backend.dart';
import '../adapters/screen_retriever_display_monitor.dart';
import '../artifacts/artifact_crypto.dart';
import '../artifacts/artifact_installer.dart';
import '../capabilities/capability.dart';
import '../capabilities/capability_broker.dart';
import '../capabilities/secure_network_fetcher.dart';
import '../capabilities/host_capability_handlers.dart';
import '../capabilities/permission_request_controller.dart';
import '../capabilities/window_capability_handlers.dart';
import '../cloud/card_install_coordinator.dart';
import '../cloud/card_catalog_controller.dart';
import '../cloud/cloud_api_client.dart';
import '../diagnostics/diagnostic_bundle.dart';
import '../runtime/local_runtime_server.dart';
import '../recovery/startup_recovery.dart';
import '../storage/local_database.dart';
import '../surfaces/surface_coordinator.dart';
import '../surfaces/overlay_mode_controller.dart';
import '../workspace/workspace_card.dart';
import '../workspace/workspace_controller.dart';
import '../workspace/installed_workspace_card_factory.dart';
import '../workspace/workspace_surface_snapshot_provider.dart';
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
    required this.workspaceController,
    required this.surfaceCoordinator,
    required this.windowBackend,
    required this.overlayModeController,
    required this.displayMonitor,
    required this.permissionRequests,
    required this.startupRecovery,
    this.cloudClient,
    this.agentStudioController,
    this.cardCatalogController,
  });

  final LocalDatabase database;
  final LocalRuntimeServer runtimeServer;
  final List<WorkspaceCard> workspaceCards;
  final List<RecoveryError> recoveryErrors;
  final WorkspaceController workspaceController;
  final SurfaceCoordinator surfaceCoordinator;
  final MultiWindowBackend windowBackend;
  final OverlayModeController overlayModeController;
  final ScreenRetrieverDisplayMonitor displayMonitor;
  final PermissionRequestController permissionRequests;
  final StartupRecovery startupRecovery;
  final CloudApiClient? cloudClient;
  final AgentStudioController? agentStudioController;
  final CardCatalogController? cardCatalogController;
  var _closed = false;
  var _platformInitialized = false;

  String createDiagnosticBundle({DateTime Function()? now}) {
    return DiagnosticBundleBuilder().build(
      runtime: DiagnosticRuntimeInfo(
        appVersion: const String.fromEnvironment(
          'APP_VERSION',
          defaultValue: 'development',
        ),
        platform: Platform.operatingSystem,
        operatingSystemVersion: Platform.operatingSystemVersion,
        locale: Platform.localeName,
        installedCardCount: database.listInstallations().length,
        activeSurfaceCount: database.listSurfaces().length,
      ),
      errors: [
        for (final error in recoveryErrors)
          DiagnosticError(
            category: 'artifact',
            message: '${error.instanceId}: ${error.message}',
          ),
      ],
      generatedAt: (now ?? DateTime.now)(),
    );
  }

  Future<void> initializePlatformSurfaces() async {
    if (_platformInitialized) {
      return;
    }
    _platformInitialized = true;
    await windowBackend.initializeBridge();
    await overlayModeController.initialize();
    await surfaceCoordinator.restorePersistedSurfaces();
    await displayMonitor.start();
  }

  Future<void> close() async {
    if (_closed) {
      return;
    }
    _closed = true;
    agentStudioController?.dispose();
    cardCatalogController?.dispose();
    permissionRequests.dispose();
    workspaceController.dispose();
    cloudClient?.close();
    displayMonitor.dispose();
    await overlayModeController.dispose();
    await runtimeServer.close();
    database.close();
    startupRecovery.markClean();
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
    final startupRecovery = StartupRecovery.start(
      File(_join(root.path, 'run.marker')),
    );
    final database = LocalDatabase.open(_join(root.path, 'agent-card.sqlite3'));
    LocalRuntimeServer? runtimeServer;
    try {
      runtimeServer = await LocalRuntimeServer.start();
      final hostCapabilities = HostCapabilityHandlers(
        clipboard: FlutterClipboardCapabilityPort(),
        externalUrls: UrlLauncherExternalUrlPort(),
        notifications: LocalNotifierCapabilityPort(),
        metrics: ProcessSystemMetricsPort(),
      );
      final permissionRequests = PermissionRequestController();
      final installations = {
        for (final record in database.listInstallations())
          record.installation.versionId: record,
      };
      final cards = <WorkspaceCard>[];
      final errors = <RecoveryError>[];
      if (startupRecovery.previousRunUnclean) {
        errors.add(
          const RecoveryError(
            instanceId: 'desktop-runtime',
            message: 'previous desktop run did not shut down cleanly',
          ),
        );
      }
      late SurfaceCoordinator surfaceCoordinator;
      late WindowCapabilityHandlers windowCapabilities;
      final workspaceCardFactory = InstalledWorkspaceCardFactory(
        runtimeServer: runtimeServer,
        database: database,
        capabilityRuntimeFactory: (instance, definition) {
          final broker =
              CapabilityBroker(
                  requestGrant: permissionRequests.requestGrant,
                  persistGrant: database.upsertGrant,
                  onGrantChanged: (grant) {
                    runtimeServer!.publishEventForInstance(
                      grant.instanceId,
                      'permission.changed',
                      {
                        'capability': grant.capability,
                        'granted': true,
                        'domains': grant.domains.toList()..sort(),
                      },
                    );
                  },
                )
                ..register('network.fetch', SecureNetworkFetcher().handle)
                ..register('clipboard.write', hostCapabilities.clipboardWrite)
                ..register('clipboard.read', hostCapabilities.clipboardRead)
                ..register('host.openExternal', hostCapabilities.openExternal)
                ..register(
                  'notification.show',
                  hostCapabilities.notificationShow,
                )
                ..register(
                  'system.metrics.get',
                  hostCapabilities.systemMetricsGet,
                )
                ..register(
                  'window.getState',
                  (context, params) =>
                      windowCapabilities.getState(context, params),
                )
                ..register(
                  'window.detach',
                  (context, params) =>
                      windowCapabilities.detach(context, params),
                )
                ..register(
                  'window.dock',
                  (context, params) => windowCapabilities.dock(context, params),
                )
                ..register(
                  'window.setAlwaysOnTop',
                  (context, params) =>
                      windowCapabilities.setAlwaysOnTop(context, params),
                )
                ..register(
                  'window.requestAttention',
                  (context, params) =>
                      windowCapabilities.requestAttention(context, params),
                )
                ..replaceGrants(
                  database.grantsForInstance(instance.instanceId),
                );
          return (
            broker: broker,
            context: CardContext(
              instanceId: instance.instanceId,
              cardId: instance.cardId,
              versionId: instance.versionId,
              declaredCapabilities: definition.capabilities.toSet(),
              networkDomains: definition.networkPolicy.domains.toSet(),
            ),
          );
        },
      );
      for (final instance in database.listInstances()) {
        if (instance.status.name == 'quarantined') {
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
        try {
          cards.add(
            workspaceCardFactory.create(
              InstalledArtifact(
                definition: installation.definition,
                contentHash: installation.installation.contentHash,
                directory: Directory(
                  _artifactDirectory(
                    root,
                    installation.installation.contentHash,
                  ),
                ),
                keyId: installation.keyId,
              ),
              instance,
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
      final workspaceController = WorkspaceController(cards);
      final snapshotProvider = WorkspaceSurfaceSnapshotProvider(
        cards: () => workspaceController.cards,
        readState: database.readState,
      );
      final windowBackend = MultiWindowBackend(
        DesktopMultiWindowDriver(),
        snapshotProvider: snapshotProvider.call,
        onBridgeMessage: (message) =>
            surfaceCoordinator.handleBridgeMessage(message),
      );
      final overlayModeController = OverlayModeController(
        shortcut: HotKeyOverlayRestoreShortcut(),
        surfaces: windowBackend,
      );
      surfaceCoordinator = SurfaceCoordinator(
        database: database,
        windows: windowBackend,
        newDetachedSurfaceId: () => _randomID('detached-'),
        onInstanceMoved: (instance) {
          workspaceController.moveInstance(
            instance.instanceId,
            surfaceId: instance.surfaceId,
            placement: instance.placement,
          );
          runtimeServer!.publishEventForInstance(
            instance.instanceId,
            'surface.changed',
            {
              'surfaceId': instance.surfaceId,
              'placement': {
                'x': instance.placement.x,
                'y': instance.placement.y,
                'width': instance.placement.width,
                'height': instance.placement.height,
              },
            },
          );
        },
        onOverlayDisplayRequested: overlayModeController.enterDisplayMode,
        onStateChanged: workspaceController.updatePersistedState,
        onCapabilityInvocation: (instanceId, method, params) async {
          final card = workspaceController.cards.singleWhere(
            (candidate) => candidate.instance.instanceId == instanceId,
          );
          final broker = card.capabilityBroker;
          final context = card.cardContext;
          if (broker == null || context == null) {
            return const {
              'ok': false,
              'errorCode': 'capabilityUnavailable',
              'message': 'capability broker is not attached',
            };
          }
          try {
            return {
              'ok': true,
              'result': await broker.invoke(
                context.withUserGesture(true),
                method,
                params,
              ),
            };
          } on CapabilityException catch (error) {
            return {
              'ok': false,
              'errorCode': error.code.name,
              'message': error.message,
            };
          }
        },
      );
      windowCapabilities = WindowCapabilityHandlers(surfaceCoordinator);
      final displayMonitor = ScreenRetrieverDisplayMonitor(surfaceCoordinator);
      final cloud = _cloudConfiguration(
        processEnvironment,
        root,
        database,
        workspaceController,
        workspaceCardFactory,
      );
      return DesktopRuntime(
        database: database,
        runtimeServer: runtimeServer,
        workspaceCards: List.unmodifiable(cards),
        recoveryErrors: List.unmodifiable(errors),
        workspaceController: workspaceController,
        surfaceCoordinator: surfaceCoordinator,
        windowBackend: windowBackend,
        overlayModeController: overlayModeController,
        displayMonitor: displayMonitor,
        permissionRequests: permissionRequests,
        startupRecovery: startupRecovery,
        cloudClient: cloud?.client,
        agentStudioController: cloud?.controller,
        cardCatalogController: cloud?.catalog,
      );
    } catch (_) {
      await runtimeServer?.close();
      database.close();
      startupRecovery.markClean();
      rethrow;
    }
  }
}

({
  CloudApiClient client,
  AgentStudioController controller,
  CardCatalogController catalog,
})?
_cloudConfiguration(
  Map<String, String> environment,
  Directory root,
  LocalDatabase database,
  WorkspaceController workspace,
  InstalledWorkspaceCardFactory workspaceCardFactory,
) {
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
  final trustedKeys = _trustedKeys(environment['AGENTCARD_TRUSTED_KEYS_JSON']);
  CardInstallCoordinator? coordinator;
  if (trustedKeys.isNotEmpty) {
    coordinator = CardInstallCoordinator(
      client: client,
      installer: ArtifactInstaller(
        root: root,
        crypto: ArtifactCrypto.native(),
        trustedKeys: trustedKeys,
      ),
      database: database,
      newInstanceId: () => _randomID('instance_'),
      newStateNamespace: () => _randomID('state_'),
      now: DateTime.now,
      workspaceCardFactory: workspaceCardFactory,
    );
  }
  Future<void> installCardVersion(String cardId, String versionId) async {
    final result = await coordinator!.installCardVersion(cardId, versionId);
    final card = result.workspaceCard;
    if (card != null) {
      workspace.add(card);
    }
  }

  return (
    client: client,
    controller: AgentStudioController(
      port: CloudGenerationPort(client),
      onReady: coordinator == null
          ? null
          : (session) async {
              final result = await coordinator!.installVersion(
                session.versionId!,
              );
              final card = result.workspaceCard;
              if (card != null) {
                workspace.add(card);
              }
            },
    ),
    catalog: CardCatalogController(
      port: _CloudCatalogClient(client),
      installVersion: coordinator == null ? null : installCardVersion,
    ),
  );
}

class _CloudCatalogClient implements CloudCatalogPort {
  const _CloudCatalogClient(this.client);

  final CloudApiClient client;

  @override
  Future<CloudCardDetail> getCard(String cardId) => client.getCard(cardId);

  @override
  Future<List<CloudCardSummary>> listCards() => client.listCards();
}

Map<String, Uint8List> _trustedKeys(String? source) {
  if (source == null || source.isEmpty) {
    return const {};
  }
  final decoded = jsonDecode(source);
  if (decoded is! Map<String, Object?>) {
    throw const FormatException('trusted keys must be a JSON object');
  }
  return {
    for (final entry in decoded.entries)
      entry.key: Uint8List.fromList(
        base64Url.decode(_padded(entry.value as String)),
      ),
  };
}

String _padded(String value) {
  return value + '=' * ((4 - value.length % 4) % 4);
}

String _randomID(String prefix) {
  final random = Random.secure();
  final value = List.generate(
    16,
    (_) => random.nextInt(256).toRadixString(16).padLeft(2, '0'),
  ).join();
  return '$prefix$value';
}

String _artifactDirectory(Directory root, String contentHash) {
  return [
    root.path,
    'artifacts',
    'sha256',
    contentHash,
  ].join(Platform.pathSeparator);
}

String _join(String first, String second) {
  return [first, second].join(Platform.pathSeparator);
}
