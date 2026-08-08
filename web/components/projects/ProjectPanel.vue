<template>
  <aside
    ref="panelRef"
    class="w-96 shrink-0 border-l border-border-hairline bg-surface overflow-y-auto flex flex-col text-body"
    tabindex="-1"
    :aria-label="isEdit ? 'Edit project' : 'New project'"
    @keydown="onKeydown"
  >
    <div class="flex items-center justify-between px-[22px] pt-[18px]">
      <span class="text-text-label">{{ isEdit ? 'Edit project' : 'New project' }}</span>
      <UiIconButton icon="close" label="Close panel" size="sm" @click="$emit('close')" />
    </div>

    <div class="px-[22px] pt-2.5">
      <textarea
        ref="titleRef"
        v-model="draft.title"
        rows="1"
        placeholder="Untitled project"
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
        placeholder="What belongs in this project?"
        class="w-full resize-none overflow-hidden bg-transparent border-0 border-b border-border-default outline-none pb-2 text-text-secondary leading-relaxed transition-colors focus:border-b-primary focus:text-text-primary"
        @input="autosize(descRef)"
        @blur="commit('description')"
      ></textarea>
    </div>

    <div class="h-px bg-border-hairline"></div>

    <div class="px-[22px] py-4 border-b border-border-hairline flex flex-col gap-3.5">
      <div class="flex items-center justify-between gap-4">
        <span class="text-text-secondary">Status</span>
        <div class="flex rounded-lg border border-border-default overflow-hidden">
          <button
            v-for="(s, i) in PROJECT_STATUSES"
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
        <div>
          <div class="text-text-secondary">Pinned</div>
          <div class="mt-0.5 text-mono-data text-text-muted">Keeps it at the top of Projects.</div>
        </div>
        <button
          type="button"
          class="flex items-center gap-1.5 h-7 px-2.5 rounded-lg border cursor-pointer transition-colors outline-none focus-visible:ring-1 focus-visible:ring-primary/30"
          :class="draft.pinned
            ? 'border-accent-edge bg-accent-tint text-primary'
            : 'border-border-default text-text-secondary hover:bg-surface-nav-hover'"
          :aria-pressed="draft.pinned"
          @click="draft.pinned = !draft.pinned; commit('pinned')"
        >
          <span class="material-symbols-outlined pin-icon" aria-hidden="true">keep</span>
          {{ draft.pinned ? 'Pinned' : 'Pin' }}
        </button>
      </div>
    </div>

    <!-- The project's own tasks, so the panel answers "what is in here" without
         a trip to the Tasks index and back. -->
    <div v-if="isEdit" class="px-[22px] pt-4 pb-2">
      <div class="flex items-baseline justify-between mb-1.5">
        <h3 class="font-headline text-headline text-text-primary">Tasks</h3>
        <span class="font-mono-data text-mono-data text-text-muted tabular-nums">{{ taskSummary }}</span>
      </div>

      <p v-if="tasksLoading" class="py-2.5 text-text-muted">Loading…</p>
      <p v-else-if="!tasks.length" class="py-2.5 text-text-muted">No tasks in this project yet.</p>

      <div v-for="(group, i) in taskGroups" :key="group.id" class="mt-2.5">
        <div v-if="i > 0" class="h-px bg-border-hairline mb-2.5"></div>
        <component
          :is="group.collapsible ? 'button' : 'div'"
          :type="group.collapsible ? 'button' : undefined"
          class="w-full flex items-center gap-[7px] -mx-2 px-2 py-1.5 rounded text-text-muted select-none"
          :class="group.collapsible ? 'cursor-pointer hover:bg-surface-nav-hover' : 'cursor-default'"
          :aria-expanded="group.collapsible ? showDone : undefined"
          @click="group.collapsible && (showDone = !showDone)"
        >
          <span
            v-if="group.collapsible"
            class="material-symbols-outlined group-chevron transition-transform duration-150"
            :class="showDone ? 'rotate-0' : '-rotate-90'"
            aria-hidden="true"
            >expand_more</span
          >
          <span class="font-label-caps text-label-caps uppercase">{{ group.label }}</span>
          <span class="font-mono-data text-mono-data text-text-faint">{{ group.tasks.length }}</span>
        </component>

        <div
          v-for="t in group.open ? group.tasks : []"
          :key="t.id"
          class="nested group/task flex items-start gap-[9px] -mx-2 px-2 py-[5px] rounded hover:bg-surface-nav-hover transition-colors duration-120"
        >
          <span
            class="mt-1.5 w-[5px] h-[5px] shrink-0 rounded-full"
            :class="t.status === 'done' ? 'bg-text-dim' : 'bg-primary'"
          ></span>
          <span
            class="flex-1 min-w-0 text-nested"
            :class="t.status === 'done' ? 'text-text-muted line-through' : 'text-text-secondary'"
          >
            {{ t.title }}
          </span>
          <div class="shrink-0 flex gap-px opacity-0 group-hover/task:opacity-100 group-focus-within/task:opacity-100 transition-opacity duration-150">
            <button
              type="button"
              class="task-action"
              :class="t.status === 'done' ? 'text-primary' : ''"
              :title="t.status === 'done' ? 'Reopen task' : 'Mark as done'"
              :aria-label="`${t.status === 'done' ? 'Reopen' : 'Complete'} ${t.title}`"
              @click="$emit('toggle-task', t)"
            >
              <span class="material-symbols-outlined task-action-icon" aria-hidden="true">check</span>
            </button>
            <NuxtLink
              :to="`/tasks?project=${encodeURIComponent(project!.id)}`"
              class="task-action"
              title="Open in Tasks"
              :aria-label="`Open ${t.title} in Tasks`"
            >
              <span class="material-symbols-outlined task-action-icon" aria-hidden="true">arrow_forward</span>
            </NuxtLink>
          </div>
        </div>
      </div>
    </div>

    <div class="flex-1 min-h-3"></div>

    <div class="px-[22px] pt-[18px] pb-7 flex items-center justify-between gap-2.5">
      <UiButton v-if="isEdit" size="sm" variant="ghost" icon="delete" @click="$emit('delete', project!)">Delete</UiButton>
      <div class="flex-1"></div>
      <UiButton size="sm" variant="ghost" @click="$emit('close')">{{ isEdit ? 'Done' : 'Cancel' }}</UiButton>
      <UiButton v-if="!isEdit" size="sm" variant="primary" :disabled="!draft.title.trim()" @click="submitCreate">
        Create project
      </UiButton>
    </div>
  </aside>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import type { NewProject, Project, ProjectStatus } from '~/composables/useProjects'
