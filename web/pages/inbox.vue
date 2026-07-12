<template>
  <InboxHeader :totalCount="items.length" :flaggedCount="flaggedCount" />

  <div class="px-section pt-base flex items-center justify-between">
    <div class="flex items-center gap-tight font-mono-data text-mono-data border border-border-hairline rounded overflow-hidden">
      <button
        v-for="t in tabs" :key="t.key"
        class="px-component py-1 transition-colors outline-none focus-visible:ring-1 focus-visible:ring-inset focus-visible:ring-primary/30"
        :class="tab === t.key ? 'bg-surface-hover text-text-primary' : 'text-text-muted hover:text-text-primary'"
        @click="tab = t.key"
      >{{ t.label }} <span class="text-text-faint">{{ t.count }}</span></button>
    </div>
  </div>

  <div v-if="tab === 'unprocessed'" class="px-section pt-base">
    <section class="bg-surface-overlay border border-border-default rounded overflow-hidden">
      <div class="flex items-center justify-between gap-component px-component py-base border-b border-border-hairline">
        <div class="flex items-center gap-base min-w-0">
          <span class="material-symbols-outlined !text-body text-text-faint">inbox</span>
          <h2 class="font-headline text-headline text-text-primary truncate">Capture</h2>
        </div>
        <div class="flex items-center gap-tight font-mono-data text-mono-data border border-border-hairline rounded overflow-hidden">
          <button
            v-for="m in entryModes" :key="m.key"
            class="px-component py-1 transition-colors outline-none focus-visible:ring-1 focus-visible:ring-inset focus-visible:ring-primary/30"
            :class="entryMode === m.key ? 'bg-surface-hover text-text-primary' : 'text-text-muted hover:text-text-primary'"
            @click="entryMode = m.key"
          >{{ m.label }}</button>
        </div>
      </div>
      <div class="p-component">
        <CaptureThought v-if="entryMode === 'quick'" embedded @submit="() => refresh()" />
        <InboxBatchDump v-else @created="onBatchCreated" />
      </div>
    </section>
  </div>

  <section v-if="tab === 'unprocessed'" class="flex-1 overflow-y-auto relative">
    <div class="flex flex-col gap-base px-section py-base pb-[120px]">
      <template v-for="group in inboxGroups" :key="group.id">
        <div v-if="group.kind === 'batch'" class="border border-border-hairline bg-surface rounded-lg overflow-hidden">
          <div class="flex items-center gap-base px-component py-base border-b border-border-hairline">
            <span class="font-mono-data text-mono-data px-tight border border-border-hairline text-text-faint bg-surface-container-low">BATCH</span>
            <div class="flex flex-col flex-1 min-w-0">
              <h3 class="font-headline text-text-primary truncate">{{ group.title }}</h3>
              <p class="font-mono-data text-mono-data text-text-muted truncate">{{ group.source }} · {{ group.items.length }} captures</p>
            </div>
            <button class="font-mono-data text-mono-data text-text-muted hover:text-text-primary rounded outline-none focus-visible:ring-1 focus-visible:ring-primary/30" @click="toggleBatchLog(group.id)">
              {{ openBatchLogs.has(group.id) ? 'Hide compact log' : 'Compact log' }}
            </button>
          </div>
          <div v-if="openBatchLogs.has(group.id)" class="px-component py-base border-b border-border-hairline bg-bg-base">
            <div
              v-for="item in group.items"
              :key="`log-${item.id}`"
              class="grid grid-cols-[44px_minmax(0,1fr)] gap-base py-tight font-body text-body"
            >
              <span class="font-mono-data text-mono-data text-text-faint">{{ itemBatchTime(item) || `#${(item.batchSequence ?? 0) + 1}` }}</span>
              <span class="text-text-muted truncate">{{ item.subtitle || item.title }}</span>
            </div>
          </div>
          <div class="flex flex-col gap-base p-base">
            <template v-for="item in group.items" :key="item.id">
              <InboxItem
                :item="item"
                :chips="chipsFor(item)"
                :selected="selected.has(item.id)"
                :isCursor="cursor === itemIndex(item.id)"
                :prepping="preppingId === item.id"
                @select="cursor = itemIndex(item.id)"
                @toggleSelect="toggleSelect(item.id)"
                @accept="accept(item)"
                @edit="openModal(item)"
                @discard="discard(item)"
                @prep="triggerPrep(item)"
                @peek="togglePeek(item)"
              />
              <div v-if="peekId === item.id" class="mx-component -mt-tight mb-tight px-component py-base border border-t-0 border-da-accent/30 bg-da-accent/[0.04]">
                <div class="flex items-center gap-tight mb-tight">
                  <span class="font-mono-data text-mono-data text-da-accent-text">DA briefing</span>
                  <button class="ml-auto font-mono-data text-mono-data text-text-muted hover:text-text-primary rounded outline-none focus-visible:ring-1 focus-visible:ring-primary/30" @click="peekId = null">close</button>
                </div>
                <p class="font-body text-body text-text-primary whitespace-pre-wrap">{{ peekBody || '…' }}</p>
              </div>
            </template>
          </div>
        </div>
        <template v-else>
          <InboxItem
            v-for="item in group.items"
            :key="item.id"
            :item="item"
            :chips="chipsFor(item)"
            :selected="selected.has(item.id)"
            :isCursor="cursor === itemIndex(item.id)"
            :prepping="preppingId === item.id"
            @select="cursor = itemIndex(item.id)"
            @toggleSelect="toggleSelect(item.id)"
            @accept="accept(item)"
            @edit="openModal(item)"
            @discard="discard(item)"
            @prep="triggerPrep(item)"
            @peek="togglePeek(item)"
          />
          <div v-for="item in group.items" v-show="peekId === item.id" :key="`peek-${item.id}`" class="mx-component -mt-tight mb-tight px-component py-base border border-t-0 border-da-accent/30 bg-da-accent/[0.04]">
            <div class="flex items-center gap-tight mb-tight">
              <span class="font-mono-data text-mono-data text-da-accent-text">DA briefing</span>
              <button class="ml-auto font-mono-data text-mono-data text-text-muted hover:text-text-primary rounded outline-none focus-visible:ring-1 focus-visible:ring-primary/30" @click="peekId = null">close</button>
            </div>
            <p class="font-body text-body text-text-primary whitespace-pre-wrap">{{ peekBody || '…' }}</p>
          </div>
        </template>
      </template>

      <UiErrorState
        v-if="error"
        title="Could not load inbox."
        message="Check that the Kernl API is running, then retry."
        :detail="error?.message ?? null"
        @retry="refresh"
      />
      <UiEmptyState
        v-if="!error && !pending && items.length === 0"
        icon="inbox"
        title="Inbox is empty."
        body="Captured thoughts and incoming material appear here before you turn them into notes, tasks, or bookmarks."
      />
    </div>

    <!-- batch bar -->
    <div
      v-if="selected.size > 0"
      class="absolute bottom-section left-1/2 -translate-x-1/2 flex items-center gap-component bg-surface-container-high border border-primary/40 px-section py-base rounded"
    >
      <span class="font-mono-data text-mono-data text-text-primary">{{ selected.size }} selected</span>
      <div class="w-[1px] h-4 bg-border-hairline"></div>
      <button class="font-mono-data text-mono-data text-status-passed hover:underline rounded outline-none focus-visible:ring-1 focus-visible:ring-primary/30" @click="acceptSelected">Process all (accept DA)</button>
      <button class="font-mono-data text-mono-data text-status-failed-text hover:underline rounded outline-none focus-visible:ring-1 focus-visible:ring-primary/30" @click="discardSelected">Discard all</button>
      <button class="font-mono-data text-mono-data text-text-muted hover:text-text-primary rounded outline-none focus-visible:ring-1 focus-visible:ring-primary/30" @click="selected.clear()">Clear</button>
    </div>

    <InboxHint v-else-if="items.length > 0" />

    <!-- Update merge review: accept/reject the changes Kernl proposes folding
         into the matched note. Deciding the last hunk applies the accepted set
         and triages the capture. -->
    <DiffSuggest
      v-if="merge"
      :hunks="merge.hunks"
      @accept="onMergeAccept"
      @reject="onMergeReject"
    />
  </section>

  <section v-else class="flex-1 overflow-y-auto">
    <div class="flex flex-col gap-base px-section py-base">
      <div
        v-for="p in processed"
        :key="p.captureId"
        class="flex items-center gap-component p-component border-b border-border-hairline"
      >
        <span class="material-symbols-outlined !text-[16px] shrink-0" :class="becameText(leadType(p))">{{ becameIcon(leadType(p)) }}</span>
        <div class="flex flex-col flex-1 min-w-0">
          <!-- One capture is routinely several nodes. Show all of them: a capture
               that became 6 nodes and reads as 1 looks like data loss. -->
          <div class="flex items-center gap-base flex-wrap">
            <span v-if="p.discarded" class="font-mono-data text-mono-data text-text-muted">Discarded</span>
            <span
              v-for="node in p.became"
              :key="node.id"
              class="font-mono-data text-mono-data"
              :class="becameText(node.type)"
            >{{ becameLabel(node) }}</span>
          </div>
          <h3 class="font-headline text-text-primary truncate" :class="p.discarded ? 'line-through text-text-muted' : ''">{{ p.title }}</h3>
        </div>
        <button class="shrink-0 font-mono-data text-mono-data text-text-muted hover:text-primary transition-colors rounded outline-none focus-visible:ring-1 focus-visible:ring-primary/30" @click="reopenCapture(p.captureId)">↩ Undo</button>
      </div>

      <UiErrorState
        v-if="processedError"
        title="Could not load processed items."
        message="Check that the Kernl API is running, then retry."
        :detail="processedError?.message ?? null"
        @retry="refreshProcessed"
      />
      <UiEmptyState
        v-if="!processedError && processed.length === 0"
        icon="history"
        title="Nothing processed yet."
        body="Processed captures appear here with a short undo window."
      />
    </div>
  </section>

  <!-- override modal -->
  <ProcessModal :item="modalItem" :projects="projects" :busy="busy" @close="modalItem = null" @confirm="confirmModal" />

  <UiToast
    :message="toast"
    :action-label="undoStack.length > 0 ? 'Undo (U)' : ''"
    icon="history"
    @action="undo"
  />
