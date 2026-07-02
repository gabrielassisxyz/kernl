<template>
  <IngestHeader :totalCount="items.length" @trigger="handleTrigger" />

  <section class="flex-1 overflow-y-auto relative">
    <!-- Intake: paste text or upload a file. Both feed the same review queue
         below as the server-path trigger. -->
    <div class="px-section pt-section flex flex-col gap-base">
      <div class="rounded border border-border-hairline bg-surface-overlay focus-within:border-primary/70 transition-colors p-component flex flex-col gap-base">
        <textarea
          v-model="pasteText"
          rows="3"
          placeholder="Paste text to ingest — meeting notes, an article, a decision…"
          class="w-full bg-transparent border-none text-body text-text-primary resize-y focus:ring-0 outline-none placeholder:text-text-faint"
        ></textarea>
        <div class="flex items-center justify-between gap-base">
          <div class="flex items-center gap-base">
            <input ref="fileInput" type="file" accept=".md,.txt,text/markdown,text/plain" class="hidden" @change="onFilePicked" />
            <UiButton variant="ghost" size="sm" icon="upload_file" @click="fileInput?.click()">Upload .md / .txt</UiButton>
            <span v-if="uploadStatus" class="font-mono-data text-mono-data text-text-muted">{{ uploadStatus }}</span>
          </div>
          <UiButton variant="primary" size="sm" :loading="pasting" :disabled="!pasteText.trim()" @click="submitPaste">Ingest text</UiButton>
        </div>
      </div>
    </div>

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

    <!-- Update merge review: accept/reject the hunks the DA proposes folding
         into the resolved target note. Accepting/rejecting the last hunk
         applies the accepted set and resolves the review. -->
    <DiffSuggest
      v-if="merge"
      :hunks="merge.hunks"
      @accept="onMergeAccept"
      @reject="onMergeReject"
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
import DiffSuggest from '~/components/notes/DiffSuggest.vue'
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

// ---- intake: paste + upload ----
const pasteText = ref('')
const pasting = ref(false)
const fileInput = ref<HTMLInputElement | null>(null)
const uploadStatus = ref('')

// Extraction runs in a detached goroutine on the server, so poll the queue for
// a short window after intake until a new review item lands.
async function pollQueueForGrowth(fromCount: number) {
  const deadline = Date.now() + 15000
  while (Date.now() < deadline) {
    await new Promise(r => setTimeout(r, 1500))
    await refresh()
    if (items.value.length > fromCount) return
  }
}

const submitPaste = async () => {
  const text = pasteText.value.trim()
  if (!text || pasting.value) return
  pasting.value = true
  const before = items.value.length
  try {
    await $fetch('/api/ingest/paste', { method: 'POST', body: { text } })
    pasteText.value = ''
    await pollQueueForGrowth(before)
  } catch (error) {
    console.error('Failed to ingest pasted text', error)
  } finally {
    pasting.value = false
  }
}

const onFilePicked = async (e: Event) => {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  uploadStatus.value = `Uploading ${file.name}…`
  const before = items.value.length
  try {
    const form = new FormData()
    form.append('file', file)
    await $fetch('/api/ingest/upload', { method: 'POST', body: form })
    uploadStatus.value = `Ingested ${file.name}`
    await pollQueueForGrowth(before)
  } catch (error) {
    console.error('Failed to upload file', error)
    uploadStatus.value = 'Upload failed — only .md / .txt are supported'
  } finally {
    input.value = '' // allow re-selecting the same file
  }
}

interface MergeHunk { id: string; content: string }
interface MergePlan { targetNoteId: string; targetTitle: string; currentBody: string; hunks: MergeHunk[] }
interface MergeState { reviewId: string; targetNoteId: string; hunks: MergeHunk[]; accepted: MergeHunk[] }

const merge = ref<MergeState | null>(null)

const handleAction = async (id: string, action: string) => {
  if (action === 'Update') {
    await startMerge(id)
    return
  }
  await resolveAction(id, action)
}

// resolveAction calls the resolve endpoint and optimistically drops the item.
const resolveAction = async (
  id: string,
  action: string,
  extra: Record<string, unknown> = {}
) => {
  try {
    await $fetch(`/api/ingest/queue/${id}/resolve`, {
      method: 'POST',
      body: { action, ...extra }
    })
    if (data.value) {
      data.value = data.value.filter(i => i.ID !== id)
    }
    if (selectedIndex.value >= items.value.length) {
      selectedIndex.value = Math.max(0, items.value.length - 1)
    }
  } catch (error) {
    console.error('Failed to process item', error)
  }
}

// startMerge asks the backend to plan the Update. With no confident target it
// falls back to Create Page; otherwise it opens the DiffSuggest review.
const startMerge = async (id: string) => {
  try {
    const plan = await $fetch<MergePlan>(`/api/ingest/queue/${id}/merge-plan`, { method: 'POST' })
    if (!plan || !plan.targetNoteId) {
      await resolveAction(id, 'Create Page')
      return
    }
    merge.value = {
      reviewId: id,
      targetNoteId: plan.targetNoteId,
      hunks: [...(plan.hunks || [])],
      accepted: []
    }
    if (merge.value.hunks.length === 0) {
      await finalizeMerge()
    }
  } catch (error) {
    console.error('Failed to plan merge', error)
  }
}

const onMergeAccept = (hunk: MergeHunk) => {
  if (!merge.value) return
  merge.value.accepted.push(hunk)
  merge.value.hunks = merge.value.hunks.filter(h => h.id !== hunk.id)
  if (merge.value.hunks.length === 0) finalizeMerge()
}

const onMergeReject = (hunk: MergeHunk) => {
  if (!merge.value) return
  merge.value.hunks = merge.value.hunks.filter(h => h.id !== hunk.id)
  if (merge.value.hunks.length === 0) finalizeMerge()
}

// finalizeMerge applies the accepted hunks. Accepting none is valid — the
// backend leaves the target unchanged but still connects it and resolves.
const finalizeMerge = async () => {
  if (!merge.value) return
  const { reviewId, targetNoteId, accepted } = merge.value
  merge.value = null
  await resolveAction(reviewId, 'Update', { targetNoteId, acceptedHunks: accepted })
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
  } else if (e.key === 's' || e.key === 'S') {
    handleAction(items.value[selectedIndex.value].ID, 'Skip')
  } else if (e.key === 'u' || e.key === 'U') {
    handleAction(items.value[selectedIndex.value].ID, 'Update')
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
