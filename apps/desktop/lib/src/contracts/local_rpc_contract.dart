abstract final class LocalRpcContract {
  static const version = 1;

  static const methods = {
    'runtime.getContext',
    'storage.get',
    'storage.set',
    'storage.delete',
    'storage.list',
    'notification.show',
    'clipboard.write',
    'clipboard.read',
    'host.openExternal',
    'network.fetch',
    'system.metrics.get',
    'system.metrics.subscribe',
    'system.metrics.unsubscribe',
    'window.getState',
    'window.detach',
    'window.dock',
    'window.setAlwaysOnTop',
    'window.requestAttention',
  };

  static const events = {
    'context.changed',
    'theme.changed',
    'online.changed',
    'surface.changed',
    'permission.changed',
    'system.metrics',
    'runtime.suspend',
    'runtime.resume',
  };

  static const stableErrors = {
    'INVALID_PARAMS',
    'PERMISSION_REQUIRED',
    'PERMISSION_DENIED',
    'CAPABILITY_UNAVAILABLE',
    'OFFLINE',
    'RATE_LIMITED',
    'TIMEOUT',
    'SESSION_EXPIRED',
    'INTERNAL',
  };
}