</template>

<script setup lang="ts">
import { ref, computed, reactive, watch, onMounted, onUnmounted } from 'vue'
import InboxHeader from '~/components/inbox/InboxHeader.vue'
import InboxItem from '~/components/inbox/InboxItem.vue'
import InboxHint from '~/components/inbox/InboxHint.vue'
import CaptureThought from '~/components/CaptureThought.vue'
import InboxBatchDump from '~/components/inbox/InboxBatchDump.vue'
import ProcessModal from '~/components/inbox/ProcessModal.vue'
import DiffSuggest from '~/components/notes/DiffSuggest.vue'
import type { InboxItemData } from '~/components/inbox/InboxItem.vue'
import { useProjects } from '~/composables/useProjects'
import {
  TARGET_META,
  hasConflictingUpdate,
  isUpdateOnly,
  normalizeActions,
  type CaptureAction,
  type Target,
} from '~/utils/inboxTargets'
import type { SuggestionChip } from '~/components/inbox/InboxItem.vue'
import UiEmptyState from '~/components/ui/UiEmptyState.vue'
import UiErrorState from '~/components/ui/UiErrorState.vue'
import UiToast from '~/components/ui/UiToast.vue'

interface ProcessedNode {
  id: string
  type: string
  title: string
  projectId: string
}

