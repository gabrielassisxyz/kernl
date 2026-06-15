<template>
  <div class="flex-1 overflow-x-auto overflow-y-hidden">
    <div class="flex gap-section h-full min-w-min px-section py-base">
      <section
        v-for="col in TASK_STATUSES"
        :key="col.id"
        class="flex flex-col w-[300px] shrink-0 h-full"
      >
        <!-- Column header -->
        <div class="flex items-center justify-between pb-base mb-base border-b border-border-hairline">
          <h2 class="font-label-caps text-label-caps text-text-muted uppercase">{{ col.label }}</h2>
          <span class="font-mono-data text-mono-data text-text-faint">{{ grouped[col.id].length }}</span>
        </div>

        <!-- Cards -->
        <div class="flex-1 overflow-y-auto flex flex-col gap-base pb-base pr-tight">
          <TaskCard
            v-for="task in grouped[col.id]"
            :key="task.id"
            :task="task"
            :project-title="projectTitles[task.projectId]"
            @open="$emit('open', task)"
          />
          <div
            v-if="grouped[col.id].length === 0"
            class="flex items-center justify-center py-section text-text-dim font-body text-body select-none"
          >
            —
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
defineEmits<{ (e: 'open', task: Task): void }>()

const grouped = computed<Record<TaskStatus, Task[]>>(() => {
  const buckets: Record<TaskStatus, Task[]> = { todo: [], in_progress: [], done: [] }
  for (const task of props.tasks) {
    ;(buckets[task.status] ?? buckets.todo).push(task)
  }
  return buckets
})
</script>
