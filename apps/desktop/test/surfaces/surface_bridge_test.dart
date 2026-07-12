import 'package:agent_card_desktop/src/surfaces/surface_bridge.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  group('SurfaceBridge', () {
    test('accepts only the registered protocol message types', () {
      for (final type in SurfaceBridgeMessageType.values) {
        final message = SurfaceBridgeMessage.fromJson({
          'type': type.wireName,
          'windowId': 'window-1',
          'instanceId': 'instance-1',
          'payload': <String, Object?>{},
        });

        expect(message.type, type);
      }

      expect(
        () => SurfaceBridgeMessage.fromJson({
          'type': 'readDatabase',
          'windowId': 'window-1',
          'instanceId': 'instance-1',
          'payload': <String, Object?>{},
        }),
        throwsA(isA<FormatException>()),
      );
    });

    test('rejects messages for instances owned by another window', () {
      final bindings = SurfaceBridgeBindings()
        ..replaceWindowInstances('window-1', ['instance-1'])
        ..replaceWindowInstances('window-2', ['instance-2']);
      final ownMessage = SurfaceBridgeMessage.fromJson({
        'type': 'focusChanged',
        'windowId': 'window-1',
        'instanceId': 'instance-1',
        'payload': {'focused': true},
      });
      final foreignMessage = SurfaceBridgeMessage.fromJson({
        'type': 'focusChanged',
        'windowId': 'window-1',
        'instanceId': 'instance-2',
        'payload': {'focused': true},
      });

      expect(bindings.accepts(ownMessage), isTrue);
      expect(bindings.accepts(foreignMessage), isFalse);
    });

    test('removes stale bindings when a surface mount changes', () {
      final bindings = SurfaceBridgeBindings()
        ..replaceWindowInstances('window-1', ['instance-1']);
      bindings.replaceWindowInstances('window-1', ['instance-2']);

      expect(bindings.owns('window-1', 'instance-1'), isFalse);
      expect(bindings.owns('window-1', 'instance-2'), isTrue);
    });
  });
}
