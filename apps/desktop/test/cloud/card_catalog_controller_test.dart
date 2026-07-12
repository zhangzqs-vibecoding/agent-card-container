import 'package:agent_card_desktop/src/cloud/card_catalog_controller.dart';
import 'package:agent_card_desktop/src/cloud/cloud_api_client.dart';
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
      expect(controller.selectedCard?.versions.single.versionId, 'ver_01');
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
      versions: [_version],
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
