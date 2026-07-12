<template>
  <UiModal
    :open="!!item"
    :title="item?.title || ''"
    kicker="PROCESS CAPTURE"
    size="lg"
    @close="$emit('close')"
  >
    <div class="flex flex-col gap-section">
      <!-- The capture itself, readable end to end. Triage is exactly when you
           need to read the whole thing, so it is never clipped away for good. -->
      <UiField label="Capture">
        <div class="rounded border border-border-hairline bg-surface-container-low px-base py-base">
          <p
            class="font-body text-body text-text-primary whitespace-pre-wrap"
            :class="expanded ? 'max-h-[40vh] overflow-y-auto' : 'line-clamp-3'"
          >{{ captureBody }}</p>
          <button
            v-if="isLongCapture"
            class="mt-tight font-mono-data text-mono-data text-text-muted hover:text-text-primary rounded outline-none focus-visible:ring-1 focus-visible:ring-primary/30"
            @click="expanded = !expanded"
          >{{ expanded ? 'Collapse' : 'Show full capture' }}</button>
        </div>
      </UiField>

      <!-- One capture becomes N nodes: review, edit, add or drop each one. -->
      <div class="flex flex-col gap-base">
        <div
          v-for="(action, i) in draft.actions"
          :key="i"
          class="flex flex-col gap-base rounded border border-border-hairline bg-surface p-component"
        >
          <div class="flex items-center justify-between gap-base">
            <span class="font-mono-data text-mono-data text-text-faint">ACTION {{ i + 1 }}</span>
            <button
              v-if="draft.actions.length > 1"
              class="flex items-center gap-tight font-mono-data text-mono-data text-text-muted hover:text-status-failed-text rounded outline-none focus-visible:ring-1 focus-visible:ring-primary/30"
              @click="removeAction(i)"
            >
              <span class="material-symbols-outlined !text-body">close</span>
              Remove
            </button>
          </div>

          <div class="grid grid-cols-3 gap-tight">
            <UiButton
              v-for="t in TARGETS"
              :key="t"
              size="sm"
              :variant="action.target === t ? targetVariant(t) : 'secondary'"
              :icon="TARGET_META[t].icon"
              @click="setTarget(i, t)"
            >
              {{ TARGET_META[t].label }}
            </UiButton>
          </div>

          <UiField v-if="action.target !== 'discard' && action.target !== 'update'" label="Title">
            <UiInput v-model="action.title" classes="h-8 w-full rounded border border-border-hairline bg-surface-container-low px-base font-body text-body text-text-primary outline-none transition-colors placeholder:text-text-muted focus:border-primary/70" />
          </UiField>

          <UiField
            v-if="action.target === 'task' || action.target === 'note' || action.target === 'bookmark'"
            :label="action.target === 'task' ? 'Project' : 'Link to (optional)'"
          >
            <UiSelect v-model="action.projectId" classes="h-8 w-full rounded border border-border-hairline bg-surface-container-low px-base font-body text-body text-text-primary outline-none transition-colors focus:border-primary/70">
              <option value="">{{ action.target === 'task' ? 'Unprocessed tasks (no project)' : 'Nothing' }}</option>
              <option v-for="p in projects" :key="p.id" :value="p.id">{{ p.title }}</option>
            </UiSelect>
          </UiField>

          <template v-if="action.target === 'project'">
            <UiField label="Description">
              <UiTextarea v-model="action.projectDescription" rows="3" classes="w-full rounded border border-border-hairline bg-surface-container-low px-base py-base font-body text-body text-text-primary outline-none transition-colors focus:border-primary/70 resize-none" />
            </UiField>
            <UiField label="Initial tasks" hint="One task per line. Kernl will create up to six tasks under the new project.">
              <UiTextarea v-model="action.initialTasksText" rows="4" classes="w-full rounded border border-border-hairline bg-surface-container-low px-base py-base font-body text-body text-text-primary outline-none transition-colors focus:border-primary/70 resize-none" />
            </UiField>
          </template>

          <p v-if="action.target === 'update'" class="font-body text-body text-text-muted">
            Kernl finds the best-matching note and proposes the changes to merge in.
            You'll review each suggested edit before anything is written.
          </p>
        </div>

        <UiButton variant="secondary" size="sm" icon="add" @click="addAction">Add action</UiButton>

        <p v-if="updateConflict" class="font-body text-body text-status-failed-text">
          An update is reviewed change by change against one note, so it has to be the only action.
          Remove the others, or pick a different target for this one.
        </p>
      </div>
    </div>

    <template #footer>
      <div class="flex items-center justify-end gap-base">
        <UiButton variant="ghost" @click="$emit('close')">Cancel</UiButton>
        <UiButton variant="primary" :loading="busy" :disabled="updateConflict" @click="confirm">
          {{ confirmLabel }}
        </UiButton>
      </div>
    </template>
  </UiModal>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import type { Project } from '~/composables/useProjects'
