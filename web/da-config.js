(function () {
  const displayNameInput = document.getElementById('display-name');
  const systemPromptInput = document.getElementById('system-prompt');
  const form = document.getElementById('persona-form');
  const status = document.getElementById('save-status');

  async function load() {
    const res = await fetch('/api/da/identity');
    const data = await res.json();
    displayNameInput.value = data.display_name || '';
    systemPromptInput.value = data.system_prompt || '';
  }

  form.addEventListener('submit', async (e) => {
    e.preventDefault();
    const body = {
      display_name: displayNameInput.value,
      system_prompt: systemPromptInput.value
    };
    const res = await fetch('/api/da/identity', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body)
    });
    if (res.status === 204) {
      status.textContent = 'Saved.';
      setTimeout(() => { status.textContent = ''; }, 2000);
    } else {
      status.textContent = 'Error saving.';
    }
  });

  load();
})();
