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
        <div class="flex gap-tight">
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
        v-if="draft.target !== 'discard'"
        :label="draft.target === 'task' ? 'Project' : 'Link to (optional)'"
      >
        <UiSelect v-model="draft.projectId" classes="h-8 w-full rounded border border-border-hairline bg-surface-container-low px-base font-body text-body text-text-primary outline-none transition-colors focus:border-primary/70 disabled:cursor-not-allowed disabled:opacity-50">
          <option value="">{{ draft.target === 'task' ? 'Unprocessed tasks (no project)' : 'Nothing' }}</option>
          <option v-for="p in projects" :key="p.id" :value="p.id">{{ p.title }}</option>
        </UiSelect>
      </UiField>

      <UiField v-if="draft.target !== 'discard'" label="Title">
        <UiInput v-model="draft.title" classes="h-8 w-full rounded border border-border-hairline bg-surface-container-low px-base font-body text-body text-text-primary outline-none transition-colors placeholder:text-text-muted focus:border-primary/70 disabled:cursor-not-allowed disabled:opacity-50" />
      </UiField>
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
type ButtonVariant = 'primary' | 'secondary' | 'ghost' | 'danger' | 'success' | 'accent'

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

function targetVariant(target: Target): ButtonVariant {
  if (target === 'discard') return 'danger'
  if (target === 'task') return 'success'
  return 'accent'
}

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
