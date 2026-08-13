<template>
  <div class="flex-1 min-h-0 overflow-y-auto flex flex-col px-7 pt-1 pb-[72px] text-body">
    <!-- Without this, a backlog that is entirely finished renders as one
         collapsed Done header over blank space, which reads as a failed load
         rather than as an answer. -->
    <p v-if="nothingOpen" class="mt-[26px] text-text-muted">Nothing open.</p>

    <section v-for="section in sections" :key="section.id" class="mt-[26px]">
      <button
        type="button"
        class="w-full flex items-baseline gap-2 pb-[7px] border-b border-border-hairline select-none cursor-pointer"
        :aria-expanded="section.open"
        @click="$emit('toggle-section', section.id)"
      >
        <span
          class="material-symbols-outlined self-center section-chevron transition-transform duration-150"
          :class="section.open ? 'rotate-0' : '-rotate-90'"
          aria-hidden="true"
          >expand_more</span
        >
        <span class="font-label-caps text-label-caps uppercase text-text-label">{{ section.label }}</span>
        <span class="font-mono-data text-mono-data text-text-faint">{{ section.tasks.length }}</span>
      </button>

      <TaskRow
        v-for="task in section.open ? section.tasks : []"
        :key="task.id"
        :task="task"
        :project-title="projectTitles[task.projectId]"
        :compact="compact"
        :confirming="confirmId === task.id"
        @open="$emit('open', $event)"
        @toggle-done="$emit('toggle-done', $event)"
        @advance="$emit('advance', $event)"
        @ask-delete="$emit('ask-delete', $event)"
        @confirm-delete="$emit('confirm-delete', $event)"
        @cancel-delete="$emit('cancel-delete')"
      />
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { isFinished, type Task, type TaskStatus } from '~/composables/useTasks'
import type { ListSortDir, ListSortField } from '~/composables/useListPreferences'
import TaskRow from '~/components/tasks/TaskRow.vue'

const props = defineProps<{
  tasks: Task[]
  projectTitles: Record<string, string>
  compact?: boolean
  confirmId?: string | null
  collapsed: Record<string, boolean>
  sortField: ListSortField
  sortDir: ListSortDir
}>()

defineEmits<{
  (e: 'open', task: Task): void
  (e: 'toggle-done', task: Task): void
  (e: 'advance', task: Task): void
  (e: 'ask-delete', task: Task): void
  (e: 'confirm-delete', task: Task): void
  (e: 'cancel-delete'): void
  (e: 'toggle-section', id: string): void
}>()

// In progress leads: it is the shortest section and the one being worked on.
// The two terminal states come last and keep themselves apart, because the
// difference between finishing something and calling it off is the reason
// closed exists at all.
const ORDER: { id: TaskStatus; label: string }[] = [
  { id: 'in_progress', label: 'In progress' },
  { id: 'todo', label: 'To do' },
  { id: 'done', label: 'Done' },
  { id: 'closed', label: 'Closed' },
]

// An empty section is dropped rather than rendered as a heading with nothing
// under it, which reads as something failing to load.
const nothingOpen = computed(
  () => props.tasks.length > 0 && props.tasks.every((t) => isFinished(t.status)),
)

const sections = computed(() =>
  ORDER.map((s) => ({
    id: s.id,
    label: s.label,
    open: !props.collapsed[s.id],
    tasks: sorted(props.tasks.filter((t) => t.status === s.id)),
  })).filter((s) => s.tasks.length > 0),
)

const sorted = (tasks: Task[]): Task[] => {
  const direction = props.sortDir === 'asc' ? 1 : -1
  return [...tasks].sort((a, b) => {
    if (props.sortField === 'name') return a.title.localeCompare(b.title) * direction
    const field = props.sortField === 'created' ? 'createdAt' : 'updatedAt'
    return (a[field] < b[field] ? -1 : a[field] > b[field] ? 1 : 0) * direction
  })
}
</script>

<style scoped>
.section-chevron {
  font-size: 12px;
  line-height: 1;
  color: var(--color-text-faint);
}
</style>