interface ProcessedRow {
  captureId: string
  title: string
  /** every node the capture became — a fan-out is several, not one */
  became: ProcessedNode[]
  discarded: boolean
  at: string
}

interface ProcessPayload {
  /** the nodes this capture becomes — one capture is routinely several things */
  actions: CaptureAction[]
  targetNoteId?: string
  acceptedHunks?: { id: string; content: string }[]
  /** the DA's proposal, echoed back so the backend can learn from the edits */
  suggestedActions?: CaptureAction[]
}

interface InboxGroup {
  id: string
  kind: 'single' | 'batch'
  title: string
  source: string
  items: InboxItemData[]
}

const { data, pending, refresh, error } = useFetch<InboxItemData[]>('/api/inbox/pending', { server: false, default: () => [] })
const { data: processedData, refresh: refreshProcessed, error: processedError } = useFetch<ProcessedRow[]>('/api/inbox/processed', { server: false, default: () => [] })
const { projects, load: loadProjects } = useProjects()

const items = computed(() => data.value || [])
const processed = computed(() => processedData.value || [])
const flaggedCount = computed(() => items.value.filter(i => actionsFor(i).length > 0).length)

const tab = ref<'unprocessed' | 'processed'>('unprocessed')
const tabs = computed(() => [
  { key: 'unprocessed' as const, label: 'Unprocessed', count: items.value.length },
  { key: 'processed' as const, label: 'Processed', count: processed.value.length },
])
const entryMode = ref<'quick' | 'batch'>('quick')
const entryModes = [
  { key: 'quick' as const, label: 'Quick capture' },
  { key: 'batch' as const, label: 'Batch dump' },
]

