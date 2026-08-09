<template>
  <div
    class="group flex flex-col p-3 rounded-xl border bg-surface text-body cursor-pointer transition-colors duration-150"
    :class="hovered ? 'border-border-default bg-surface-hover' : 'border-border-hairline'"
    @mouseenter="hovered = true"
    @mouseleave="hovered = false"
    @click="$emit('open', task)"
  >
    <div class="flex items-start gap-2">
      <span
        class="flex-1 min-w-0 leading-snug"
        :class="done ? 'text-text-muted line-through' : 'text-text-primary'"
      >
        {{ task.title }}
      </span>
    </div>

    <div class="flex items-center gap-2 mt-2">
      <span
        v-if="projectTitle"
        class="min-w-0 truncate font-mono-data text-mono-data text-text-muted bg-surface-chip rounded-sm px-[5px] py-px"
      >
        {{ projectTitle }}
      </span>
      <span
        v-if="task.dueDate"
        class="shrink-0 font-mono-data text-mono-data"
        :class="late ? 'text-status-failed-text' : 'text-text-muted'"
      >
        {{ formatDueDate(task.dueDate) }}
      </span>
      <div class="flex-1"></div>
      <div class="flex gap-px opacity-0 group-hover:opacity-100 group-focus-within:opacity-100 transition-opacity duration-150" @click.stop>
        <button
          type="button"
          class="card-action"
          :title="advanceTitle"
          :aria-label="`${advanceTitle}: ${task.title}`"
          @click="$emit('advance', task)"
        >
          <span class="material-symbols-outlined card-action-icon" aria-hidden="true">arrow_forward</span>
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import type { Task } from '~/composables/useTasks'
import { formatDueDate, isOverdue } from '~/utils/time'

const props = defineProps<{ task: Task; projectTitle?: string }>()
defineEmits<{ (e: 'open', task: Task): void; (e: 'advance', task: Task): void }>()

const hovered = ref(false)
const done = computed(() => props.task.status === 'done')

// A finished task is never late, however old its deadline.
const late = computed(() => !done.value && isOverdue(props.task.dueDate))

const ADVANCE_TITLE = { todo: 'Start', in_progress: 'Complete', done: 'Reopen' } as const
const advanceTitle = computed(() => ADVANCE_TITLE[props.task.status] ?? 'Advance')
</script>

<style scoped>
.card-action {
  display: flex;
  padding: 3px;
  border-radius: var(--radius);
  color: var(--color-text-muted);
  cursor: pointer;
  transition: background-color 120ms ease-out, color 120ms ease-out;
}
.card-action:hover {
  background-color: var(--color-surface-control-hover);
  color: var(--color-primary);
}
.card-action-icon {
  font-size: 12px;
  line-height: 1;
}
</style>
