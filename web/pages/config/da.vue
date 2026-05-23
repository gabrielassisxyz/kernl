<template>
  <div class="max-w-2xl">
    <h2 class="text-lg text-[#64748b] uppercase tracking-wider mb-4">Persona Editor</h2>
    <form @submit.prevent="save" class="flex flex-col gap-4">
      <label class="flex flex-col gap-1 text-sm text-[#94a3b8]">
        Display Name
        <input
          v-model="displayName"
          type="text"
          class="bg-[#1e293b] border border-[#334155] text-[#e2e8f0] p-2 rounded font-mono text-sm"
        />
      </label>
      <label class="flex flex-col gap-1 text-sm text-[#94a3b8]">
        System Prompt
        <textarea
          v-model="systemPrompt"
          rows="10"
          class="bg-[#1e293b] border border-[#334155] text-[#e2e8f0] p-2 rounded font-mono text-sm resize-y"
        ></textarea>
      </label>
      <div class="flex items-center gap-3">
        <button
          type="submit"
          class="bg-[#1d4ed8] text-white px-4 py-2 rounded font-mono text-sm hover:bg-[#2563eb] transition-colors"
        >
          Save
        </button>
        <span v-if="statusText" class="text-sm" :class="statusClass">{{ statusText }}</span>
      </div>
    </form>
  </div>
</template>

<script setup lang="ts">
const displayName = ref('');
const systemPrompt = ref('');
const statusText = ref('');
const statusClass = ref('text-[#6ee7b7]');

async function load() {
  const res = await fetch('/api/da/identity');
  const data = await res.json();
  displayName.value = data.display_name || '';
  systemPrompt.value = data.system_prompt || '';
}

async function save() {
  const body = { display_name: displayName.value, system_prompt: systemPrompt.value };
  const res = await fetch('/api/da/identity', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
  if (res.status === 204) {
    statusText.value = 'Saved.';
    statusClass.value = 'text-[#6ee7b7]';
    setTimeout(() => {
      statusText.value = '';
    }, 2000);
  } else {
    statusText.value = 'Error saving.';
    statusClass.value = 'text-[#fca5a5]';
  }
}

onMounted(() => {
  load();
});
</script>
