export type AgentCardRpcMethod =
  | 'runtime.getContext'
  | 'storage.get'
  | 'storage.set'
  | 'storage.delete'
  | 'storage.list'
  | 'notification.show'
  | 'clipboard.write'
  | 'clipboard.read'
  | 'host.openExternal'
  | 'network.fetch'
  | 'system.metrics.get'
  | 'system.metrics.subscribe'
  | 'system.metrics.unsubscribe'
  | 'window.getState'
  | 'window.detach'
  | 'window.dock'
  | 'window.setAlwaysOnTop'
  | 'window.requestAttention';

export type AgentCardEvent =
  | 'context.changed'
  | 'theme.changed'
  | 'online.changed'
  | 'surface.changed'
  | 'permission.changed'
  | 'system.metrics'
  | 'runtime.suspend'
  | 'runtime.resume';

export type AgentCardStableErrorCode =
  | 'INVALID_PARAMS'
  | 'PERMISSION_REQUIRED'
  | 'PERMISSION_DENIED'
  | 'CAPABILITY_UNAVAILABLE'
  | 'OFFLINE'
  | 'RATE_LIMITED'
  | 'TIMEOUT'
  | 'SESSION_EXPIRED'
  | 'INTERNAL';

export interface AgentCardContext {
  instanceId: string;
  cardId: string;
  versionId: string;
  locale: string;
  theme: 'light' | 'dark' | 'system';
  surface: 'workspace' | 'overlay' | 'detached';
  online: boolean;
}

interface BootstrapRuntime {
  getContext(): Promise<AgentCardContext>;
  invoke(method: AgentCardRpcMethod, params?: Record<string, unknown>): Promise<unknown>;
  subscribe(event: AgentCardEvent, handler: (payload: unknown) => void): () => void;
}

declare global {
  interface Window {
    agentCard?: BootstrapRuntime;
  }
}

export class AgentCardRpcError extends Error {
  constructor(
    public readonly code: AgentCardStableErrorCode | 'UNKNOWN',
    message: string,
  ) {
    super(message);
    this.name = 'AgentCardRpcError';
  }
}

function runtime(): BootstrapRuntime {
  if (typeof window === 'undefined' || !window.agentCard) {
    throw new AgentCardRpcError('SESSION_EXPIRED', 'AgentCard runtime is unavailable');
  }
  return window.agentCard;
}

async function invoke<T>(
  method: AgentCardRpcMethod,
  params: Record<string, unknown> = {},
): Promise<T> {
  try {
    return (await runtime().invoke(method, params)) as T;
  } catch (error) {
    if (error instanceof AgentCardRpcError) throw error;
    const candidate = error as { code?: unknown; message?: unknown };
    const code = typeof candidate?.code === 'string'
      ? candidate.code as AgentCardStableErrorCode
      : 'UNKNOWN';
    const message = typeof candidate?.message === 'string' ? candidate.message : 'Runtime invocation failed';
    throw new AgentCardRpcError(code, message);
  }
}

export const agentCard = Object.freeze({
  async getContext(): Promise<AgentCardContext> {
    try {
      return await runtime().getContext();
    } catch (error) {
      if (error instanceof AgentCardRpcError) throw error;
      throw new AgentCardRpcError('SESSION_EXPIRED', 'AgentCard runtime is unavailable');
    }
  },
  invoke,
  subscribe(event: AgentCardEvent, handler: (payload: unknown) => void): () => void {
    return runtime().subscribe(event, handler);
  },
  storage: Object.freeze({
    get(key: string): Promise<{ value: unknown }> {
      return invoke('storage.get', { key });
    },
    set(key: string, value: unknown): Promise<{ stored: boolean }> {
      return invoke('storage.set', { key, value });
    },
    delete(key: string): Promise<{ deleted: boolean }> {
      return invoke('storage.delete', { key });
    },
    list(): Promise<{ keys: string[] }> {
      return invoke('storage.list');
    },
  }),
});
