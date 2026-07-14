<template>
  <!-- The card is centred in what it is given, and what it is given grows with
       the screen. On a 4K panel a fixed 620px column reads as abandoned, not
       focused: the type scale, the measure and the padding all step up together
       so the card keeps its proportions instead of shrinking into the middle. -->
  <div class="focus-stage flex min-h-full items-center justify-center px-section py-section">
    <UiEmptyState
      v-if="!current"
      icon="check_circle"
      title="Nothing left to triage."
      body="Every capture has been processed. Esc goes back to the list."
    />

    <div
      v-else
      class="w-full flex flex-col gap-section rounded-lg border border-border-default bg-surface"
      :style="{ maxWidth: 'var(--focus-col)', minHeight: 'var(--focus-min-h)', padding: 'var(--focus-pad)' }"
    >
      <!-- Where you are in the pile: the one thing a card view hides and a list
           gives you for free. -->
      <div class="flex flex-col gap-tight">
        <div class="flex items-baseline gap-base font-mono-data text-[length:var(--focus-data)]">
          <span class="text-text-primary">{{ pad(index + 1) }}<span class="text-text-dim"> / </span>{{ pad(items.length) }}</span>
          <span class="text-text-faint truncate">{{ provenance }}</span>
          <span class="ml-auto text-text-muted shrink-0">Esc list</span>
        </div>
        <div class="h-[2px] w-full bg-border-hairline overflow-hidden">
          <div
            class="h-full bg-primary transition-[width] duration-200 ease-out motion-reduce:transition-none"
            :style="{ width: progress }"
          ></div>
        </div>
      </div>

      <!-- The capture, whole and unrewritten. In focus mode it is the hero: the
           point of one-at-a-time is that you read the thing before you file it. -->
      <!-- The card grows wider than the prose does: the measure stays at 70ch so a
           bigger screen buys a bigger card, never a longer line to track. -->
      <p class="max-w-[70ch] font-body text-[length:var(--focus-read)] leading-[1.65] text-text-primary whitespace-pre-wrap text-pretty">{{ captureBody }}</p>

      <div class="flex items-baseline gap-base border-t border-border-hairline pt-base font-mono-data text-[length:var(--focus-data)]">
        <span class="font-label-caps text-text-muted shrink-0">Becomes</span>
        <span class="text-text-faint">{{ nodes.length }} {{ nodes.length === 1 ? 'node' : 'nodes' }}</span>
        <span v-if="nodes.length > 1" class="text-text-dim">j k moves the cursor</span>
      </div>

      <p v-if="nodes.length === 0" class="font-body text-[length:var(--focus-ui)] text-text-muted">
        The DA has not classified this capture yet. <span class="text-text-primary">A</span> adds a node by hand,
        <span class="text-text-primary">S</span> skips to the next.
      </p>

      <ul v-else class="flex flex-col gap-tight">
        <li
          v-for="(node, i) in nodes"
          :key="i"
          class="flex gap-base rounded px-base py-base transition-colors"
          :class="i === cursor ? 'bg-surface-hover' : ''"
        >
          <span
            class="shrink-0 font-mono-data text-[length:var(--focus-data)] leading-[1.8]"
            :class="i === cursor ? 'text-primary' : 'text-transparent'"
            aria-hidden="true"
          >▸</span>

          <span
            class="shrink-0 w-[7em] self-start flex items-center gap-tight px-tight rounded border font-mono-data text-[length:var(--focus-data)] leading-[1.8]"
            :class="TARGET_META[node.target].chip"
          >
            <span class="material-symbols-outlined !text-[1.1em]" aria-hidden="true">{{ TARGET_META[node.target].icon }}</span>
            {{ TARGET_META[node.target].label }}
          </span>

          <div class="flex flex-col gap-tight min-w-0 flex-1">
            <div class="flex items-baseline gap-base min-w-0">
              <input
                v-if="isEditing(i, 'title')"
                ref="editorEl"
                v-model="buffer"
                class="flex-1 min-w-0 bg-transparent border-b border-primary/40 font-body text-[length:var(--focus-ui)] text-text-primary outline-none"
                @blur="commit"
                @keydown.enter.prevent="commit"
                @keydown.escape.prevent.stop="editing = null"
              />
              <span v-else class="font-body text-[length:var(--focus-ui)] text-text-primary truncate">{{ nodeTitle(node) }}</span>

              <span v-if="projectLabel(node)" class="shrink-0 font-mono-data text-[length:var(--focus-data)] text-text-muted">{{ projectLabel(node) }}</span>
              <span v-for="tag in node.tags || []" :key="tag" class="shrink-0 font-mono-data text-[length:var(--focus-data)] text-text-faint">#{{ tag }}</span>
              <span v-if="node.dueDate" class="shrink-0 ml-auto font-mono-data text-[length:var(--focus-data)] text-tertiary">{{ dueLabel(node) }}</span>
            </div>

            <textarea
              v-if="isEditing(i, 'body')"
              ref="editorEl"
              v-model="buffer"
              :rows="bufferRows"
              class="w-full bg-transparent border border-primary/40 rounded px-tight py-0.5 font-body text-[length:var(--focus-ui)] text-text-primary outline-none resize-none"
              @blur="commit"
              @keydown.escape.prevent.stop="editing = null"
            />
            <p v-else-if="description(node)" class="font-body text-[length:var(--focus-ui)] leading-[1.55] text-text-muted line-clamp-3">{{ description(node) }}</p>

            <!-- The type strip teaches its own keymap: the number beside each type
                 IS the key. No legend to memorise, no dropdown to open. It lives in
                 the text column, so it lines up with the title at any scale. -->
            <div v-if="i === cursor" class="flex flex-wrap gap-tight pt-tight">
              <button
                v-for="(t, n) in TARGETS"
                :key="t"
                class="flex items-center gap-tight px-tight rounded border font-mono-data text-[length:var(--focus-data)] transition-colors cursor-pointer outline-none focus-visible:ring-1 focus-visible:ring-primary/30"
                :class="node.target === t ? TARGET_META[t].chip : 'border-border-hairline text-text-muted hover:text-text-primary hover:border-border-default'"
                @click="setTarget(i, t)"
              >
                <span :class="node.target === t ? '' : 'text-text-dim'">{{ n + 1 }}</span>
                {{ TARGET_META[t].label }}
              </button>
            </div>
          </div>
        </li>
      </ul>

      <p v-if="updateConflict" class="font-body text-[length:var(--focus-ui)] text-status-failed-text">
        An update is reviewed change by change against one note, so it has to be the only node.
        Retype it, or drop the others with X.
      </p>

      <!-- Stuck on where this goes? Argue it out with the DA, without leaving the
           card. It only proposes: accepting rewrites the nodes above, and ⏎ is
           still what writes them. -->
      <InboxDaPanel
        v-if="daOpen"
        :capture-id="current.id"
        :draft="nodes"
        class="-mx-[var(--focus-pad)] border-y"
        @accept="onDaRouting"
        @close="daOpen = false"
      />

      <!-- In focus mode the keymap is not a hint, it is the interface — so it sits
           at the foot of the card on one unbroken line, not wherever the content
           happens to end. -->
      <div class="mt-auto flex flex-wrap items-center gap-x-base gap-y-tight border-t border-border-hairline pt-base font-mono-data text-[length:var(--focus-data)] text-text-muted">
        <span v-for="key in KEYS" :key="key.k" class="whitespace-nowrap">
          <span class="text-text-primary">{{ key.k }}</span> {{ key.label }}
        </span>
      </div>
    </div>

    <UiCommandPalette
      v-if="palette === 'project'"
      label="Project"
      placeholder="Filter projects…"
      :options="projectOptions"
      @pick="onPickProject"
      @close="palette = null"
    />
    <UiCommandPalette
      v-else-if="palette === 'due'"
      label="Due"
      placeholder="today · tomorrow · +3 · 2026-08-01"
      :options="dueOptions"
      allow-raw
      @pick="onPickDue"
      @close="palette = null"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import type { Project } from '~/composables/useProjects'
