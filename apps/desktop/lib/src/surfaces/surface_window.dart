import 'dart:async';
import 'dart:convert';

import 'package:flutter/material.dart';

import '../adapters/in_app_webview_port.dart';
import '../code_card/code_card_host.dart';
import '../native_card/native_card_controller.dart';
import '../native_card/native_card_renderer.dart';
import '../native_card/native_card_spec.dart';
import '../runtime/runtime_activity_budget.dart';
import 'surface_bridge.dart';

enum SurfaceCardRuntime { native, code }

class SurfaceCardSnapshot {
  const SurfaceCardSnapshot._({
    required this.instanceId,
    required this.runtime,
    required this.state,
    this.nativeSpec,
    this.sessionId,
    this.origin,
    this.entrypoint,
  });

  factory SurfaceCardSnapshot.fromJson(Map<String, Object?> json) {
    final runtime = switch (json['runtime']) {
      'native' => SurfaceCardRuntime.native,
      'code' => SurfaceCardRuntime.code,
      _ => throw const FormatException('surface card runtime is invalid'),
    };
    final allowed = runtime == SurfaceCardRuntime.native
        ? const {'instanceId', 'runtime', 'spec', 'state'}
        : const {'instanceId', 'runtime', 'sessionId', 'origin', 'entrypoint'};
    final unknown = json.keys.where((key) => !allowed.contains(key));
    if (unknown.isNotEmpty) {
      throw FormatException(
        'surface card has unknown fields: ${unknown.join(', ')}',
      );
    }
    final instanceId = _requiredString(json, 'instanceId');
    if (runtime == SurfaceCardRuntime.native) {
      return SurfaceCardSnapshot._(
        instanceId: instanceId,
        runtime: runtime,
        nativeSpec: NativeCardSpec.fromJson(_object(json['spec'], 'spec')),
        state: Map.unmodifiable(_object(json['state'], 'state')),
      );
    }
    final sessionId = _requiredString(json, 'sessionId');
    final origin = Uri.tryParse(_requiredString(json, 'origin'));
    final entrypoint = _requiredString(json, 'entrypoint');
    if (origin == null ||
        origin.scheme != 'http' ||
        origin.host != '127.0.0.1' ||
        !origin.hasPort ||
        origin.userInfo.isNotEmpty ||
        origin.path.isNotEmpty ||
        origin.hasQuery ||
        origin.hasFragment) {
      throw const FormatException('CodeCard surface origin is invalid');
    }
    if (!entrypoint.startsWith('/') || entrypoint.contains('..')) {
      throw const FormatException('CodeCard surface entrypoint is invalid');
    }
    return SurfaceCardSnapshot._(
      instanceId: instanceId,
      runtime: runtime,
      sessionId: sessionId,
      origin: origin,
      entrypoint: entrypoint,
      state: const {},
    );
  }

  final String instanceId;
  final SurfaceCardRuntime runtime;
  final NativeCardSpec? nativeSpec;
  final Map<String, Object?> state;
  final String? sessionId;
  final Uri? origin;
  final String? entrypoint;

  Map<String, Object?> toJson() {
    if (runtime == SurfaceCardRuntime.native) {
      return {
        'instanceId': instanceId,
        'runtime': 'native',
        'spec': nativeSpec!.toJson(),
        'state': state,
      };
    }
    return {
      'instanceId': instanceId,
      'runtime': 'code',
      'sessionId': sessionId,
      'origin': origin.toString(),
      'entrypoint': entrypoint,
    };
  }
}

class SurfaceWindowArguments {
  const SurfaceWindowArguments({
    required this.surfaceId,
    required this.surfaceType,
    required this.ownerWindowId,
    required this.alwaysOnTop,
    required this.instanceIds,
    this.cards = const [],
    this.bounds,
  });

  final String surfaceId;
  final String surfaceType;
  final String ownerWindowId;
  final bool alwaysOnTop;
  final List<String> instanceIds;
  final List<SurfaceCardSnapshot> cards;
  final Rect? bounds;

