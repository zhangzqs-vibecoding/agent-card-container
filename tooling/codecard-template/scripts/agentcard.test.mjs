import { afterEach, describe, expect, it, vi } from 'vitest';
import { agentCard, AgentCardRpcError } from '../src/agentcard.ts';

const invoke = vi.fn();
const getContext = vi.fn();
const subscribe = vi.fn();

Object.defineProperty(globalThis, 'window', {
  value: {},
  configurable: true,
});

afterEach(() => {
  invoke.mockReset();
  getContext.mockReset();
  subscribe.mockReset();
  Reflect.deleteProperty(window, 'agentCard');
});

describe('agentCard SDK', () => {
  it('delegates only typed local runtime methods', async () => {
    window.agentCard = { invoke, getContext, subscribe };
    invoke.mockResolvedValue({ stored: true });

    await expect(agentCard.storage.set('note', { text: 'offline' })).resolves.toEqual({ stored: true });
    expect(invoke).toHaveBeenCalledWith('storage.set', { key: 'note', value: { text: 'offline' } });
  });

  it('fails closed when the trusted bootstrap is unavailable', async () => {
    await expect(agentCard.getContext()).rejects.toThrow('AgentCard runtime is unavailable');
  });

  it('keeps stable RPC error codes', async () => {
    window.agentCard = { invoke, getContext, subscribe };
    const failure = Object.assign(new Error('denied'), { code: 'PERMISSION_DENIED' });
    invoke.mockRejectedValue(failure);

    await expect(agentCard.invoke('clipboard.write', { text: 'x' })).rejects.toEqual(
      new AgentCardRpcError('PERMISSION_DENIED', 'denied'),
    );
  });
});