import { PROJECT_STATUSES } from '~/composables/useProjects'
import { useTasks, type Task, type TaskStatus } from '~/composables/useTasks'
import UiButton from '~/components/ui/UiButton.vue'
import UiIconButton from '~/components/ui/UiIconButton.vue'

const props = defineProps<{
  /** null puts the panel in create mode; nothing is written until Create project. */
  project: Project | null
}>()

type ProjectPatch = { title?: string; description?: string; status?: ProjectStatus; pinned?: boolean; tags?: string[] }

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'patch', id: string, patch: ProjectPatch): void
  (e: 'create', project: NewProject): void
  (e: 'delete', project: Project): void
  (e: 'toggle-task', task: Task): void
}>()

const isEdit = computed(() => props.project !== null)

type Draft = { title: string; description: string; status: ProjectStatus; pinned: boolean; tags: string[] }
const blank = (): Draft => ({ title: '', description: '', status: 'active', pinned: false, tags: [] })

const draft = ref<Draft>(blank())
const tagsText = ref('')
const showDone = ref(false)
const titleRef = ref<HTMLTextAreaElement | null>(null)
const descRef = ref<HTMLTextAreaElement | null>(null)
const panelRef = ref<HTMLElement | null>(null)

// Its own useTasks instance: the composable hands back fresh refs per call, so
// the panel's project-scoped list cannot disturb whatever else is loaded.
const { tasks, loading: tasksLoading, load: loadTasks } = useTasks()

// In progress leads, to do follows, done is last and closed - the same order
// and the same default as the Tasks index, so the two do not disagree about
// what a backlog looks like.
const GROUPS: { id: TaskStatus; label: string }[] = [
  { id: 'in_progress', label: 'In progress' },
  { id: 'todo', label: 'To do' },
  { id: 'done', label: 'Done' },
]

