import 'package:flutter/material.dart';

import 'catalog_renderer.dart';
import 'native_card_controller.dart';
import 'native_card_spec.dart';

class NativeCardRenderer extends StatelessWidget {
  const NativeCardRenderer({
    required this.spec,
    required this.controller,
    super.key,
  });

  final NativeCardSpec spec;
  final NativeCardController controller;

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: controller,
      builder: (context, _) {
        return Material(
          type: MaterialType.transparency,
          child: CatalogRenderer(
            controller: controller,
          ).render(context, spec.root),
        );
      },
    );
  }
}