  static SurfaceWindowArguments? tryParse(String source) {
    try {
      final value = jsonDecode(source);
      if (value is! Map<String, Object?> || value['kind'] != 'surface') {
        return null;
      }
      final surfaceId = value['surfaceId'];
      final surfaceType = value['surfaceType'];
      final ownerWindowId = value['ownerWindowId'];
      final alwaysOnTop = value['alwaysOnTop'];
      final rawInstances = value['instanceIds'];
      final rawCards = value['cards'];
      if (surfaceId is! String ||
          surfaceId.isEmpty ||
          (surfaceType != 'overlay' && surfaceType != 'detached') ||
          ownerWindowId is! String ||
          ownerWindowId.isEmpty ||
          alwaysOnTop is! bool ||
          rawInstances is! List ||
          rawInstances.any((item) => item is! String || item.isEmpty)) {
        return null;
      }
      final instances = rawInstances.cast<String>();
      if (instances.toSet().length != instances.length) {
        return null;
      }
      final cards = rawCards == null
          ? const <SurfaceCardSnapshot>[]
          : (rawCards as List)
                .map(
                  (card) => SurfaceCardSnapshot.fromJson(
                    _object(card, 'surface card'),
                  ),
                )
                .toList(growable: false);
      if (cards.map((card) => card.instanceId).toSet().length != cards.length ||
          cards.any((card) => !instances.contains(card.instanceId))) {
        return null;
      }
      return SurfaceWindowArguments(
        surfaceId: surfaceId,
        surfaceType: surfaceType as String,
        ownerWindowId: ownerWindowId,
        alwaysOnTop: alwaysOnTop,
        instanceIds: List.unmodifiable(instances),
        cards: List.unmodifiable(cards),
        bounds: _rect(value['bounds']),
      );
    } catch (_) {
      return null;
    }
  }
}

SurfaceBridgeMessage buildSurfaceCloseRequest(
  SurfaceWindowArguments arguments,
  String windowId,
) {
  if (arguments.surfaceType != 'detached' ||
      arguments.instanceIds.length != 1) {
    throw const FormatException(
      'only a single-instance detached window can request close',
    );
  }
  return SurfaceBridgeMessage(
    type: SurfaceBridgeMessageType.hostEvent,
    windowId: windowId,
    instanceId: arguments.instanceIds.single,
    payload: const {'event': 'windowCloseRequested'},
  );
}

SurfaceBridgeMessage buildOverlayDisplayRequest(
  SurfaceWindowArguments arguments,
  String windowId,
) {
  if (arguments.surfaceType != 'overlay' || arguments.instanceIds.isEmpty) {
    throw const FormatException(
      'only a populated overlay can request display mode',
    );
  }
  return SurfaceBridgeMessage(
    type: SurfaceBridgeMessageType.hostEvent,
    windowId: windowId,
    instanceId: arguments.instanceIds.first,
    payload: const {'event': 'overlayDisplayRequested'},
  );
}

List<SurfaceBridgeMessage> buildSurfacePlacementMessages(
  SurfaceWindowArguments arguments,
  String windowId,
  Rect bounds, {
  String? monitorId,
}) {
  if (bounds.width <= 0 || bounds.height <= 0) {
    throw const FormatException('surface window bounds are invalid');
  }
  return List.unmodifiable(
    arguments.instanceIds.map(
      (instanceId) => SurfaceBridgeMessage(
        type: SurfaceBridgeMessageType.placementChanged,
        windowId: windowId,
        instanceId: instanceId,
        payload: {
          'x': bounds.left,
          'y': bounds.top,
          'width': bounds.width,
          'height': bounds.height,
          if (monitorId != null) 'monitorId': monitorId,
        },
      ),
    ),
  );
}

List<SurfaceBridgeMessage> buildSurfaceFocusMessages(
  SurfaceWindowArguments arguments,
  String windowId, {
  required bool focused,
}) {
  return List.unmodifiable(
    arguments.instanceIds.map(
      (instanceId) => SurfaceBridgeMessage(
        type: SurfaceBridgeMessageType.focusChanged,
        windowId: windowId,
        instanceId: instanceId,
        payload: {'focused': focused},
      ),
    ),
  );
}

class SurfaceWindowModel extends ChangeNotifier {
  SurfaceWindowModel(this.arguments)
    : _instanceIds = List.of(arguments.instanceIds),
      _cards = List.of(arguments.cards);

  final SurfaceWindowArguments arguments;
  List<String> _instanceIds;
  List<SurfaceCardSnapshot> _cards;

  List<String> get instanceIds => List.unmodifiable(_instanceIds);
  List<SurfaceCardSnapshot> get cards => List.unmodifiable(_cards);