import type { InboxItemData } from '~/components/inbox/InboxRow.vue'
import UiCommandPalette, { type PaletteOption } from '~/components/ui/UiCommandPalette.vue'
import UiEmptyState from '~/components/ui/UiEmptyState.vue'
import {
  TARGETS,
  TARGET_META,
  captureProvenance,
  normalizeActions,
  type CaptureAction,
  type Target,
} from '~/utils/inboxTargets'

const KEYS = [
  { k: '1-6', label: 'type' },
  { k: 'T', label: 'title' },
  { k: 'B', label: 'body' },
  { k: 'P', label: 'project' },
  { k: 'W', label: 'when' },
  { k: 'X', label: 'drop' },
  { k: 'A', label: 'add' },
  { k: '/', label: 'ask DA' },
  { k: '⏎', label: 'process' },
  { k: 'D', label: 'discard' },
  { k: 'S', label: 'skip' },
  { k: 'U', label: 'undo' },
]

const props = defineProps<{
  /** the unprocessed pile, in the same order the list shows it */
  items: InboxItemData[]
  projects: Project[]
  busy?: boolean
}>()

const emit = defineEmits<{
  (e: 'process', payload: { item: InboxItemData; actions: CaptureAction[] }): void
  (e: 'discard', item: InboxItemData): void
  (e: 'undo'): void
  (e: 'close'): void
}>()