const cursor = ref(0)
const selected = reactive(new Set<string>())
const busy = ref(false)
const modalItem = ref<InboxItemData | null>(null)

// Undo stack of processed capture ids (most recent last); backs U / the toast.
const undoStack = ref<string[]>([])
const toast = ref('')
let toastTimer: ReturnType<typeof setTimeout> | null = null
function showToast(msg: string) {
  toast.value = msg
  if (toastTimer) clearTimeout(toastTimer)
  toastTimer = setTimeout(() => (toast.value = ''), 6000)
}

const projectTitle = (id?: string) => projects.value.find(p => p.id === id)?.title || ''

/** the nodes the DA proposes this capture becomes; empty while unclassified */
function actionsFor(item: InboxItemData): CaptureAction[] {
  return normalizeActions(item.suggestedActions)
}

// chipsFor renders the proposal for the row: null while the DA is still
// classifying, otherwise one chip per node the capture will become.
function chipsFor(item: InboxItemData): SuggestionChip[] | null {
  const actions = actionsFor(item)
  if (actions.length === 0) return null
  return actions.map(action => ({ target: action.target, label: chipLabel(action) }))
}

function chipLabel(action: CaptureAction): string {
  const base = TARGET_META[action.target].label
  if (action.target === 'task') {
    const parent = projectTitle(action.projectId)
    return parent ? `Task · ${parent}` : 'Task · Unprocessed'
  }
  if (action.target === 'project') {
    const count = action.initialTasks?.length || 0
    return count ? `Project · ${count} tasks` : 'Project'
  }
  return base
}

const openBatchLogs = reactive(new Set<string>())

const inboxGroups = computed<InboxGroup[]>(() => {
  const out: InboxGroup[] = []
  const byBatch = new Map<string, InboxGroup>()
  for (const item of items.value) {
    if (!item.batchId) {
      out.push({ id: item.id, kind: 'single', title: item.title, source: item.type || 'CAPTURE', items: [item] })
      continue
    }
    let group = byBatch.get(item.batchId)
    if (!group) {
      group = {
        id: item.batchId,
        kind: 'batch',
        title: item.batchContextTitle || 'Batch dump',
        source: (item.batchSource || item.type || 'batch').toUpperCase(),
        items: [],
      }
      byBatch.set(item.batchId, group)
      out.push(group)
    }
    group.items.push(item)
  }
  for (const group of out) {
    if (group.kind === 'batch') {
      group.items.sort((a, b) => (a.batchSequence ?? 0) - (b.batchSequence ?? 0))
    }
  }
  return out
})
const orderedItems = computed(() => inboxGroups.value.flatMap(group => group.items))

