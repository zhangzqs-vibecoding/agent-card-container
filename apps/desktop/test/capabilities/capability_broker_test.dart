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
      var confirmations = 0;
      final broker = CapabilityBroker(
        requestGrant: (context, capability, params) async {
          confirmations++;
          return PermissionGrant(
            instanceId: context.instanceId,
            versionId: context.versionId,
            capability: capability,
          );
        },
      );
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
      expect(confirmations, 0);
    });

    test(
      'rejects an invalid network URL before requesting permission',
      () async {
        var confirmations = 0;
        final broker = CapabilityBroker(
          requestGrant: (context, capability, params) async {
            confirmations++;
            return PermissionGrant(
              instanceId: context.instanceId,
              versionId: context.versionId,
              capability: capability,
            );
          },
        );
        broker.register('network.fetch', (_, _) async => {'status': 200});

        await expectLater(
          () => broker.invoke(
            _context(
              declared: const {'network.fetch'},
              networkDomains: const {'api.example.com'},
            ),
            'network.fetch',
            {'url': 'file:///etc/passwd'},
          ),
          throwsA(
            isA<CapabilityException>().having(
              (error) => error.code,
              'code',
              CapabilityErrorCode.invalidParams,
            ),
          ),
        );
        expect(confirmations, 0);
      },
    );

    test('requests host.openExternal permission for each new domain', () async {
      final requestedDomains = <String>[];
      final broker = CapabilityBroker(
        requestGrant: (context, capability, params) async {
          final domain = Uri.parse(params['url']! as String).host;
          requestedDomains.add(domain);
          return PermissionGrant(
            instanceId: context.instanceId,
            versionId: context.versionId,
            capability: capability,
            domains: {domain},
          );
        },
      );
      broker.register('host.openExternal', (_, _) async => {'opened': true});
      final context = _context(declared: const {'host.openExternal'});

      await broker.invoke(context, 'host.openExternal', {
        'url': 'https://docs.example.com/one',
      });
      await broker.invoke(context, 'host.openExternal', {
        'url': 'https://docs.example.com/two',
      });
      await broker.invoke(context, 'host.openExternal', {
        'url': 'https://support.example.com/',
      });

      expect(requestedDomains, ['docs.example.com', 'support.example.com']);
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

    test('automatically grants storage and self-window management', () async {
      final broker = CapabilityBroker()
        ..register('storage.get', (_, _) async => {'value': null})
        ..register('window.getState', (_, _) async => {'surface': 'workspace'});
      final context = _context(
        declared: const {'storage', 'window.manageSelf'},
      );

      expect(await broker.invoke(context, 'storage.get', const {}), {
        'value': null,
      });
      expect(await broker.invoke(context, 'window.getState', const {}), {
        'surface': 'workspace',
      });
    });

    test('requests and persists a narrowly scoped first-use grant', () async {
      final persisted = <PermissionGrant>[];
      final changed = <PermissionGrant>[];
      final broker = CapabilityBroker(
        requestGrant: (context, capability, params) async => PermissionGrant(
          instanceId: context.instanceId,
          versionId: context.versionId,
          capability: capability,
          domains: {Uri.parse(params['url']! as String).host},
        ),
        persistGrant: persisted.add,
        onGrantChanged: changed.add,
      );
      broker.register('network.fetch', (_, _) async => {'status': 200});
      final context = _context(
        declared: const {'network.fetch'},
        networkDomains: const {'api.example.com'},
      );

      final result = await broker.invoke(context, 'network.fetch', {
        'url': 'https://api.example.com/data',
      });

      expect(result, {'status': 200});
      expect(persisted.single.domains, {'api.example.com'});
      expect(changed, persisted);
    });

    test('confirms every clipboard read even after a stored grant', () async {
      var confirmations = 0;
      final broker = CapabilityBroker(
        requestGrant: (context, capability, params) async {
          confirmations++;
          return PermissionGrant(
            instanceId: context.instanceId,
            versionId: context.versionId,
            capability: capability,
          );
        },
      );
      broker
        ..register('clipboard.read', (_, _) async => {'text': 'value'})
        ..replaceGrants({
          const PermissionGrant(
            instanceId: 'instance-1',
            versionId: 'version-1',
            capability: 'clipboard.read',
          ),
        });
      final context = _context(
        declared: const {'clipboard.read'},
        userGesture: true,
      );

      await broker.invoke(context, 'clipboard.read', const {});
      await broker.invoke(context, 'clipboard.read', const {});

      expect(confirmations, 2);
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