const index = ref(0)
const cursor = ref(0)
const palette = ref<'project' | 'due' | null>(null)
const daOpen = ref(false)

const current = computed<InboxItemData | null>(() => props.items[index.value] ?? null)
const captureBody = computed(() => current.value?.subtitle || current.value?.title || '')
const provenance = computed(() =>
  current.value
    ? captureProvenance(current.value.batchSource || current.value.type, current.value.batchTimestamp)
    : '',
)
const progress = computed(() =>
  props.items.length === 0 ? '0%' : `${((index.value + 1) / props.items.length) * 100}%`,
)
const pad = (n: number) => String(n).padStart(2, '0')

// Processing removes the capture from the pile, so the card that slides into
// this slot is the next one — the advance is the list shrinking under it. What
// this has to survive is running off the end.
watch(() => props.items.length, (length) => {
  if (index.value > length - 1) index.value = Math.max(0, length - 1)
})

// ---- the draft ----
// Edits live per capture, so skipping forward and coming back does not throw
// away the retyping you already did. They are still local: nothing is written
// until you process.
const drafts = ref<Record<string, CaptureAction[]>>({})

const nodes = computed<CaptureAction[]>(() => {
  const item = current.value
  if (!item) return []
  if (!drafts.value[item.id]) drafts.value[item.id] = normalizeActions(item.suggestedActions)
  return drafts.value[item.id]
})

watch(current, () => {
  cursor.value = 0
  editing.value = null
  palette.value = null
  // A DA conversation belongs to the capture it was about.
  daOpen.value = false
})

const activeNode = computed<CaptureAction | null>(() => nodes.value[cursor.value] ?? null)
const updateConflict = computed(
  () => nodes.value.length > 1 && nodes.value.some(n => n.target === 'update'),
)

function patch(i: number, fields: Partial<CaptureAction>) {
  const item = current.value
  if (!item) return
  const next = [...nodes.value]
  next[i] = { ...next[i], ...fields }
  drafts.value[item.id] = next
}

function setTarget(i: number, target: Target) {
  patch(i, { target })
  cursor.value = i
}

function addNode() {
  const item = current.value
  if (!item) return
  drafts.value[item.id] = [...nodes.value, { target: 'note', title: '', body: captureBody.value }]
  cursor.value = nodes.value.length - 1
}

function dropNode() {
  const item = current.value
  if (!item || nodes.value.length <= 1) return
  drafts.value[item.id] = nodes.value.filter((_, i) => i !== cursor.value)
  cursor.value = Math.min(cursor.value, nodes.value.length - 1)
}

// The DA proposes; this is where the user's acceptance lands. It replaces the
// whole routing — a routing is a set of nodes, not a patch — and still writes
// nothing: ⏎ does that.
function onDaRouting(actions: CaptureAction[]) {
  const item = current.value
  if (!item || actions.length === 0) return
  drafts.value[item.id] = actions
  cursor.value = 0
}

// ---- editing in place ----
type Field = 'title' | 'body'

const editing = ref<{ index: number; field: Field } | null>(null)
const buffer = ref('')
const editorEl = ref<(HTMLInputElement | HTMLTextAreaElement)[]>([])

const isEditing = (i: number, field: Field) =>
  editing.value?.index === i && editing.value.field === field
