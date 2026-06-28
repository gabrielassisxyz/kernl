<template>
  <div class="tag-hierarchy p-component bg-bg-base text-body">
    <h3 class="font-label-caps text-label-caps text-text-muted mb-component">Tags</h3>
    <div v-if="loading" class="font-body text-body text-text-muted">Loading...</div>
    <div v-else>
      <div v-for="(node, name) in tree" :key="name" class="mb-2">
        <button class="w-full flex items-center gap-2 cursor-pointer text-text-primary hover:bg-surface-hover focus-visible:bg-surface-hover focus:outline-none focus-visible:ring-1 focus-visible:ring-da-accent px-base py-1.5 rounded" @click="toggle(name)" :aria-expanded="expanded[name]" :aria-controls="`tag-group-${name}`">
          <span class="text-text-muted font-mono-data text-mono-data w-3 flex justify-center">{{ expanded[name] ? '▼' : '▶' }}</span>
          <span class="font-medium">#{{ name }}</span>
        </button>
        <div v-if="expanded[name]" :id="`tag-group-${name}`" class="ml-component pl-component border-l border-border-hairline mt-1 flex flex-col gap-1">
          <button v-for="file in node.files" :key="file" class="w-full text-left py-1 px-base text-text-muted hover:text-text-primary hover:bg-bg-elevated focus-visible:bg-bg-elevated focus:outline-none focus-visible:ring-1 focus-visible:ring-da-accent rounded cursor-pointer truncate font-mono-data" @click="$emit('select', file)">
            {{ file }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'

const tree = ref({})
const expanded = ref({})
const loading = ref(true)

defineEmits(['select'])

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
