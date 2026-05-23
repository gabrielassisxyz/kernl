(function () {
  const el = document.getElementById('scope-selector');
  if (!el) return;

  async function loadScopes() {
    const res = await fetch('/api/nodes');
    const nodes = await res.json();
    const select = document.createElement('select');
    select.id = 'scope-select';
    const optAll = document.createElement('option');
    optAll.value = '';
    optAll.textContent = 'All notes';
    select.appendChild(optAll);
    nodes.forEach(n => {
      const opt = document.createElement('option');
      opt.value = n.id;
      opt.textContent = n.title + ' (' + n.type + ')';
      select.appendChild(opt);
    });
    el.innerHTML = '';
    el.appendChild(select);
    select.addEventListener('change', () => {
      window.scopeSelectorValue = select.value;
    });
  }

  loadScopes();
})();