const bufferRows = computed(() => Math.min(10, buffer.value.split('\n').length + 1))

async function edit(field: Field) {
  const node = activeNode.value
  if (!node) return
  editing.value = { index: cursor.value, field }
  buffer.value = field === 'title' ? nodeTitle(node) : description(node)
  await nextTick()
  const el = editorEl.value[0]
  el?.focus()
  el?.select()
}

function commit() {
  const target = editing.value
  editing.value = null
  if (!target) return
  const value = buffer.value.trim()
  // An emptied title would silently fall back to the capture's own title, which
  // reads as the edit being thrown away. An empty edit is simply not a change.
  if (target.field === 'title' && !value) return
  patch(target.index, { [target.field]: value })
}

// ---- the palettes ----
const projectOptions = computed<PaletteOption[]>(() => [
  { id: '', label: 'No project' },
  ...props.projects.map(p => ({ id: p.id, label: p.title })),
])

const iso = (d: Date) =>
  `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`

function inDays(days: number): string {
  const d = new Date()
  d.setDate(d.getDate() + days)
  return iso(d)
}

const dueOptions = computed<PaletteOption[]>(() => {
  const today = new Date()
  // Saturday, or today if it already is one.
  const toSaturday = (6 - today.getDay() + 7) % 7
  const toMonday = (8 - today.getDay()) % 7 || 7
  return [
    { id: inDays(0), label: 'Today', hint: dueHint(inDays(0)) },
    { id: inDays(1), label: 'Tomorrow', hint: dueHint(inDays(1)) },
    { id: inDays(toSaturday), label: 'This weekend', hint: dueHint(inDays(toSaturday)) },
    { id: inDays(toMonday), label: 'Next week', hint: dueHint(inDays(toMonday)) },
    { id: '', label: 'No deadline' },
  ]
})

const dueHint = (value: string) => dueLabel({ target: 'task', title: '', dueDate: value })

function onPickProject(id: string) {
  const node = activeNode.value
  if (node) {
    // A task's project is its parent; anything else is merely linked to it.
    patch(cursor.value, node.target === 'task' ? { projectId: id } : { linkTo: id })
  }
  palette.value = null
}

// "+3" and a raw "2026-08-01" both come back through here, so both are parsed
// before anything is written onto the node.
function onPickDue(value: string) {
  const raw = value.trim()
  let due = ''
  if (/^\+\d+$/.test(raw)) due = inDays(Number(raw.slice(1)))
  else if (/^\d{4}-\d{2}-\d{2}$/.test(raw)) due = raw
  if (raw && !due) { palette.value = null; return }
  if (activeNode.value) patch(cursor.value, { dueDate: due })
  palette.value = null
}

// ---- rendering a node ----
function nodeTitle(node: CaptureAction): string {
  const title = (node.title || '').trim()
  if (title) return title
  if (node.target === 'update') return 'Merge into the matching note'
  if (node.target === 'discard') return 'Drop this fragment'
  return current.value?.title || ''
}

// The fragment this node owns. The capture it gets appended to is right above;
// printing it again under every node is noise.
function description(node: CaptureAction): string {
  const body = (node.body || '').trim()
  if (!body || body === captureBody.value) return ''
  return body
}

function projectLabel(node: CaptureAction): string {
  if (node.target === 'project') {
    const count = node.initialTasks?.length || 0
    return count ? `· ${count} tasks` : ''
  }
  const id = node.target === 'task' ? node.projectId : node.linkTo
  const parent = props.projects.find(p => p.id === id)?.title
  return parent ? `· ${parent}` : ''
}

function dueLabel(node: CaptureAction): string {
  const [y, m, d] = (node.dueDate || '').split('-').map(Number)
  if (!y || !m || !d) return ''
  return new Date(y, m - 1, d).toLocaleDateString('en-US', { month: 'short', day: 'numeric' })
}

// ---- the loop ----
function process() {
  const item = current.value
  if (!item || props.busy || nodes.value.length === 0 || updateConflict.value) return
  emit('process', { item, actions: nodes.value })
}

function discard() {
  const item = current.value
  if (!item || props.busy) return
  emit('discard', item)
}

