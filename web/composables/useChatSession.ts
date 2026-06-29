import { ref } from 'vue';

export interface SSEMessage {
  content: string;
}

export interface SSEState {
  event: string;
  messages?: { role: string; content: string }[];
  pending_permission?: PermissionEvent;
}

export interface PermissionEvent {
  tool_call_id: string;
  node_id: string;
  node_path: string;
  description: string;
}

export interface LearnedCandidate {
  subject: string;
  statement: string;
}

export function useChatSession() {
  const messages = ref<{ role: string; content: string }[]>([]);
  const pendingPermission = ref<PermissionEvent | null>(null);
  const learnedCandidate = ref<LearnedCandidate | null>(null);
  const error = ref<string | null>(null);
  const isStreaming = ref(false);

  let sessionId: string | null = null;
  let eventSource: EventSource | null = null;

  function ensureSession(): Promise<void> {
    if (sessionId) return Promise.resolve();
    return fetch('/api/chat/sessions', { method: 'POST' })
      .then((r) => r.json())
      .then((data) => {
        sessionId = data.id;
      });
  }

  function connectSSE(): void {
    if (eventSource) eventSource.close();
    if (!sessionId) return;

    eventSource = new EventSource(`/api/chat/sessions/${sessionId}/events`);
    eventSource.onmessage = (e: MessageEvent) => {
      const data: SSEState = JSON.parse(e.data);
      switch (data.event) {
        case 'token':
          messages.value.push({ role: 'assistant', content: data.content || '' });
          break;
        case 'state':
          messages.value = (data.messages || []).map((m) => ({ ...m }));
          if (data.pending_permission) {
            pendingPermission.value = data.pending_permission;
          }
          break;
        case 'permission_required':
          pendingPermission.value = {
            tool_call_id: (data as unknown as PermissionEvent).tool_call_id,
            node_id: (data as unknown as PermissionEvent).node_id,
            node_path: (data as unknown as PermissionEvent).node_path,
            description: (data as unknown as PermissionEvent).description,
          };
          break;
        case 'learned_candidate':
          learnedCandidate.value = {
            subject: (data as unknown as LearnedCandidate).subject || '',
            statement: (data as unknown as LearnedCandidate).statement || '',
          };
          break;
        case 'done':
          eventSource?.close();
          eventSource = null;
          isStreaming.value = false;
          break;
        case 'error':
          error.value = (data as unknown as { message: string }).message || 'Unknown error';
          eventSource?.close();
          eventSource = null;
          isStreaming.value = false;
          break;
      }
    };
    eventSource.onerror = () => {
      eventSource?.close();
      eventSource = null;
      isStreaming.value = false;
    };
  }

  async function sendMessage(content: string, scopeNodeId?: string): Promise<void> {
    await ensureSession();
    messages.value.push({ role: 'user', content });
    await fetch(`/api/chat/sessions/${sessionId}/messages`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ content, scope_node_id: scopeNodeId || '' }),
    });
    isStreaming.value = true;
    connectSSE();
  }

  async function resolvePermission(
    toolCallId: string,
    action: string,
    feedback?: string
  ): Promise<void> {
    const body: Record<string, string> = { tool_call_id: toolCallId, action };
    if (feedback) body.feedback = feedback;
    await fetch(`/api/chat/sessions/${sessionId}/resolve-permission`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
    pendingPermission.value = null;
    connectSSE();
  }

  async function keepCandidate(statement: string): Promise<void> {
    const candidate = learnedCandidate.value;
    if (!candidate || !sessionId) return;
    learnedCandidate.value = null;
    await fetch(`/api/chat/sessions/${sessionId}/learned`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ action: 'keep', subject: candidate.subject, statement }),
    });
  }

  async function discardCandidate(): Promise<void> {
    const candidate = learnedCandidate.value;
    if (!candidate || !sessionId) return;
    learnedCandidate.value = null;
    await fetch(`/api/chat/sessions/${sessionId}/learned`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ action: 'discard', statement: candidate.statement }),
    });
  }

  function loadSession(): void {
    ensureSession().then(connectSSE);
  }

  return {
    messages,
    pendingPermission,
    learnedCandidate,
    error,
    isStreaming,
    sendMessage,
    resolvePermission,
    keepCandidate,
    discardCandidate,
    loadSession,
  };
}
