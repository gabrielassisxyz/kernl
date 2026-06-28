<template>
  <div class="px-margin pt-margin pb-margin max-w-3xl">
    <header class="mb-section">
      <h1 class="font-headline text-display text-text-primary">DA identity</h1>
      <p class="mt-tight font-body text-body text-text-muted">Edit the local assistant persona used by the workspace.</p>
    </header>

    <form @submit.prevent="save" class="flex flex-col gap-component">
      <UiField label="Display name">
        <UiInput v-model="displayName" />
      </UiField>
      <UiField label="System prompt">
        <UiTextarea
          v-model="systemPrompt"
          rows="10"
          classes="w-full rounded border border-border-hairline bg-bg-base px-component py-base font-mono-data text-mono-data text-text-primary outline-none transition-colors placeholder:text-text-muted focus:border-primary/70 disabled:cursor-not-allowed disabled:opacity-50 resize-y"
        />
      </UiField>
      <div class="flex items-center gap-component">
        <UiButton type="submit" variant="primary">Save</UiButton>
        <span v-if="statusText" class="font-body text-body" :class="statusClass">{{ statusText }}</span>
      </div>
    </form>
  </div>
</template>

<script setup lang="ts">
import UiButton from '~/components/ui/UiButton.vue'
import UiField from '~/components/ui/UiField.vue'
import UiInput from '~/components/ui/UiInput.vue'
import UiTextarea from '~/components/ui/UiTextarea.vue'

const displayName = ref('')
const systemPrompt = ref('')
const statusText = ref('')
const statusClass = ref('text-status-passed')

async function load() {
  const res = await fetch('/api/da/identity')
  const data = await res.json()
  displayName.value = data.display_name || ''
  systemPrompt.value = data.system_prompt || ''
}

async function save() {
  const body = { display_name: displayName.value, system_prompt: systemPrompt.value }
  const res = await fetch('/api/da/identity', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (res.status === 204) {
    statusText.value = 'Saved.'
    statusClass.value = 'text-status-passed'
    setTimeout(() => {
      statusText.value = ''
    }, 2000)
  } else {
    statusText.value = 'Error saving.'
    statusClass.value = 'text-status-failed-text'
  }
}

onMounted(() => {
  load()
})
</script>
