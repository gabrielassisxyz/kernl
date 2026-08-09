<template>
  <!-- An aside, not a dialog: the list beside it stays visible and usable, so
       trapping focus here or claiming aria-modal would both be lies. -->
  <aside
    ref="panelRef"
    class="w-96 shrink-0 border-l border-border-hairline bg-surface overflow-y-auto flex flex-col text-body"
    tabindex="-1"
    :aria-label="isEdit ? 'Edit task' : 'New task'"
    @keydown="onKeydown"
  >
    <div class="flex items-center justify-between px-[22px] pt-[18px]">
      <span class="text-text-label">{{ isEdit ? 'Edit task' : 'New task' }}</span>
      <UiIconButton icon="close" label="Close panel" size="sm" @click="$emit('close')" />
    </div>

    <div class="px-[22px] pt-2.5">
      <textarea
        ref="titleRef"
        v-model="draft.title"
        rows="1"
        placeholder="What needs doing?"
        class="panel-title w-full resize-none overflow-hidden bg-transparent border-0 border-b border-transparent outline-none pt-1 pb-2 text-text-heading focus:border-b-primary"
        @input="autosize(titleRef)"
        @blur="commit('title')"
      ></textarea>
    </div>

    <div class="px-[22px] pt-3 pb-4">
      <div class="flex items-baseline justify-between gap-4">
        <span class="shrink-0 text-mono-data text-text-muted">Tags</span>
        <input
          v-model="tagsText"
          type="text"
          placeholder="comma, separated"
          class="min-w-0 flex-1 text-right bg-transparent border-0 border-b border-transparent outline-none pb-1.5 font-mono-data text-mono-data text-text-secondary focus:border-b-primary focus:text-text-primary"
          @blur="commit('tags')"
        />
      </div>

      <div class="mt-4 mb-1 text-mono-data text-text-muted">Description</div>
      <textarea
        ref="descRef"
        v-model="draft.description"
        rows="2"
        placeholder="Optional context, links, acceptance criteria."
        class="w-full resize-none overflow-hidden bg-transparent border-0 border-b border-border-default outline-none pb-2 text-text-secondary leading-relaxed transition-colors focus:border-b-primary focus:text-text-primary"
        @input="autosize(descRef)"
        @blur="commit('description')"
      ></textarea>
    </div>

    <!-- The briefing the DA prepared from the capture this task came from. It
         sits above the fields because it is the context for editing them. -->
    <section v-if="briefing" class="mx-[22px] mb-4 rounded-lg border border-da-accent/30 bg-da-accent/[0.04] px-3 py-2">
      <div class="flex items-center gap-1.5 mb-1.5">
        <span class="material-symbols-outlined da-icon text-da-accent-text" aria-hidden="true">auto_awesome</span>
        <h3 class="font-label-caps text-label-caps uppercase text-da-accent-text">DA briefing</h3>
      </div>
      <p class="whitespace-pre-wrap text-text-secondary">{{ briefing.body }}</p>
    </section>

    <div class="h-px bg-border-hairline"></div>

    <div class="px-[22px] py-4 border-b border-border-hairline flex flex-col gap-3.5">
      <div class="flex items-center justify-between gap-4">
        <span class="text-text-secondary">Status</span>
        <div class="flex rounded-lg border border-border-default overflow-hidden">
          <button
            v-for="(s, i) in TASK_STATUSES"
            :key="s.id"
            type="button"
            class="px-2.5 py-1 cursor-pointer transition-colors outline-none focus-visible:ring-1 focus-visible:ring-inset focus-visible:ring-primary/30"
            :class="[
              i > 0 ? 'border-l border-border-default' : '',
              draft.status === s.id ? 'bg-accent-tint text-primary font-medium' : 'text-text-secondary hover:bg-surface-nav-hover',
            ]"
            @click="draft.status = s.id; commit('status')"
          >
            {{ s.label }}
          </button>
        </div>
      </div>

      <div class="flex items-center justify-between gap-4">
        <span class="shrink-0 text-text-secondary">Project</span>
        <!-- A select is as wide as its widest option, and a project title has
             no length limit, so an uncapped one pushes the whole panel sideways
             into a horizontal scrollbar. Capped, it ellipsises instead and
             lines up on the right with the due date below it. -->
        <select
          v-model="draft.projectId"
          class="project-select h-7 min-w-0 max-w-[190px] pl-2 pr-1 rounded-lg bg-bg-elevated border border-border-default outline-none cursor-pointer font-mono-data text-mono-data text-text-secondary focus:border-primary/70"
          @change="commit('projectId')"
        >
          <option value="">unassigned</option>
          <option v-for="p in projects" :key="p.id" :value="p.id">{{ p.title }}</option>
        </select>
      </div>

      <div class="flex items-center justify-between gap-4">
        <span class="text-text-secondary">Due date</span>
        <div class="flex items-center gap-2">
          <span v-if="late" class="font-mono-data text-mono-data text-status-failed-text">Overdue</span>
          <button
            v-if="draft.dueDate"
            type="button"
            class="font-mono-data text-mono-data text-text-muted hover:text-text-primary outline-none focus-visible:ring-1 focus-visible:ring-primary/30 cursor-pointer"
            @click="draft.dueDate = ''; commit('dueDate')"
          >
            Clear
          </button>
          <input
            v-model="draft.dueDate"
            type="date"
            class="h-7 px-2 rounded-lg bg-bg-elevated border border-border-default outline-none font-mono-data text-mono-data text-text-secondary [color-scheme:dark] focus:border-primary/70"
            @change="commit('dueDate')"
          />
        </div>
      </div>
    </div>

    <div v-if="isEdit" class="px-[22px] py-4 flex flex-col gap-2 font-mono-data text-mono-data">
      <div class="flex items-baseline justify-between gap-3">
        <span class="text-text-muted">Created</span>
        <span class="text-text-label">{{ formatTimestamp(task!.createdAt) }}</span>
      </div>
      <div class="flex items-baseline justify-between gap-3">
        <span class="text-text-muted">Updated</span>
        <span class="text-text-label">{{ formatTimestamp(task!.updatedAt) }}</span>
      </div>
    </div>

    <div class="flex-1 min-h-3"></div>

    <div class="px-[22px] pt-[18px] pb-7 flex items-center justify-between gap-2.5">
      <UiButton v-if="isEdit" size="sm" variant="ghost" icon="delete" @click="$emit('delete', task!)">Delete</UiButton>
      <div class="flex-1"></div>
      <UiButton size="sm" variant="ghost" @click="$emit('close')">{{ isEdit ? 'Done' : 'Cancel' }}</UiButton>
      <UiButton v-if="!isEdit" size="sm" variant="primary" :disabled="!draft.title.trim()" @click="submitCreate">
        Create task
      </UiButton>
    </div>
  </aside>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import type { Task, TaskStatus, NewTask, TaskPatch } from '~/composables/useTasks'
