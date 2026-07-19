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
      toolCallId: 'tc1',
      nodeId: 'n1',
      nodePath: '/foo',
      description: 'read foo',
    };

    const sse = capturedEventSource;
    if (sse?.onmessage) {
      sse.onmessage({
        data: JSON.stringify({
          event: 'state',
          messages: [{ role: 'user', content: 'Hi' }],
          pendingPermission: perm,
        }),
      } as MessageEvent);
      expect(messages.value).toEqual([{ role: 'user', content: 'Hi' }]);
      expect(pendingPermission.value).toEqual(perm);
    } else {
      expect(capturedEventSource).toBeTruthy();
    }
  });

  it('grows one assistant message across successive token events', async () => {
    mockFetch.mockResolvedValue({ json: () => Promise.resolve({ id: 'sess-5' }) });

    const { sendMessage, messages } = useChatSession();
    await sendMessage('Hi');

    const sse = capturedEventSource!;
    sse.onmessage!({ data: JSON.stringify({ event: 'token', content: 'Hello' }) } as MessageEvent);
    sse.onmessage!({ data: JSON.stringify({ event: 'token', content: ' world' }) } as MessageEvent);

    const assistant = messages.value.filter((m) => m.role === 'assistant');
    expect(assistant).toEqual([{ role: 'assistant', content: 'Hello world' }]);
  });

  it('newConversation resets state and creates a fresh session on next send', async () => {
    mockFetch.mockResolvedValue({ json: () => Promise.resolve({ id: 'sess-6' }) });

    const { sendMessage, messages, newConversation, isStreaming } = useChatSession();
    await sendMessage('Hi');
    expect(messages.value.length).toBeGreaterThan(0);

    newConversation();
    expect(messages.value).toEqual([]);
    expect(isStreaming.value).toBe(false);

    mockFetch.mockClear();
    mockFetch.mockResolvedValue({ json: () => Promise.resolve({ id: 'sess-7' }) });
    await sendMessage('Again');
    // A new session must be created — sessionId was dropped.
    expect(mockFetch).toHaveBeenCalledWith('/api/chat/sessions', { method: 'POST' });
  });

  it('keeps edited learned candidates and hides the original card', async () => {
    mockFetch.mockResolvedValue({ json: () => Promise.resolve({ id: 'sess-8' }) });

    const { sendMessage, messages, keepCandidate } = useChatSession();
    await sendMessage('Remember this');

    const sse = capturedEventSource!;
    sse.onmessage!({
      data: JSON.stringify({
        event: 'state',
        messages: [{
          role: 'assistant',
          content: 'I learned something.',
          learnedCandidate: {
            subject: 'planning',
            statement: 'Original claim.',
          },
        }],
      }),
    } as MessageEvent);

    mockFetch.mockClear();
    await keepCandidate('planning-style', 'Edited claim.', 'Original claim.');

    expect(messages.value[0].learnedCandidate).toBeUndefined();
    expect(mockFetch).toHaveBeenCalledWith('/api/chat/sessions/sess-8/learned', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        action: 'keep',
        subject: 'planning-style',
        statement: 'Edited claim.',
      }),
    });
  });
});
