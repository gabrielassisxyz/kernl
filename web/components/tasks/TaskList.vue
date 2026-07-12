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
        <tr
          v-for="task in tasks"
          :key="task.id"
          class="group border-b border-border-hairline hover:bg-surface-hover cursor-pointer transition-colors duration-150"
          @click="$emit('open', task)"
        >
          <td class="py-base pr-base align-middle">
            <span class="block w-1.5 h-1.5 rounded-full" :class="dotClass(task)"></span>
          </td>
          <td
            class="py-base pr-section font-body text-body max-w-0 truncate"
            :class="task.status === 'done' ? 'text-text-muted line-through' : 'text-text-primary'"
          >
            {{ task.title }}
          </td>
          <td class="py-base pr-section font-mono-data text-mono-data text-text-faint whitespace-nowrap">
            {{ projectTitles[task.projectId] || '—' }}
          </td>
          <td class="py-base pr-section font-mono-data text-mono-data text-text-faint whitespace-nowrap">{{ statusLabel(task) }}</td>
          <td
            class="py-base pr-section font-mono-data text-mono-data whitespace-nowrap"
            :class="late(task) ? 'text-status-failed-text' : 'text-text-faint'"
          >
            {{ formatDueDate(task.dueDate) || '—' }}
          </td>
          <td class="py-base font-mono-data text-mono-data text-text-faint whitespace-nowrap">{{ formatTimestamp(task.updatedAt) }}</td>
        </tr>
      </tbody>
    </table>
  </div>
</template>

<script setup lang="ts">
import type { Task, TaskStatus } from '~/composables/useTasks'
import { TASK_STATUSES } from '~/composables/useTasks'
import { formatDueDate, formatTimestamp, isOverdue } from '~/utils/time'

defineProps<{ tasks: Task[]; projectTitles: Record<string, string> }>()
defineEmits<{ (e: 'open', task: Task): void }>()

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
