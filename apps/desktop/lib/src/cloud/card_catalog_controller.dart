import 'package:flutter/foundation.dart';

import '../capabilities/capability.dart';
import '../cards/card_instance.dart';
import 'card_version_lifecycle.dart';
import 'cloud_api_client.dart';

abstract interface class CloudCatalogPort {
  Future<List<CloudCardSummary>> listCards();

  Future<CloudCardDetail> getCard(String cardId);
}

enum CatalogVersionAction { install, current, upgrade, rollback }

typedef ActiveInstanceForCard = CardInstance? Function(String cardId);
typedef PrepareCatalogVersionChange =
    Future<CatalogVersionChangeProposal> Function(
      String instanceId,
      String cardId,
      CloudCardVersion targetVersion,
    );

class CatalogVersionChangeProposal {
  const CatalogVersionChangeProposal({
    required this.instance,
    required this.targetVersion,
    required this.difference,
    required this.apply,
  });

  final CardInstance instance;
  final CloudCardVersion targetVersion;
  final CardVersionDifference difference;
  final Future<void> Function(
    CardVersionDecision decision,
    Set<PermissionGrant> approvedGrants,
  )
  apply;
}

class CardCatalogController extends ChangeNotifier {
  CardCatalogController({
    required CloudCatalogPort port,
    Future<void> Function(String cardId, String versionId)? installVersion,
    ActiveInstanceForCard? activeInstanceForCard,
    PrepareCatalogVersionChange? prepareVersionChange,
  }) : _port = port,
       _installVersion = installVersion,
       _activeInstanceForCard = activeInstanceForCard,
       _prepareVersionChange = prepareVersionChange;

  final CloudCatalogPort _port;
  final Future<void> Function(String cardId, String versionId)? _installVersion;
  final ActiveInstanceForCard? _activeInstanceForCard;
  final PrepareCatalogVersionChange? _prepareVersionChange;
  List<CloudCardSummary> _cards = const [];
  CloudCardDetail? _selectedCard;
  String? _installingVersionId;
  String? _changingVersionId;
  String? _errorMessage;
  var _loading = false;
  var _disposed = false;

  List<CloudCardSummary> get cards => _cards;
  CloudCardDetail? get selectedCard => _selectedCard;
  String? get installingVersionId => _installingVersionId;
  String? get changingVersionId => _changingVersionId;
  String? get errorMessage => _errorMessage;
  bool get loading => _loading;
  bool get installationAvailable => _installVersion != null;
  bool get versionChangeAvailable => _prepareVersionChange != null;

  CatalogVersionAction actionFor(CloudCardVersion version) {
    final active = _activeInstanceForCard?.call(version.cardId);
    if (active == null) return CatalogVersionAction.install;
    if (active.versionId == version.versionId) {
      return CatalogVersionAction.current;
    }
    final current = _selectedCard?.versions.where(
      (candidate) => candidate.versionId == active.versionId,
    );
    if (current == null || current.isEmpty) {
      return CatalogVersionAction.install;
    }
    return _compareDisplayVersions(
              version.displayVersion,
              current.single.displayVersion,
            ) >
            0
        ? CatalogVersionAction.upgrade
        : CatalogVersionAction.rollback;
  }

  Future<void> refresh() async {
    _beginOperation();
    try {
      _cards = await _port.listCards();
    } catch (_) {
      _errorMessage = '无法加载卡片库，请检查云端连接';
    } finally {
      _endOperation();
    }
  }

  Future<void> selectCard(String cardId) async {
    _beginOperation();
    try {
      _selectedCard = await _port.getCard(cardId);
    } catch (_) {
      _errorMessage = '无法加载版本历史，请稍后重试';
    } finally {
      _endOperation();
    }
  }

  Future<void> install(String versionId) async {
    final card = _selectedCard;
    final installVersion = _installVersion;
    if (card == null ||
        _installingVersionId != null ||
        installVersion == null) {
      return;
    }
    _installingVersionId = versionId;
    _errorMessage = null;
    _notify();
    try {
      await installVersion(card.cardId, versionId);
    } catch (_) {
      _errorMessage = '版本安装失败，请重试';
    } finally {
      _installingVersionId = null;
      _notify();
    }
  }

  Future<CatalogVersionChangeProposal?> prepareChange(String versionId) async {
    final card = _selectedCard;
    final prepare = _prepareVersionChange;
    if (card == null || prepare == null || _changingVersionId != null) {
      return null;
    }
    final target = card.versions.where(
      (version) => version.versionId == versionId,
    );
    final active = _activeInstanceForCard?.call(card.cardId);
    if (target.isEmpty || active == null) return null;
    _changingVersionId = versionId;
    _errorMessage = null;
    _notify();
    try {
      return await prepare(active.instanceId, card.cardId, target.single);
    } catch (_) {
      _changingVersionId = null;
      _errorMessage = '无法准备版本切换，请重新下载后再试';
      _notify();
      return null;
    }
  }

  Future<void> applyChange(
    CatalogVersionChangeProposal proposal,
    CardVersionDecision decision,
    Set<PermissionGrant> approvedGrants,
  ) async {
    if (_changingVersionId != proposal.targetVersion.versionId) return;
    try {
      await proposal.apply(decision, approvedGrants);
    } catch (_) {
      _errorMessage = '版本切换失败，当前版本仍可继续使用';
    } finally {
      _changingVersionId = null;
      _notify();
    }
  }

  void cancelChange() {
    if (_changingVersionId == null) return;
    _changingVersionId = null;
    _notify();
  }

  void _beginOperation() {
    _loading = true;
    _errorMessage = null;
    _notify();
  }

  void _endOperation() {
    _loading = false;
    _notify();
  }

  void _notify() {
    if (!_disposed) {
      notifyListeners();
    }
  }

  @override
  void dispose() {
    _disposed = true;
    super.dispose();
  }
}

int _compareDisplayVersions(String left, String right) {
  final leftParts = _displayVersionParts(left);
  final rightParts = _displayVersionParts(right);
  for (var index = 0; index < 3; index++) {
    final comparison = leftParts[index].compareTo(rightParts[index]);
    if (comparison != 0) return comparison;
  }
  return 0;
}

List<int> _displayVersionParts(String value) {
  final match = RegExp(
    r'^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$',
  ).firstMatch(value);
  if (match == null) {
    throw FormatException('invalid card display version: $value');
  }
  return [for (var index = 1; index <= 3; index++) int.parse(match[index]!)];
}
