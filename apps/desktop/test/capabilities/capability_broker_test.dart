import 'package:agent_card_desktop/src/capabilities/capability.dart';
import 'package:agent_card_desktop/src/capabilities/capability_broker.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  group('CapabilityBroker', () {
    test('rejects capabilities not declared by the card', () async {
      final broker = CapabilityBroker();
      broker.register('notification.show', (_, _) async => {'shown': true});

      await expectLater(
        () => broker.invoke(
          _context(declared: const {}),
          'notification.show',
          const {},
        ),
        throwsA(
          isA<CapabilityException>().having(
            (error) => error.code,
            'code',
            CapabilityErrorCode.permissionDenied,
          ),
        ),
      );
    });

    test('rejects missing and mismatched grants', () async {
      final broker = CapabilityBroker();
      broker.register('notification.show', (_, _) async => {'shown': true});
      final context = _context(declared: const {'notification.show'});

      await expectLater(
        () => broker.invoke(context, 'notification.show', const {}),
        throwsA(
          isA<CapabilityException>().having(
            (error) => error.code,
            'code',
            CapabilityErrorCode.permissionRequired,
          ),
        ),
      );

      broker.replaceGrants({
        const PermissionGrant(
          instanceId: 'instance-1',
          versionId: 'wrong-version',
          capability: 'notification.show',
        ),
      });
      await expectLater(
        () => broker.invoke(context, 'notification.show', const {}),
        throwsA(isA<CapabilityException>()),
      );
    });

    test('rejects expanded network domains', () async {
      final broker = CapabilityBroker();
      broker.register('network.fetch', (_, _) async => {'status': 200});
      broker.replaceGrants({
        const PermissionGrant(
          instanceId: 'instance-1',
          versionId: 'version-1',
          capability: 'network.fetch',
          domains: {'api.example.com'},
        ),
      });
      final context = _context(
        declared: const {'network.fetch'},
        networkDomains: const {'api.example.com'},
      );

      await expectLater(
        () => broker.invoke(context, 'network.fetch', {
          'url': 'https://evil.example/data',
        }),
        throwsA(
          isA<CapabilityException>().having(
            (error) => error.code,
            'code',
            CapabilityErrorCode.permissionDenied,
          ),
        ),
      );
    });

    test(
      'rejects network IP literals even when listed in the manifest',
      () async {
        final broker = CapabilityBroker();
        broker.register('network.fetch', (_, _) async => {'status': 200});
        broker.replaceGrants({
          const PermissionGrant(
            instanceId: 'instance-1',
            versionId: 'version-1',
            capability: 'network.fetch',
            domains: {'93.184.216.34'},
          ),
        });

        await expectLater(
          () => broker.invoke(
            _context(
              declared: const {'network.fetch'},
              networkDomains: const {'93.184.216.34'},
            ),
            'network.fetch',
            {'url': 'https://93.184.216.34/data'},
          ),
          throwsA(
            isA<CapabilityException>().having(
              (error) => error.code,
              'code',
              CapabilityErrorCode.permissionDenied,
            ),
          ),
        );
      },
    );

    test('requires a user gesture for clipboard reads', () async {
      final broker = CapabilityBroker();
      broker.register('clipboard.read', (_, _) async => {'text': 'secret'});
      broker.replaceGrants({
        const PermissionGrant(
          instanceId: 'instance-1',
          versionId: 'version-1',
          capability: 'clipboard.read',
        ),
      });

      await expectLater(
        () => broker.invoke(
          _context(declared: const {'clipboard.read'}),
          'clipboard.read',
          const {},
        ),
        throwsA(
          isA<CapabilityException>().having(
            (error) => error.code,
            'code',
            CapabilityErrorCode.permissionRequired,
          ),
        ),
      );
    });

    test('invokes a registered handler after all policy checks', () async {
      final broker = CapabilityBroker();
      broker.register('window.getState', (context, params) async {
        return {'instanceId': context.instanceId, 'surface': 'workspace'};
      });
      broker.replaceGrants({
        const PermissionGrant(
          instanceId: 'instance-1',
          versionId: 'version-1',
          capability: 'window.manageSelf',
        ),
      });

      final result = await broker.invoke(
        _context(declared: const {'window.manageSelf'}),
        'window.getState',
        const {},
      );

      expect(result, {'instanceId': 'instance-1', 'surface': 'workspace'});
    });
  });
}

CardContext _context({
  required Set<String> declared,
  Set<String> networkDomains = const {},
  bool userGesture = false,
}) {
  return CardContext(
    instanceId: 'instance-1',
    cardId: 'card-1',
    versionId: 'version-1',
    declaredCapabilities: declared,
    networkDomains: networkDomains,
    userGesture: userGesture,
  );
}
