<template>
  <UiModal
    :open="!!item"
    :title="item?.title || ''"
    kicker="PROCESS CAPTURE"
    size="lg"
    @close="$emit('close')"
  >
    <div class="flex flex-col gap-section">
      <UiField label="Convert to">
        <div class="grid grid-cols-3 gap-tight">
          <UiButton
            v-for="t in TARGETS"
            :key="t"
            class="flex-1"
            size="sm"
            :variant="draft.target === t ? targetVariant(t) : 'secondary'"
            :icon="TARGET_META[t].icon"
            @click="draft.target = t"
          >
            {{ TARGET_META[t].label }}
          </UiButton>
        </div>
      </UiField>

      <UiField
        v-if="draft.target !== 'discard' && draft.target !== 'update' && draft.target !== 'project'"
        :label="draft.target === 'task' ? 'Project' : 'Link to (optional)'"
      >
        <UiSelect v-model="draft.projectId" classes="h-8 w-full rounded border border-border-hairline bg-surface-container-low px-base font-body text-body text-text-primary outline-none transition-colors focus:border-primary/70 disabled:cursor-not-allowed disabled:opacity-50">
          <option value="">{{ draft.target === 'task' ? 'Unprocessed tasks (no project)' : 'Nothing' }}</option>
          <option v-for="p in projects" :key="p.id" :value="p.id">{{ p.title }}</option>
        </UiSelect>
      </UiField>

      <UiField v-if="draft.target !== 'discard' && draft.target !== 'update'" label="Title">
        <UiInput v-model="draft.title" classes="h-8 w-full rounded border border-border-hairline bg-surface-container-low px-base font-body text-body text-text-primary outline-none transition-colors placeholder:text-text-muted focus:border-primary/70 disabled:cursor-not-allowed disabled:opacity-50" />
      </UiField>

      <template v-if="draft.target === 'project'">
        <UiField label="Description">
          <UiTextarea v-model="draft.projectDescription" rows="4" classes="w-full rounded border border-border-hairline bg-surface-container-low px-base py-base font-body text-body text-text-primary outline-none transition-colors placeholder:text-text-muted focus:border-primary/70 disabled:cursor-not-allowed disabled:opacity-50 resize-none" />
        </UiField>
        <UiField label="Initial tasks" hint="One task per line. Kernl will create up to six tasks under the new project.">
          <UiTextarea v-model="draft.initialTasksText" rows="5" classes="w-full rounded border border-border-hairline bg-surface-container-low px-base py-base font-body text-body text-text-primary outline-none transition-colors placeholder:text-text-muted focus:border-primary/70 disabled:cursor-not-allowed disabled:opacity-50 resize-none" />
        </UiField>
      </template>

      <p v-if="draft.target === 'update'" class="font-body text-body text-text-muted">
        Kernl finds the best-matching note and proposes the changes to merge in.
        You'll review each suggested edit before anything is written.
      </p>
    </div>

    <template #footer>
      <div class="flex items-center justify-end gap-base">
        <UiButton variant="ghost" @click="$emit('close')">Cancel</UiButton>
        <UiButton variant="primary" :loading="busy" @click="confirm">Process</UiButton>
      </div>
    </template>
  </UiModal>
</template>

<script setup lang="ts">
import { reactive, watch } from 'vue'
import type { Project } from '~/composables/useProjects'
import { TARGETS, TARGET_META, normalizeTarget, type Target } from '~/utils/inboxTargets'
import type { InboxItemData } from '~/components/inbox/InboxItem.vue'
import UiButton from '~/components/ui/UiButton.vue'
import UiField from '~/components/ui/UiField.vue'
import UiInput from '~/components/ui/UiInput.vue'
import UiModal from '~/components/ui/UiModal.vue'
import UiSelect from '~/components/ui/UiSelect.vue'
import UiTextarea from '~/components/ui/UiTextarea.vue'
type ButtonVariant = 'primary' | 'secondary' | 'ghost' | 'danger' | 'success' | 'accent'

const props = defineProps<{
  item: InboxItemData | null
  projects: Project[]
  busy?: boolean
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'confirm', payload: { target: Target; projectId: string; linkTo: string; title: string; projectTitle?: string; projectDescription?: string; initialTasks?: string[] }): void
}>()

const draft = reactive<{ target: Target; projectId: string; title: string; projectDescription: string; initialTasksText: string }>({
  target: 'note',
  projectId: '',
  title: '',
  projectDescription: '',
  initialTasksText: '',
})

function targetVariant(target: Target): ButtonVariant {
  if (target === 'discard') return 'danger'
  if (target === 'task') return 'success'
  if (target === 'project') return 'accent'
  return 'accent'
}

// Re-seed the draft from the DA suggestion (or a neutral default) each time a
// new capture opens the modal.
watch(() => props.item, (item) => {
  if (!item) return
  draft.target = normalizeTarget(item.suggestedAction) ?? 'note'
  draft.projectId = item.suggestedProjectId ?? ''
  // Title tracks the capture itself by default; only a project target uses the
  // suggested project name (see the draft.target watcher below for the same
  // rule applied when the user switches targets interactively).
  draft.title = draft.target === 'project' && item.suggestedProjectTitle ? item.suggestedProjectTitle : item.title
  draft.projectDescription = item.suggestedProjectDescription || item.subtitle || ''
  draft.initialTasksText = (item.suggestedInitialTasks || []).join('\n')
}, { immediate: true })

// Keep the title aligned with the chosen target when the user flips it after
// opening: project -> suggested project name, note/task/link -> the capture's
// own title. Only swap the field while it still holds the previous target's
// un-edited value, so a title the user typed by hand is never overwritten.
watch(() => draft.target, (target, previousTarget) => {
  const item = props.item
  if (!item) return
  if (target === 'project') {
    if (item.suggestedProjectTitle && draft.title === item.title) {
      draft.title = item.suggestedProjectTitle
    }
    return
  }
  if (previousTarget === 'project' && draft.title === item.suggestedProjectTitle) {
    draft.title = item.title
  }
})

function taskLines(): string[] {
  return draft.initialTasksText
    .split('\n')
    .map(line => line.trim())
    .filter(Boolean)
}

function confirm() {
  if (!props.item) return
  // Update merges into a note (resolved + reviewed by the parent), so it carries
  // no project/link/title here.
  if (draft.target === 'update') {
    emit('confirm', { target: 'update', projectId: '', linkTo: '', title: '' })
    return
  }
  if (draft.target === 'project') {
    emit('confirm', {
      target: 'project',
      projectId: '',
      linkTo: '',
      title: draft.title,
      projectTitle: draft.title,
      projectDescription: draft.projectDescription,
      initialTasks: taskLines(),
    })
    return
  }
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