import { TARGETS, TARGET_META, normalizeActions, type CaptureAction, type Target } from '~/utils/inboxTargets'
import type { InboxItemData } from '~/components/inbox/InboxItem.vue'
import UiButton from '~/components/ui/UiButton.vue'
import UiField from '~/components/ui/UiField.vue'
import UiInput from '~/components/ui/UiInput.vue'
import UiModal from '~/components/ui/UiModal.vue'
import UiSelect from '~/components/ui/UiSelect.vue'
import UiTextarea from '~/components/ui/UiTextarea.vue'
type ButtonVariant = 'primary' | 'secondary' | 'ghost' | 'danger' | 'success' | 'accent'

// A capture long enough that the collapsed view would hide something.
const LONG_CAPTURE_CHARS = 180

// DraftAction is one editable row. initialTasksText is the textarea-friendly
// form of initialTasks; everything else mirrors CaptureAction.
interface DraftAction {
  target: Target
  title: string
  body: string
  projectId: string
  projectDescription: string
  initialTasksText: string
  tags: string[]
}

const props = defineProps<{
  item: InboxItemData | null
  projects: Project[]
  busy?: boolean
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'confirm', payload: { actions: CaptureAction[] }): void
}>()

const draft = reactive<{ actions: DraftAction[] }>({ actions: [] })
const expanded = ref(false)

const captureBody = computed(() => props.item?.subtitle || props.item?.title || '')
const isLongCapture = computed(() => captureBody.value.length > LONG_CAPTURE_CHARS || captureBody.value.includes('\n'))

const updateConflict = computed(
  () => draft.actions.length > 1 && draft.actions.some(a => a.target === 'update'),
)
const confirmLabel = computed(() =>
  draft.actions.length > 1 ? `Process ${draft.actions.length} actions` : 'Process',
)

function targetVariant(target: Target): ButtonVariant {
  if (target === 'discard') return 'danger'
  if (target === 'task') return 'success'
  return 'accent'
}

// blankAction seeds a new row from the capture itself, so "Add action" starts
// from something usable rather than an empty form.
function blankAction(item: InboxItemData): DraftAction {
  return {
    target: 'note',
    title: item.title,
    body: '',
    projectId: '',
    projectDescription: item.subtitle || '',
    initialTasksText: '',
    tags: [],
  }
}

function toDraft(action: CaptureAction, item: InboxItemData): DraftAction {
  return {
    target: action.target,
    title: action.title || item.title,
    body: action.body || '',
    projectId: action.projectId || action.linkTo || '',
    projectDescription: action.projectDescription || action.body || item.subtitle || '',
    initialTasksText: (action.initialTasks || []).join('\n'),
    tags: action.tags || [],
  }
}

// Re-seed the draft from the DA's suggested actions (or a single neutral note)
// each time a capture opens the modal.
watch(() => props.item, (item) => {
  if (!item) return
  expanded.value = false
  const suggested = normalizeActions(item.suggestedActions)
  draft.actions = suggested.length > 0
    ? suggested.map(a => toDraft(a, item))
    : [blankAction(item)]
}, { immediate: true })

function setTarget(index: number, target: Target) {
  const action = draft.actions[index]
  // projectId means "parent project" for a task and "link to" for a note or
  // bookmark; it means nothing for the rest, so drop a stale selection.
  if (target !== 'task' && target !== 'note' && target !== 'bookmark') action.projectId = ''
  action.target = target
}

function addAction() {
  if (!props.item) return
  draft.actions.push(blankAction(props.item))
}

function removeAction(index: number) {
  draft.actions.splice(index, 1)
}

function taskLines(text: string): string[] {
  return text.split('\n').map(line => line.trim()).filter(Boolean)
}

// toAction projects a draft row back onto the wire shape, sending only the
// fields its target actually uses.
function toAction(draftAction: DraftAction): CaptureAction {
  const { target, title, body, projectId } = draftAction
  if (target === 'update' || target === 'discard') {
    return { target, title }
  }
  if (target === 'project') {
    return {
      target,
      title,
      body,
      projectTitle: title,
      projectDescription: draftAction.projectDescription,
      initialTasks: taskLines(draftAction.initialTasksText),
      tags: draftAction.tags,
    }
  }
  const isTask = target === 'task'
  return {
    target,
    title,
    body,
    projectId: isTask ? projectId : '',
    linkTo: isTask ? '' : projectId,
    tags: draftAction.tags,
  }
}

function confirm() {
  if (!props.item || draft.actions.length === 0 || updateConflict.value) return
  emit('confirm', { actions: draft.actions.map(toAction) })
}
</script>
