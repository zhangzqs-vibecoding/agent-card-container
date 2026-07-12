import 'dart:convert';

import 'package:flutter/material.dart';

class SurfaceWindowArguments {
  const SurfaceWindowArguments({
    required this.surfaceId,
    required this.surfaceType,
    required this.ownerWindowId,
    required this.alwaysOnTop,
    required this.instanceIds,
    this.bounds,
  });

  final String surfaceId;
  final String surfaceType;
  final String ownerWindowId;
  final bool alwaysOnTop;
  final List<String> instanceIds;
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
      return SurfaceWindowArguments(
        surfaceId: surfaceId,
        surfaceType: surfaceType as String,
        ownerWindowId: ownerWindowId,
        alwaysOnTop: alwaysOnTop,
        instanceIds: List.unmodifiable(instances),
        bounds: _rect(value['bounds']),
      );
    } catch (_) {
      return null;
    }
  }
}

class SurfaceWindowModel extends ChangeNotifier {
  SurfaceWindowModel(this.arguments)
    : _instanceIds = List.of(arguments.instanceIds);

  final SurfaceWindowArguments arguments;
  List<String> _instanceIds;

  List<String> get instanceIds => List.unmodifiable(_instanceIds);

  void updateInstances(List<String> instanceIds) {
    if (instanceIds.any((id) => id.isEmpty) ||
        instanceIds.toSet().length != instanceIds.length) {
      throw const FormatException('surface instance IDs are invalid');
    }
    _instanceIds = List.of(instanceIds);
    notifyListeners();
  }
}

class SurfaceWindowApp extends StatelessWidget {
  const SurfaceWindowApp({required this.model, super.key});

  final SurfaceWindowModel model;

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      debugShowCheckedModeBanner: false,
      theme: ThemeData.dark(useMaterial3: true),
      home: AnimatedBuilder(
        animation: model,
        builder: (context, _) => Scaffold(
          backgroundColor: model.arguments.surfaceType == 'overlay'
              ? Colors.transparent
              : const Color(0xFF0B0E0F),
          body: SafeArea(
            child: Center(
              child: Wrap(
                spacing: 12,
                runSpacing: 12,
                children: [
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
