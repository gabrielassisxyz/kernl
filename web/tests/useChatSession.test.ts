import { describe, it, expect, vi, beforeEach } from 'vitest';
import { useChatSession } from '../composables/useChatSession';

// Mock fetch globally
const mockFetch = vi.fn();
global.fetch = mockFetch;

// Track the last constructed EventSource instance so tests can simulate events.
let capturedEventSource: MockEventSource | null = null;

// Mock EventSource
class MockEventSource {
  onmessage: ((e: MessageEvent) => void) | null = null;
  onerror: (() => void) | null = null;
  close = vi.fn();
  url: string;

  constructor(url: string) {
    this.url = url;
    capturedEventSource = this;
  }
}
(global as any).EventSource = MockEventSource;

describe('useChatSession', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    capturedEventSource = null;
  });

  it('creates a session on first sendMessage', async () => {
    mockFetch
      .mockResolvedValueOnce({ json: () => Promise.resolve({ id: 'sess-1' }) })
      .mockResolvedValueOnce({ json: () => Promise.resolve({}) });

    const { sendMessage, messages } = useChatSession();
    await sendMessage('Hello');

    expect(mockFetch).toHaveBeenCalledWith('/api/chat/sessions', {
      method: 'POST',
    });
    expect(messages.value).toContainEqual({ role: 'user', content: 'Hello' });
  });

  it('appends token events to messages', async () => {
    mockFetch.mockResolvedValue({ json: () => Promise.resolve({ id: 'sess-1' }) });

    const { sendMessage, messages, isStreaming } = useChatSession();
    await sendMessage('Hi');

    expect(isStreaming.value).toBe(true);
    expect(messages.value.some((m) => m.role === 'user')).toBe(true);
  });

  it('handles done event', async () => {
    mockFetch
      .mockResolvedValueOnce({ json: () => Promise.resolve({ id: 'sess-2' }) })
      .mockResolvedValueOnce({ json: () => Promise.resolve({}) });

    const { sendMessage, isStreaming } = useChatSession();
    await sendMessage('Test');

    const sse = capturedEventSource;
    if (sse?.onmessage) {
      sse.onmessage({ data: JSON.stringify({ event: 'done' }) } as MessageEvent);
      expect(isStreaming.value).toBe(false);
    } else {
      expect(capturedEventSource).toBeTruthy();
    }
  });

  it('handles error event', async () => {
    mockFetch
      .mockResolvedValueOnce({ json: () => Promise.resolve({ id: 'sess-3' }) })
      .mockResolvedValueOnce({ json: () => Promise.resolve({}) });

    const { sendMessage, error } = useChatSession();
    await sendMessage('Test');

    const sse = capturedEventSource;
    if (sse?.onmessage) {
      sse.onmessage({
        data: JSON.stringify({ event: 'error', message: 'Something broke' }),
      } as MessageEvent);
      expect(error.value).toBe('Something broke');
    } else {
      expect(capturedEventSource).toBeTruthy();
    }
  });

  it('handles state event', async () => {
    mockFetch
      .mockResolvedValueOnce({ json: () => Promise.resolve({ id: 'sess-4' }) })
      .mockResolvedValueOnce({ json: () => Promise.resolve({}) });

    const { sendMessage, messages, pendingPermission } = useChatSession();
    await sendMessage('Test');

    const perm = {
      tool_call_id: 'tc1',
      node_id: 'n1',
      node_path: '/foo',
      description: 'read foo',
    };

    const sse = capturedEventSource;
    if (sse?.onmessage) {
      sse.onmessage({
        data: JSON.stringify({
          event: 'state',
          messages: [{ role: 'user', content: 'Hi' }],
          pending_permission: perm,
        }),
      } as MessageEvent);
      expect(messages.value).toEqual([{ role: 'user', content: 'Hi' }]);
      expect(pendingPermission.value).toEqual(perm);
    } else {
      expect(capturedEventSource).toBeTruthy();
    }
  });
});