function itemIndex(id: string): number {
  const idx = orderedItems.value.findIndex(i => i.id === id)
  return idx >= 0 ? idx : 0
}

function itemBatchTime(item: InboxItemData): string {
  const match = (item.batchTimestamp || '').match(/\b\d{1,2}:\d{2}(?::\d{2})?\b/)
  return match ? match[0].slice(0, 5) : ''
}

function toggleBatchLog(id: string) {
  if (openBatchLogs.has(id)) openBatchLogs.delete(id)
  else openBatchLogs.add(id)
}

async function onBatchCreated(batchId: string) {
  if (batchId) openBatchLogs.add(batchId)
  await refresh()
  showToast('Batch captures created')
}

// ---- processing ----
async function postProcess(id: string, payload: ProcessPayload) {
  await $fetch(`/api/inbox/${id}/process`, { method: 'POST', body: payload })
  if (data.value) data.value = data.value.filter(i => i.id !== id)
  selected.delete(id)
  if (cursor.value >= orderedItems.value.length) cursor.value = Math.max(0, orderedItems.value.length - 1)
}

// accept takes the DA's whole proposal as-is: every node it suggested, in one
// post. Nothing is dropped just because the capture turned out to be two things.
async function accept(item: InboxItemData, quiet = false) {
  const actions = actionsFor(item)
  if (actions.length === 0) return
  // Update is never one-click: it opens the merge review so the user vets every
  // change before it touches a note.
  if (isUpdateOnly(actions)) { await startCaptureMerge(item); return }
  busy.value = true
  try {
    await postProcess(item.id, { actions, suggestedActions: actions })
    undoStack.value.push(item.id)
    await refreshProcessed()
    if (!quiet) showToast(processedToast(item, actions))
  } catch (e) { console.error('accept failed', e) } finally { busy.value = false }
}

function processedToast(item: InboxItemData, actions: CaptureAction[]): string {
  if (actions.length > 1) return `Processed into ${actions.length} nodes: ${item.title}`
  return `Processed: ${item.title}`
}

async function discard(item: InboxItemData, quiet = false) {
  busy.value = true
  try {
    await postProcess(item.id, { actions: [{ target: 'discard', title: item.title }] })
    undoStack.value.push(item.id)
    await refreshProcessed()
    if (!quiet) showToast(`Discarded: ${item.title}`)
  } catch (e) { console.error('discard failed', e) } finally { busy.value = false }
}

async function acceptSelected() {
  // Update needs an interactive review, so it is excluded from batch accept;
  // those rows stay for one-by-one handling.
  const targets = items.value.filter(i => {
    const actions = actionsFor(i)
    return selected.has(i.id) && actions.length > 0 && !isUpdateOnly(actions)
  })
  for (const i of targets) await accept(i, true)
  showToast(`Processed ${targets.length} — U to undo last`)
}
async function discardSelected() {
  const targets = items.value.filter(i => selected.has(i.id))
  for (const i of targets) await discard(i, true)
  showToast(`Discarded ${targets.length} — U to undo last`)
}

function openModal(item: InboxItemData) { modalItem.value = item }

async function confirmModal(payload: { actions: CaptureAction[] }) {
  const item = modalItem.value
  if (!item || payload.actions.length === 0) return
  // An update is reviewed hunk by hunk against one note, so it cannot be one leg
  // of a fan-out (the modal blocks that) and it hands off to the merge review.
  if (hasConflictingUpdate(payload.actions)) return
  if (isUpdateOnly(payload.actions)) {
    modalItem.value = null
    await startCaptureMerge(item)
    return
  }
  busy.value = true
  try {
    // Echo the DA's original proposal so the backend can learn from the edits.
    await postProcess(item.id, { actions: payload.actions, suggestedActions: actionsFor(item) })
    undoStack.value.push(item.id)
    await refreshProcessed()
    modalItem.value = null
    showToast(processedToast(item, payload.actions))
  } catch (e) { console.error('process failed', e) } finally { busy.value = false }
}

