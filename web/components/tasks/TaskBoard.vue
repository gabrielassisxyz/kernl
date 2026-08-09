<template>
  <div class="task-board flex-1 min-w-0 overflow-hidden">
    <!-- The columns share the width instead of holding a fixed one: a board
         that scrolls sideways hides a whole column behind an edge, and the
         count of columns is small enough for the split to stay readable. -->
    <div class="flex gap-section h-full min-w-0 px-section py-base">
      <section
        v-for="col in TASK_STATUSES"
        :key="col.id"
        class="flex flex-col flex-1 min-w-0 h-full"
      >
        <!-- Column header -->
        <div class="flex items-center gap-2 pb-2 mb-2.5 border-b border-border-hairline">
          <h2 class="flex-1 min-w-0 truncate font-label-caps text-label-caps text-text-label uppercase">{{ col.label }}</h2>
          <span class="font-mono-data text-mono-data text-text-faint">{{ grouped[col.id].length }}</span>
        </div>

        <!-- Cards -->
        <div class="column-scroll flex-1 overflow-y-auto flex flex-col gap-base pb-base pr-tight">
          <TaskCard
            v-for="task in grouped[col.id]"
            :key="task.id"
            :task="task"
            :project-title="projectTitles[task.projectId]"
            @open="$emit('open', task)"
            @advance="$emit('advance', task)"
          />
          <div
            v-if="grouped[col.id].length === 0"
            class="flex items-center justify-center py-section text-text-faint font-body text-body select-none"
          >
            &mdash;
          </div>
        </div>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { type Task, type TaskStatus, TASK_STATUSES } from '~/composables/useTasks'
import TaskCard from '~/components/tasks/TaskCard.vue'

const props = defineProps<{ tasks: Task[]; projectTitles: Record<string, string> }>()
defineEmits<{ (e: 'open', task: Task): void; (e: 'advance', task: Task): void }>()

const grouped = computed<Record<TaskStatus, Task[]>>(() => {
  const buckets: Record<TaskStatus, Task[]> = { todo: [], in_progress: [], done: [], closed: [] }
  for (const task of props.tasks) {
    ;(buckets[task.status] ?? buckets.todo).push(task)
  }
  return buckets
})
</script>

<style scoped>
/* The board is the one surface where a scrollbar sits inside the layout rather
   than at its edge, so the platform default reads as a seam between columns.
   The thumb is inset by its own border, which is what keeps it off the cards. */
.column-scroll {
  scrollbar-width: thin;
  scrollbar-color: var(--color-surface-control-hover) transparent;
}
.column-scroll::-webkit-scrollbar {
  width: 8px;
}
.column-scroll::-webkit-scrollbar-track {
  background: transparent;
}
.column-scroll::-webkit-scrollbar-thumb {
  background-color: var(--color-surface-control-hover);
  border: 2px solid transparent;
  background-clip: content-box;
  border-radius: var(--radius-full);
}
.column-scroll:hover::-webkit-scrollbar-thumb {
  background-color: var(--color-border-default);
}
</style>
