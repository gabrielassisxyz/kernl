(function () {
  var BEAD_STATES = {
    queue:     { label: 'queue',     color: '#6b7280' },
    active:    { label: 'active',    color: '#3b82f6' },
    review:    { label: 'review',    color: '#f59e0b' },
    approved:  { label: 'approved',  color: '#10b981' },
    rejected:  { label: 'rejected',  color: '#ef4444' },
    done:      { label: 'done',      color: '#8b5cf6' },
    cancelled: { label: 'cancelled', color: '#9ca3af' }
  };

  var el = {
    status:   document.getElementById('connection-status'),
    input:    document.getElementById('epic-id-input'),
    btn:      document.getElementById('connect-btn'),
    beadList: document.getElementById('bead-list'),
    sessList: document.getElementById('session-list'),
    errList:  document.getElementById('error-list')
  };

  var source = null;
  var eventBuffer = [];
  var beads = {};
  var sessions = {};
  var errors = [];
  var pollTimer = null;
  var pollInterval = 2000;

  function setStatus(cls, text) {
    el.status.className = 'status-' + cls;
    el.status.textContent = text;
  }

  function connectEpic(epicId) {
    disconnect();
    var sseUrl = '/api/epics/' + encodeURIComponent(epicId) + '/events';
    source = new EventSource(sseUrl);
    source.onopen = function () {
      setStatus('connected', 'connected');
    };
    source.onmessage = function (e) {
      try {
        var data = JSON.parse(e.data);
        handleEvent(data);
      } catch (_) { /* ignore malformed */ }
    };
    source.onerror = function () {
      setStatus('disconnected', 'disconnected');
      source.close();
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
          list.forEach(function (b) { beads[b.ID || b.id] = b; });
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

    if (evt.BeadID || evt.beadID) {
      var bid = evt.BeadID || evt.beadID;
      if (!beads[bid]) {
        beads[bid] = { ID: bid, Title: bid, State: evt.NewState || 'queue' };
      }
      if (evt.NewState) beads[bid].State = evt.NewState;
      if (evt.Title) beads[bid].Title = evt.Title;
    }

    if (evt.SessionID || evt.sessionID) {
      var sid = evt.SessionID || evt.sessionID;
      sessions[sid] = {
        id: sid,
        bead: evt.BeadID || evt.beadID || '',
        agent: evt.Agent || evt.agent || '',
        started: evt.Time || new Date().toISOString()
      };
    }

    if (evt.Type === 'SessionError' || evt.type === 'session-error' || evt.error || evt.Error) {
      var errMsg = evt.error || evt.Error || evt.Type || evt.type || 'unknown error';
      errors.push({
        time: evt.Time || new Date().toISOString(),
        bead: evt.BeadID || evt.beadID || '',
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

  function renderBeads() {
    var ids = Object.keys(beads);
    if (!ids.length) {
      el.beadList.innerHTML = '<div class="empty">no beads</div>';
      return;
    }
    var html = '';
    ids.sort().forEach(function (id) {
      var b = beads[id];
      var st = BEAD_STATES[b.State] || { label: b.State || 'unknown', color: '#9ca3af' };
      html += '<div class="bead-card" style="border-left: 4px solid ' + st.color + '">'
        + '<span class="bead-id">' + esc(id) + '</span>'
        + '<span class="bead-state" style="color:' + st.color + '">' + esc(st.label) + '</span>'
        + '<span class="bead-title">' + esc(b.Title || '') + '</span>'
        + '</div>';
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

  fetch('/api/beads')
    .then(function (r) { return r.json(); })
    .then(function (list) {
      list.forEach(function (b) { beads[b.ID || b.id] = b; });
      render();
    })
    .catch(function () {});
})();
