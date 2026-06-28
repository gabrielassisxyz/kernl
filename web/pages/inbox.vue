<template>
  <InboxHeader :totalCount="items.length" :flaggedCount="flaggedCount" />

  <div class="px-section pt-base flex items-center justify-between">
    <div class="flex items-center gap-tight font-mono-data text-[11px] border border-border-hairline rounded overflow-hidden">
      <button
        v-for="t in tabs" :key="t.key"
        class="px-component py-1 transition-colors outline-none focus-visible:ring-1 focus-visible:ring-inset focus-visible:ring-primary/30"
        :class="tab === t.key ? 'bg-surface-hover text-text-primary' : 'text-text-muted hover:text-text-primary'"
        @click="tab = t.key"
      >{{ t.label }} <span class="text-text-faint">{{ t.count }}</span></button>
    </div>
  </div>

  <div v-if="tab === 'unprocessed'" class="px-section pt-base">
    <CaptureThought @submit="() => refresh()" />
  </div>

  <section v-if="tab === 'unprocessed'" class="flex-1 overflow-y-auto relative">
    <div class="flex flex-col gap-base px-section py-base pb-[120px]">
      <template v-for="(item, index) in items" :key="item.id">
        <InboxItem
          :item="item"
          :suggestion="suggestionFor(item)"
          :selected="selected.has(item.id)"
          :isCursor="cursor === index"
          :prepping="preppingId === item.id"
          @select="cursor = index"
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
            <button class="ml-auto font-mono-data text-[10px] text-text-muted hover:text-text-primary rounded outline-none focus-visible:ring-1 focus-visible:ring-primary/30" @click="peekId = null">close</button>
          </div>
          <p class="font-body text-body text-text-primary whitespace-pre-wrap">{{ peekBody || '…' }}</p>
        </div>
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
      <span class="font-mono-data text-[11px] text-text-primary">{{ selected.size }} selected</span>
      <div class="w-[1px] h-4 bg-border-hairline"></div>
      <button class="font-mono-data text-[11px] text-status-passed hover:underline rounded outline-none focus-visible:ring-1 focus-visible:ring-primary/30" @click="acceptSelected">Process all (accept DA)</button>
      <button class="font-mono-data text-[11px] text-status-failed-text hover:underline rounded outline-none focus-visible:ring-1 focus-visible:ring-primary/30" @click="discardSelected">Discard all</button>
      <button class="font-mono-data text-[11px] text-text-muted hover:text-text-primary rounded outline-none focus-visible:ring-1 focus-visible:ring-primary/30" @click="selected.clear()">Clear</button>
    </div>

    <InboxHint v-else-if="items.length > 0" />
  </section>

  <section v-else class="flex-1 overflow-y-auto">
    <div class="flex flex-col gap-base px-section py-base">
      <div
        v-for="p in processed"
        :key="p.captureId"
        class="flex items-center gap-component p-component border-b border-border-hairline"
      >
        <span class="material-symbols-outlined !text-[16px] shrink-0" :class="becameText(p.became)">{{ becameIcon(p.became) }}</span>
        <div class="flex flex-col flex-1 min-w-0">
          <div class="flex items-center gap-base">
            <span class="font-mono-data text-mono-data" :class="becameText(p.became)">{{ becameLabel(p) }}</span>
          </div>
          <h3 class="font-headline text-text-primary truncate" :class="p.became === 'discard' ? 'line-through text-text-muted' : ''">{{ p.title }}</h3>
        </div>
        <button class="shrink-0 font-mono-data text-[11px] text-text-muted hover:text-primary transition-colors rounded outline-none focus-visible:ring-1 focus-visible:ring-primary/30" @click="reopenCapture(p.captureId)">↩ Undo</button>
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
import { ref, computed, reactive, onMounted, onUnmounted } from 'vue'
import InboxHeader from '~/components/inbox/InboxHeader.vue'
import InboxItem from '~/components/inbox/InboxItem.vue'
import InboxHint from '~/components/inbox/InboxHint.vue'
import CaptureThought from '~/components/CaptureThought.vue'
import ProcessModal from '~/components/inbox/ProcessModal.vue'
import type { InboxItemData } from '~/components/inbox/InboxItem.vue'
import { useProjects } from '~/composables/useProjects'
import { TARGET_META, normalizeTarget, type Target } from '~/utils/inboxTargets'
import UiEmptyState from '~/components/ui/UiEmptyState.vue'
import UiErrorState from '~/components/ui/UiErrorState.vue'
import UiToast from '~/components/ui/UiToast.vue'

interface ProcessedRow {
  captureId: string
  title: string
  became: string
  targetId: string
  targetTitle: string
  projectId: string
  at: string
}

interface ProcessPayload { target: Target; projectId: string; linkTo: string; title: string }

