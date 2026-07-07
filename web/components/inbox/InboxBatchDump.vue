<template>
  <section>
    <CaptureThought
      v-model="text"
      placeholder="Paste a thread, list, notes dump, or copied conversation..."
      embedded
      manual-submit
      :save-on-enter="false"
      :max-rows="10"
      action-label="Create captures"
      action-icon="inbox"
      :loading="analyzing"
      upload-enabled
      upload-label="Upload file"
      @submit="openReview"
    />
    <p v-if="error" class="mt-base font-mono-data text-mono-data text-status-failed-text truncate">{{ error }}</p>

    <UiModal
      :open="reviewOpen"
      title="Review batch"
      kicker="BATCH DUMP"
      size="xl"
      align="top"
      @close="reviewOpen = false"
    >
      <div class="grid grid-cols-1 lg:grid-cols-[320px_minmax(0,1fr)] gap-section">
        <div class="flex flex-col gap-component">
          <UiField label="Context title">
            <UiInput v-model="contextTitle" classes="h-8 w-full rounded border border-border-hairline bg-surface-container-low px-base font-body text-body text-text-primary outline-none transition-colors placeholder:text-text-muted focus:border-primary/70" />
          </UiField>
          <UiField label="Source">
            <UiSelect v-model="source" classes="h-8 w-full rounded border border-border-hairline bg-surface-container-low px-base font-body text-body text-text-primary outline-none transition-colors focus:border-primary/70" @change="reanalyze">
              <option value="whatsapp">WhatsApp</option>
              <option value="text">Text</option>
              <option value="chat">Chat log</option>
              <option value="manual">Manual dump</option>
            </UiSelect>
          </UiField>
          <UiField label="Separator">
            <UiSelect v-model="separator" classes="h-8 w-full rounded border border-border-hairline bg-surface-container-low px-base font-body text-body text-text-primary outline-none transition-colors focus:border-primary/70" @change="reanalyze">
              <option value="auto">Auto</option>
              <option value="whatsapp">Chat timestamps</option>
              <option value="lines">One per line</option>
              <option value="blocks">Blank lines</option>
              <option value="divider">Divider line</option>
              <option value="markdown">Markdown headings</option>
              <option value="semantic">Semantic</option>
            </UiSelect>
          </UiField>
          <div class="border border-border-hairline bg-bg-base rounded px-base py-base">
            <p class="font-mono-data text-mono-data text-text-muted">{{ segments.length }} captures will be created</p>
            <p v-if="separator === 'semantic'" class="mt-tight font-body text-body text-text-muted">
              Semantic splitting needs the backend to understand the dump; without that, Kernl keeps the dump as one capture.
            </p>
          </div>
          <p v-if="reviewError" class="font-mono-data text-mono-data text-status-failed-text">{{ reviewError }}</p>
        </div>

        <div class="border border-border-hairline bg-bg-base rounded overflow-hidden min-w-0">
          <div class="flex items-center justify-between gap-base px-base py-tight border-b border-border-hairline">
            <span class="font-mono-data text-mono-data text-text-muted">Preview</span>
            <span class="font-mono-data text-mono-data text-text-faint">time + excerpt</span>
          </div>
          <div class="max-h-[420px] overflow-y-auto">
            <div
              v-for="segment in segments"
              :key="segment.sequence"
              class="grid grid-cols-[44px_minmax(0,1fr)] gap-base px-base py-base border-b border-border-hairline last:border-b-0"
            >
              <span class="font-mono-data text-mono-data text-text-faint">{{ segmentTime(segment) || `#${segment.sequence + 1}` }}</span>
              <p class="font-body text-body text-text-muted whitespace-pre-wrap">{{ segment.body }}</p>
            </div>
            <div v-if="segments.length === 0" class="px-base py-component font-body text-body text-text-muted">
              No captures detected yet.
            </div>
          </div>
        </div>
      </div>

      <template #footer>
        <div class="flex items-center justify-end gap-base">
          <UiButton variant="ghost" @click="reviewOpen = false">Cancel</UiButton>
          <UiButton variant="primary" icon="inbox" :loading="creating" :disabled="segments.length === 0" @click="createBatch">
            Create {{ segments.length }} captures
          </UiButton>
        </div>
      </template>
    </UiModal>
  </section>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import CaptureThought from '~/components/CaptureThought.vue'
import UiButton from '~/components/ui/UiButton.vue'
import UiField from '~/components/ui/UiField.vue'
import UiInput from '~/components/ui/UiInput.vue'
import UiModal from '~/components/ui/UiModal.vue'
import UiSelect from '~/components/ui/UiSelect.vue'

interface BatchSegment {
  body: string
  timestamp?: string
  sequence: number
  parseConfidence: string
}

interface BatchAnalysis {
  source: string
  separator: string
  suggestedContextTitle: string
  segments: BatchSegment[]
}

const emit = defineEmits<{
  (e: 'created', batchId: string): void
}>()

const text = ref('')
const source = ref('text')
const separator = ref('auto')
const contextTitle = ref('')
const segments = ref<BatchSegment[]>([])
const analyzing = ref(false)
const creating = ref(false)
const reviewOpen = ref(false)
const error = ref('')
const reviewError = ref('')

const canSubmit = computed(() => text.value.trim().length > 0)

function requestBody() {
  return {
    text: text.value.trim(),
    source: reviewOpen.value ? source.value : '',
    separator: separator.value,
    contextTitle: contextTitle.value.trim(),
  }
}

function segmentTime(segment: BatchSegment): string {
  const match = (segment.timestamp || '').match(/\b\d{1,2}:\d{2}(?::\d{2})?\b/)
  return match ? match[0].slice(0, 5) : ''
}

function applyAnalysis(analysis: BatchAnalysis, updateTitle: boolean) {
  source.value = analysis.source || 'text'
  separator.value = analysis.separator || 'semantic'
  if (updateTitle || !contextTitle.value.trim()) {
    contextTitle.value = analysis.suggestedContextTitle || ''
  }
  segments.value = analysis.segments || []
}

async function analyze(updateTitle: boolean) {
  const res = await $fetch<BatchAnalysis>('/api/inbox/batch/analyze', { method: 'POST', body: requestBody() })
  applyAnalysis(res, updateTitle)
}

async function openReview() {
  if (!canSubmit.value) return
  analyzing.value = true
  error.value = ''
  reviewError.value = ''
  try {
    await analyze(true)
    reviewOpen.value = true
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Could not analyze batch.'
  } finally {
    analyzing.value = false
  }
}

async function reanalyze() {
  if (!reviewOpen.value || !canSubmit.value) return
  reviewError.value = ''
  try {
    await analyze(false)
  } catch (e) {
    reviewError.value = e instanceof Error ? e.message : 'Could not refresh preview.'
  }
}

async function createBatch() {
  if (!canSubmit.value || segments.value.length === 0) return
  creating.value = true
  reviewError.value = ''
  try {
    const res = await $fetch<{ batchId: string; segments: BatchSegment[] }>('/api/inbox/batch', { method: 'POST', body: requestBody() })
    text.value = ''
    segments.value = res.segments || []
    reviewOpen.value = false
    emit('created', res.batchId)
  } catch (e) {
    reviewError.value = e instanceof Error ? e.message : 'Could not create batch.'
  } finally {
    creating.value = false
  }
}
</script>
