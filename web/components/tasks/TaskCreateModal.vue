<template>
  <UiModal open title="New task" align="top" @close="$emit('close')">
    <form class="flex flex-col gap-section" @submit.prevent="submit">
      <UiField label="Title">
        <UiInput ref="titleInput" v-model="title" required placeholder="What needs doing?" />
      </UiField>

      <UiField label="Description">
        <UiTextarea v-model="description" rows="3" placeholder="Optional" />
      </UiField>

      <div class="flex gap-section">
        <UiField class="flex-1" label="Project">
          <UiSelect v-model="projectId">
            <option value="">No project</option>
            <option v-for="p in projects" :key="p.id" :value="p.id">{{ p.title }}</option>
          </UiSelect>
        </UiField>

        <UiField class="flex-1" label="Status">
          <UiSelect v-model="status">
            <option v-for="s in TASK_STATUSES" :key="s.id" :value="s.id">{{ s.label }}</option>
          </UiSelect>
        </UiField>
      </div>

      <UiField label="Due date">
        <UiInput v-model="dueDate" type="date" />
      </UiField>

      <p v-if="error" class="font-mono-data text-mono-data text-status-failed-text">{{ error }}</p>
    </form>

    <template #footer>
      <div class="flex items-center justify-end gap-base">
        <UiButton variant="ghost" @click="$emit('close')">Cancel</UiButton>
        <UiButton type="submit" variant="primary" :loading="saving" :disabled="!title.trim()" @click="submit">
          Create
        </UiButton>
      </div>
    </template>
  </UiModal>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useTasks, TASK_STATUSES, type TaskStatus } from '~/composables/useTasks'
import { useProjects, type Project } from '~/composables/useProjects'
import UiButton from '~/components/ui/UiButton.vue'
import UiField from '~/components/ui/UiField.vue'
import UiInput from '~/components/ui/UiInput.vue'
import UiModal from '~/components/ui/UiModal.vue'
import UiSelect from '~/components/ui/UiSelect.vue'
import UiTextarea from '~/components/ui/UiTextarea.vue'

const props = defineProps<{ defaultProjectId?: string }>()
const emit = defineEmits<{ (e: 'close'): void; (e: 'created', id: string): void }>()

const { create } = useTasks()
const { projects, load: loadProjects } = useProjects()

const title = ref('')
const description = ref('')
const projectId = ref(props.defaultProjectId ?? '')
const status = ref<TaskStatus>('todo')
const dueDate = ref('')
const saving = ref(false)
const error = ref<string | null>(null)
const titleInput = ref<{ focus: () => void } | null>(null)

onMounted(() => {
  titleInput.value?.focus()
  loadProjects()
})

async function submit() {
  if (!title.value.trim() || saving.value) return
  saving.value = true
  error.value = null
  try {
    const id = await create({
      title: title.value.trim(),
      description: description.value.trim(),
      projectId: projectId.value,
      status: status.value,
      dueDate: dueDate.value,
    })
    emit('created', id)
    emit('close')
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e)
  } finally {
    saving.value = false
  }
}
</script>
