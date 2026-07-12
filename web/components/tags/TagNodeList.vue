<template>
  <div class="flex flex-col gap-[1px]">
    <p v-if="loading" class="px-component py-base font-body text-body text-text-muted">Loading…</p>
    <p v-else-if="error" class="px-component py-base font-mono-data text-mono-data text-status-failed-text">{{ error }}</p>
    <p v-else-if="!nodes.length" class="px-component py-base font-body text-body text-text-muted">
      Nothing tagged <span class="font-mono-data text-mono-data">{{ tag }}</span> yet.
    </p>

    <component
      :is="isNavigable(node) ? 'button' : 'div'"
      v-for="node in nodes"
      :key="node.id"
      :type="isNavigable(node) ? 'button' : undefined"
      class="flex items-center gap-component w-full px-component py-base rounded text-left transition-colors"
      :class="isNavigable(node)
        ? 'cursor-pointer hover:bg-surface-hover outline-none focus-visible:ring-1 focus-visible:ring-primary/30'
        : 'cursor-default'"
      @click="isNavigable(node) && $emit('open', node)"
    >
      <span
        class="material-symbols-outlined !text-body shrink-0"
        :style="{ color: colorForType(node.type) }"
        aria-hidden="true"
      >{{ metaForType(node.type).icon }}</span>

      <span class="flex-1 min-w-0 truncate font-body text-body text-text-primary">{{ node.title || 'Untitled' }}</span>

      <span class="shrink-0 font-mono-data text-mono-data text-text-faint">{{ labelForType(node.type) }}</span>
      <span class="shrink-0 font-mono-data text-mono-data text-text-faint">{{ formatTimestamp(node.updatedAt) }}</span>
    </component>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { fetchTagNodes, type TaggedNode } from '~/composables/useTags'
import { colorForType, labelForType, metaForType } from '~/utils/nodeTypes'
import { formatTimestamp } from '~/utils/time'

// Everything under one tag, of every type — the answer to "what do I have on
// subject X". Rows are only clickable for the types that have a surface to open;
// the rest are context, not dead links. Where an open goes is the parent's call:
// the tags page navigates away, the Notes sidebar opens the note in place.
const props = withDefaults(defineProps<{ tag: string; type?: string }>(), { type: '' })

defineEmits<{ (e: 'open', node: TaggedNode): void }>()

const nodes = ref<TaggedNode[]>([])
const loading = ref(false)
const error = ref<string | null>(null)

const isNavigable = (node: TaggedNode): boolean =>
  (node.type === 'note' && !!node.path) || node.type === 'task' || node.type === 'project'

watch(
  () => [props.tag, props.type],
  async () => {
    if (!props.tag) {
      nodes.value = []
      return
    }
    loading.value = true
    error.value = null
    try {
      nodes.value = await fetchTagNodes(props.tag, props.type)
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
    } finally {
      loading.value = false
    }
  },
  { immediate: true }
)
</script>
