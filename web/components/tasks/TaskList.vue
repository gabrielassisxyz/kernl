<template>
  <div class="flex-1 overflow-auto px-section py-base">
    <table class="w-full border-collapse">
      <thead>
        <tr class="border-b border-border-hairline text-left">
          <th class="w-6 py-base pr-base"></th>
          <th class="py-base pr-section font-label-caps text-label-caps text-text-muted uppercase">Title</th>
          <th class="py-base pr-section font-label-caps text-label-caps text-text-muted uppercase">Project</th>
          <th class="py-base pr-section font-label-caps text-label-caps text-text-muted uppercase">Status</th>
          <th class="py-base pr-section font-label-caps text-label-caps text-text-muted uppercase whitespace-nowrap">Due</th>
          <th class="py-base font-label-caps text-label-caps text-text-muted uppercase whitespace-nowrap">Updated</th>
        </tr>
      </thead>
      <tbody>
        <tr v-if="!openTasks.length && doneTasks.length" class="border-b border-border-hairline">
          <td class="py-base pr-base"></td>
          <td colspan="5" class="py-base font-body text-body text-text-muted">Nothing open.</td>
        </tr>

        <template v-for="row in rows" :key="row.key">
          <tr v-if="!row.task" class="border-b border-border-hairline">
            <td colspan="6" class="py-tight">
              <button
                type="button"
                class="flex items-center gap-tight py-base font-label-caps text-label-caps text-text-muted uppercase hover:text-text-primary transition-colors cursor-pointer outline-none focus-visible:ring-1 focus-visible:ring-primary/30"
                :aria-expanded="showDone"
                @click="showDone = !showDone"
              >
                <span
                  class="material-symbols-outlined !text-[18px] transition-transform duration-150"
                  :class="showDone ? '' : '-rotate-90'"
                  aria-hidden="true"
                  >expand_more</span
                >
                Done
                <span class="font-mono-data text-mono-data text-text-faint normal-case">{{ doneTasks.length }}</span>
              </button>
            </td>
          </tr>

          <tr
            v-else
            class="group border-b border-border-hairline hover:bg-surface-hover cursor-pointer transition-colors duration-150"
            @click="$emit('open', row.task)"
          >
            <td class="py-base pr-base align-middle">
              <span class="block w-1.5 h-1.5 rounded-full" :class="dotClass(row.task)"></span>
            </td>
            <td
              class="py-base pr-section font-body text-body max-w-0 truncate"
              :class="row.task.status === 'done' ? 'text-text-muted line-through' : 'text-text-primary'"
            >
              {{ row.task.title }}
            </td>
            <td class="py-base pr-section font-mono-data text-mono-data text-text-faint whitespace-nowrap">
              {{ projectTitles[row.task.projectId] || ' - ' }}
            </td>
            <td class="py-base pr-section font-mono-data text-mono-data text-text-faint whitespace-nowrap">{{ statusLabel(row.task) }}</td>
            <td
              class="py-base pr-section font-mono-data text-mono-data whitespace-nowrap"
              :class="late(row.task) ? 'text-status-failed-text' : 'text-text-faint'"
            >
              {{ formatDueDate(row.task.dueDate) || ' - ' }}
            </td>
            <td class="py-base font-mono-data text-mono-data text-text-faint whitespace-nowrap">{{ formatTimestamp(row.task.updatedAt) }}</td>
          </tr>
        </template>
      </tbody>
    </table>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import type { Task, TaskStatus } from '~/composables/useTasks'
import { TASK_STATUSES } from '~/composables/useTasks'
import { formatDueDate, formatTimestamp, isOverdue } from '~/utils/time'

const props = defineProps<{ tasks: Task[]; projectTitles: Record<string, string> }>()
defineEmits<{ (e: 'open', task: Task): void }>()

const openTasks = computed(() => props.tasks.filter((t) => t.status !== 'done'))
const doneTasks = computed(() => props.tasks.filter((t) => t.status === 'done'))

// Collapsed on arrival. A backlog with history is mostly history - the import
// this was sized for is 258 finished entries against 55 open ones - so an
// expanded done section buries the open work under its own archive. The kanban
// is untouched: it already gives done a column of its own, and a column that
// holds its own scroll drowns nothing.
const showDone = ref(false)

// One table with one header rather than two stacked ones: the done rows carry
// the same six columns, and a second <table> would give them their own widths
// and its own scroll container inside this one.
const rows = computed<{ key: string; task?: Task }[]>(() => {
  const out: { key: string; task?: Task }[] = openTasks.value.map((t) => ({ key: t.id, task: t }))
  if (!doneTasks.value.length) return out
  out.push({ key: 'done-toggle' })
  if (showDone.value) out.push(...doneTasks.value.map((t) => ({ key: t.id, task: t })))
  return out
})

const STATUS_DOT: Record<TaskStatus, string> = {
  todo: 'bg-text-dim',
  in_progress: 'bg-status-running',
  done: 'bg-status-passed',
}
const dotClass = (t: Task) => STATUS_DOT[t.status] ?? 'bg-text-dim'
const statusLabel = (t: Task) => TASK_STATUSES.find((s) => s.id === t.status)?.label ?? t.status
// A finished task is never late, however old its deadline.
const late = (t: Task) => t.status !== 'done' && isOverdue(t.dueDate)
</script>