// Skipping leaves the capture in the inbox and walks past it: triage you are not
// ready to make is a decision, not a failure.
const step = (delta: number) => {
  if (props.items.length === 0) return
  index.value = Math.min(Math.max(index.value + delta, 0), props.items.length - 1)
}

function onKey(e: KeyboardEvent) {
  // The palette, the DA panel and the inline editors own their keys while they
  // are open — each has a text field of its own, and j/k/1-6 are letters there.
  if (palette.value || editing.value) return
  const tag = (e.target as HTMLElement | null)?.tagName
  if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return
  if (e.metaKey || e.ctrlKey || e.altKey) return

  // Escape closes the DA panel before it leaves focus mode: the innermost thing
  // open is the thing Escape means.
  if (e.key === 'Escape' && daOpen.value) { e.preventDefault(); daOpen.value = false; return }
  if (e.key === 'Escape') { e.preventDefault(); emit('close'); return }
  if (e.key === 'u' || e.key === 'U') { e.preventDefault(); emit('undo'); return }
  if (!current.value) return

  const digit = Number(e.key)
  if (digit >= 1 && digit <= TARGETS.length && activeNode.value) {
    e.preventDefault()
    setTarget(cursor.value, TARGETS[digit - 1])
    return
  }

  switch (e.key) {
    case 'j': case 'ArrowDown':
      e.preventDefault()
      if (nodes.value.length) cursor.value = (cursor.value + 1) % nodes.value.length
      break
    case 'k': case 'ArrowUp':
      e.preventDefault()
      if (nodes.value.length) cursor.value = (cursor.value - 1 + nodes.value.length) % nodes.value.length
      break
    case 'Enter': e.preventDefault(); process(); break
    case 'd': case 'D': e.preventDefault(); discard(); break
    case 's': case 'S': case 'ArrowRight': e.preventDefault(); step(1); break
    case 'ArrowLeft': e.preventDefault(); step(-1); break
    case 't': case 'T': e.preventDefault(); edit('title'); break
    case 'b': case 'B': e.preventDefault(); edit('body'); break
    case 'a': case 'A': e.preventDefault(); addNode(); break
    case 'x': case 'X': e.preventDefault(); dropNode(); break
    case '/':
      e.preventDefault()
      daOpen.value = true
      break
    case 'p': case 'P':
      e.preventDefault()
      if (activeNode.value) palette.value = 'project'
      break
    case 'w': case 'W':
      e.preventDefault()
      // Only a task carries a deadline; a date on a note would never be read.
      if (activeNode.value?.target === 'task') palette.value = 'due'
      break
  }
}

onMounted(() => window.addEventListener('keydown', onKey))
onUnmounted(() => window.removeEventListener('keydown', onKey))
</script>

<style scoped>
/* One ladder, four rungs. The reading size, the UI size, the data size, the
   measure and the padding all step together, so the card keeps its proportions
   at every width instead of scaling one axis and stranding the others.
 *
 * Steps, not a fluid clamp: this is product UI, and a continuously-resizing type
 * scale makes a tool feel unstable. The measure stays near 70ch at every rung —
 * a wider screen buys a bigger card, never a longer line.
 *
 * Viewport-based, so a window snapped to half a 4K display gets the rung that
 * fits the window, not the one that fits the monitor. */
.focus-stage {
  --focus-read: 15px;
  --focus-ui: 13px;
  --focus-data: 12px;
  --focus-col: 720px;
  --focus-min-h: 380px;
  --focus-pad: 24px;
}

@media (min-width: 1280px) {
  .focus-stage {
    --focus-read: 16px;
    --focus-ui: 14px;
    --focus-data: 12px;
    --focus-col: 820px;
    --focus-min-h: 440px;
    --focus-pad: 28px;
  }
}

@media (min-width: 1700px) {
  .focus-stage {
    --focus-read: 18px;
    --focus-ui: 15px;
    --focus-data: 13px;
    --focus-col: 940px;
    --focus-min-h: 520px;
    --focus-pad: 32px;
  }
}

@media (min-width: 2200px) {
  .focus-stage {
    --focus-read: 20px;
    --focus-ui: 16px;
    --focus-data: 14px;
    --focus-col: 1080px;
    --focus-min-h: 620px;
    --focus-pad: 40px;
  }
}
</style>