  void updateInstances(List<String> instanceIds) {
    if (instanceIds.any((id) => id.isEmpty) ||
        instanceIds.toSet().length != instanceIds.length) {
      throw const FormatException('surface instance IDs are invalid');
    }
    _instanceIds = List.of(instanceIds);
    _cards = _cards
        .where((card) => _instanceIds.contains(card.instanceId))
        .toList();
    notifyListeners();
  }

  void updateCards(List<SurfaceCardSnapshot> cards) {
    final ids = cards.map((card) => card.instanceId).toList();
    if (ids.toSet().length != ids.length) {
      throw const FormatException('surface card snapshots are duplicated');
    }
    _cards = List.of(cards);
    _instanceIds = ids;
    notifyListeners();
  }
}

class SurfaceWindowApp extends StatelessWidget {
  const SurfaceWindowApp({
    required this.model,
    this.onEnterOverlayDisplayMode,
    this.onCapabilityInvocation,
    this.onStateChanged,
    this.onCodeCardLaunch,
    super.key,
  });

  final SurfaceWindowModel model;
  final Future<void> Function()? onEnterOverlayDisplayMode;
  final Future<Object?> Function(
    String instanceId,
    String method,
    Map<String, Object?> params,
  )?
  onCapabilityInvocation;
  final Future<void> Function(String instanceId, Map<String, Object?> state)?
  onStateChanged;
  final Future<void> Function(String instanceId, bool succeeded)?
  onCodeCardLaunch;