const { data, pending, refresh, error } = useFetch<InboxItemData[]>('/api/inbox/pending', { server: false, default: () => [] })
const { data: processedData, refresh: refreshProcessed, error: processedError } = useFetch<ProcessedRow[]>('/api/inbox/processed', { server: false, default: () => [] })
const { projects, load: loadProjects } = useProjects()

const items = computed(() => data.value || [])
const processed = computed(() => processedData.value || [])
const flaggedCount = computed(() => items.value.filter(i => !!normalizeTarget(i.suggestedAction)).length)

const tab = ref<'unprocessed' | 'processed'>('unprocessed')
const tabs = computed(() => [
  { key: 'unprocessed' as const, label: 'Unprocessed', count: items.value.length },
  { key: 'processed' as const, label: 'Processed', count: processed.value.length },
])

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

function suggestionFor(item: InboxItemData): { target: Target; projectTitle: string } | null {
  const target = normalizeTarget(item.suggestedAction)
  if (!target) return null
  return { target, projectTitle: projectTitle(item.suggestedProjectId) }
}

// ---- processing ----
async function postProcess(id: string, payload: ProcessPayload) {
  await $fetch(`/api/inbox/${id}/process`, { method: 'POST', body: payload })
  if (data.value) data.value = data.value.filter(i => i.id !== id)
  selected.delete(id)
  if (cursor.value >= items.value.length) cursor.value = Math.max(0, items.value.length - 1)
}

async function accept(item: InboxItemData, quiet = false) {
  const s = suggestionFor(item)
  if (!s) return
  busy.value = true
  try {
    await postProcess(item.id, { target: s.target, projectId: item.suggestedProjectId || '', linkTo: '', title: item.title })
    undoStack.value.push(item.id)
    await refreshProcessed()
    if (!quiet) showToast(`Processed: ${item.title}`)
  } catch (e) { console.error('accept failed', e) } finally { busy.value = false }
}

async function discard(item: InboxItemData, quiet = false) {
  busy.value = true
  try {
    await postProcess(item.id, { target: 'discard', projectId: '', linkTo: '', title: item.title })
    undoStack.value.push(item.id)
    await refreshProcessed()
    if (!quiet) showToast(`Discarded: ${item.title}`)
  } catch (e) { console.error('discard failed', e) } finally { busy.value = false }
}

async function acceptSelected() {
  const targets = items.value.filter(i => selected.has(i.id) && normalizeTarget(i.suggestedAction))
  for (const i of targets) await accept(i, true)
  showToast(`Processed ${targets.length} — U to undo last`)
}
async function discardSelected() {
  const targets = items.value.filter(i => selected.has(i.id))
  for (const i of targets) await discard(i, true)
  showToast(`Discarded ${targets.length} — U to undo last`)
}

function openModal(item: InboxItemData) { modalItem.value = item }

async function confirmModal(payload: ProcessPayload) {
  const item = modalItem.value
  if (!item) return
  busy.value = true
  try {
    await postProcess(item.id, payload)
    undoStack.value.push(item.id)
    await refreshProcessed()
    modalItem.value = null
    showToast(`Processed: ${item.title}`)
  } catch (e) { console.error('process failed', e) } finally { busy.value = false }
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
function becameLabel(p: ProcessedRow): string {
  const base = knownTarget(p.became) ? TARGET_META[p.became].label : p.became
  return p.projectId ? `${base} · ${projectTitle(p.projectId)}` : base
}

// ---- keyboard ----
function onKey(e: KeyboardEvent) {
  const tg = e.target as HTMLElement | null
  if (tg && (tg.tagName === 'INPUT' || tg.tagName === 'TEXTAREA' || tg.tagName === 'SELECT')) return
  if (modalItem.value) return
  if (e.key === 'u' || e.key === 'U') { undo(); return }
  if (tab.value !== 'unprocessed' || items.value.length === 0) return
  const item = items.value[cursor.value]
  if (e.key === 'ArrowDown') { e.preventDefault(); cursor.value = (cursor.value + 1) % items.value.length }
  else if (e.key === 'ArrowUp') { e.preventDefault(); cursor.value = (cursor.value - 1 + items.value.length) % items.value.length }
  else if (e.key === 'Enter') { if (item) suggestionFor(item) ? accept(item) : openModal(item) }
  else if (e.key === 'e' || e.key === 'E') { if (item) openModal(item) }
  else if (e.key === 'd' || e.key === 'D') { if (item) discard(item) }
  else if (e.key === 'x' || e.key === 'X') { if (item) toggleSelect(item.id) }
}

onMounted(() => {
  loadProjects()
  window.addEventListener('keydown', onKey)
})
onUnmounted(() => window.removeEventListener('keydown', onKey))
</script>
