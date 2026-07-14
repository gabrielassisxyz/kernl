<template>
  <InboxHeader :totalCount="items.length" :flaggedCount="flaggedCount" />

  <div class="px-section pt-base flex items-center justify-between">
    <div class="flex items-center gap-tight font-mono-data text-mono-data border border-border-hairline rounded overflow-hidden">
      <button
        v-for="t in tabs" :key="t.key"
        class="px-component py-1 transition-colors outline-none focus-visible:ring-1 focus-visible:ring-inset focus-visible:ring-primary/30 cursor-pointer"
        :class="tab === t.key ? 'bg-surface-hover text-text-primary' : 'text-text-muted hover:text-text-primary'"
        @click="tab = t.key"
      >{{ t.label }} <span class="text-text-faint">{{ t.count }}</span></button>
    </div>
    <div class="flex items-center gap-component">
      <!-- Focus is a mode, not the front door: the list stays the way in. -->
      <button
        v-if="tab === 'unprocessed' && (items.length > 0 || focus)"
        class="flex items-center gap-tight px-base py-1 rounded border font-mono-data text-mono-data transition-colors outline-none focus-visible:ring-1 focus-visible:ring-primary/30 cursor-pointer"
        :class="focus ? 'border-primary/40 text-primary bg-primary/10' : 'border-border-hairline text-text-muted hover:text-text-primary hover:border-border-default'"
        :aria-pressed="focus"
        @click="focus = !focus"
      >
        <span class="material-symbols-outlined !text-body leading-none" aria-hidden="true">center_focus_strong</span>
        Focus
        <!-- The key rides the button's own colour at lower emphasis: a fixed grey
             would sink into the active state, which is the one you look at. -->
        <span class="opacity-60">F</span>
      </button>
    </div>
  </div>

  <div v-if="tab === 'unprocessed' && !focus" class="px-section pt-base">
    <section class="bg-surface-overlay border border-border-default rounded overflow-hidden">
      <div class="flex items-center justify-between gap-component px-component py-base border-b border-border-hairline">
        <div class="flex items-center gap-base min-w-0">
          <span class="material-symbols-outlined !text-body text-text-faint">inbox</span>
          <h2 class="font-headline text-headline text-text-primary truncate">Capture</h2>
        </div>
        <div class="flex items-center gap-tight font-mono-data text-mono-data border border-border-hairline rounded overflow-hidden">
          <button
            v-for="m in entryModes" :key="m.key"
            class="px-component py-1 transition-colors outline-none focus-visible:ring-1 focus-visible:ring-inset focus-visible:ring-primary/30 cursor-pointer"
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
    <!-- One capture at a time, hands on the keyboard. Same pile, same order, same
         write path — only the unit of attention changes. -->
    <InboxFocusCard
      v-if="focus"
      :items="orderedItems"
      :projects="projects"
      :busy="busy"
      @process="processCapture($event.item, $event.actions)"
      @discard="discard($event)"
      @undo="undo"
      @close="focus = false"
    />

    <template v-else>
    <div class="flex flex-col gap-base px-section py-base">
      <template v-for="group in inboxGroups" :key="group.id">
        <!-- Not clipped: a row's type dropdown opens past the card's edge. -->
        <div v-if="group.kind === 'batch'" class="border border-border-hairline bg-surface-container-lowest rounded-lg">
          <div class="flex items-center gap-base px-component py-base border-b border-border-hairline">
            <span class="font-mono-data text-mono-data px-tight border border-border-hairline text-text-faint bg-surface-container-low">BATCH</span>
            <div class="flex flex-col flex-1 min-w-0">
              <h3 class="font-headline text-text-primary truncate">{{ group.title }}</h3>
              <p class="font-mono-data text-mono-data text-text-muted truncate">{{ group.source }} · {{ group.items.length }} captures</p>
            </div>
          </div>
          <div class="flex flex-col gap-base p-base">
            <template v-for="item in group.items" :key="item.id">
              <InboxRow
                :item="item"
                :proposals="proposalsFor(item)"
                :selected="selected.has(item.id)"
                :isCursor="cursor === itemIndex(item.id)"
                :expanded="openId === item.id"
                :prepping="preppingId === item.id"
                @toggle="toggleDrawer(item)"
                @toggleSelect="toggleSelect(item.id)"
                @accept="accept(item)"
                @edit="openModal(item)"
                @discard="discard(item)"
                @prep="triggerPrep(item)"
                @peek="togglePeek(item)"
              >
                <template #drawer>
                  <InboxPeek v-if="peekId === item.id" :body="peekBody" @close="peekId = null" />
                  <Transition name="drawer">
                    <InboxDrawer
                      v-if="openId === item.id"
                      :item="item"
                      :projects="projects"
                      :busy="busy"
                      @process="confirmDrawer(item, $event)"
                      @edit="openModal(item)"
                      @discard="discard(item)"
                      @close="openId = null"
                    />
                  </Transition>
                </template>
              </InboxRow>
            </template>
          </div>
        </div>

        <template v-else>
          <InboxRow
            v-for="item in group.items"
            :key="item.id"
            :item="item"
            :proposals="proposalsFor(item)"
            :selected="selected.has(item.id)"
            :isCursor="cursor === itemIndex(item.id)"
            :expanded="openId === item.id"
            :prepping="preppingId === item.id"
            @toggle="toggleDrawer(item)"
            @toggleSelect="toggleSelect(item.id)"
            @accept="accept(item)"
            @edit="openModal(item)"
            @discard="discard(item)"
            @prep="triggerPrep(item)"
            @peek="togglePeek(item)"
          >
            <template #drawer>
              <InboxPeek v-if="peekId === item.id" :body="peekBody" @close="peekId = null" />
              <Transition name="drawer">
                <InboxDrawer
                  v-if="openId === item.id"
                  :item="item"
                  :projects="projects"
                  :busy="busy"
                  @process="confirmDrawer(item, $event)"
                  @edit="openModal(item)"
                  @discard="discard(item)"
                  @close="openId = null"
                />
              </Transition>
            </template>
          </InboxRow>
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

    <!-- The bar rides the scroll. Absolute inside the scroller pinned it to the
         list's box, not to what you are looking at, so it drifted mid-screen. -->
    <div
      v-if="items.length > 0"
      class="sticky bottom-0 z-dropdown flex justify-center px-section pt-component pb-section pointer-events-none"
    >
      <div
        v-if="selected.size > 0"
        class="pointer-events-auto flex items-center gap-component bg-surface-container-high border border-primary/40 px-section py-base rounded shadow-lg"
      >
        <span class="font-mono-data text-mono-data text-text-primary">{{ selected.size }} selected</span>
        <div class="w-[1px] h-4 bg-border-hairline"></div>
        <button class="font-mono-data text-mono-data text-status-passed hover:underline rounded outline-none focus-visible:ring-1 focus-visible:ring-primary/30 cursor-pointer" @click="acceptSelected">Process all (accept DA)</button>
        <button class="font-mono-data text-mono-data text-status-failed-text hover:underline rounded outline-none focus-visible:ring-1 focus-visible:ring-primary/30 cursor-pointer" @click="discardSelected">Discard all</button>
        <button class="font-mono-data text-mono-data text-text-muted hover:text-text-primary rounded outline-none focus-visible:ring-1 focus-visible:ring-primary/30 cursor-pointer" @click="selected.clear()">Clear</button>
      </div>

      <InboxHint v-else class="pointer-events-auto" />
    </div>
    </template>

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
        <button class="shrink-0 font-mono-data text-mono-data text-text-muted hover:text-primary transition-colors rounded outline-none focus-visible:ring-1 focus-visible:ring-primary/30 cursor-pointer" @click="reopenCapture(p.captureId)">↩ Undo</button>
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

  <!-- The deep editor: retitle, refile, rewrite a description, add or drop a
       node. The drawer is for reading and retyping; this is for rework. -->
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
import InboxRow from '~/components/inbox/InboxRow.vue'
import InboxDrawer from '~/components/inbox/InboxDrawer.vue'
import InboxFocusCard from '~/components/inbox/InboxFocusCard.vue'
import InboxPeek from '~/components/inbox/InboxPeek.vue'
import InboxHint from '~/components/inbox/InboxHint.vue'
import ProcessModal from '~/components/inbox/ProcessModal.vue'
import CaptureThought from '~/components/CaptureThought.vue'
import InboxBatchDump from '~/components/inbox/InboxBatchDump.vue'
import DiffSuggest from '~/components/notes/DiffSuggest.vue'
import type { InboxItemData, Proposal } from '~/components/inbox/InboxRow.vue'
import { useProjects } from '~/composables/useProjects'
import {
  TARGET_META,
  applySourceContext,
  captureProvenance,
  hasConflictingUpdate,
  isUpdateOnly,
  normalizeActions,
  type CaptureAction,
  type Target,
} from '~/utils/inboxTargets'
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
/** the one capture whose drawer is open: reading a capture is a focused act */
const openId = ref<string | null>(null)
/** triage one capture at a time, keyboard only — a mode, never the default */
const focus = ref(false)
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

