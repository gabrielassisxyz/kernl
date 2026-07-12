<template>
  <UiModal open title="New project" align="top" @close="$emit('close')">
    <form class="flex flex-col gap-section" @submit.prevent="submit">
      <UiField label="Title">
        <UiInput ref="titleInput" v-model="title" required placeholder="What are you organizing?" />
      </UiField>

      <UiField label="Description">
        <UiTextarea v-model="description" rows="3" placeholder="Optional" />
      </UiField>

      <UiField label="Status">
        <UiSelect v-model="status">
          <option v-for="s in PROJECT_STATUSES" :key="s.id" :value="s.id">{{ s.label }}</option>
        </UiSelect>
      </UiField>

      <UiField label="Tags">
        <TagInput v-model="tags" aria-label="Project tags" />
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
import { useProjects, PROJECT_STATUSES, type ProjectStatus } from '~/composables/useProjects'
import UiButton from '~/components/ui/UiButton.vue'
import UiField from '~/components/ui/UiField.vue'
import UiInput from '~/components/ui/UiInput.vue'
import UiModal from '~/components/ui/UiModal.vue'
import UiSelect from '~/components/ui/UiSelect.vue'
import UiTextarea from '~/components/ui/UiTextarea.vue'
import TagInput from '~/components/tags/TagInput.vue'

const emit = defineEmits<{ (e: 'close'): void; (e: 'created', id: string): void }>()

const { create } = useProjects()

const title = ref('')
const description = ref('')
const status = ref<ProjectStatus>('active')
const tags = ref<string[]>([])
const saving = ref(false)
const error = ref<string | null>(null)
const titleInput = ref<{ focus: () => void } | null>(null)

onMounted(() => titleInput.value?.focus())

async function submit() {
  if (!title.value.trim() || saving.value) return
  saving.value = true
  error.value = null
  try {
    const id = await create({
      title: title.value.trim(),
      description: description.value.trim(),
      status: status.value,
      tags: tags.value,
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
