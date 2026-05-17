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
        html += '<div class="bead-card" style="border-left: 4px solid ' + style.color + '">'
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
