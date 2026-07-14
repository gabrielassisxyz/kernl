<template>
  <div class="border-t border-border-hairline bg-bg-base px-component py-base flex flex-col gap-base rounded-b-lg">
    <!-- The capture, whole. Triage is exactly when you need to read the entire
         thing, so here it is never truncated and never rewritten. -->
    <div class="flex items-baseline gap-base">
      <span class="font-label-caps text-label-caps text-text-muted shrink-0">Capture</span>
      <span class="font-mono-data text-mono-data text-text-faint truncate">{{ provenance }}</span>
    </div>
    <p class="font-body text-body text-text-primary whitespace-pre-wrap max-h-[32vh] overflow-y-auto">{{ captureBody }}</p>

    <div class="flex items-baseline gap-base pt-tight border-t border-border-hairline">
      <span class="font-label-caps text-label-caps text-text-muted shrink-0">Becomes</span>
      <span class="font-mono-data text-mono-data text-text-faint">{{ nodes.length }} {{ nodes.length === 1 ? 'node' : 'nodes' }}</span>
      <span class="font-mono-data text-mono-data text-text-dim">click a type to change it · click a text to edit it</span>
    </div>

    <!-- One line per node, its description under it. Type, title and description
         are editable here; anything deeper — refiling, adding a node — is the editor. -->
    <ul class="flex flex-col">
      <li v-for="(node, i) in nodes" :key="i" class="flex items-center gap-base py-tight min-w-0">
        <!-- The type chip is centred against the whole node, title and
             description together, not against the title line alone. -->
        <div class="relative shrink-0" data-picker>
          <button
            class="w-[84px] flex items-center gap-tight px-tight rounded border font-mono-data text-mono-data transition-colors cursor-pointer outline-none focus-visible:ring-1 focus-visible:ring-primary/30"
            :class="TARGET_META[node.target].chip"
            :aria-expanded="picker === i"
            aria-haspopup="menu"
            @click="togglePicker(i)"
          >
            <span class="material-symbols-outlined !text-body" aria-hidden="true">{{ TARGET_META[node.target].icon }}</span>
            {{ TARGET_META[node.target].label }}
            <span
              class="material-symbols-outlined !text-body ml-auto opacity-60 transition-transform duration-150 ease-out motion-reduce:transition-none"
              :class="picker === i ? 'rotate-180' : ''"
              aria-hidden="true"
            >expand_more</span>
          </button>

          <div
            v-if="picker === i"
            class="absolute top-full left-0 mt-tight w-[132px] flex flex-col p-tight gap-0.5 rounded border border-border-default bg-surface-overlay shadow-lg z-dropdown"
            role="menu"
          >
            <button
              v-for="t in TARGETS"
              :key="t"
              role="menuitem"
              class="flex items-center gap-tight px-tight py-0.5 rounded border font-mono-data text-mono-data transition-colors cursor-pointer outline-none focus-visible:ring-1 focus-visible:ring-primary/30"
              :class="node.target === t ? TARGET_META[t].chip : 'border-transparent text-text-muted hover:text-text-primary hover:bg-surface-hover'"
              @click="setTarget(i, t)"
            >
              <span class="material-symbols-outlined !text-body" aria-hidden="true">{{ TARGET_META[t].icon }}</span>
              {{ TARGET_META[t].label }}
            </button>
          </div>
        </div>

        <div class="flex flex-col min-w-0 flex-1">
          <div class="flex items-baseline gap-base min-w-0">
            <input
              v-if="isEditing(i, 'title')"
              ref="editorEl"
              v-model="buffer"
              class="flex-1 min-w-0 bg-transparent border-b border-primary/40 font-body text-body text-text-primary outline-none"
              @blur="commit"
              @keydown.enter.prevent="commit"
              @keydown.escape.stop="editing = null"
            />
            <span
              v-else
              class="font-body text-body text-text-primary truncate rounded px-tight -mx-tight cursor-text hover:bg-surface-hover"
              @click="edit(i, 'title', nodeTitle(node))"
            >{{ nodeTitle(node) }}</span>

            <span v-if="projectLabel(node)" class="shrink-0 font-mono-data text-mono-data text-text-muted">{{ projectLabel(node) }}</span>
            <span v-for="tag in node.tags || []" :key="tag" class="shrink-0 font-mono-data text-mono-data text-text-faint">#{{ tag }}</span>
            <span v-if="node.dueDate" class="shrink-0 ml-auto font-mono-data text-mono-data text-tertiary">{{ dueLabel(node) }}</span>
          </div>

          <textarea
            v-if="isEditing(i, 'body')"
            ref="editorEl"
            v-model="buffer"
            :rows="bufferRows"
            class="w-full mt-tight bg-transparent border border-primary/40 rounded px-tight py-0.5 font-body text-body text-text-primary outline-none resize-none"
            @blur="commit"
            @keydown.escape.stop="editing = null"
          />
          <p
            v-else-if="description(node)"
            class="font-body text-body text-text-muted line-clamp-2 rounded px-tight -mx-tight cursor-text hover:bg-surface-hover"
            @click="edit(i, 'body', description(node))"
          >{{ description(node) }}</p>
        </div>
      </li>
    </ul>

    <div class="flex items-center gap-base pt-tight border-t border-border-hairline">
      <span class="font-mono-data text-mono-data text-text-dim">The capture is carried into each task's description.</span>
      <div class="ml-auto flex items-center gap-base font-mono-data text-mono-data">
        <button class="px-base py-0.5 rounded border border-border-hairline text-text-muted hover:text-text-primary transition-colors cursor-pointer" @click="$emit('close')">Close</button>
        <button class="px-base py-0.5 rounded border border-border-hairline text-text-muted hover:text-text-primary transition-colors cursor-pointer" @click="$emit('edit')">Edit…</button>
        <button class="px-base py-0.5 rounded border border-border-hairline text-text-muted hover:text-status-failed-text hover:border-status-failed/40 transition-colors cursor-pointer" @click="$emit('discard')">Discard</button>
        <button
          class="px-base py-0.5 rounded border border-status-passed/40 text-status-passed hover:bg-status-passed/10 transition-colors cursor-pointer disabled:opacity-45 disabled:cursor-not-allowed"
          :disabled="busy || updateConflict || nodes.length === 0"
          @click="confirm"
        >{{ confirmLabel }}</button>
      </div>
    </div>

    <p v-if="updateConflict" class="font-body text-body text-status-failed-text">
      An update is reviewed change by change against one note, so it has to be the only node.
      Open the editor to drop the others, or retype this one.
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import type { Project } from '~/composables/useProjects'
import type { InboxItemData } from '~/components/inbox/InboxRow.vue'
import {
  TARGETS,
  TARGET_META,
  captureProvenance,
  normalizeActions,
  type CaptureAction,
  type Target,
} from '~/utils/inboxTargets'

