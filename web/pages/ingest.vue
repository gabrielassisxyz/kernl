<template>
  <!-- Header Strip -->
  <IngestHeader :totalCount="items.length" @trigger="handleTrigger" />

  <!-- Ingest Queue -->
  <section class="flex-1 overflow-y-auto hide-scrollbar relative">
    <div class="flex flex-col">
      <IngestItem 
        v-for="(item, index) in items" 
        :key="item.ID" 
        :item="item" 
        :isSelected="selectedIndex === index"
        @select="selectedIndex = index"
        @action="(action) => handleAction(item.ID, action)"
      />
    </div>
    
    <!-- Empty State -->
    <div v-if="!pending && items.length === 0" class="flex flex-col items-center justify-center py-break text-text-muted">
      <span class="material-symbols-outlined text-[32px] mb-component">queue</span>
      <p class="font-body">Ingest queue is empty</p>
    </div>
  </section>

  <!-- Command Bar Hint -->
  <IngestHint v-if="items.length > 0" />
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import IngestHeader from '~/components/ingest/IngestHeader.vue'
import IngestItem from '~/components/ingest/IngestItem.vue'
import IngestHint from '~/components/ingest/IngestHint.vue'
import type { IngestReviewData } from '~/components/ingest/IngestItem.vue'

const { data, pending, refresh } = useFetch<IngestReviewData[]>('/api/ingest/queue', {
  server: false,
  default: () => []
})

const items = computed(() => data.value || [])
const selectedIndex = ref(0)

const handleAction = async (id: string, action: string) => {
  try {
    await $fetch(`/api/ingest/queue/${id}/resolve`, {
      method: 'POST',
      body: { action }
    })
    // Optimistically remove the item (only on success; unimplemented actions
    // return 501 and throw, leaving the item in the queue).
    if (data.value) {
      data.value = data.value.filter(i => i.ID !== id)
    }
    // Adjust selection if needed
    if (selectedIndex.value >= items.value.length) {
      selectedIndex.value = Math.max(0, items.value.length - 1)
    }
  } catch (error) {
    console.error('Failed to process item', error)
  }
}

const handleTrigger = async () => {
  try {
    const filePath = window.prompt("Enter file path to ingest:", "/tmp/test.md")
    if (!filePath) return
    const nodeId = window.prompt("Enter node ID (optional):", "") || ""
    await $fetch('/api/ingest/trigger', {
      method: 'POST',
      body: { file_path: filePath, node_id: nodeId }
    })
    setTimeout(refresh, 1000)
  } catch (error) {
    console.error('Failed to trigger ingest', error)
  }
}

const handleKeydown = (e: KeyboardEvent) => {
  // Prevent defaults for app-wide shortcuts if necessary
  if (items.value.length === 0) {
    if (e.key === 't' || e.key === 'T') {
      handleTrigger()
    }
    return
  }
  
  if (e.key === 'ArrowDown') {
    e.preventDefault()
    selectedIndex.value = (selectedIndex.value + 1) % items.value.length
  } else if (e.key === 'ArrowUp') {
    e.preventDefault()
    selectedIndex.value = (selectedIndex.value - 1 + items.value.length) % items.value.length
  } else if (e.key === 'c' || e.key === 'C') {
    handleAction(items.value[selectedIndex.value].ID, 'Create Page')
  } else if (e.key === 'd' || e.key === 'D') {
    handleAction(items.value[selectedIndex.value].ID, 'Deep Research')
  } else if (e.key === 's' || e.key === 'S') {
    handleAction(items.value[selectedIndex.value].ID, 'Skip')
  } else if (e.key === 'u' || e.key === 'U') {
    handleAction(items.value[selectedIndex.value].ID, 'Update')
  } else if (e.key === 'a' || e.key === 'A') {
    handleAction(items.value[selectedIndex.value].ID, 'Add Contradiction Callout')
  } else if (e.key === 't' || e.key === 'T') {
    handleTrigger()
  }
}

onMounted(() => {
  window.addEventListener('keydown', handleKeydown)
})

onUnmounted(() => {
  window.removeEventListener('keydown', handleKeydown)
})
</script>
