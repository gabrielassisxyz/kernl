(function () {
  const messagesEl = document.getElementById('messages');
  const inputEl = document.getElementById('message-input');
  const sendBtn = document.getElementById('send-btn');
  const banner = document.getElementById('permission-banner');
  const permDesc = document.getElementById('permission-desc');

  let sessionId = localStorage.getItem('chatSessionId') || '';
  let eventSource = null;
  let pendingPermission = null;

  async function ensureSession() {
    if (sessionId) return;
    const res = await fetch('/api/chat/sessions', { method: 'POST' });
    const data = await res.json();
    sessionId = data.id;
    localStorage.setItem('chatSessionId', sessionId);
  }

  function appendMessage(role, content) {
    const div = document.createElement('div');
    div.className = 'msg ' + role;
    div.textContent = content;
    messagesEl.appendChild(div);
    messagesEl.scrollTop = messagesEl.scrollHeight;
  }

  function connectSSE() {
    if (eventSource) { eventSource.close(); }
    eventSource = new EventSource('/api/chat/sessions/' + sessionId + '/events');
    eventSource.onmessage = (e) => {
      const data = JSON.parse(e.data);
      if (data.event === 'token') {
        appendMessage('assistant', data.content);
      } else if (data.event === 'done') {
        eventSource.close();
        eventSource = null;
      } else if (data.event === 'error') {
        appendMessage('error', data.message);
        eventSource.close();
        eventSource = null;
      } else if (data.event === 'state') {
        messagesEl.innerHTML = '';
        (data.messages || []).forEach(m => appendMessage(m.role, m.content));
        if (data.pending_permission) {
          showPermission(data.pending_permission);
        }
      } else if (data.event === 'permission_required') {
        showPermission(data);
      }
    };
    eventSource.onerror = () => {
      eventSource.close();
      eventSource = null;
    };
  }

  function showPermission(pp) {
    pendingPermission = pp;
    banner.classList.remove('hidden');
    permDesc.textContent = 'Agent wants to read: ' + (pp.node_path || pp.node_id);
  }

  function hidePermission() {
    pendingPermission = null;
    banner.classList.add('hidden');
  }

  async function sendMessage() {
    await ensureSession();
    const content = inputEl.value.trim();
    if (!content) return;
    const scopeNodeId = window.scopeSelectorValue || '';
    appendMessage('user', content);
    inputEl.value = '';
    await fetch('/api/chat/sessions/' + sessionId + '/messages', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ content, scope_node_id: scopeNodeId })
    });
    connectSSE();
  }

  async function resolvePermission(action, feedback) {
    if (!pendingPermission) return;
    const body = { tool_call_id: pendingPermission.tool_call_id, action };
    if (feedback) body.feedback = feedback;
    await fetch('/api/chat/sessions/' + sessionId + '/resolve-permission', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body)
    });
    hidePermission();
    connectSSE();
  }

  sendBtn.addEventListener('click', sendMessage);
  inputEl.addEventListener('keydown', (e) => { if (e.key === 'Enter') sendMessage(); });

  document.getElementById('btn-approve').addEventListener('click', () => resolvePermission('approve'));
  document.getElementById('btn-deny').addEventListener('click', () => resolvePermission('deny'));
  document.getElementById('btn-deny-feedback').addEventListener('click', () => resolvePermission('deny_with_feedback', 'User indicated this is not relevant.'));

  ensureSession().then(connectSSE);
})();
