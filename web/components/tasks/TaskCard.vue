<template>
  <div
    class="group flex flex-col gap-base p-component rounded-lg border border-border-hairline bg-surface hover:bg-surface-hover hover:border-border-default transition-colors duration-150 cursor-pointer"
    @click="$emit('open', task)"
  >
    <div class="flex items-start gap-base">
      <span class="mt-[6px] w-1.5 h-1.5 rounded-full shrink-0" :class="dotClass"></span>
      <h3
        class="font-headline text-text-primary leading-snug"
        :class="{ 'line-through text-text-muted': task.status === 'done' }"
      >
        {{ task.title }}
      </h3>
    </div>

    <div v-if="projectTitle" class="pl-component">
      <span class="inline-flex items-center gap-tight font-mono-data text-[10px] text-text-dim">
        <span class="material-symbols-outlined !text-[12px]">folder_open</span>
        {{ projectTitle }}
      </span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { Task, TaskStatus } from '~/composables/useTasks'

const props = defineProps<{ task: Task; projectTitle?: string }>()
defineEmits<{ (e: 'open', task: Task): void }>()

const STATUS_DOT: Record<TaskStatus, string> = {
  todo: 'bg-text-dim',
  in_progress: 'bg-status-running',
  done: 'bg-status-passed',
}
const dotClass = computed(() => STATUS_DOT[props.task.status] ?? 'bg-text-dim')
</script>
