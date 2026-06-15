<template>
  <Transition
    enter-active-class="transition-opacity duration-150"
    enter-from-class="opacity-0"
    leave-active-class="transition-opacity duration-150"
    leave-to-class="opacity-0"
  >
    <div v-if="task" class="absolute inset-0 z-40 bg-black/40" @click="$emit('close')">
      <aside
        class="absolute top-0 right-0 h-full w-full max-w-[440px] bg-surface border-l border-border-hairline flex flex-col"
        @click.stop
      >
        <!-- Header -->
        <header class="px-section py-component border-b border-border-hairline flex items-start gap-component">
          <span class="mt-[7px] w-1.5 h-1.5 rounded-full shrink-0" :class="dotClass"></span>
          <div class="flex-1 min-w-0">
            <h2
              class="font-headline text-text-primary leading-snug"
              :class="{ 'line-through text-text-muted': task.status === 'done' }"
            >
              {{ task.title }}
            </h2>
            <div v-if="projectTitle" class="flex items-center gap-tight mt-tight font-mono-data text-mono-data text-text-dim">
              <span class="material-symbols-outlined !text-[12px]">folder_open</span>
              {{ projectTitle }}
            </div>
          </div>
          <button
            class="text-text-muted hover:text-text-primary transition-colors duration-150 shrink-0 cursor-pointer"
            title="Close"
            @click="$emit('close')"
          >
            <span class="material-symbols-outlined !text-[20px]">close</span>
          </button>
        </header>

        <!-- Body -->
        <div class="flex-1 overflow-y-auto px-section py-component flex flex-col gap-section">
          <section>
            <h3 class="font-label-caps text-label-caps text-text-muted uppercase mb-base">Status</h3>
            <select
              :value="task.status"
              class="bg-bg-base border border-border-hairline rounded-md px-component py-base font-body text-body text-text-primary focus:outline-none focus:border-primary/60"
              @change="$emit('set-status', task!.id, ($event.target as HTMLSelectElement).value)"
            >
              <option v-for="s in TASK_STATUSES" :key="s.id" :value="s.id">{{ s.label }}</option>
            </select>
          </section>

          <section v-if="task.description">
            <h3 class="font-label-caps text-label-caps text-text-muted uppercase mb-base">Description</h3>
            <p class="font-body text-body text-text-primary whitespace-pre-wrap">{{ task.description }}</p>
          </section>
          <p v-else class="font-body text-body text-text-dim">No description.</p>
        </div>

        <!-- Footer meta -->
        <footer class="px-section py-base border-t border-border-hairline flex items-center justify-end">
          <span class="font-mono-data text-mono-data text-text-faint">updated {{ formatTimestamp(task.updatedAt) }}</span>
        </footer>
      </aside>
    </div>
  </Transition>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { Task, TaskStatus } from '~/composables/useTasks'
import { TASK_STATUSES } from '~/composables/useTasks'
import { formatTimestamp } from '~/utils/time'

const props = defineProps<{ task: Task | null; projectTitle?: string }>()
defineEmits<{ (e: 'close'): void; (e: 'set-status', id: string, status: string): void }>()

const STATUS_DOT: Record<TaskStatus, string> = {
  todo: 'bg-text-dim',
  in_progress: 'bg-status-running',
  done: 'bg-status-passed',
}
const dotClass = computed(() => (props.task ? STATUS_DOT[props.task.status] ?? 'bg-text-dim' : 'bg-text-dim'))
</script>