import { TASK_STATUSES, isFinished } from '~/composables/useTasks'
import type { Project } from '~/composables/useProjects'
import { formatTimestamp, isOverdue } from '~/utils/time'
import UiButton from '~/components/ui/UiButton.vue'
import UiIconButton from '~/components/ui/UiIconButton.vue'

const props = defineProps<{
  /** null puts the panel in create mode; nothing is written until Create task. */
  task: Task | null
  projects: Project[]
  briefing?: { body: string } | null
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'patch', id: string, patch: TaskPatch): void
  (e: 'create', task: NewTask): void
  (e: 'delete', task: Task): void
}>()

const isEdit = computed(() => props.task !== null)

type Draft = {
  title: string
  description: string
  status: TaskStatus
  projectId: string
  dueDate: string
  tags: string[]
}

const blank = (): Draft => ({
  title: '', description: '', status: 'todo', projectId: '', dueDate: '', tags: [],
})

const draft = ref<Draft>(blank())
const tagsText = ref('')
const titleRef = ref<HTMLTextAreaElement | null>(null)
const descRef = ref<HTMLTextAreaElement | null>(null)
const panelRef = ref<HTMLElement | null>(null)

// A task nobody is doing is not late, whether it was finished or called off.
const late = computed(() => !isFinished(draft.value.status) && isOverdue(draft.value.dueDate))