const props = defineProps<{
  item: InboxItemData
  projects: Project[]
  busy?: boolean
}>()

const emit = defineEmits<{
  (e: 'process', payload: { actions: CaptureAction[] }): void
  (e: 'discard'): void
  (e: 'edit'): void
  (e: 'close'): void
}>()

const captureBody = computed(() => props.item.subtitle || props.item.title || '')
const provenance = computed(() =>
  captureProvenance(props.item.batchSource || props.item.type, props.item.batchTimestamp),
)

/** the DA's proposal, retypeable and retitlable in place; refiling is the editor's job */
const nodes = ref<CaptureAction[]>([])
const picker = ref<number | null>(null)

watch(() => props.item, (item) => {
  nodes.value = normalizeActions(item.suggestedActions)
  picker.value = null
  editing.value = null
}, { immediate: true })

const updateConflict = computed(
  () => nodes.value.length > 1 && nodes.value.some(n => n.target === 'update'),
)
const confirmLabel = computed(() =>
  nodes.value.length > 1 ? `Process ${nodes.value.length}` : 'Process',
)

// ---- the type picker ----
function togglePicker(index: number) {
  picker.value = picker.value === index ? null : index
}

function setTarget(index: number, target: Target) {
  nodes.value[index] = { ...nodes.value[index], target }
  picker.value = null
}

// The menu is a real dropdown, so it has to close on a click that lands anywhere
// else — including another node's row, which has no handler of its own.
function onPointerDown(e: PointerEvent) {
  if (picker.value === null) return
  const el = e.target as HTMLElement | null
  if (!el?.closest('[data-picker]')) picker.value = null
}

onMounted(() => document.addEventListener('pointerdown', onPointerDown))
onUnmounted(() => document.removeEventListener('pointerdown', onPointerDown))

// ---- editing a node's text in place ----
// What you read in the drawer is what gets written, so the title and the
// description are edited right where they are read. Blur commits; Escape reverts.
type Field = 'title' | 'body'

const editing = ref<{ index: number; field: Field } | null>(null)
const buffer = ref('')
const editorEl = ref<(HTMLInputElement | HTMLTextAreaElement)[]>([])

const isEditing = (index: number, field: Field) =>
  editing.value?.index === index && editing.value.field === field

const bufferRows = computed(() => Math.min(8, buffer.value.split('\n').length + 1))

async function edit(index: number, field: Field, value: string) {
  picker.value = null
  editing.value = { index, field }
  buffer.value = value
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
  // An emptied title would render as the capture's own title and read as a
  // silent revert, so an empty edit is simply not a change.
  if (target.field === 'title' && !value) return
  nodes.value[target.index] = { ...nodes.value[target.index], [target.field]: value }
}

function nodeTitle(node: CaptureAction): string {
  const title = (node.title || '').trim()
  if (title) return title
  if (node.target === 'update') return 'Merge into the matching note'
  if (node.target === 'discard') return 'Drop this fragment'
  return props.item.title
}

// The fragment this node owns — not the composed description. The capture it
// gets appended to is right above; printing it again four times is noise.
function description(node: CaptureAction): string {
  const body = (node.body || '').trim()
  if (!body || body === captureBody.value) return ''
  return body
}

function projectLabel(node: CaptureAction): string {
  if (node.target === 'task') {
    const parent = props.projects.find(p => p.id === node.projectId)?.title
    return parent ? `· ${parent}` : ''
  }
  if (node.target === 'project') {
    const count = node.initialTasks?.length || 0
    return count ? `· ${count} tasks` : ''
  }
  return ''
}

function dueLabel(node: CaptureAction): string {
  const [y, m, d] = (node.dueDate || '').split('-').map(Number)
  if (!y || !m || !d) return ''
  return new Date(y, m - 1, d).toLocaleDateString('en-US', { month: 'short', day: 'numeric' })
}

function confirm() {
  if (nodes.value.length === 0 || updateConflict.value) return
  emit('process', { actions: nodes.value })
}
</script>
