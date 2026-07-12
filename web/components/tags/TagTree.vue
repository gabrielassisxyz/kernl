<template>
  <div class="flex flex-col gap-[1px]">
    <p v-if="loading" class="px-component py-base font-body text-body text-text-muted">Loading…</p>
    <p v-else-if="error" class="px-component py-base font-mono-data text-mono-data text-status-failed-text">{{ error }}</p>
    <p v-else-if="!rows.length" class="px-component py-base font-body text-body text-text-muted">{{ emptyText }}</p>

    <div v-for="row in rows" :key="row.name" class="flex items-center">
      <button
        type="button"
        class="flex items-center justify-center w-5 h-7 shrink-0 text-text-faint hover:text-text-primary transition-colors outline-none focus-visible:ring-1 focus-visible:ring-primary/30 rounded"
        :class="{ 'cursor-pointer': row.hasChildren, 'invisible': !row.hasChildren }"
        :style="{ marginLeft: `${row.depth * 12}px` }"
        :aria-label="expanded[row.name] ? `Collapse ${row.name}` : `Expand ${row.name}`"
        :aria-expanded="!!expanded[row.name]"
        @click="toggle(row.name)"
      >
        <span
          class="material-symbols-outlined !text-[16px] transition-transform duration-150"
          :class="expanded[row.name] ? '' : '-rotate-90'"
          aria-hidden="true"
        >expand_more</span>
      </button>

      <button
        type="button"
        class="flex flex-1 min-w-0 items-center gap-tight px-base h-7 rounded font-body text-body transition-colors cursor-pointer outline-none focus-visible:ring-1 focus-visible:ring-primary/30"
        :class="row.name === selected
          ? 'bg-surface-hover text-text-primary'
          : 'text-text-muted hover:text-text-primary hover:bg-surface-hover/60'"
        @click="$emit('select', row.name)"
      >
        <span class="material-symbols-outlined !text-[15px] text-text-faint" aria-hidden="true">tag</span>
        <span class="flex-1 min-w-0 truncate text-left">{{ row.segment }}</span>
        <span class="shrink-0 font-mono-data text-mono-data text-text-faint">{{ row.count }}</span>
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { type TagNode } from '~/composables/useTags'

// The nested tag surface. The API returns the hierarchy the `/` convention
// implies; this renders it as a tree of segments, so `homelab/nas` shows as `nas`
// under `homelab`. With `type` set it narrows to one node type — that is how the
// Notes sidebar keeps its note-only view while the tags page shows everything.
//
// Presentational: the owning surface fetches the tree, because it also needs the
// same data to reload after a tag is written and (on the tags page) to build the
// type filter.
const props = withDefaults(defineProps<{
  tree: TagNode[]
  type?: string
  selected?: string
  loading?: boolean
  error?: string | null
  emptyText?: string
}>(), {
  type: '',
  selected: '',
  loading: false,
  error: null,
  emptyText: 'No tags yet.',
})

defineEmits<{ (e: 'select', name: string): void }>()

const expanded = ref<Record<string, boolean>>({})

const toggle = (name: string) => {
  expanded.value[name] = !expanded.value[name]
}

// Counts are subtree-inclusive, so a `type` filter can hide a whole branch by
// asking one question per node: does it hold anything of this type at all?
const countOf = (node: TagNode): number => (props.type ? node.byType?.[props.type] ?? 0 : node.count)

interface Row {
  name: string
  segment: string
  count: number
  depth: number
  hasChildren: boolean
}

// Flattened rather than recursive: a flat row list with a depth offset renders the
// same tree without a self-referencing component, and collapsing is one lookup.
const rows = computed<Row[]>(() => {
  const out: Row[] = []
  const walk = (nodes: TagNode[], depth: number) => {
    for (const node of nodes) {
      const count = countOf(node)
      if (count === 0) continue
      const children = (node.children ?? []).filter((c) => countOf(c) > 0)
      out.push({ name: node.name, segment: node.segment, count, depth, hasChildren: children.length > 0 })
      if (expanded.value[node.name]) walk(children, depth + 1)
    }
  }
  walk(props.tree, 0)
  return out
})
</script>
