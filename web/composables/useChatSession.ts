import { ref } from 'vue';
import type { RawCaptureAction } from '~/utils/inboxTargets';

export interface SSEMessage {
  content: string;
}

export interface SSEState {
  event: string;
  messages?: { role: string; content: string; learnedCandidate?: LearnedCandidate }[];
  pendingPermission?: PermissionEvent;
}

export interface PermissionEvent {
  toolCallId: string;
  nodeId: string;
  nodePath: string;
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

// A routing the DA proposed via the suggest_routing tool: what the capture
// should become. Like a diff, it is a PROPOSAL — accepting it only replaces the
// draft in the editor, and the user still processes the capture themselves.
export interface RoutingSuggestion {
  captureId: string;
  rationale: string;
  actions: RawCaptureAction[];
}

export function useChatSession() {
  const messages = ref<{ role: string; content: string; learnedCandidate?: LearnedCandidate }[]>([]);
  const pendingPermission = ref<PermissionEvent | null>(null);
  const diffSuggestion = ref<DiffSuggestion | null>(null);
  const routingSuggestion = ref<RoutingSuggestion | null>(null);
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
          if (data.pendingPermission) {
            pendingPermission.value = data.pendingPermission;
          }
          break;
        case 'permission_required':
          pendingPermission.value = {
            toolCallId: (data as unknown as PermissionEvent).toolCallId,
            nodeId: (data as unknown as PermissionEvent).nodeId,
            nodePath: (data as unknown as PermissionEvent).nodePath,
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
        case 'routing': {
          const r = data as unknown as RoutingSuggestion;
          routingSuggestion.value = { captureId: r.captureId, rationale: r.rationale || '', actions: r.actions || [] };
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

  // draftActions is the routing the user currently has on screen. It rides along
  // with the message because the LLM runs from the persisted session, not from
  // this request: a draft left in the browser never reaches the DA, which would
  // then discuss a routing the user has already edited away.
  async function sendMessage(
    content: string,
    scopeNodeId?: string,
    draftActions?: RawCaptureAction[],
  ): Promise<void> {
    await ensureSession();
    messages.value.push({ role: 'user', content });
    await fetch(`/api/chat/sessions/${sessionId}/messages`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        content,
        scope_node_id: scopeNodeId || '',
        draftActions: draftActions || [],
      }),
    });
    isStreaming.value = true;
    connectSSE();
  }

  async function resolvePermission(
    toolCallId: string,
    action: string,
    feedback?: string
  ): Promise<void> {
    const body: Record<string, string> = { toolCallId, action };
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
      if (m.learnedCandidate && m.learnedCandidate.statement === originalStatement) {
        m.learnedCandidate = undefined;
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
      if (m.learnedCandidate && m.learnedCandidate.statement === statement) {
        m.learnedCandidate = undefined;
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

  function dismissRouting(): void {
    routingSuggestion.value = null;
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
    routingSuggestion.value = null;
    error.value = null;
    isStreaming.value = false;
  }

  return {
    messages,
    pendingPermission,
    diffSuggestion,
    routingSuggestion,
    error,
    isStreaming,
    sendMessage,
    resolvePermission,
    keepCandidate,
    discardCandidate,
    applyDiff,
    dismissDiff,
    dismissRouting,
    loadSession,
    newConversation,
  };
}