// proposalsFor renders the DA's proposal as the row's headline: null while it is
// still classifying, otherwise one line per node the capture becomes.
function proposalsFor(item: InboxItemData): Proposal[] | null {
  const actions = actionsFor(item)
  if (actions.length === 0) return null
  return actions.map(action => ({
    target: action.target,
    title: proposalTitle(action, item),
    projectLabel: proposalProject(action),
    dueLabel: formatDueDate(action.dueDate),
    tags: action.tags || [],
  }))
}

function proposalTitle(action: CaptureAction, item: InboxItemData): string {
  const title = (action.title || '').trim()
  if (title) return title
  if (action.target === 'update') return 'Merge into the matching note'
  if (action.target === 'discard') return 'Drop this fragment'
  return item.title
}

// A task with no project is just a task — "Unprocessed" is the bucket it lands
// in, not a fact worth spending a line on.
function proposalProject(action: CaptureAction): string {
  if (action.target === 'task') {
    const parent = projectTitle(action.projectId)
    return parent ? `· ${parent}` : ''
  }
  if (action.target === 'project') {
    const count = action.initialTasks?.length || 0
    return count ? `· ${count} tasks` : ''
  }
  return ''
}

// "2026-04-02" → "Apr 2". The year is noise for a deadline you are triaging now.
function formatDueDate(due?: string): string {
  if (!due) return ''
  const [y, m, d] = due.split('-').map(Number)
  if (!y || !m || !d) return due
  return new Date(y, m - 1, d).toLocaleDateString('en-US', { month: 'short', day: 'numeric' })
}

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

