<template>
  <Teleport to="body">
    <Transition name="fade">
      <div v-if="item" class="fixed inset-0 z-[60] flex items-center justify-center p-section bg-black/50" @click.self="$emit('close')">
        <div class="w-full max-w-lg flex flex-col rounded-lg border border-border-hairline bg-surface overflow-hidden">
          <header class="px-section py-component border-b border-border-hairline">
            <div class="font-mono-data text-[11px] text-text-faint mb-tight">PROCESS CAPTURE</div>
            <h2 class="font-headline text-headline text-text-primary">{{ item.title }}</h2>
          </header>

          <div class="px-section py-component flex flex-col gap-section">
            <!-- target -->
            <div>
              <div class="font-label-caps text-[10px] tracking-widest text-text-muted uppercase mb-base">Convert to</div>
              <div class="flex gap-tight">
                <button
                  v-for="t in TARGETS" :key="t"
                  class="flex-1 flex items-center justify-center gap-tight px-base py-1.5 rounded border font-mono-data text-[11px] transition-colors"
                  :class="draft.target === t ? TARGET_META[t].chip + ' font-medium' : 'border-border-hairline text-text-muted hover:text-text-primary'"
                  @click="draft.target = t"
                >
                  <span class="material-symbols-outlined !text-[13px]">{{ TARGET_META[t].icon }}</span>{{ TARGET_META[t].label }}
                </button>
              </div>
            </div>

            <!-- project (task) / link-to (note, bookmark) -->
            <div v-if="draft.target !== 'discard'">
              <div class="font-label-caps text-[10px] tracking-widest text-text-muted uppercase mb-base">
                {{ draft.target === 'task' ? 'Project' : 'Link to (optional)' }}
              </div>
              <select v-model="draft.projectId" class="w-full bg-surface-container-low border border-border-hairline rounded px-base py-1.5 font-body text-body text-text-primary">
                <option value="">{{ draft.target === 'task' ? '— Unprocessed tasks (no project) —' : '— Nothing —' }}</option>
                <option v-for="p in projects" :key="p.id" :value="p.id">{{ p.title }}</option>
              </select>
            </div>

            <!-- title -->
            <div v-if="draft.target !== 'discard'">
              <div class="font-label-caps text-[10px] tracking-widest text-text-muted uppercase mb-base">Title</div>
              <input v-model="draft.title" class="w-full bg-surface-container-low border border-border-hairline rounded px-base py-1.5 font-body text-body text-text-primary" />
            </div>
          </div>

          <footer class="px-section py-base border-t border-border-hairline flex items-center justify-end gap-base">
            <button class="px-component py-1.5 font-body text-body text-text-muted hover:text-text-primary transition-colors" @click="$emit('close')">Cancel</button>
            <button
              class="px-component py-1.5 rounded font-body text-body bg-primary/15 text-primary border border-primary/40 hover:bg-primary/25 transition-colors disabled:opacity-50"
              :disabled="busy"
              @click="confirm"
            >Process</button>
          </footer>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { reactive, watch } from 'vue'
import type { Project } from '~/composables/useProjects'
import { TARGETS, TARGET_META, normalizeTarget, type Target } from '~/utils/inboxTargets'
import type { InboxItemData } from '~/components/inbox/InboxItem.vue'

const props = defineProps<{
  item: InboxItemData | null
  projects: Project[]
  busy?: boolean
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'confirm', payload: { target: Target; projectId: string; linkTo: string; title: string }): void
}>()

const draft = reactive<{ target: Target; projectId: string; title: string }>({ target: 'note', projectId: '', title: '' })

// Re-seed the draft from the DA suggestion (or a neutral default) each time a
// new capture opens the modal.
watch(() => props.item, (item) => {
  if (!item) return
  draft.target = normalizeTarget(item.suggestedAction) ?? 'note'
  draft.projectId = item.suggestedProjectId ?? ''
  draft.title = item.title
}, { immediate: true })

function confirm() {
  if (!props.item) return
  // projectId only applies to a task; for note/bookmark the same select feeds linkTo.
  const isTask = draft.target === 'task'
  emit('confirm', {
    target: draft.target,
    projectId: isTask ? draft.projectId : '',
    linkTo: !isTask && draft.target !== 'discard' ? draft.projectId : '',
    title: draft.title,
  })
}
</script>

<style scoped>
.fade-enter-active, .fade-leave-active { transition: opacity 140ms ease; }
.fade-enter-from, .fade-leave-to { opacity: 0; }
</style>