  @override
  Widget build(BuildContext context) {
    final activeIndexes = RuntimeActivityBudget.activeIndexes(
      model.cards.map(
        (card) => card.runtime == SurfaceCardRuntime.code
            ? CardRuntimeKind.code
            : CardRuntimeKind.native,
      ),
    );
    return MaterialApp(
      debugShowCheckedModeBanner: false,
      theme: ThemeData.dark(useMaterial3: true),
      home: AnimatedBuilder(
        animation: model,
        builder: (context, _) => Scaffold(
          backgroundColor: model.arguments.surfaceType == 'overlay'
              ? Colors.transparent
              : const Color(0xFF0B0E0F),
          floatingActionButton:
              model.arguments.surfaceType == 'overlay' &&
                  onEnterOverlayDisplayMode != null
              ? FloatingActionButton.small(
                  key: const Key('enter-overlay-display-mode'),
                  tooltip: '进入展示模式（Ctrl+Shift+F12 恢复编辑）',
                  onPressed: onEnterOverlayDisplayMode,
                  child: const Icon(Icons.touch_app_outlined),
                )
              : null,
          body: SafeArea(
            child: Center(
              child: Wrap(
                spacing: 12,
                runSpacing: 12,
                children: [
                  for (final (index, card) in model.cards.indexed)
                    SizedBox(
                      width: 420,
                      height: 280,
                      child: Card(
                        child: _SurfaceCardView(
                          key: ValueKey(card.instanceId),
                          snapshot: card,
                          active: activeIndexes.contains(index),
                          capabilityInvocation: onCapabilityInvocation,
                          onStateChanged: onStateChanged,
                          onCodeCardLaunch: onCodeCardLaunch,
                        ),
                      ),
                    ),
                  if (model.cards.isEmpty)
                    for (final instanceId in model.instanceIds)
                      Card(
                        child: Padding(
                          padding: const EdgeInsets.all(20),
                          child: Text('正在挂载 $instanceId'),
                        ),
                      ),
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }
}

class _SurfaceCardView extends StatefulWidget {
  const _SurfaceCardView({
    required this.snapshot,
    required this.active,
    this.capabilityInvocation,
    this.onStateChanged,
    this.onCodeCardLaunch,
    super.key,
  });

  final SurfaceCardSnapshot snapshot;
  final bool active;
  final Future<Object?> Function(
    String instanceId,
    String method,
    Map<String, Object?> params,
  )?
  capabilityInvocation;
  final Future<void> Function(String instanceId, Map<String, Object?> state)?
  onStateChanged;
  final Future<void> Function(String instanceId, bool succeeded)?
  onCodeCardLaunch;

  @override
  State<_SurfaceCardView> createState() => _SurfaceCardViewState();
}

class _SurfaceCardViewState extends State<_SurfaceCardView> {
  NativeCardController? _controller;
  InAppWebViewPort? _webView;
  Object? _codeCardError;
  var _codeCardMounted = false;
  Future<void> _statePublishTail = Future.value();

  @override
  void initState() {
    super.initState();
    if (!widget.active) {
      return;
    }
    _createRuntime();
  }

  @override
  void didUpdateWidget(covariant _SurfaceCardView oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.active == widget.active) {
      return;
    }
    _disposeRuntime();
    if (widget.active) {
      _createRuntime();
    }
  }

  void _createRuntime() {
    final snapshot = widget.snapshot;
    if (snapshot.nativeSpec case final spec?) {
      _controller = NativeCardController(
        {...spec.initialState, ...snapshot.state},
        capabilityInvocation: widget.capabilityInvocation == null
            ? null
            : (method, params) => widget.capabilityInvocation!(
                snapshot.instanceId,
                method,
                params,
              ),
      );
      _controller?.addListener(_publishState);
    } else {
      final port = InAppWebViewPort();
      _webView = port;
      unawaited(_mountCodeCard(port, snapshot));
    }
  }

  void _disposeRuntime() {
    _controller?.removeListener(_publishState);
    _controller?.dispose();
    _controller = null;
    final webView = _webView;
    _webView = null;
    _codeCardMounted = false;
    _codeCardError = null;
    if (webView != null) {
      unawaited(webView.dispose());
    }
  }

  void _publishState() {
    final callback = widget.onStateChanged;
    final controller = _controller;
    if (callback == null || controller == null) {
      return;
    }
    final state = controller.state;
    _statePublishTail = _statePublishTail.then(
      (_) => _sendState(callback, state),
    );
  }

  Future<void> _sendState(
    Future<void> Function(String, Map<String, Object?>) callback,
    Map<String, Object?> state,
  ) async {
    try {
      await callback(widget.snapshot.instanceId, state);
    } catch (_) {
      // A later state change retries while the main engine remains authoritative.
    }
  }

  Future<void> _mountCodeCard(
    InAppWebViewPort port,
    SurfaceCardSnapshot snapshot,
  ) async {
    try {
      final capabilities = await port.inspectCapabilities();
      if (!capabilities.canEnforceCodeCardPolicy) {
        throw const CodeCardUnavailableException(
          'platform WebView cannot enforce CodeCard isolation policy',
        );
      }
      final origin = snapshot.origin!;
      await port.mount(
        WebViewConfiguration(
          policy: CodeCardWebPolicy(origin),
          developerToolsEnabled: false,
          userDataKey: snapshot.sessionId!,
        ),
        origin.replace(path: snapshot.entrypoint),
      );
      if (!mounted || !identical(port, _webView)) {
        return;
      }
      setState(() => _codeCardMounted = true);
      await widget.onCodeCardLaunch?.call(snapshot.instanceId, true);
    } catch (error) {
      if (!mounted || !identical(port, _webView)) {
        return;
      }
      await widget.onCodeCardLaunch?.call(snapshot.instanceId, false);
      setState(() => _codeCardError = error);
    }
  }

  @override
  void dispose() {
    _disposeRuntime();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    if (!widget.active) {
      return const Center(child: Text('已暂停：卡片不可见或超过活动上限'));
    }
    final spec = widget.snapshot.nativeSpec;
    final controller = _controller;
    if (spec != null && controller != null) {
      return Padding(
        padding: const EdgeInsets.all(14),
        child: NativeCardRenderer(spec: spec, controller: controller),
      );
    }
    if (_codeCardError != null) {
      return const Center(child: Text('当前平台无法安全挂载 CodeCard'));
    }
    final webView = _webView;
    if (!_codeCardMounted || webView == null) {
      return const Center(child: CircularProgressIndicator());
    }
    return CodeCardWebView(port: webView);
  }
}

Rect? _rect(Object? value) {
  if (value == null) {
    return null;
  }
  if (value is! Map<String, Object?>) {
    throw const FormatException('surface bounds must be an object');
  }
  final x = value['x'];
  final y = value['y'];
  final width = value['width'];
  final height = value['height'];
  if (x is! num ||
      y is! num ||
      width is! num ||
      height is! num ||
      width <= 0 ||
      height <= 0) {
    throw const FormatException('surface bounds are invalid');
  }
  return Rect.fromLTWH(
    x.toDouble(),
    y.toDouble(),
    width.toDouble(),
    height.toDouble(),
  );
}

String _requiredString(Map<String, Object?> json, String key) {
  final value = json[key];
  if (value is! String || value.isEmpty) {
    throw FormatException('$key must be a non-empty string');
  }
  return value;
}

Map<String, Object?> _object(Object? value, String context) {
  if (value is! Map<String, Object?>) {
    throw FormatException('$context must be an object');
  }
  return value;
}
