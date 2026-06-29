<template>
  <div class="note-list">
    <div v-if="loading" class="note-list__status">Loading…</div>
    <p v-else-if="files.length === 0" class="note-list__empty">
      No notes yet. Create your first note with the <span class="material-symbols-outlined !text-[14px] align-text-bottom">add</span> above.
    </p>
    <div v-else-if="filtered.length === 0" class="note-list__status">No notes match “{{ query }}”.</div>
    <div v-else class="note-list__items">
      <button
        v-for="file in filtered"
        :key="file"
        type="button"
        class="note-row"
        :class="{ 'note-row--active': file === selected }"
        :title="file"
        @click="$emit('select', file)"
      >
        <span class="note-row__dot" aria-hidden="true"></span>
        <span class="note-row__name">{{ displayName(file) }}</span>
        <span v-if="folderOf(file)" class="note-row__dir">{{ folderOf(file) }}</span>
      </button>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'

const props = defineProps({
  selected: { type: String, default: null },
  query: { type: String, default: '' },
})

defineEmits(['select'])

const files = ref([])
const loading = ref(true)

const filtered = computed(() => {
  const q = props.query.trim().toLowerCase()
  if (!q) return files.value
  return files.value.filter((f) => f.toLowerCase().includes(q))
})

const displayName = (file) => {
  const base = file.split('/').pop() || file
  return base.replace(/\.md$/, '')
}

const folderOf = (file) => {
  const idx = file.lastIndexOf('/')
  return idx === -1 ? '' : file.slice(0, idx)
}

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

<style scoped>
.note-list {
  padding: var(--spacing-base);
}

.note-list__status {
  padding: var(--spacing-base) var(--spacing-component);
  font-family: var(--font-body);
  font-size: 13px;
  color: var(--color-text-muted);
}

.note-list__empty {
  padding: var(--spacing-component);
  font-family: var(--font-body);
  font-size: 13px;
  line-height: 1.5;
  color: var(--color-text-muted);
}

.note-list__items {
  display: flex;
  flex-direction: column;
  gap: 1px;
}

.note-row {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  padding: 6px 10px;
  border-radius: var(--radius-lg);
  text-align: left;
  color: var(--color-text-muted);
  cursor: pointer;
  transition: background-color 120ms ease, color 120ms ease;
}

.note-row:hover {
  background-color: color-mix(in srgb, var(--color-surface-hover) 60%, transparent);
  color: var(--color-text-primary);
}

.note-row:focus-visible {
  outline: none;
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--color-da-accent) 70%, transparent);
}

.note-row--active {
  background-color: var(--color-surface-hover);
  color: var(--color-text-primary);
}

.note-row__dot {
  width: 5px;
  height: 5px;
  flex-shrink: 0;
  border-radius: 50%;
  background-color: var(--color-text-dim);
  transition: background-color 120ms ease;
}

.note-row:hover .note-row__dot,
.note-row--active .note-row__dot {
  background-color: var(--color-node-note);
}

.note-row__name {
  flex: 0 1 auto;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 13px;
}

.note-row__dir {
  flex: 0 0 auto;
  max-width: 40%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-family: var(--font-mono-data);
  font-size: 10px;
  color: var(--color-text-dim);
}
</style>
