import 'dart:convert';

import 'package:agent_card_desktop/src/surfaces/surface_window.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test('accepts only explicit surface-engine arguments', () {
    expect(SurfaceWindowArguments.tryParse(''), isNull);
    expect(
      SurfaceWindowArguments.tryParse(jsonEncode({'kind': 'other'})),
      isNull,
    );

    final arguments = SurfaceWindowArguments.tryParse(
      jsonEncode({
        'kind': 'surface',
        'surfaceId': 'overlay-monitor-a',
        'surfaceType': 'overlay',
        'ownerWindowId': 'main-window',
        'alwaysOnTop': true,
        'instanceIds': ['instance-1'],
      }),
    );

    expect(arguments?.surfaceId, 'overlay-monitor-a');
    expect(arguments?.ownerWindowId, 'main-window');
    expect(arguments?.instanceIds, ['instance-1']);
  });

  test('updates only the instances mounted by the owning surface', () {
    final model = SurfaceWindowModel(
      SurfaceWindowArguments.tryParse(
        jsonEncode({
          'kind': 'surface',
          'surfaceId': 'detached-1',
          'surfaceType': 'detached',
          'ownerWindowId': 'main-window',
          'alwaysOnTop': false,
          'instanceIds': ['instance-1'],
        }),
      )!,
    );
    addTearDown(model.dispose);

    model.updateInstances(['instance-2']);

    expect(model.instanceIds, ['instance-2']);
  });
}
