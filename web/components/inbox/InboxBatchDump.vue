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
      :loading="splitting"
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
            <UiSelect v-model="source" classes="h-8 w-full rounded border border-border-hairline bg-surface-container-low px-base font-body text-body text-text-primary outline-none transition-colors focus:border-primary/70" @change="resplit">
              <option value="whatsapp">WhatsApp</option>
              <option value="text">Text</option>
              <option value="chat">Chat log</option>
              <option value="manual">Manual dump</option>
            </UiSelect>
          </UiField>
          <UiField label="Separator">
            <UiSelect v-model="separator" classes="h-8 w-full rounded border border-border-hairline bg-surface-container-low px-base font-body text-body text-text-primary outline-none transition-colors focus:border-primary/70" @change="resplit">
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

          <!-- A merge deletes a message from the record. The model may suggest one;
               only the human applies it. -->
          <div v-if="enriching || enriched" class="border border-border-hairline bg-bg-base rounded px-base py-base">
            <p class="font-mono-data text-mono-data text-text-muted">Suggested merges</p>
            <p v-if="enriching" class="mt-tight font-body text-body text-text-muted">Looking for messages that say the same thing…</p>
            <p v-else-if="proposals.length === 0" class="mt-tight font-body text-body text-text-muted">None — every message stays its own capture.</p>
            <div
              v-for="(proposal, i) in proposals"
              :key="i"
              class="mt-base flex flex-col gap-tight border-t border-border-hairline pt-base first:border-t-0 first:pt-0 first:mt-tight"
            >
              <p class="font-mono-data text-mono-data text-text-faint">{{ proposalLabel(proposal) }}</p>
              <p v-if="proposal.reason" class="font-body text-body text-text-muted">{{ proposal.reason }}</p>
              <div class="flex items-center gap-base">
                <UiButton :variant="proposal.accepted ? 'primary' : 'ghost'" @click="proposal.accepted = true">Merge</UiButton>
                <UiButton :variant="proposal.accepted ? 'ghost' : 'primary'" @click="proposal.accepted = false">Keep separate</UiButton>
              </div>
            </div>
          </div>
          <p v-if="reviewError" class="font-mono-data text-mono-data text-status-failed-text">{{ reviewError }}</p>
        </div>

        <div class="border border-border-hairline bg-bg-base rounded overflow-hidden min-w-0">
          <div class="flex items-center justify-between gap-base px-base py-tight border-b border-border-hairline">
            <span class="font-mono-data text-mono-data text-text-muted">Preview</span>
            <span class="font-mono-data text-mono-data text-text-faint">your messages, as written</span>
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
  sender?: string
  timestamp?: string
  sequence: number
  parseConfidence: string
}

// FinalBatchSegment mirrors inbox.FinalBatchSegment (internal/inbox/batch_enrichment.go).
// Its body is only ever a preview: the server rebuilds every body from the
// pasted text and honors nothing here but sourceSequences — the merges the user
// accepted.
interface FinalBatchSegment {
  body: string
  sender?: string
  timestamp?: string
  sequence: number
  sourceSequences: number[]
  confidence?: string
}

interface MergeProposal {
  sourceSequences: number[]
  reason?: string
}

interface ReviewableMerge extends MergeProposal {
  accepted: boolean
}

interface BatchPreview {
  source: string
  separator: string
  suggestedContextTitle: string
  segments: BatchSegment[]
  finalSegments: FinalBatchSegment[]
}

interface BatchAnalysis extends BatchPreview {
  mergeProposals?: MergeProposal[]
}

const emit = defineEmits<{
  (e: 'created', batchId: string): void
}>()

const text = ref('')
const source = ref('text')
const separator = ref('auto')
const contextTitle = ref('')
const rawSegments = ref<BatchSegment[]>([])
const proposals = ref<ReviewableMerge[]>([])
const splitting = ref(false)
const enriching = ref(false)
const enriched = ref(false)
const creating = ref(false)
const reviewOpen = ref(false)
const error = ref('')
const reviewError = ref('')

const canSubmit = computed(() => text.value.trim().length > 0)

