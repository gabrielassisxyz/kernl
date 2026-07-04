import { ref } from 'vue';

export interface SSEMessage {
  content: string;
}

export interface SSEState {
  event: string;
  messages?: { role: string; content: string; learned_candidate?: LearnedCandidate }[];
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

export interface SuggestHunk {
  id: string;
  from: number;
  to: number;
  content: string;
}

// A note edit the DA proposed via the suggest_note_edit tool, awaiting the
// user's accept/reject.
export interface DiffSuggestion {
  noteId: string;
  notePath: string;
  hunks: SuggestHunk[];
}

export function useChatSession() {
  const messages = ref<{ role: string; content: string; learned_candidate?: LearnedCandidate }[]>([]);
  const pendingPermission = ref<PermissionEvent | null>(null);
  const diffSuggestion = ref<DiffSuggestion | null>(null);
  const error = ref<string | null>(null);
  const isStreaming = ref(false);

  let sessionId: string | null = null;
  let eventSource: EventSource | null = null;
  // Index of the assistant message currently being streamed, so successive
  // token events grow ONE message instead of pushing one message per token.
  let streamingIndex: number | null = null;

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
    streamingIndex = null;

    eventSource = new EventSource(`/api/chat/sessions/${sessionId}/events`);
    eventSource.onmessage = (e: MessageEvent) => {
      const data: SSEState = JSON.parse(e.data);
      switch (data.event) {
        case 'token':
          if (streamingIndex === null) {
            messages.value.push({ role: 'assistant', content: data.content || '' });
            streamingIndex = messages.value.length - 1;
          } else {
            messages.value[streamingIndex].content += data.content || '';
          }
          break;
        case 'state':
          // Authoritative replace: the server persists assistant turns too, so
          // the state snapshot is the full transcript.
          messages.value = (data.messages || []).map((m) => ({ ...m }));
          streamingIndex = null;
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
        case 'assistant_done':
          isStreaming.value = false;
          break;
        case 'diff': {
          const d = data as unknown as { noteId: string; notePath: string; hunks: SuggestHunk[] };
          diffSuggestion.value = { noteId: d.noteId, notePath: d.notePath, hunks: d.hunks || [] };
          break;
        }
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

  async function keepCandidate(subject: string, statement: string, originalStatement = statement): Promise<void> {
    if (!sessionId) return;
    const finalSubject = subject.trim();
    const finalStatement = statement.trim();
    if (!finalSubject || !finalStatement) return;

    // Optimistic UI update: hide the card.
    messages.value.forEach(m => {
      if (m.learned_candidate && m.learned_candidate.statement === originalStatement) {
        m.learned_candidate = undefined;
      }
    });
    await fetch(`/api/chat/sessions/${sessionId}/learned`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ action: 'keep', subject: finalSubject, statement: finalStatement }),
    });
  }

  async function discardCandidate(statement: string): Promise<void> {
    if (!sessionId) return;
    // Optimistic UI update: hide the card
    messages.value.forEach(m => {
      if (m.learned_candidate && m.learned_candidate.statement === statement) {
        m.learned_candidate = undefined;
      }
    });
    await fetch(`/api/chat/sessions/${sessionId}/learned`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ action: 'discard', statement }),
    });
  }

  function loadSession(): void {
    ensureSession().then(connectSSE);
  }

  // Apply the accepted hunks of a DA-proposed note edit. The DA only ever
  // proposes; this user-initiated call is what actually writes the file.
  async function applyDiff(acceptedHunks: SuggestHunk[]): Promise<void> {
    const suggestion = diffSuggestion.value;
    if (!suggestion) return;
    diffSuggestion.value = null;
    if (acceptedHunks.length === 0) return;
    await fetch('/api/notes/apply-hunks', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ path: suggestion.notePath, hunks: acceptedHunks }),
    });
  }

  function dismissDiff(): void {
    diffSuggestion.value = null;
  }

  // Drop the current session client-side; the next send creates a fresh one.
  // The old session node stays in the graph (no delete endpoint yet).
  function newConversation(): void {
    eventSource?.close();
    eventSource = null;
    sessionId = null;
    streamingIndex = null;
    messages.value = [];
    pendingPermission.value = null;
    diffSuggestion.value = null;
    error.value = null;
    isStreaming.value = false;
  }

  return {
    messages,
    pendingPermission,
    diffSuggestion,
    error,
    isStreaming,
    sendMessage,
    resolvePermission,
    keepCandidate,
    discardCandidate,
    applyDiff,
    dismissDiff,
    loadSession,
    newConversation,
  };
}
