<template>
  <IngestHeader :totalCount="items.length" @trigger="handleTrigger" />

  <section class="flex-1 overflow-y-auto relative">
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
    
    <UiEmptyState
      v-if="!pending && items.length === 0"
      icon="queue"
      title="Queue is empty."
      body="Files queued for ingest review appear here."
      action-label="Trigger Ingest"
      action-icon="play_arrow"
      @action="handleTrigger"
    />
  </section>

  <IngestHint v-if="items.length > 0" />

  <UiModal :open="showTriggerModal" title="Trigger Ingest" size="sm" @close="closeTriggerModal">
    <div class="flex flex-col gap-section">
      <UiField label="File path">
        <UiInput
          ref="triggerFileInput"
          v-model="triggerFilePath"
          placeholder="/tmp/test.md"
          @keydown.enter="submitTrigger"
          @keydown.esc="closeTriggerModal"
        />
      </UiField>
      <UiField label="Node ID (optional)">
        <UiInput
          v-model="triggerNodeId"
          placeholder=""
          @keydown.enter="submitTrigger"
          @keydown.esc="closeTriggerModal"
        />
      </UiField>
    </div>

    <template #footer>
      <div class="flex items-center justify-end gap-base">
        <UiButton variant="ghost" @click="closeTriggerModal">Cancel</UiButton>
        <UiButton variant="primary" :disabled="!triggerFilePath.trim()" @click="submitTrigger">Trigger</UiButton>
      </div>
    </template>
  </UiModal>
</template>

<script setup lang="ts">
import { ref, computed, nextTick, onMounted, onUnmounted } from 'vue'
import IngestHeader from '~/components/ingest/IngestHeader.vue'
import IngestItem from '~/components/ingest/IngestItem.vue'
import IngestHint from '~/components/ingest/IngestHint.vue'
import type { IngestReviewData } from '~/components/ingest/IngestItem.vue'
import UiButton from '~/components/ui/UiButton.vue'
import UiEmptyState from '~/components/ui/UiEmptyState.vue'
import UiField from '~/components/ui/UiField.vue'
import UiInput from '~/components/ui/UiInput.vue'
import UiModal from '~/components/ui/UiModal.vue'

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

const showTriggerModal = ref(false)
const triggerFilePath = ref('/tmp/test.md')
const triggerNodeId = ref('')
const triggerFileInput = ref<{ focus: () => void } | null>(null)

const handleTrigger = async () => {
  triggerFilePath.value = '/tmp/test.md'
  triggerNodeId.value = ''
  showTriggerModal.value = true
  await nextTick()
  triggerFileInput.value?.focus()
}

const closeTriggerModal = () => {
  showTriggerModal.value = false
}

const submitTrigger = async () => {
  const filePath = triggerFilePath.value.trim()
  if (!filePath) return
  showTriggerModal.value = false
  try {
    await $fetch('/api/ingest/trigger', {
      method: 'POST',
      body: { file_path: filePath, node_id: triggerNodeId.value.trim() }
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
