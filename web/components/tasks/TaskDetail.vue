<template>
  <Transition
    enter-active-class="transition-opacity duration-150"
    enter-from-class="opacity-0"
    leave-active-class="transition-opacity duration-150"
    leave-to-class="opacity-0"
  >
    <div v-if="task" class="absolute inset-0 z-modal bg-black/40" @click="$emit('close')">
      <aside
        ref="panelRef"
        role="dialog"
        aria-modal="true"
        :aria-labelledby="titleId"
        class="absolute top-0 right-0 h-full w-full max-w-[440px] bg-surface border-l border-border-hairline flex flex-col"
        @click.stop
        @keydown="onPanelKeydown"
      >
        <!-- Header -->
        <header class="px-section py-component border-b border-border-hairline flex items-start gap-component">
          <span class="mt-[7px] w-1.5 h-1.5 rounded-full shrink-0" :class="dotClass"></span>
          <div class="flex-1 min-w-0">
            <h2
              :id="titleId"
              class="font-headline text-text-primary leading-snug"
              :class="{ 'line-through text-text-muted': task.status === 'done' }"
            >
              {{ task.title }}
            </h2>
            <div class="flex items-center gap-component mt-tight font-mono-data text-mono-data text-text-faint">
              <span v-if="projectTitle" class="flex items-center gap-tight">
                <span class="material-symbols-outlined !text-mono-data">folder_open</span>
                {{ projectTitle }}
              </span>
              <span v-if="briefing" class="flex items-center gap-tight text-da-accent-text">
                <span class="material-symbols-outlined !text-mono-data">auto_awesome</span>
                DA brief
              </span>
            </div>
          </div>
          <UiIconButton icon="close" label="Close task detail" @click="$emit('close')" />
        </header>

        <!-- Body -->
        <div class="flex-1 overflow-y-auto px-section py-component flex flex-col gap-section">
          <!-- DA briefing prepared from the originating capture -->
          <section v-if="briefing" class="rounded-lg border border-da-accent/30 bg-da-accent/[0.04] px-component py-base">
            <div class="flex items-center gap-tight mb-base">
              <span class="material-symbols-outlined !text-body text-da-accent-text">auto_awesome</span>
              <h3 class="font-label-caps text-label-caps text-da-accent-text uppercase">DA Briefing</h3>
            </div>
            <p class="font-body text-body text-text-primary whitespace-pre-wrap">{{ briefing.body }}</p>
          </section>

          <section>
            <h3 class="font-label-caps text-label-caps text-text-muted uppercase mb-base">Status</h3>
            <UiSelect
              :model-value="task.status"
              classes="h-9 rounded border border-border-hairline bg-bg-base px-component font-body text-body text-text-primary outline-none transition-colors focus:border-primary/70"
              @change="$emit('set-status', task!.id, ($event.target as HTMLSelectElement).value)"
            >
              <option v-for="s in TASK_STATUSES" :key="s.id" :value="s.id">{{ s.label }}</option>
            </UiSelect>
          </section>

          <section>
            <h3 class="font-label-caps text-label-caps text-text-muted uppercase mb-base">Tags</h3>
            <TagInput v-model="tags" aria-label="Task tags" />
          </section>

          <section v-if="task.description">
            <h3 class="font-label-caps text-label-caps text-text-muted uppercase mb-base">Description</h3>
            <p class="font-body text-body text-text-primary whitespace-pre-wrap">{{ task.description }}</p>
          </section>
          <p v-else class="font-body text-body text-text-faint">No description.</p>
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
import { computed, nextTick, ref, watch } from 'vue'
import type { Task, TaskStatus } from '~/composables/useTasks'
import { TASK_STATUSES } from '~/composables/useTasks'
import { formatTimestamp } from '~/utils/time'
import UiIconButton from '~/components/ui/UiIconButton.vue'
import UiSelect from '~/components/ui/UiSelect.vue'
import TagInput from '~/components/tags/TagInput.vue'

const props = defineProps<{ task: Task | null; projectTitle?: string }>()
const emit = defineEmits<{
  (e: 'close'): void
  (e: 'set-status', id: string, status: string): void
  (e: 'set-tags', id: string, tags: string[]): void
}>()

// This drawer is where a task's tags are edited — the tasks page has no edit modal.
// The write is immediate (a chip added is a chip meant), and the parent owns it, the
// same way it owns the status write.
const tags = ref<string[]>([])
watch(() => props.task, (task) => {
  tags.value = [...(task?.tags ?? [])]
}, { immediate: true })

watch(tags, (next) => {
  const id = props.task?.id
  if (!id) return
  const current = props.task?.tags ?? []
  // Only a real change is worth a request: this also fires when the drawer opens
  // and the watch above seeds the chips from the task.
  if (next.length === current.length && next.every((t, i) => t === current[i])) return
  emit('set-tags', id, next)
})

// --- Dialog semantics & focus management ---
const panelRef = ref<HTMLElement | null>(null)
const titleId = 'task-detail-title'
// Selector covers all standard interactive elements; excludes disabled ones.
const FOCUSABLE = 'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'
let previousFocus: HTMLElement | null = null

function getFocusableEls(): HTMLElement[] {
  if (!panelRef.value) return []
  return Array.from(panelRef.value.querySelectorAll<HTMLElement>(FOCUSABLE))
}

function onPanelKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') {
    // Stop propagation so the parent's window-level handler doesn't also fire.
    e.stopPropagation()
    emit('close')
    return
  }
  if (e.key === 'Tab') {
    const focusable = getFocusableEls()
    if (!focusable.length) { e.preventDefault(); return }
    const first = focusable[0]
    const last = focusable[focusable.length - 1]
    if (e.shiftKey) {
      if (document.activeElement === first) { e.preventDefault(); last.focus() }
    } else {
      if (document.activeElement === last) { e.preventDefault(); first.focus() }
    }
  }
}

// On open: save current focus and move into the panel.
// On close: restore focus to the element that triggered the drawer.
watch(() => props.task, async (newTask, oldTask) => {
  if (newTask && !oldTask) {
    previousFocus = document.activeElement as HTMLElement | null
    await nextTick()
    getFocusableEls()[0]?.focus()
  } else if (!newTask && oldTask) {
    previousFocus?.focus()
    previousFocus = null
  }
})

// DA briefing surfaced for this task (via its briefing edge to the prep note).
const briefing = ref<{ id: string; title: string; body: string } | null>(null)
watch(() => props.task?.id, async (id) => {
  briefing.value = null
  if (!id) return
  try {
    briefing.value = await $fetch<{ id: string; title: string; body: string }>(`/api/nodes/${id}/briefing`)
  } catch {
    briefing.value = null // 404 = no briefing
  }
}, { immediate: true })

const STATUS_DOT: Record<TaskStatus, string> = {
  todo: 'bg-text-dim',
  in_progress: 'bg-status-running',
  done: 'bg-status-passed',
}
const dotClass = computed(() => (props.task ? STATUS_DOT[props.task.status] ?? 'bg-text-dim' : 'bg-text-dim'))
</script>
