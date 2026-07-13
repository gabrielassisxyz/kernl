<template>
  <div class="flex flex-col gap-base border border-border-hairline bg-surface-container-low p-component rounded">
    <div class="flex items-center justify-between gap-base">
      <span class="font-mono-data text-mono-data text-text-faint">NODE {{ index + 1 }}</span>
      <button
        v-if="removable"
        class="flex items-center gap-tight font-mono-data text-mono-data text-text-muted hover:text-status-failed-text rounded outline-none focus-visible:ring-1 focus-visible:ring-primary/30 cursor-pointer"
        @click="$emit('remove')"
      >
        <span class="material-symbols-outlined !text-body">close</span>
        Remove
      </button>
    </div>

    <!-- Flex, not a 6-col grid: equal 1fr cells are narrower than "Bookmark" and
         "Discard", so those buttons overflowed their cell and ran into the
         neighbour. Each chip sizes to its own label and the gap is honoured. -->
    <div class="flex flex-wrap gap-tight">
      <UiButton
        v-for="t in TARGETS"
        :key="t"
        size="xs"
        :variant="action.target === t ? targetVariant(t) : 'secondary'"
        :icon="TARGET_META[t].icon"
        :icon-size="14"
        @click="setTarget(t)"
      >
        {{ TARGET_META[t].label }}
      </UiButton>
    </div>

    <UiField v-if="action.target !== 'discard' && action.target !== 'update'" label="Title">
      <UiInput v-model="action.title" :classes="inputClasses" />
    </UiField>

    <div v-if="action.target === 'task'" class="grid grid-cols-2 gap-base">
      <UiField label="Project">
        <UiSelect v-model="action.projectId" :classes="inputClasses">
          <option value="">No project</option>
          <option v-for="p in projects" :key="p.id" :value="p.id">{{ p.title }}</option>
        </UiSelect>
      </UiField>
      <!-- The DA proposes a due date only when the capture states one, and dates
           it from the day the message was written — not from today. -->
      <UiField label="Due date">
        <div class="flex items-center gap-base">
          <UiInput v-model="action.dueDate" type="date" :classes="inputClasses" />
          <button
            v-if="action.dueDate"
            class="shrink-0 font-mono-data text-mono-data text-text-muted hover:text-text-primary rounded outline-none focus-visible:ring-1 focus-visible:ring-primary/30 cursor-pointer"
            @click="action.dueDate = ''"
          >Clear</button>
        </div>
      </UiField>
    </div>

    <UiField
      v-else-if="action.target === 'note' || action.target === 'bookmark'"
      label="Link to (optional)"
    >
      <UiSelect v-model="action.projectId" :classes="inputClasses">
        <option value="">Nothing</option>
        <option v-for="p in projects" :key="p.id" :value="p.id">{{ p.title }}</option>
      </UiSelect>
    </UiField>

    <!-- A task read three weeks from now has to stand on its own, so its
         description carries the capture it came from, not just the fragment. -->
    <UiField
      v-if="action.target === 'task'"
      label="Description"
      hint="Written to the task. The capture is carried in as context — edit freely."
    >
      <UiTextarea v-model="action.body" rows="3" :classes="textareaClasses" />
    </UiField>

    <template v-if="action.target === 'project'">
      <UiField label="Description">
        <UiTextarea v-model="action.projectDescription" rows="3" :classes="textareaClasses" />
      </UiField>
      <UiField label="Initial tasks" hint="One per line, up to six.">
        <UiTextarea v-model="action.initialTasksText" rows="4" :classes="textareaClasses" />
      </UiField>
    </template>

    <!-- The placeholder is a prompt, not an example: real-looking tags in an empty
         field read as tags already applied. The examples live in the hint, where
         they cannot be mistaken for a value. -->
    <UiField
      v-if="action.target !== 'discard' && action.target !== 'update'"
      label="Tags"
      hint="Comma separated, e.g. to-read, behavior"
    >
      <UiInput v-model="action.tagsText" :classes="inputClasses" placeholder="Choose tags" />
    </UiField>

    <p v-if="action.target === 'update'" class="font-body text-body text-text-muted">
      Kernl finds the best-matching note and proposes the changes to merge in.
      You'll review each suggested edit before anything is written.
    </p>
  </div>
</template>

<script setup lang="ts">
import type { Project } from '~/composables/useProjects'
import { TARGETS, TARGET_META, type DraftAction, type Target } from '~/utils/inboxTargets'
import UiButton from '~/components/ui/UiButton.vue'
import UiField from '~/components/ui/UiField.vue'
import UiInput from '~/components/ui/UiInput.vue'
import UiSelect from '~/components/ui/UiSelect.vue'
import UiTextarea from '~/components/ui/UiTextarea.vue'

type ButtonVariant = 'primary' | 'secondary' | 'ghost' | 'danger' | 'success' | 'accent'

const inputClasses = 'h-8 w-full rounded border border-border-hairline bg-surface px-base font-body text-body text-text-primary outline-none transition-colors placeholder:text-text-faint focus:border-primary/70'
const textareaClasses = 'w-full rounded border border-border-hairline bg-surface px-base py-base font-body text-body text-text-primary outline-none transition-colors focus:border-primary/70 resize-y'

const props = defineProps<{
  /** the draft row this editor mutates in place; the drawer owns the array */
  action: DraftAction
  index: number
  projects: Project[]
  removable?: boolean
}>()

defineEmits<{ (e: 'remove'): void }>()

function targetVariant(target: Target): ButtonVariant {
  if (target === 'discard') return 'danger'
  if (target === 'task') return 'success'
  return 'accent'
}

function setTarget(target: Target) {
  // projectId means "parent project" for a task and "link to" for a note or
  // bookmark; it means nothing for the rest, so drop a stale selection.
  if (target !== 'task' && target !== 'note' && target !== 'bookmark') props.action.projectId = ''
  props.action.target = target
}
</script>
