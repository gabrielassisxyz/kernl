<template>
  <UiModal open title="Edit project" align="top" @close="$emit('close')">
    <form class="flex flex-col gap-section" @submit.prevent="submit">
      <UiField label="Title">
        <UiInput ref="titleInput" v-model="title" required placeholder="Project title" />
      </UiField>

      <UiField label="Description">
        <UiTextarea v-model="description" rows="3" placeholder="Optional" />
      </UiField>

      <UiField label="Status">
        <UiSelect v-model="status">
          <option v-for="s in PROJECT_STATUSES" :key="s.id" :value="s.id">{{ s.label }}</option>
        </UiSelect>
      </UiField>

      <p v-if="error" class="font-mono-data text-mono-data text-status-failed-text">{{ error }}</p>
    </form>

    <template #footer>
      <div class="flex items-center justify-end gap-base">
        <UiButton variant="ghost" @click="$emit('close')">Cancel</UiButton>
        <UiButton type="submit" variant="primary" :loading="saving" :disabled="!title.trim()" @click="submit">
          Save changes
        </UiButton>
      </div>
    </template>
  </UiModal>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { PROJECT_STATUSES, type Project, type ProjectStatus } from '~/composables/useProjects'
import UiButton from '~/components/ui/UiButton.vue'
import UiField from '~/components/ui/UiField.vue'
import UiInput from '~/components/ui/UiInput.vue'
import UiModal from '~/components/ui/UiModal.vue'
import UiSelect from '~/components/ui/UiSelect.vue'
import UiTextarea from '~/components/ui/UiTextarea.vue'

const props = defineProps<{ project: Project }>()
const emit = defineEmits<{
  (e: 'close'): void
  (e: 'save', patch: { title: string; description: string; status: ProjectStatus }): void
}>()

const title = ref(props.project.title)
const description = ref(props.project.description)
const status = ref<ProjectStatus>(props.project.status)
const saving = ref(false)
const error = ref<string | null>(null)
const titleInput = ref<{ focus: () => void } | null>(null)

onMounted(() => titleInput.value?.focus())

function submit() {
  if (!title.value.trim() || saving.value) return
  emit('save', {
    title: title.value.trim(),
    description: description.value.trim(),
    status: status.value,
  })
}

// The parent drives the request; expose state setters for feedback.
defineExpose({
  setSaving: (v: boolean) => { saving.value = v },
  setError: (msg: string | null) => { error.value = msg },
})
</script>
