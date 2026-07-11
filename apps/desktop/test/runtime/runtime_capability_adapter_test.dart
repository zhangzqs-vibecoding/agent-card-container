import 'package:agent_card_desktop/src/capabilities/capability.dart';
import 'package:agent_card_desktop/src/capabilities/capability_broker.dart';
import 'package:agent_card_desktop/src/runtime/local_runtime_server.dart';
import 'package:agent_card_desktop/src/runtime/runtime_capability_adapter.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test(
    'forwards the immutable card identity through CapabilityBroker',
    () async {
      final broker = CapabilityBroker()
        ..replaceGrants({
          const PermissionGrant(
            instanceId: 'instance-1',
            versionId: 'version-1',
            capability: 'notification.show',
          ),
        })
        ..register('notification.show', (context, params) async {
          return {'instanceId': context.instanceId, 'title': params['title']};
        });
      final adapter = RuntimeCapabilityAdapter(
        broker: broker,
        cardContext: CardContext(
          instanceId: 'instance-1',
          cardId: 'card-1',
          versionId: 'version-1',
          declaredCapabilities: {'notification.show'},
        ),
      );

      expect(
        await adapter.handle(
          const RuntimeRpcContext(
            instanceId: 'instance-1',
            cardId: 'card-1',
            versionId: 'version-1',
          ),
          'notification.show',
          {'title': '完成'},
        ),
        {'instanceId': 'instance-1', 'title': '完成'},
      );
    },
  );

  test('maps broker errors and rejects mismatched session identity', () async {
    final adapter = RuntimeCapabilityAdapter(
      broker: CapabilityBroker(),
      cardContext: CardContext(
        instanceId: 'instance-1',
        cardId: 'card-1',
        versionId: 'version-1',
        declaredCapabilities: {'notification.show'},
      ),
    );

    await expectLater(
      adapter.handle(
        const RuntimeRpcContext(
          instanceId: 'instance-1',
          cardId: 'card-1',
          versionId: 'version-1',
        ),
        'notification.show',
        const {},
      ),
      throwsA(
        isA<RuntimeRpcException>().having(
          (error) => error.code,
          'code',
          'PERMISSION_REQUIRED',
        ),
      ),
    );
    await expectLater(
      adapter.handle(
        const RuntimeRpcContext(
          instanceId: 'other',
          cardId: 'card-1',
          versionId: 'version-1',
        ),
        'notification.show',
        const {},
      ),
      throwsA(
        isA<RuntimeRpcException>().having(
          (error) => error.code,
          'code',
          'SESSION_EXPIRED',
        ),
      ),
    );
  });
}
