import 'package:agent_card_desktop/src/cloud/card_catalog_controller.dart';
import 'package:agent_card_desktop/src/cloud/card_version_lifecycle.dart';
import 'package:agent_card_desktop/src/cloud/cloud_api_client.dart';
import 'package:agent_card_desktop/src/cards/card_instance.dart';
import 'package:agent_card_desktop/src/capabilities/capability.dart';
import 'package:agent_card_desktop/src/surfaces/surface.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test(
    'loads cards, selects history and installs an explicit version',
    () async {
      final port = _FakeCatalogPort();
      final installed = <String>[];
      final controller = CardCatalogController(
        port: port,
        installVersion: (cardId, versionId) async {
          installed.add('$cardId/$versionId');
        },
      );
      addTearDown(controller.dispose);

      await controller.refresh();
      await controller.selectCard('card_01');
      await controller.install('ver_01');

      expect(controller.cards.single.title, '番茄钟');
      expect(
        controller.selectedCard?.versions.any(
          (version) => version.versionId == 'ver_01',
        ),
        isTrue,
      );
      expect(installed, ['card_01/ver_01']);
      expect(controller.installingVersionId, isNull);
      expect(controller.errorMessage, isNull);
    },
  );

  test('exposes catalog failures without discarding loaded cards', () async {
    final port = _FakeCatalogPort();
    final controller = CardCatalogController(
      port: port,
      installVersion: (_, _) async {},
    );
    addTearDown(controller.dispose);
    await controller.refresh();
    port.failDetails = true;

    await controller.selectCard('card_01');

    expect(controller.cards, isNotEmpty);
    expect(controller.errorMessage, isNotNull);
    expect(controller.loading, isFalse);
  });

  test('classifies current upgrade rollback and install actions', () async {
    final controller = CardCatalogController(
      port: _FakeCatalogPort(),
      installVersion: (_, _) async {},
      activeInstanceForCard: (cardId) =>
          cardId == 'card_01' ? _instance('ver_01') : null,
    );
    addTearDown(controller.dispose);
    await controller.selectCard('card_01');

    expect(controller.actionFor(_version), CatalogVersionAction.current);
    expect(controller.actionFor(_newVersion), CatalogVersionAction.upgrade);
    expect(controller.actionFor(_oldVersion), CatalogVersionAction.rollback);

    final withoutInstance = CardCatalogController(
      port: _FakeCatalogPort(),
      installVersion: (_, _) async {},
    );
    addTearDown(withoutInstance.dispose);
    expect(withoutInstance.actionFor(_version), CatalogVersionAction.install);
  });

  test(
    'prepares and applies one version change without duplicate actions',
    () async {
      final applied = <CardVersionDecision>[];
      final proposal = CatalogVersionChangeProposal(
        instance: _instance('ver_01'),
        targetVersion: _newVersion,
        difference: CardVersionDifference(
          addedCapabilities: const ['clipboard.read'],
          removedCapabilities: const [],
          addedDomains: const [],
          removedDomains: const [],
          currentStateSchemaVersion: 1,
          targetStateSchemaVersion: 1,
        ),
        apply: (decision, grants) async {
          applied.add(decision);
          expect(grants.single.capability, 'clipboard.read');
        },
      );
      final controller = CardCatalogController(
        port: _FakeCatalogPort(),
        activeInstanceForCard: (_) => _instance('ver_01'),
        prepareVersionChange: (_, _, _) async => proposal,
      );
      addTearDown(controller.dispose);
      await controller.selectCard('card_01');

      final prepared = await controller.prepareChange('ver_02');
      expect(prepared, same(proposal));
      expect(controller.changingVersionId, 'ver_02');
      expect(await controller.prepareChange('ver_02'), isNull);

      await controller.applyChange(proposal, CardVersionDecision.reuseState, {
        const PermissionGrant(
          instanceId: 'instance-1',
          versionId: 'ver_02',
          capability: 'clipboard.read',
        ),
      });
      expect(applied, [CardVersionDecision.reuseState]);
      expect(controller.changingVersionId, isNull);
      expect(controller.errorMessage, isNull);
    },
  );
}

class _FakeCatalogPort implements CloudCatalogPort {
  var failDetails = false;

  @override
  Future<List<CloudCardSummary>> listCards() async => [
    CloudCardSummary(
      cardId: 'card_01',
      title: '番茄钟',
      description: '离线计时器',
      latestVersion: _version,
    ),
  ];

  @override
  Future<CloudCardDetail> getCard(String cardId) async {
    if (failDetails) {
      throw StateError('offline');
    }
    return CloudCardDetail(
      cardId: cardId,
      title: '番茄钟',
      description: '离线计时器',
      versions: [_newVersion, _version, _oldVersion],
    );
  }
}

final _version = CloudCardVersion(
  versionId: 'ver_01',
  cardId: 'card_01',
  runtime: 'native',
  displayVersion: '1.0.0',
  title: '番茄钟',
  description: '离线计时器',
  artifactSha256: List.filled(64, 'a').join(),
  keyId: 'key-1',
  preview: const {},
  createdAt: DateTime.utc(2026, 7, 12),
);

final _newVersion = CloudCardVersion(
  versionId: 'ver_02',
  cardId: 'card_01',
  runtime: 'native',
  displayVersion: '1.0.1',
  title: '番茄钟',
  description: '离线计时器',
  artifactSha256: List.filled(64, 'b').join(),
  keyId: 'key-1',
  preview: const {},
  createdAt: DateTime.utc(2026, 7, 13),
);

final _oldVersion = CloudCardVersion(
  versionId: 'ver_00',
  cardId: 'card_01',
  runtime: 'native',
  displayVersion: '0.9.0',
  title: '番茄钟',
  description: '离线计时器',
  artifactSha256: List.filled(64, 'c').join(),
  keyId: 'key-1',
  preview: const {},
  createdAt: DateTime.utc(2026, 7, 11),
);

CardInstance _instance(String versionId) {
  return CardInstance(
    instanceId: 'instance-1',
    cardId: 'card_01',
    versionId: versionId,
    surfaceId: 'workspace-main',
    placement: const CardPlacement(x: 0, y: 0, width: 4, height: 3),
    stateNamespace: 'state-1',
    status: CardInstanceStatus.active,
  );
}