function autosize(el: HTMLTextAreaElement | null) {
  if (!el) return
  el.style.height = 'auto'
  el.style.height = `${el.scrollHeight}px`
}

const parseTags = (text: string) =>
  text.split(',').map((t) => t.trim()).filter(Boolean)

// Editing autosaves per field, on blur. Creating cannot: there is no node to
// patch until the first write, which is what the Create button is for.
function commit(field: keyof Draft) {
  const task = props.task
  if (!task) return
  const d = draft.value
  switch (field) {
    case 'title': {
      const next = d.title.trim()
      // Mirrors the API guard: a blank title is refused there, so restore the
      // last good one rather than send a patch that will fail.
      if (!next) { d.title = task.title; return }
      if (next !== task.title) emit('patch', task.id, { title: next })
      return
    }
    case 'description':
      if (d.description !== task.description) emit('patch', task.id, { description: d.description })
      return
    case 'status':
      if (d.status !== task.status) emit('patch', task.id, { status: d.status })
      return
    case 'projectId':
      if (d.projectId !== task.projectId) emit('patch', task.id, { projectId: d.projectId })
      return
    case 'dueDate':
      if (d.dueDate !== task.dueDate) emit('patch', task.id, { dueDate: d.dueDate })
      return
    case 'tags': {
      const next = parseTags(tagsText.value)
      if (next.join(',') !== (task.tags ?? []).join(',')) emit('patch', task.id, { tags: next })
      return
    }
  }
}

function submitCreate() {
  const d = draft.value
  if (!d.title.trim()) return
  emit('create', {
    title: d.title.trim(),
    description: d.description,
    status: d.status,
    projectId: d.projectId || undefined,
    dueDate: d.dueDate || undefined,
    tags: parseTags(tagsText.value),
  })
}

// A panel that autosaves still needs a way out that does not require finding
// the button. Ctrl/Cmd+Enter finishes from any field, Escape leaves.
function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') {
    e.stopPropagation()
    // Blur first, so the field being typed in commits. Escape here means
    // "leave", not "undo": every other edit in this panel saves itself, and
    // discarding only whichever field happened to have focus would be a rule
    // nobody could predict.
    ;(e.target as HTMLElement)?.blur?.()
    emit('close')
    return
  }
  if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
    e.preventDefault()
    if (isEdit.value) {
      ;(e.target as HTMLElement)?.blur?.() // flush the field being edited
      emit('close')
    } else {
      submitCreate()
    }
  }
}

// Reload the draft whenever the panel changes what it points at, and never
// carry a half-typed field across: a blur would then write it to another task.
watch(
  () => props.task?.id ?? null,
  async () => {
    const t = props.task
    draft.value = t
      ? {
          title: t.title,
          description: t.description,
          status: t.status,
          projectId: t.projectId,
          dueDate: t.dueDate,
          tags: t.tags ?? [],
        }
      : blank()
    tagsText.value = (t?.tags ?? []).join(', ')
    await nextTick()
    autosize(titleRef.value)
    autosize(descRef.value)
    // Focus lands in the panel on open, and on the title when there is none yet.
    // Create starts in the title; editing focuses the panel itself, so a
    // keystroke does not land in a field the user never aimed at.
    if (t) panelRef.value?.focus()
    else titleRef.value?.focus()
  },
  { immediate: true },
)

</script>

<style scoped>
.panel-title {
  font-size: 19px;
  font-weight: 600;
  line-height: 1.32;
  letter-spacing: -0.01em;
}

.da-icon {
  font-size: 14px;
  line-height: 1;
}

/* Capped, a long project title has to end somewhere; ending it in an ellipsis
   is what keeps the control the same size as the due date beside it. */
.project-select {
  text-overflow: ellipsis;
}
</style>