// ---- update merge review ----
interface MergeHunk { id: string; content: string }
interface MergePlan { targetNoteId: string; targetTitle: string; currentBody: string; hunks: MergeHunk[] }
interface MergeState { captureId: string; captureTitle: string; targetNoteId: string; targetTitle: string; hunks: MergeHunk[]; accepted: MergeHunk[] }

const merge = ref<MergeState | null>(null)

// startCaptureMerge plans the merge. With no confident target it falls back to a
// plain note; otherwise it opens DiffSuggest for accept/reject.
async function startCaptureMerge(item: InboxItemData) {
  try {
    const plan = await $fetch<MergePlan>(`/api/inbox/${item.id}/merge-plan`, { method: 'POST' })
    if (!plan || !plan.targetNoteId) {
      busy.value = true
      try {
        await postProcess(item.id, { actions: [{ target: 'note', title: item.title }] })
        undoStack.value.push(item.id)
        await refreshProcessed()
        showToast(`No matching note — saved as a note: ${item.title}`)
      } finally { busy.value = false }
      return
    }
    merge.value = {
      captureId: item.id,
      captureTitle: item.title,
      targetNoteId: plan.targetNoteId,
      targetTitle: plan.targetTitle,
      hunks: [...(plan.hunks || [])],
      accepted: []
    }
    if (merge.value.hunks.length === 0) await finalizeMerge()
  } catch (e) { console.error('merge plan failed', e) }
}

function onMergeAccept(hunk: MergeHunk) {
  if (!merge.value) return
  merge.value.accepted.push(hunk)
  merge.value.hunks = merge.value.hunks.filter(h => h.id !== hunk.id)
  if (merge.value.hunks.length === 0) finalizeMerge()
}

function onMergeReject(hunk: MergeHunk) {
  if (!merge.value) return
  merge.value.hunks = merge.value.hunks.filter(h => h.id !== hunk.id)
  if (merge.value.hunks.length === 0) finalizeMerge()
}

// finalizeMerge applies the accepted hunks. Accepting none is valid — the note
// is left unchanged but the capture is still triaged into it.
async function finalizeMerge() {
  if (!merge.value) return
  const { captureId, captureTitle, targetNoteId, targetTitle, accepted } = merge.value
  merge.value = null
  busy.value = true
  try {
    await postProcess(captureId, {
      actions: [{ target: 'update', title: '' }],
      targetNoteId,
      acceptedHunks: accepted,
    })
    undoStack.value.push(captureId)
    await refreshProcessed()
    showToast(`Merged into ${targetTitle || 'note'}: ${captureTitle}`)
  } catch (e) { console.error('merge apply failed', e) } finally { busy.value = false }
}

function toggleSelect(id: string) {
  if (selected.has(id)) selected.delete(id)
  else selected.add(id)
}

// ---- DA prep (briefing) ----
const peekId = ref<string | null>(null)
const peekBody = ref('')
const preppingId = ref<string | null>(null)

async function triggerPrep(item: InboxItemData) {
  preppingId.value = item.id
  try {
    const res = await $fetch<{ body: string }>(`/api/inbox/${item.id}/prep`, { method: 'POST' })
    peekId.value = item.id
    peekBody.value = res.body
    await refresh()
  } catch (e) { console.error('prep failed', e) } finally { preppingId.value = null }
}

async function togglePeek(item: InboxItemData) {
  if (peekId.value === item.id) { peekId.value = null; return }
  peekId.value = item.id
  peekBody.value = ''
  try {
    const res = await $fetch<{ body: string }>(`/api/inbox/${item.id}/prep`)
    peekBody.value = res.body
  } catch (e) { console.error('peek failed', e); peekBody.value = '(no briefing)' }
}

