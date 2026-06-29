<template>
  <div class="tag-hierarchy">
    <div v-if="loading" class="tag-hierarchy__status">Loading…</div>
    <p v-else-if="isEmpty" class="tag-hierarchy__status">No tags yet. Add tags in a note's properties.</p>
    <div v-else class="tag-hierarchy__groups">
      <div v-for="(node, name) in tree" :key="name" class="tag-group">
        <button
          type="button"
          class="tag-group__head"
          :aria-expanded="!!expanded[name]"
          :aria-controls="`tag-group-${name}`"
          @click="toggle(name)"
        >
          <span class="material-symbols-outlined tag-group__chevron !text-[18px]" :class="{ 'is-open': expanded[name] }" aria-hidden="true">expand_more</span>
          <span class="material-symbols-outlined tag-group__icon !text-[15px]" aria-hidden="true">tag</span>
          <span class="tag-group__name">{{ name }}</span>
          <span class="tag-group__count">{{ node.files.length }}</span>
        </button>
        <div v-if="expanded[name]" :id="`tag-group-${name}`" class="tag-group__files">
          <button
            v-for="file in node.files"
            :key="file"
            type="button"
            class="tag-file"
            :class="{ 'tag-file--active': file === selected }"
            :title="file"
            @click="$emit('select', file)"
          >
            <span class="tag-file__dot" aria-hidden="true"></span>
            <span class="tag-file__name">{{ displayName(file) }}</span>
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'

const props = defineProps({
  selected: { type: String, default: null },
})

defineEmits(['select'])

const tree = ref({})
const expanded = ref({})
const loading = ref(true)

const isEmpty = computed(() => Object.keys(tree.value).length === 0)

const displayName = (file) => {
  const base = file.split('/').pop() || file
  return base.replace(/\.md$/, '')
}

const toggle = (name) => {
  expanded.value[name] = !expanded.value[name]
}

onMounted(async () => {
  try {
    // Tag hierarchy comes from the graph in one request (node_tags + note_paths),
    // shaped as { tag: { files: [...] } } — no per-file frontmatter parsing.
    const res = await fetch('/api/notes/tags')
    if (res.ok) {
      tree.value = await res.json()
    }
  } catch (e) {
    console.error('Error fetching tags', e)
  } finally {
    loading.value = false
  }
})
</script>

<style scoped>
.tag-hierarchy {
  padding: var(--spacing-base);
}

.tag-hierarchy__status {
  padding: var(--spacing-base) var(--spacing-component);
  font-family: var(--font-body);
  font-size: 13px;
  line-height: 1.5;
  color: var(--color-text-muted);
}

.tag-hierarchy__groups {
  display: flex;
  flex-direction: column;
  gap: 1px;
}

.tag-group__head {
  display: flex;
  align-items: center;
  gap: 4px;
  width: 100%;
  padding: 5px 8px;
  border-radius: var(--radius-lg);
  text-align: left;
  color: var(--color-text-primary);
  cursor: pointer;
  transition: background-color 120ms ease;
}

.tag-group__head:hover {
  background-color: color-mix(in srgb, var(--color-surface-hover) 60%, transparent);
}

.tag-group__head:focus-visible {
  outline: none;
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--color-da-accent) 70%, transparent);
}

.tag-group__chevron {
  color: var(--color-text-faint);
  transition: transform 150ms cubic-bezier(0.22, 1, 0.36, 1);
}

.tag-group__chevron.is-open {
  transform: rotate(0deg);
}

.tag-group__chevron:not(.is-open) {
  transform: rotate(-90deg);
}

.tag-group__icon {
  color: var(--color-node-note);
}

.tag-group__name {
  flex: 1 1 auto;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 13px;
  font-weight: 500;
}

.tag-group__count {
  flex-shrink: 0;
  font-family: var(--font-mono-data);
  font-size: 11px;
  color: var(--color-text-dim);
}

.tag-group__files {
  display: flex;
  flex-direction: column;
  gap: 1px;
  margin: 1px 0 2px 13px;
  padding-left: 9px;
  border-left: 1px solid var(--color-border-hairline);
}

.tag-file {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  padding: 5px 9px;
  border-radius: var(--radius-lg);
  text-align: left;
  color: var(--color-text-muted);
  cursor: pointer;
  transition: background-color 120ms ease, color 120ms ease;
}

.tag-file:hover {
  background-color: color-mix(in srgb, var(--color-surface-hover) 60%, transparent);
  color: var(--color-text-primary);
}

.tag-file:focus-visible {
  outline: none;
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--color-da-accent) 70%, transparent);
}

.tag-file--active {
  background-color: var(--color-surface-hover);
  color: var(--color-text-primary);
}

.tag-file__dot {
  width: 4px;
  height: 4px;
  flex-shrink: 0;
  border-radius: 50%;
  background-color: var(--color-text-dim);
  transition: background-color 120ms ease;
}

.tag-file:hover .tag-file__dot,
.tag-file--active .tag-file__dot {
  background-color: var(--color-node-note);
}

.tag-file__name {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 13px;
}
</style>