const taskGroups = computed(() =>
  GROUPS.map((g) => ({
    id: g.id,
    label: g.label,
    collapsible: g.id === 'done',
    open: g.id === 'done' ? showDone.value : true,
    tasks: tasks.value.filter((t) => t.status === g.id),
  })).filter((g) => g.tasks.length > 0),
)

const taskSummary = computed(() => {
  if (!tasks.value.length) return ''
  const open = tasks.value.filter((t) => t.status !== 'done').length
  return `${open} open · ${tasks.value.length} total`
})

function autosize(el: HTMLTextAreaElement | null) {
  if (!el) return
  el.style.height = 'auto'
  el.style.height = `${el.scrollHeight}px`
}

const parseTags = (text: string) => text.split(',').map((t) => t.trim()).filter(Boolean)

// Editing autosaves per field, on blur. Creating cannot: there is no node to
// patch until the first write, which is what the Create button is for.
function commit(field: keyof Draft) {
  const p = props.project
  if (!p) return
  const d = draft.value
  switch (field) {
    case 'title': {
      const next = d.title.trim()
      // Mirrors the API guard - a blank title is refused there, so restore the
      // last good one rather than send a patch that will fail.
      if (!next) { d.title = p.title; return }
      if (next !== p.title) emit('patch', p.id, { title: next })
      return
    }
    case 'description':
      if (d.description !== p.description) emit('patch', p.id, { description: d.description })
      return
    case 'status':
      if (d.status !== p.status) emit('patch', p.id, { status: d.status })
      return
    case 'pinned':
      if (d.pinned !== p.pinned) emit('patch', p.id, { pinned: d.pinned })
      return
    case 'tags': {
      const next = parseTags(tagsText.value)
      if (next.join(',') !== (p.tags ?? []).join(',')) emit('patch', p.id, { tags: next })
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
    pinned: d.pinned,
    tags: parseTags(tagsText.value),
  })
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') {
    e.stopPropagation()
    // Blur first so the field being typed in commits. Escape means "leave",
    // not "undo": every other edit here saves itself.
    ;(e.target as HTMLElement)?.blur?.()
    emit('close')
    return
  }
  if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
    e.preventDefault()
    if (isEdit.value) {
      ;(e.target as HTMLElement)?.blur?.()
      emit('close')
    } else {
      submitCreate()
    }
  }
}

watch(
  () => props.project?.id ?? null,
  async (id) => {
    const p = props.project
    draft.value = p
      ? { title: p.title, description: p.description, status: p.status, pinned: p.pinned, tags: p.tags ?? [] }
      : blank()
    tagsText.value = (p?.tags ?? []).join(', ')
    showDone.value = false
    if (id) loadTasks(id)
    await nextTick()
    autosize(titleRef.value)
    autosize(descRef.value)
    // Create starts in the title; editing focuses the panel itself, so a
    // keystroke does not land in a field the user never aimed at.
    if (p) panelRef.value?.focus()
    else titleRef.value?.focus()
  },
  { immediate: true },
)

// The parent applies the write; the panel refetches so its own list agrees.
defineExpose({ reloadTasks: () => props.project && loadTasks(props.project.id) })
</script>

<style scoped>
.panel-title {
  font-size: 19px;
  font-weight: 600;
  line-height: 1.32;
  letter-spacing: -0.01em;
}

.pin-icon {
  font-size: 13px;
  line-height: 1;
}

.group-chevron {
  font-size: 12px;
  line-height: 1;
  color: var(--color-text-faint);
}

/* A step below the body: these are nested inside a panel that is itself
   secondary to the list, and they are read as a summary rather than as work. */
.text-nested {
  font-size: 12.5px;
  line-height: 1.45;
}

.task-action {
  display: flex;
  padding: 3px;
  border-radius: var(--radius);
  color: var(--color-text-muted);
  cursor: pointer;
  transition: background-color 120ms ease-out, color 120ms ease-out;
}
.task-action:hover {
  background-color: var(--color-surface-control-hover);
  color: var(--color-text-primary);
}
.task-action-icon {
  font-size: 12px;
  line-height: 1;
}
</style>
