<template>
  <IngestHeader :totalCount="items.length" @trigger="handleTrigger" />

  <section class="flex-1 overflow-y-auto relative">
    <!-- Intake: paste text or upload a file. Both feed the same review queue
         below as the server-path trigger. -->
    <div class="px-section py-section border-b border-border-hairline flex flex-col gap-base shrink-0">
      <div class="bg-surface-overlay border border-border-default focus-within:border-primary/70 transition-colors p-component flex flex-col gap-2 relative rounded z-10">
        <div class="flex items-start gap-3">
          <span class="text-text-faint font-mono-data text-mono-data pt-[2px]">&gt;</span>
          <textarea
            ref="inputEl"
            v-model="pasteText"
            @keydown="onPasteKeydown"
            rows="3"
            autofocus
            placeholder="Paste text to ingest — meeting notes, an article, a decision…"
            class="w-full bg-transparent border-none text-text-primary font-mono-data text-mono-data focus:ring-0 resize-none p-0 placeholder:text-text-faint leading-relaxed outline-none"
          ></textarea>
          <span v-show="!pasteText" class="blinking-cursor absolute top-[18px] left-[32px] pointer-events-none font-mono-data text-mono-data h-[14px]">_</span>
        </div>

        <div class="flex items-center justify-between mt-1">
          <div class="flex items-center gap-4 text-text-faint font-mono-data text-mono-data tracking-wide">
            <input ref="fileInput" type="file" multiple accept=".pdf,.docx,.csv,.xlsx,.txt,.png,.jpg,.jpeg,.py,.java,.kt,.md,text/*,image/*,application/pdf" class="hidden" @change="onFilePicked" />
            <button class="hover:text-text-primary transition-colors flex items-center gap-1 outline-none focus-visible:ring-1 focus-visible:ring-primary/30 rounded cursor-pointer" @click="fileInput?.click()">
              <span class="material-symbols-outlined !text-[16px]">upload_file</span>
              Upload files
            </button>
            <span v-if="uploadStatus" class="text-text-muted">{{ uploadStatus }}</span>
          </div>
          <div class="flex gap-4 text-text-faint font-mono-data text-mono-data tracking-wide">
            <span class="hidden sm:inline">[SHIFT+ENTER] new line</span>
            <span>[ENTER] save</span>
          </div>
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
const inputEl = ref<HTMLTextAreaElement | null>(null)
const uploadStatus = ref('')

const onPasteKeydown = (e: KeyboardEvent) => {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault()
    submitPaste()
  }
}

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
  const files = input.files
  if (!files || files.length === 0) return
  uploadStatus.value = `Uploading ${files.length} file(s)…`
  const before = items.value.length
  try {
    for (let i = 0; i < files.length; i++) {
      const file = files[i]
      const form = new FormData()
      form.append('file', file)
      await $fetch('/api/ingest/upload', { method: 'POST', body: form })
    }
    uploadStatus.value = `Ingested ${files.length} file(s)`
    await pollQueueForGrowth(before)
  } catch (error) {
    console.error('Failed to upload file(s)', error)
    uploadStatus.value = 'Upload failed'
  } finally {
    input.value = '' // allow re-selecting the same files
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
  const tg = e.target as HTMLElement | null
  if (tg && (tg.tagName === 'INPUT' || tg.tagName === 'TEXTAREA' || tg.tagName === 'SELECT')) return

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
  inputEl.value?.focus()
  window.addEventListener('keydown', handleKeydown)
})

onUnmounted(() => {
  window.removeEventListener('keydown', handleKeydown)
})
</script>

<style scoped>
.blinking-cursor {
  font-weight: bold;
  font-size: 1.2em;
  color: var(--color-text-muted);
  animation: 1s blink step-end infinite;
}

@keyframes blink {
  from, to { color: transparent; }
  50% { color: var(--color-text-muted); }
}
</style>
