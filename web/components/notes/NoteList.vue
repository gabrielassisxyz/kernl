<template>
  <div class="note-list p-component border-t border-border-default bg-bg-base text-body">
    <h3 class="font-label-caps text-label-caps text-text-muted mb-component">All Notes</h3>
    <div v-if="loading" class="font-body text-body text-text-muted">Loading...</div>
    <div v-else class="flex flex-col gap-1">
      <button v-for="file in files" :key="file" class="w-full text-left py-1 px-base text-text-muted hover:text-text-primary hover:bg-bg-elevated focus-visible:bg-bg-elevated focus:outline-none focus-visible:ring-1 focus-visible:ring-da-accent rounded cursor-pointer truncate font-mono-data" @click="$emit('select', file)">
        {{ file }}
      </button>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'

const files = ref([])
const loading = ref(true)

defineEmits(['select'])

const refresh = async () => {
  loading.value = true
  try {
    // Flat disk-truth list so brand-new untagged notes have somewhere to appear.
    const res = await fetch('/api/vault/list')
    if (res.ok) {
      const data = await res.json()
      files.value = data.files || []
    }
  } catch (e) {
    console.error('Error fetching vault list', e)
  } finally {
    loading.value = false
  }
}

onMounted(refresh)

defineExpose({ refresh })
</script>
