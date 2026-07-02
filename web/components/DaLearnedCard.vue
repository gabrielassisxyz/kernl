<template>
  <div class="border border-border-default rounded-[4px] px-[11px] py-[10px] bg-surface-overlay">
    <div class="font-mono-data text-[9.5px] tracking-[0.08em] text-text-faint mb-[4px]">
      DA · learned<span v-if="!isEditing && subject"> · {{ subject }}</span>
    </div>

    <!-- Read mode: the proposed memory as the DA understood it. -->
    <p v-if="!isEditing" class="text-[12px] leading-[1.45] text-text-primary">{{ statement }}</p>

    <!-- Edit mode: correct the topic and wording before keeping. Editing the
         subject lets the user merge a near-duplicate into an existing topic. -->
    <template v-else>
      <input
        v-model="draftSubject"
        placeholder="Topic"
        class="w-full mb-[6px] bg-surface border border-border-default rounded-[3px] px-[9px] py-[5px] text-[11px] text-text-primary outline-none focus:border-da-accent custom-caret"
        @keydown.escape.prevent="cancelEdit"
      />
      <textarea
        ref="editRef"
        v-model="draft"
        rows="3"
        class="w-full bg-surface border border-border-default rounded-[3px] px-[9px] py-[6px] text-[12px] leading-[1.45] text-text-primary resize-none outline-none focus:border-da-accent custom-caret"
        @keydown.escape.prevent="cancelEdit"
      ></textarea>
    </template>

    <div class="mt-[8px] flex justify-end gap-[4px]">
      <template v-if="!isEditing">
        <UiButton variant="accent" size="xs" icon="check" :icon-size="12" @click="$emit('keep', statement, subject)">Keep</UiButton>
        <UiButton variant="secondary" size="xs" icon="edit" :icon-size="12" @click="startEdit">Edit</UiButton>
        <UiButton variant="ghost" size="xs" icon="close" :icon-size="12" @click="$emit('discard')">Discard</UiButton>
      </template>
      <template v-else>
        <UiButton variant="accent" size="xs" icon="check" :icon-size="12" :disabled="!draft.trim() || !draftSubject.trim()" @click="saveEdit">Keep</UiButton>
        <UiButton variant="ghost" size="xs" @click="cancelEdit">Cancel</UiButton>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, nextTick } from 'vue'
import UiButton from '~/components/ui/UiButton.vue'

const props = defineProps<{
  subject: string
  statement: string
}>()

const emit = defineEmits<{
  (e: 'keep', statement: string, subject: string): void
  (e: 'discard'): void
}>()

const isEditing = ref(false)
const draft = ref('')
const draftSubject = ref('')
const editRef = ref<HTMLTextAreaElement | null>(null)

const startEdit = async () => {
  draft.value = props.statement
  draftSubject.value = props.subject
  isEditing.value = true
  await nextTick()
  editRef.value?.focus()
}

const cancelEdit = () => {
  isEditing.value = false
  draft.value = ''
  draftSubject.value = ''
}

// Edit is Keep with a modified subject/statement — same write path, edited text.
const saveEdit = () => {
  const next = draft.value.trim()
  const nextSubject = draftSubject.value.trim()
  if (!next || !nextSubject) return
  isEditing.value = false
  emit('keep', next, nextSubject)
}
</script>
