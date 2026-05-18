(function () {
  // Visual category palette. Categories — not raw states — keep the mapping
  // small and tolerant to workflow vocabulary changes (queued/active/review
  // are the universal shapes; the workflow defines the verb).
  var CATEGORY_STYLE = {
    queued:    { color: '#6b7280' }, // gray — agent-claimable, waiting
    active:    { color: '#3b82f6' }, // blue — agent working
    review:    { color: '#f59e0b' }, // amber — under review
    gate:      { color: '#ec4899' }, // pink — human/merger handoff
    done:      { color: '#10b981' }, // green — terminal success
    blocked:   { color: '#ef4444' }, // red — blocked / abandoned
    unknown:   { color: '#9ca3af' }
  };

  // Map a raw workflow state to one of the visual categories above.
  // Lives in JS (not Go-shared) because color choices are pure UI concerns.
  function classifyState(state) {
    if (!state) return 'unknown';
    var s = String(state).toLowerCase();
    if (s === 'closed' || s === 'shipped' || s === 'done')         return 'done';
    if (s === 'blocked' || s === 'abandoned' || s === 'cancelled' ||
        s === 'rejected' || s === 'deferred')                       return 'blocked';
    if (s === 'awaiting_integration' || s === 'awaiting_pr_review') return 'gate';
    if (s.indexOf('_review') !== -1)                                return 'review';
    if (s.indexOf('ready_for_') === 0 || s === 'open' || s === 'queue') return 'queued';
    return 'active'; // planning / implementation / shipment / in_progress
  }

  var el = {
    status:    document.getElementById('connection-status'),
    input:     document.getElementById('epic-id-input'),
    btn:       document.getElementById('connect-btn'),
    beadList:  document.getElementById('bead-list'),
    sessList:  document.getElementById('session-list'),
    errList:   document.getElementById('error-list'),
    logPanel:  document.getElementById('log-panel'),
    logTitle:  document.getElementById('log-title'),
    logClose:  document.getElementById('log-close'),
    logStatus: document.getElementById('log-status'),
    logStream: document.getElementById('log-stream'),
    nudgeGenericBtn:  document.getElementById('nudge-generic-btn'),
    nudgeStatusBtn:   document.getElementById('nudge-status-btn'),
    nudgeComposer:    document.getElementById('nudge-composer'),
    nudgeLabel:       document.getElementById('nudge-composer-label'),
    nudgeText:        document.getElementById('nudge-composer-text'),
    nudgeSend:        document.getElementById('nudge-send-btn'),
    nudgeCancel:      document.getElementById('nudge-cancel-btn'),
    nudgeStatusText:  document.getElementById('nudge-composer-status')
  };

  var source = null;
  var eventBuffer = [];
  var beads = {};
  var sessions = {};
  var errors = [];
  var pollTimer = null;
  var pollInterval = 2000;

  // Agent-log panel state
  var selectedBeadId = null;
  var logSource = null;
  var logSessionId = null;
  var logAutoScroll = true;
  var logToolEls = {};   // tool_use_id → DOM element (Claude Code dialect)
  var logActiveToolEl = null;  // opencode dialect: most recent tool element
  var logActiveThinkingEl = null;
  var logActiveTextEl = null;

  function setStatus(cls, text) {
    el.status.className = 'status-' + cls;
    el.status.textContent = text;
  }

  // Field accessor that tolerates both camelCase (json tags from Go) and
  // PascalCase (struct field names) — events come both ways depending on
  // who emits them.
  function pick(obj /*, ...keys */) {
    for (var i = 1; i < arguments.length; i++) {
      var k = arguments[i];
      if (obj && obj[k] != null && obj[k] !== '') return obj[k];
    }
    return '';
  }

  function connectEpic(epicId) {
    disconnect();
    setStatus('connecting', 'connecting…');
    var sseUrl = '/api/epics/' + encodeURIComponent(epicId) + '/events';
    source = new EventSource(sseUrl);
    source.onopen = function () {
      setStatus('connected', 'connected · ' + epicId);
    };
    source.onmessage = function (e) {
      try {
        var data = JSON.parse(e.data);
        handleEvent(data);
      } catch (_) { /* ignore malformed */ }
    };
    source.onerror = function () {
      setStatus('disconnected', 'disconnected');
      if (source) source.close();
      source = null;
      startPolling();
    };
  }

  function disconnect() {
    if (source) {
      source.close();
      source = null;
    }
    stopPolling();
  }

  function startPolling() {
    if (pollTimer) return;
    pollTimer = setInterval(function () {
      fetch('/api/beads')
        .then(function (r) { return r.json(); })
        .then(function (list) {
          beads = {};
          (list || []).forEach(function (b) {
            var id = pick(b, 'id', 'ID');
            if (id) beads[id] = b;
          });
          render();
        })
        .catch(function () {});
    }, pollInterval);
  }

  function stopPolling() {
    if (pollTimer) {
      clearInterval(pollTimer);
      pollTimer = null;
    }
  }

  function handleEvent(evt) {
    eventBuffer.push(evt);
    if (eventBuffer.length > 500) eventBuffer.shift();

    var bid = pick(evt, 'beadID', 'BeadID', 'beadId');
    if (bid) {
      var newState = pick(evt, 'newState', 'NewState', 'state', 'State', 'detail', 'Detail');
      var title    = pick(evt, 'title', 'Title');
      if (!beads[bid]) {
        beads[bid] = { id: bid, title: bid, state: newState || 'open' };
      }
      if (newState) beads[bid].state = newState;
      if (title)    beads[bid].title = title;
      // Track the wall-clock of the most recent change so the card can
      // show "10s ago" / "2m ago" — helps tell stalled stages from
      // actively-progressing ones.
      beads[bid].__updated = new Date().toLocaleTimeString();
    }

    var sid = pick(evt, 'sessionID', 'SessionID', 'sessionId');
    if (sid) {
      sessions[sid] = {
        id: sid,
        bead:    pick(evt, 'beadID', 'BeadID', 'beadId'),
        agent:   pick(evt, 'agent', 'Agent'),
        started: pick(evt, 'time', 'Time') || new Date().toISOString()
      };
    }

    var type = pick(evt, 'type', 'Type');
    var errVal = pick(evt, 'error', 'Error');
    if (type === 'SessionError' || type === 'session-error' || errVal) {
      var errMsg = errVal || pick(evt, 'detail', 'Detail') || type || 'unknown error';
      errors.push({
        time:    pick(evt, 'time', 'Time') || new Date().toISOString(),
        bead:    pick(evt, 'beadID', 'BeadID', 'beadId'),
        message: errMsg
      });
      if (errors.length > 50) errors.shift();
    }

    render();
  }

  function render() {
    renderBeads();
    renderSessions();
    renderErrors();
  }

  // Render order for category sections — top of list = most urgent to watch.
  var CATEGORY_ORDER = ['active', 'review', 'gate', 'queued', 'blocked', 'done', 'unknown'];
  var CATEGORY_LABEL = {
    active:  'Active (agent working)',
    review:  'Under review',
    gate:    'Human / merger gate',
    queued:  'Queued',
    blocked: 'Blocked',
    done:    'Done',
    unknown: 'Unknown state'
  };

  function renderBeads() {
    var ids = Object.keys(beads);
    if (!ids.length) {
      el.beadList.innerHTML = '<div class="empty">no beads</div>';
      return;
    }

    // Bucket by category so reading the panel scales past 3 beads.
    var groups = {};
    ids.sort().forEach(function (id) {
      var b     = beads[id];
      var state = pick(b, 'state', 'State') || 'unknown';
      var cat   = classifyState(state);
      (groups[cat] = groups[cat] || []).push({ id: id, state: state, title: pick(b, 'title', 'Title'), updated: b.__updated });
    });

    var html = '';
    CATEGORY_ORDER.forEach(function (cat) {
      var bucket = groups[cat];
      if (!bucket || !bucket.length) return;
      var style = CATEGORY_STYLE[cat] || CATEGORY_STYLE.unknown;
      html += '<div class="bead-group">'
        + '<h3 class="bead-group-header" style="border-left: 4px solid ' + style.color + ';color:' + style.color + '">'
        + esc(CATEGORY_LABEL[cat] || cat) + ' <span class="bead-group-count">' + bucket.length + '</span>'
        + '</h3>';
      bucket.forEach(function (b) {
        var when = b.updated ? ' · <span class="bead-updated">' + esc(b.updated) + '</span>' : '';
        var selectedClass = (b.id === selectedBeadId) ? ' selected' : '';
        html += '<div class="bead-card' + selectedClass + '" data-bead-id="' + esc(b.id) + '" style="border-left: 4px solid ' + style.color + '">'
          + '<span class="bead-id">' + esc(b.id) + '</span>'
          + '<span class="bead-state" style="color:' + style.color + '" title="' + esc(cat) + '">' + esc(b.state) + '</span>'
          + '<span class="bead-title">' + esc(b.title) + '</span>'
          + when
          + '</div>';
      });
      html += '</div>';
    });
    el.beadList.innerHTML = html;
  }

  function renderSessions() {
    var ids = Object.keys(sessions);
    if (!ids.length) {
      el.sessList.innerHTML = '<div class="empty">no sessions</div>';
      return;
    }
    var html = '';
    ids.sort().forEach(function (id) {
      var s = sessions[id];
      html += '<div class="session-card">'
        + '<span class="session-id">' + esc(id) + '</span>'
        + '<span class="session-bead">bead: ' + esc(s.bead) + '</span>'
        + '<span class="session-agent">' + esc(s.agent) + '</span>'
        + '<a class="session-sse" href="/api/sessions/' + encodeURIComponent(id) + '/events" target="_blank">SSE</a>'
        + '</div>';
    });
    el.sessList.innerHTML = html;
  }

  function renderErrors() {
    if (!errors.length) {
      el.errList.innerHTML = '<div class="empty">no errors</div>';
      return;
    }
    var html = '';
    errors.slice().reverse().forEach(function (e) {
      html += '<div class="error-card">'
        + '<span class="error-time">' + esc(e.time) + '</span>'
        + '<span class="error-bead">bead: ' + esc(e.bead) + '</span>'
        + '<span class="error-msg">' + esc(e.message) + '</span>'
        + '</div>';
    });
    el.errList.innerHTML = html;
  }

  function esc(s) {
    s = '' + (s || '');
    return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
  }

  el.btn.addEventListener('click', function () {
    var epicId = el.input.value.trim();
    if (epicId) connectEpic(epicId);
  });

  el.input.addEventListener('keydown', function (e) {
    if (e.key === 'Enter') {
      var epicId = el.input.value.trim();
      if (epicId) connectEpic(epicId);
    }
  });

  // Initial bead snapshot.
  fetch('/api/beads')
    .then(function (r) { return r.json(); })
    .then(function (list) {
      (list || []).forEach(function (b) {
        var id = pick(b, 'id', 'ID');
        if (id) beads[id] = b;
      });
      render();
    })
    .catch(function () {});

  // --- Agent log panel ---------------------------------------------------

  el.beadList.addEventListener('click', function (e) {
    var card = e.target.closest && e.target.closest('.bead-card');
    if (!card) return;
    var bid = card.getAttribute('data-bead-id');
    if (!bid) return;
    openLogFor(bid);
  });

  el.logClose.addEventListener('click', function () {
    closeLog();
  });

  el.logStream.addEventListener('scroll', function () {
    // If user scrolled near bottom, keep auto-scrolling; otherwise pause it.
    var nearBottom = el.logStream.scrollHeight - el.logStream.scrollTop - el.logStream.clientHeight < 40;
    logAutoScroll = nearBottom;
  });

  function openLogFor(beadId) {
    selectedBeadId = beadId;
    render();
    el.logPanel.hidden = false;
    el.logTitle.textContent = 'Agent log · ' + beadId;
    el.logStream.innerHTML = '';
    setLogStatus('looking up session…', '');
    resetLogState();
    closeNudgeComposer();
    closeLogSource();

    var sid = findSessionIdForBead(beadId);
    if (sid) {
      subscribeLog(sid, beadId);
      return;
    }

    // Fallback: query backend for active sessions on the current epic.
    var epicId = el.input.value.trim();
    if (!epicId) {
      setLogStatus('no active session', 'error');
      return;
    }
    fetch('/api/epics/' + encodeURIComponent(epicId) + '/sessions')
      .then(function (r) { return r.json(); })
      .then(function (list) {
        var match = (list || []).find(function (s) {
          return pick(s, 'beadId', 'BeadID', 'beadID') === beadId;
        });
        if (!match) {
          setLogStatus('no active session for ' + beadId, 'error');
          return;
        }
        var matchedSid = pick(match, 'sessionId', 'SessionID', 'sessionID');
        if (!matchedSid) {
          setLogStatus('session id missing', 'error');
          return;
        }
        subscribeLog(matchedSid, beadId);
      })
      .catch(function () {
        setLogStatus('failed to look up session', 'error');
      });
  }

  function closeLog() {
    selectedBeadId = null;
    closeLogSource();
    closeNudgeComposer();
    el.logPanel.hidden = true;
    el.logStream.innerHTML = '';
    render();
  }

  function findSessionIdForBead(beadId) {
    // sessions{} is populated by the epic SSE stream as SessionStarted events arrive.
    var ids = Object.keys(sessions);
    for (var i = 0; i < ids.length; i++) {
      if (sessions[ids[i]].bead === beadId) return ids[i];
    }
    return null;
  }

  function subscribeLog(sessionId, beadId) {
    logSessionId = sessionId;
    setLogStatus('connecting to ' + sessionId + '…', '');
    var url = '/api/sessions/' + encodeURIComponent(sessionId) + '/events';
    logSource = new EventSource(url);
    logSource.onopen = function () {
      setLogStatus('live · ' + sessionId, 'live');
    };
    logSource.onmessage = function (e) {
      try {
        var outer = JSON.parse(e.data);
        handleLogEvent(outer);
      } catch (_) { /* ignore */ }
    };
    logSource.onerror = function () {
      setLogStatus('disconnected · ' + sessionId, 'error');
    };
  }

  function closeLogSource() {
    if (logSource) {
      logSource.close();
      logSource = null;
    }
    logSessionId = null;
  }

  function setLogStatus(text, cls) {
    el.logStatus.textContent = text;
    el.logStatus.className = 'log-status' + (cls ? ' ' + cls : '');
  }

  function resetLogState() {
    logAutoScroll = true;
    logToolEls = {};
    logActiveToolEl = null;
    logActiveThinkingEl = null;
    logActiveTextEl = null;
  }

  function handleLogEvent(outer) {
    var type = outer && outer.type;
    var content = outer && (outer.content || outer.data || '');

    if (type === 'stderr') {
      renderStderr(String(content));
      return;
    }
    if (type === 'exit') {
      renderExit(String(content));
      return;
    }
    if (type === 'error') {
      renderStderr('[error] ' + String(content));
      return;
    }
    if (type !== 'stdout' && type !== undefined && type !== '') {
      // Other infrastructure events (agent_failure, beat_state_observed, etc.) — ignore.
      return;
    }

    // stdout: content is an NDJSON line from the agent.
    var inner;
    try { inner = JSON.parse(content); } catch (_) {
      // Not JSON — show as raw text.
      if (String(content).trim()) renderText(String(content));
      return;
    }
    routeInnerEvent(inner);
  }

  // Route an agent-emitted inner event to the right renderer. Handles both
  // the opencode and Claude Code dialects; unknown shapes are dropped.
  function routeInnerEvent(inner) {
    if (!inner || typeof inner !== 'object') return;
    var t = inner.type;

    // --- opencode dialect ---
    if (t === 'message.part.updated' && inner.part) {
      var part = inner.part;
      if (part.type === 'text')      return renderText(part.text || '', part.id);
      if (part.type === 'reasoning') return renderThinking(part.text || '', part.id);
      if (part.type === 'tool')      return renderOpencodeTool(part);
      return;
    }
    if (t === 'session_idle' || t === 'session.idle') return renderSeparator();

    // --- Claude Code dialect ---
    if (t === 'assistant' && inner.message && Array.isArray(inner.message.content)) {
      inner.message.content.forEach(function (block) {
        if (block.type === 'text')     renderText(block.text || '');
        else if (block.type === 'thinking') renderThinking(block.thinking || block.text || '');
        else if (block.type === 'tool_use') renderClaudeToolUse(block);
      });
      return;
    }
    if (t === 'user' && inner.message && Array.isArray(inner.message.content)) {
      inner.message.content.forEach(function (block) {
        if (block.type === 'tool_result') renderClaudeToolResult(block);
      });
      return;
    }
    if (t === 'result') return renderSeparator();
  }

  function renderText(text, partId) {
    if (!text) return;
    // For opencode streaming: text parts arrive incrementally — replace body when same partId.
    if (partId && logActiveTextEl && logActiveTextEl.dataset.partId === partId) {
      logActiveTextEl.textContent = text;
    } else {
      var div = document.createElement('div');
      div.className = 'log-text';
      div.textContent = text;
      if (partId) div.dataset.partId = partId;
      el.logStream.appendChild(div);
      logActiveTextEl = div;
    }
    autoScroll();
  }

  function renderThinking(text, partId) {
    if (!text) return;
    if (partId && logActiveThinkingEl && logActiveThinkingEl.dataset.partId === partId) {
      logActiveThinkingEl.querySelector('.log-thinking-body').textContent = text;
    } else {
      var wrap = document.createElement('div');
      wrap.className = 'log-thinking';
      if (partId) wrap.dataset.partId = partId;
      var label = document.createElement('div');
      label.className = 'log-thinking-label';
      label.textContent = 'Thinking';
      var body = document.createElement('div');
      body.className = 'log-thinking-body';
      body.textContent = text;
      wrap.appendChild(label);
      wrap.appendChild(body);
      el.logStream.appendChild(wrap);
      logActiveThinkingEl = wrap;
    }
    autoScroll();
  }

  function renderOpencodeTool(part) {
    // part: { id, type:'tool', tool, state: { status, input, output, error } }
    var id = part.id || ('t-' + Math.random());
    var state = part.state || {};
    var existing = logToolEls[id];
    if (!existing) {
      existing = buildToolBlock(part.tool || 'tool');
      logToolEls[id] = existing;
      el.logStream.appendChild(existing.wrap);
      logActiveToolEl = existing;
    }
    var inputStr = stringifyToolInput(state.input);
    if (inputStr) existing.input.textContent = truncate(inputStr, 240, 3);
    var status = state.status || '';
    if (status === 'completed' || status === 'done' || status === 'success') {
      existing.statusEl.textContent = '✓';
      existing.statusEl.className = 'log-tool-status ok';
      var out = state.output != null ? String(state.output) : '';
      existing.result.textContent = '↳ ' + truncate(out, 240, 3);
      existing.result.classList.remove('err');
    } else if (status === 'error' || status === 'failed') {
      existing.statusEl.textContent = '✗';
      existing.statusEl.className = 'log-tool-status err';
      existing.result.textContent = '↳ ' + truncate(state.error || state.output || 'error', 240, 3);
      existing.result.classList.add('err');
    }
    autoScroll();
  }

  function renderClaudeToolUse(block) {
    var id = block.id || ('t-' + Math.random());
    var existing = logToolEls[id];
    if (existing) return;
    var built = buildToolBlock(block.name || 'tool');
    built.input.textContent = truncate(stringifyToolInput(block.input), 240, 3);
    logToolEls[id] = built;
    el.logStream.appendChild(built.wrap);
    autoScroll();
  }

  function renderClaudeToolResult(block) {
    var id = block.tool_use_id;
    var existing = id && logToolEls[id];
    if (!existing) return;
    var isErr = block.is_error === true;
    existing.statusEl.textContent = isErr ? '✗' : '✓';
    existing.statusEl.className = 'log-tool-status ' + (isErr ? 'err' : 'ok');
    var content = block.content;
    var text;
    if (typeof content === 'string') text = content;
    else if (Array.isArray(content)) {
      text = content.map(function (c) { return c.text || c.type || ''; }).join('\n');
    } else text = JSON.stringify(content || '');
    existing.result.textContent = '↳ ' + truncate(text, 240, 3);
    if (isErr) existing.result.classList.add('err');
    autoScroll();
  }

  function buildToolBlock(name) {
    var wrap = document.createElement('div');
    wrap.className = 'log-tool';
    var head = document.createElement('div');
    head.className = 'log-tool-head';
    var statusEl = document.createElement('span');
    statusEl.className = 'log-tool-status';
    statusEl.textContent = '▶';
    var nameEl = document.createElement('span');
    nameEl.textContent = name;
    head.appendChild(statusEl);
    head.appendChild(nameEl);
    var input = document.createElement('div');
    input.className = 'log-tool-input';
    var result = document.createElement('div');
    result.className = 'log-tool-result';
    wrap.appendChild(head);
    wrap.appendChild(input);
    wrap.appendChild(result);
    return { wrap: wrap, statusEl: statusEl, input: input, result: result };
  }

  function renderStderr(text) {
    var div = document.createElement('div');
    div.className = 'log-stderr';
    div.textContent = '[stderr] ' + text;
    el.logStream.appendChild(div);
    autoScroll();
  }

  function renderExit(text) {
    var div = document.createElement('div');
    div.className = 'log-exit';
    div.textContent = 'session exited (code ' + (text || '0') + ')';
    el.logStream.appendChild(div);
    autoScroll();
    setLogStatus('exited · ' + (logSessionId || ''), '');
  }

  function renderSeparator() {
    var hr = document.createElement('div');
    hr.className = 'log-separator';
    el.logStream.appendChild(hr);
    // Reset per-turn streaming anchors so a new turn opens a fresh text/thinking block.
    logActiveTextEl = null;
    logActiveThinkingEl = null;
    autoScroll();
  }

  function autoScroll() {
    if (logAutoScroll) el.logStream.scrollTop = el.logStream.scrollHeight;
  }

  function stringifyToolInput(input) {
    if (input == null) return '';
    if (typeof input === 'string') return input;
    try { return JSON.stringify(input); } catch (_) { return String(input); }
  }

  function truncate(s, maxChars, maxLines) {
    s = String(s || '');
    var lines = s.split('\n');
    var truncated = false;
    if (lines.length > maxLines) {
      lines = lines.slice(0, maxLines);
      truncated = true;
    }
    s = lines.join('\n');
    if (s.length > maxChars) { s = s.slice(0, maxChars); truncated = true; }
    return truncated ? s + ' …' : s;
  }

  // --- Nudge composer ----------------------------------------------------

  var nudgeActivePreset = null;

  el.nudgeGenericBtn.addEventListener('click', function () { openNudgeComposer('generic'); });
  el.nudgeStatusBtn.addEventListener('click',  function () { openNudgeComposer('advance_status'); });
  el.nudgeCancel.addEventListener('click', closeNudgeComposer);
  el.nudgeSend.addEventListener('click', sendNudge);

  function openNudgeComposer(preset) {
    if (!logSessionId) {
      setNudgeStatus('no active session for this bead', 'error');
      el.nudgeComposer.hidden = false;
      return;
    }
    nudgeActivePreset = preset;
    el.nudgeComposer.hidden = false;
    el.nudgeLabel.textContent = preset === 'advance_status'
      ? 'Status nudge — edit and send to bump the agent to advance status'
      : 'Nudge — edit and send to resume an interrupted agent';
    setNudgeStatus('loading preset…', '');
    el.nudgeText.value = '';
    el.nudgeText.disabled = true;
    el.nudgeSend.disabled = true;

    fetchNudgePrompts(logSessionId).then(function (prompts) {
      el.nudgeText.value = prompts[preset] || '';
      el.nudgeText.disabled = false;
      el.nudgeSend.disabled = false;
      el.nudgeText.focus();
      setNudgeStatus(prompts.running
        ? 'agent is currently running — nudge will be rejected (409)'
        : 'editable — send when ready', prompts.running ? 'error' : '');
    }).catch(function (e) {
      el.nudgeText.disabled = false;
      el.nudgeSend.disabled = false;
      setNudgeStatus('failed to load preset: ' + e.message, 'error');
    });
  }

  function closeNudgeComposer() {
    el.nudgeComposer.hidden = true;
    nudgeActivePreset = null;
    setNudgeStatus('', '');
  }

  function setNudgeStatus(text, cls) {
    el.nudgeStatusText.textContent = text;
    el.nudgeStatusText.className = 'nudge-composer-status' + (cls ? ' ' + cls : '');
  }

  function fetchNudgePrompts(sessionId) {
    return fetch('/api/sessions/' + encodeURIComponent(sessionId) + '/nudge-prompts')
      .then(function (r) {
        if (!r.ok) return r.json().then(function (j) { throw new Error(j.error || r.statusText); });
        return r.json();
      })
      .then(function (data) {
        return {
          generic: data.generic || '',
          advance_status: data.advance_status || '',
          running: !!data.running
        };
      });
  }

  function sendNudge() {
    if (!logSessionId || !nudgeActivePreset) return;
    var prompt = el.nudgeText.value.trim();
    if (!prompt) {
      setNudgeStatus('prompt is empty', 'error');
      return;
    }
    el.nudgeSend.disabled = true;
    setNudgeStatus('dispatching…', '');
    var body = JSON.stringify({ preset: 'custom', prompt: prompt });
    fetch('/api/sessions/' + encodeURIComponent(logSessionId) + '/nudge', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: body
    })
      .then(function (r) {
        if (r.status === 202) {
          setNudgeStatus('dispatched — watch the log', 'ok');
          // Auto-close after a brief confirmation so the live stream is unobstructed.
          setTimeout(closeNudgeComposer, 1200);
          return;
        }
        return r.json().then(function (j) {
          var msg = j.error || ('HTTP ' + r.status);
          if (r.status === 409) msg = 'agent is still running — wait for it to settle';
          if (r.status === 404) msg = 'session is unknown to the orchestrator (driver did not register it)';
          if (r.status === 422) msg = 'no opencode session id captured — cannot resume conversation';
          setNudgeStatus(msg, 'error');
          el.nudgeSend.disabled = false;
        });
      })
      .catch(function (e) {
        setNudgeStatus('network error: ' + e.message, 'error');
        el.nudgeSend.disabled = false;
      });
  }

  // Auto-connect when ?epic=<id> is in the URL. `kernl epic run` prints
  // a URL with this param so the dashboard latches onto the active run
  // without the user typing the ID by hand. Also accepts /?epic=<id> or
  // a hash fragment for paste-friendliness.
  function autoConnectFromURL() {
    var fromQuery = new URLSearchParams(window.location.search).get('epic');
    var fromHash  = window.location.hash && window.location.hash.replace(/^#\/?(epic=)?/, '');
    var epicId    = fromQuery || fromHash;
    if (!epicId) return;
    el.input.value = epicId;
    connectEpic(epicId);
  }
  autoConnectFromURL();
})();