// ---- undo ----
async function reopenCapture(id: string) {
  try {
    await $fetch(`/api/inbox/${id}/reopen`, { method: 'POST' })
    const i = undoStack.value.indexOf(id)
    if (i >= 0) undoStack.value.splice(i, 1)
    await Promise.all([refresh(), refreshProcessed()])
  } catch (e) { console.error('reopen failed', e) }
}
async function undo() {
  const id = undoStack.value.pop()
  if (!id) return
  await reopenCapture(id)
  showToast('Restored to inbox')
}

// ---- processed chip helpers ----
const knownTarget = (b: string): b is Target => b in TARGET_META
const becameIcon = (b: string) => (knownTarget(b) ? TARGET_META[b].icon : 'help')
const becameText = (b: string) => (knownTarget(b) ? TARGET_META[b].text : 'text-text-muted')
// The row's icon follows the first node the capture became; the chips below it
// name them all.
const leadType = (p: ProcessedRow) => (p.discarded ? 'discard' : (p.became[0]?.type ?? ''))
function becameLabel(node: ProcessedNode): string {
  const base = knownTarget(node.type) ? TARGET_META[node.type].label : node.type
  return node.projectId ? `${base} · ${projectTitle(node.projectId)}` : base
}

// ---- keyboard ----
function onKey(e: KeyboardEvent) {
  const tg = e.target as HTMLElement | null
  if (tg && (tg.tagName === 'INPUT' || tg.tagName === 'TEXTAREA' || tg.tagName === 'SELECT')) return
  if (modalItem.value) return
  if (e.key === 'u' || e.key === 'U') { undo(); return }
  if (tab.value !== 'unprocessed' || orderedItems.value.length === 0) return
  const item = orderedItems.value[cursor.value]
  if (e.key === 'ArrowDown') { e.preventDefault(); cursor.value = (cursor.value + 1) % orderedItems.value.length }
  else if (e.key === 'ArrowUp') { e.preventDefault(); cursor.value = (cursor.value - 1 + orderedItems.value.length) % orderedItems.value.length }
  else if (e.key === 'Enter') { if (item) actionsFor(item).length > 0 ? accept(item) : openModal(item) }
  else if (e.key === 'e' || e.key === 'E') { if (item) openModal(item) }
  else if (e.key === 'd' || e.key === 'D') { if (item) discard(item) }
  else if (e.key === 'x' || e.key === 'X') { if (item) toggleSelect(item.id) }
}

// ---- classification polling ----
// The DA classifies captures in a background loop with no push channel, so poll
// while any item is still awaiting a suggestion; stop once all are classified
// or the tab is hidden, and resume on focus.
const AWAITING = (i: InboxItemData) => actionsFor(i).length === 0
const hasPendingClassification = computed(() => items.value.some(AWAITING))
let pollTimer: ReturnType<typeof setInterval> | null = null

function stopPolling() {
  if (pollTimer) { clearInterval(pollTimer); pollTimer = null }
}
function startPolling() {
  if (pollTimer || typeof document === 'undefined' || document.hidden) return
  pollTimer = setInterval(async () => {
    if (document.hidden || !hasPendingClassification.value) { stopPolling(); return }
    await refresh()
  }, 3000)
}

watch(hasPendingClassification, (pending) => {
  if (pending) startPolling()
  else stopPolling()
})

function onVisibility() {
  if (document.hidden) stopPolling()
  else if (hasPendingClassification.value) { refresh(); startPolling() }
}

onMounted(() => {
  loadProjects()
  window.addEventListener('keydown', onKey)
  document.addEventListener('visibilitychange', onVisibility)
  if (hasPendingClassification.value) startPolling()
})
onUnmounted(() => {
  window.removeEventListener('keydown', onKey)
  document.removeEventListener('visibilitychange', onVisibility)
  stopPolling()
})
</script>
