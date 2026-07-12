import 'package:flutter/foundation.dart';

import 'cloud_api_client.dart';

abstract interface class CloudCatalogPort {
  Future<List<CloudCardSummary>> listCards();

  Future<CloudCardDetail> getCard(String cardId);
}

class CardCatalogController extends ChangeNotifier {
  CardCatalogController({
    required CloudCatalogPort port,
    Future<void> Function(String cardId, String versionId)? installVersion,
  }) : _port = port,
       _installVersion = installVersion;

  final CloudCatalogPort _port;
  final Future<void> Function(String cardId, String versionId)? _installVersion;
  List<CloudCardSummary> _cards = const [];
  CloudCardDetail? _selectedCard;
  String? _installingVersionId;
  String? _errorMessage;
  var _loading = false;
  var _disposed = false;

  List<CloudCardSummary> get cards => _cards;
  CloudCardDetail? get selectedCard => _selectedCard;
  String? get installingVersionId => _installingVersionId;
  String? get errorMessage => _errorMessage;
  bool get loading => _loading;
  bool get installationAvailable => _installVersion != null;

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