async function onBatchCreated() {
  await refresh()
  showToast('Batch captures created')
}

// ---- the drawer ----
function toggleDrawer(item: InboxItemData) {
  cursor.value = itemIndex(item.id)
  openId.value = openId.value === item.id ? null : item.id
}

// ---- processing ----
async function postProcess(id: string, payload: ProcessPayload) {
  await $fetch(`/api/inbox/${id}/process`, { method: 'POST', body: payload })
  if (data.value) data.value = data.value.filter(i => i.id !== id)
  selected.delete(id)
  if (openId.value === id) openId.value = null
  if (cursor.value >= orderedItems.value.length) cursor.value = Math.max(0, orderedItems.value.length - 1)
}

// A fanned-out task owns only its fragment of the capture; carry the capture in
// as its source context, exactly as the drawer shows it, so one-click Accept and
// an edited Process write the same task.
function withContext(actions: CaptureAction[], item: InboxItemData): CaptureAction[] {
  return applySourceContext(
    actions,
    item.subtitle || item.title || '',
    captureProvenance(item.batchSource || item.type, item.batchTimestamp),
  )
}

// accept takes the DA's whole proposal as-is: every node it suggested, in one
// post. Nothing is dropped just because the capture turned out to be two things.
const accept = (item: InboxItemData, quiet = false) => processCapture(item, actionsFor(item), quiet)

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

function openModal(item: InboxItemData) {
  modalItem.value = item
}

// The drawer, the modal and one-click Accept all land here, so a capture writes
// the same nodes whichever surface triaged it.
async function processCapture(item: InboxItemData, actions: CaptureAction[], quiet = false) {
  if (actions.length === 0) return
  // An update is reviewed hunk by hunk against one note, so it cannot be one leg
  // of a fan-out (both surfaces block that) and it hands off to the merge review.
  if (hasConflictingUpdate(actions)) return
  if (isUpdateOnly(actions)) {
    openId.value = null
    modalItem.value = null
    await startCaptureMerge(item)
    return
  }
  busy.value = true
  try {
    // Echo the DA's original proposal so the backend can learn from the edits.
    await postProcess(item.id, { actions: withContext(actions, item), suggestedActions: actionsFor(item) })
    undoStack.value.push(item.id)
    await refreshProcessed()
    modalItem.value = null
    if (!quiet) showToast(processedToast(item, actions))
  } catch (e) { console.error('process failed', e) } finally { busy.value = false }
}

const confirmDrawer = (item: InboxItemData, payload: { actions: CaptureAction[] }) =>
  processCapture(item, payload.actions)

function confirmModal(payload: { actions: CaptureAction[] }) {
  const item = modalItem.value
  if (item) processCapture(item, payload.actions)
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
  // Focus mode owns every key while it is up, including Escape and U.
  if (focus.value) return
  if (e.key === 'u' || e.key === 'U') { undo(); return }
  if (tab.value !== 'unprocessed' || orderedItems.value.length === 0) return
  if (e.key === 'f' || e.key === 'F') { e.preventDefault(); focus.value = true; return }
  const item = orderedItems.value[cursor.value]
  if (e.key === 'Escape' && openId.value) { e.preventDefault(); openId.value = null }
  else if (e.key === 'ArrowDown') { e.preventDefault(); cursor.value = (cursor.value + 1) % orderedItems.value.length }
  else if (e.key === 'ArrowUp') { e.preventDefault(); cursor.value = (cursor.value - 1 + orderedItems.value.length) % orderedItems.value.length }
  else if (e.key === 'Enter') { if (item) actionsFor(item).length > 0 ? accept(item) : toggleDrawer(item) }
  else if (e.key === 'o' || e.key === 'O') { if (item) toggleDrawer(item) }
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

watch(hasPendingClassification, (isPending) => {
  if (isPending) startPolling()
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

<style scoped>
/* The drawer belongs to the row it opens from: it slides out of the head rather
   than appearing beside it. Transform + opacity only — the height change is the
   layout doing its job, not an animation. */
.drawer-enter-active,
.drawer-leave-active {
  transition: opacity 150ms ease-out, transform 180ms cubic-bezier(0.22, 1, 0.36, 1);
}
.drawer-enter-from,
.drawer-leave-to {
  opacity: 0;
  transform: translateY(-6px);
}

@media (prefers-reduced-motion: reduce) {
  .drawer-enter-active,
  .drawer-leave-active {
    transition: opacity 100ms linear;
  }
  .drawer-enter-from,
  .drawer-leave-to {
    transform: none;
  }
}
</style>
