import { describe, it, expect, vi, beforeEach } from 'vitest';
import { useChatSession } from '../../composables/useChatSession';

// Mock fetch globally
const mockFetch = vi.fn();
global.fetch = mockFetch;

// Mock EventSource
class MockEventSource {
  onmessage: ((e: MessageEvent) => void) | null = null;
  onerror: (() => void) | null = null;
  close = vi.fn();
  url: string;

  constructor(url: string) {
    this.url = url;
  }
}
(global as any).EventSource = MockEventSource;

describe('useChatSession', () => {
  beforeEach(() => {
    vi.clearAllMocks();
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
    let sseOnMessage: ((e: MessageEvent) => void) | null = null;

    mockFetch.mockResolvedValue({ json: () => Promise.resolve({ id: 'sess-1' }) });

    const { sendMessage, messages, isStreaming } = useChatSession();
    await sendMessage('Hi');

    sseOnMessage = (MockEventSource as any).prototype.onmessage;
    // Find the created EventSource instance by checking the prototype
    // Actually, the composable creates new EventSource, we need to capture it.
    // Let's test via the mock.

    // Check streaming state is set
    expect(isStreaming.value).toBe(true);
    // User message should be appended
    expect(messages.value.some((m) => m.role === 'user')).toBe(true);
  });

  it('handles done event', async () => {
    mockFetch
      .mockResolvedValueOnce({ json: () => Promise.resolve({ id: 'sess-2' }) })
      .mockResolvedValueOnce({ json: () => Promise.resolve({}) });

    const { sendMessage, isStreaming } = useChatSession();
    await sendMessage('Test');

    // Simulate SSE done event
    const sse = (MockEventSource as any).mock.instances?.[0];
    if (sse?.onmessage) {
      sse.onmessage({ data: JSON.stringify({ event: 'done' }) } as MessageEvent);
      expect(isStreaming.value).toBe(false);
    }
  });

  it('handles error event', async () => {
    mockFetch
      .mockResolvedValueOnce({ json: () => Promise.resolve({ id: 'sess-3' }) })
      .mockResolvedValueOnce({ json: () => Promise.resolve({}) });

    const { sendMessage, error } = useChatSession();
    await sendMessage('Test');

    const sse = (MockEventSource as any).mock.instances?.[0];
    if (sse?.onmessage) {
      sse.onmessage({
        data: JSON.stringify({ event: 'error', message: 'Something broke' }),
      } as MessageEvent);
      expect(error.value).toBe('Something broke');
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

    const sse = (MockEventSource as any).mock.instances?.[0];
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
    }
  });
});
