<template>
  <div
    class="fixed inset-0 z-[60] flex items-start justify-center bg-black/50 pt-[12vh] px-base"
    @click.self="$emit('close')"
  >
    <div class="w-full max-w-[480px] rounded-lg border border-border-hairline bg-surface shadow-xl">
      <header class="px-section pt-section pb-component border-b border-border-hairline">
        <h2 class="font-headline text-headline text-text-primary font-medium">New task</h2>
      </header>

      <form class="px-section py-section flex flex-col gap-section" @submit.prevent="submit">
        <label class="flex flex-col gap-tight">
          <span class="font-label-caps text-label-caps text-text-muted uppercase">Title</span>
          <input
            ref="titleInput"
            v-model="title"
            type="text"
            required
            placeholder="What needs doing?"
            class="bg-bg-base border border-border-hairline rounded-md px-component py-base font-body text-body text-text-primary focus:outline-none focus:border-primary/60"
          />
        </label>

        <label class="flex flex-col gap-tight">
          <span class="font-label-caps text-label-caps text-text-muted uppercase">Description</span>
          <textarea
            v-model="description"
            rows="3"
            placeholder="Optional"
            class="bg-bg-base border border-border-hairline rounded-md px-component py-base font-body text-body text-text-primary resize-none focus:outline-none focus:border-primary/60"
          ></textarea>
        </label>

        <div class="flex gap-section">
          <label class="flex-1 flex flex-col gap-tight">
            <span class="font-label-caps text-label-caps text-text-muted uppercase">Project</span>
            <select
              v-model="projectId"
              class="bg-bg-base border border-border-hairline rounded-md px-component py-base font-body text-body text-text-primary focus:outline-none focus:border-primary/60"
            >
              <option value="">No project</option>
              <option v-for="p in projects" :key="p.id" :value="p.id">{{ p.title }}</option>
            </select>
          </label>

          <label class="flex-1 flex flex-col gap-tight">
            <span class="font-label-caps text-label-caps text-text-muted uppercase">Status</span>
            <select
              v-model="status"
              class="bg-bg-base border border-border-hairline rounded-md px-component py-base font-body text-body text-text-primary focus:outline-none focus:border-primary/60"
            >
              <option v-for="s in TASK_STATUSES" :key="s.id" :value="s.id">{{ s.label }}</option>
            </select>
          </label>
        </div>

        <p v-if="error" class="font-mono-data text-mono-data text-status-failed">{{ error }}</p>

        <div class="flex items-center justify-end gap-base pt-base">
          <button
            type="button"
            class="px-component py-base font-body text-body text-text-muted hover:text-text-primary transition-colors cursor-pointer"
            @click="$emit('close')"
          >
            Cancel
          </button>
          <button
            type="submit"
            :disabled="saving || !title.trim()"
            class="px-section py-base font-body text-body rounded-md bg-primary text-white cursor-pointer disabled:opacity-40 disabled:cursor-not-allowed hover:bg-primary/90 transition-colors"
          >
            {{ saving ? 'Creating…' : 'Create' }}
          </button>
        </div>
      </form>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useTasks, TASK_STATUSES, type TaskStatus } from '~/composables/useTasks'
import { useProjects, type Project } from '~/composables/useProjects'

const props = defineProps<{ defaultProjectId?: string }>()
const emit = defineEmits<{ (e: 'close'): void; (e: 'created', id: string): void }>()

const { create } = useTasks()
const { projects, load: loadProjects } = useProjects()

const title = ref('')
const description = ref('')
const projectId = ref(props.defaultProjectId ?? '')
const status = ref<TaskStatus>('todo')
const saving = ref(false)
const error = ref<string | null>(null)
const titleInput = ref<HTMLInputElement | null>(null)

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