// The captures as they will be created: one per message, plus the merges the
// user accepted. Mirrors buildFinalSegments in internal/inbox/batch.go — the
// server rebuilds this from the same rules, and its answer is the one that counts.
const finalSegments = computed<FinalBatchSegment[]>(() => {
  const groupOf = new Map<number, number[]>()
  for (const proposal of proposals.value) {
    if (!proposal.accepted) continue
    const members = proposal.sourceSequences.filter(seq => !groupOf.has(seq))
    if (members.length < 2) continue
    members.sort((a, b) => a - b)
    for (const seq of members) groupOf.set(seq, members)
  }

  const bySeq = new Map(rawSegments.value.map(seg => [seg.sequence, seg]))
  const out: FinalBatchSegment[] = []
  for (const seg of rawSegments.value) {
    const members = groupOf.get(seg.sequence) ?? [seg.sequence]
    if (members[0] !== seg.sequence) continue
    const anchor = bySeq.get(members[0])!
    out.push({
      body: members.map(seq => bySeq.get(seq)?.body ?? '').join('\n\n'),
      sender: anchor.sender,
      timestamp: anchor.timestamp,
      sequence: out.length,
      sourceSequences: members,
      confidence: anchor.parseConfidence,
    })
  }
  return out
})

const segments = computed<BatchSegment[]>(() =>
  finalSegments.value.map(seg => ({
    body: seg.body,
    sender: seg.sender,
    timestamp: seg.timestamp,
    sequence: seg.sequence,
    parseConfidence: seg.confidence ?? '',
  })),
)

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

function proposalLabel(proposal: MergeProposal): string {
  const bySeq = new Map(rawSegments.value.map(seg => [seg.sequence, seg]))
  return proposal.sourceSequences
    .map(seq => {
      const seg = bySeq.get(seq)
      return seg ? segmentTime(seg) || `#${seq + 1}` : `#${seq + 1}`
    })
    .join(' + ')
}

function applySplit(preview: BatchPreview, updateTitle: boolean) {
  source.value = preview.source || 'text'
  separator.value = preview.separator || 'semantic'
  if (updateTitle || !contextTitle.value.trim()) {
    contextTitle.value = preview.suggestedContextTitle || ''
  }
  rawSegments.value = preview.segments || []
  proposals.value = []
  enriched.value = false
}

// The modal opens on the mechanical split — reading your own messages must not
// wait on a model. Enrichment lands afterwards, as a layer on top.
async function openReview() {
  if (!canSubmit.value) return
  splitting.value = true
  error.value = ''
  reviewError.value = ''
  try {
    const preview = await $fetch<BatchPreview>('/api/inbox/batch/preview', { method: 'POST', body: requestBody() })
    applySplit(preview, true)
    reviewOpen.value = true
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Could not split the paste.'
    return
  } finally {
    splitting.value = false
  }
  enrich(true)
}

async function enrich(updateTitle: boolean) {
  enriching.value = true
  try {
    const analysis = await $fetch<BatchAnalysis>('/api/inbox/batch/analyze', { method: 'POST', body: requestBody() })
    // The split stays the parser's; only the title and the merge offers are the
    // model's, and a merge offer starts out rejected.
    if (updateTitle && !contextTitle.value.trim()) {
      contextTitle.value = analysis.suggestedContextTitle || ''
    }
    proposals.value = (analysis.mergeProposals || []).map(p => ({ ...p, accepted: false }))
    enriched.value = true
  } catch {
    // Enrichment is optional: without it the batch is still the messages as
    // pasted, which is the thing that matters.
    proposals.value = []
  } finally {
    enriching.value = false
  }
}

async function resplit() {
  if (!reviewOpen.value || !canSubmit.value) return
  reviewError.value = ''
  try {
    const preview = await $fetch<BatchPreview>('/api/inbox/batch/preview', { method: 'POST', body: requestBody() })
    applySplit(preview, false)
  } catch (e) {
    reviewError.value = e instanceof Error ? e.message : 'Could not refresh preview.'
    return
  }
  enrich(false)
}

async function createBatch() {
  if (!canSubmit.value || segments.value.length === 0) return
  creating.value = true
  reviewError.value = ''
  try {
    // rawSegments is the split that was reviewed (it is the LLM's own cut in
    // semantic mode, which the server cannot re-derive); finalSegments carries
    // the merges accepted here. The server takes the bodies from neither.
    const res = await $fetch<{ batchId: string; segments: BatchSegment[] }>('/api/inbox/batch', {
      method: 'POST',
      body: { ...requestBody(), rawSegments: rawSegments.value, finalSegments: finalSegments.value },
    })
    text.value = ''
    rawSegments.value = []
    proposals.value = []
    enriched.value = false
    reviewOpen.value = false
    emit('created', res.batchId)
  } catch (e) {
    reviewError.value = e instanceof Error ? e.message : 'Could not create batch.'
  } finally {
    creating.value = false
  }
}
</script>
