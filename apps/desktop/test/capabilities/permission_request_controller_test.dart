import 'package:agent_card_desktop/src/capabilities/capability.dart';
import 'package:agent_card_desktop/src/capabilities/permission_request_controller.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test(
    'queues first-use prompts and grants only the requested domain',
    () async {
      final controller = PermissionRequestController();
      addTearDown(controller.dispose);
      final context = CardContext(
        instanceId: 'instance-1',
        cardId: 'card-1',
        versionId: 'version-1',
        declaredCapabilities: const {'network.fetch'},
        networkDomains: const {'api.example.com'},
      );

      final result = controller.requestGrant(context, 'network.fetch', {
        'url': 'https://api.example.com/data',
      });
      expect(controller.current?.domain, 'api.example.com');

      controller.approve();

      expect((await result)?.domains, {'api.example.com'});
      expect(controller.current, isNull);
    },
  );

  test('denial completes the pending request without a grant', () async {
    final controller = PermissionRequestController();
    addTearDown(controller.dispose);
    final context = CardContext(
      instanceId: 'instance-1',
      cardId: 'card-1',
      versionId: 'version-1',
      declaredCapabilities: const {'notification.show'},
    );

    final result = controller.requestGrant(
      context,
      'notification.show',
      const {},
    );
    controller.deny();

    expect(await result, isNull);
  });
}
