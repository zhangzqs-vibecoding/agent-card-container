import '../capabilities/capability.dart';
import '../capabilities/capability_broker.dart';
import 'local_runtime_server.dart';

class RuntimeCapabilityAdapter {
  const RuntimeCapabilityAdapter({
    required this.broker,
    required this.cardContext,
  });

  final CapabilityBroker broker;
  final CardContext cardContext;

  Future<Object?> handle(
    RuntimeRpcContext runtimeContext,
    String method,
    Map<String, Object?> params,
  ) async {
    if (runtimeContext.instanceId != cardContext.instanceId ||
        runtimeContext.cardId != cardContext.cardId ||
        runtimeContext.versionId != cardContext.versionId) {
      throw const RuntimeRpcException(
        'SESSION_EXPIRED',
        'Runtime session identity does not match the card instance',
      );
    }
    try {
      return await broker.invoke(cardContext, method, params);
    } on CapabilityException catch (error) {
      throw RuntimeRpcException(_stableCode(error.code), error.message);
    }
  }
}

String _stableCode(CapabilityErrorCode code) {
  return switch (code) {
    CapabilityErrorCode.invalidParams => 'INVALID_PARAMS',
    CapabilityErrorCode.permissionRequired => 'PERMISSION_REQUIRED',
    CapabilityErrorCode.permissionDenied => 'PERMISSION_DENIED',
    CapabilityErrorCode.capabilityUnavailable => 'CAPABILITY_UNAVAILABLE',
    CapabilityErrorCode.offline => 'OFFLINE',
    CapabilityErrorCode.rateLimited => 'RATE_LIMITED',
    CapabilityErrorCode.timeout => 'TIMEOUT',
    CapabilityErrorCode.sessionExpired => 'SESSION_EXPIRED',
    CapabilityErrorCode.internal => 'INTERNAL',
  };
}
