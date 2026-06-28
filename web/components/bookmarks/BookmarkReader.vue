<template>
  <div class="flex flex-col h-full bg-surface-container-low">
    <!-- Reader Header -->
    <div class="px-section py-break border-b border-border-hairline bg-surface">
      <a :href="bookmark.URL" target="_blank" rel="noopener noreferrer" class="group flex items-center gap-tight mb-component text-text-muted hover:text-primary transition-colors w-fit">
        <span class="font-mono-data text-mono-data truncate max-w-lg">{{ bookmark.URL }}</span>
        <span class="material-symbols-outlined text-body opacity-0 group-hover:opacity-100 transition-opacity">open_in_new</span>
      </a>
      <h1 class="font-headline text-display text-text-primary leading-tight mb-component">{{ bookmark.Title || 'Untitled Bookmark' }}</h1>
      <p class="font-body text-text-muted leading-relaxed" v-if="bookmark.Description">{{ bookmark.Description }}</p>
    </div>

    <!-- Reader Content & Highlights -->
    <div class="flex-1 overflow-y-auto px-section py-break flex flex-col gap-break">
      <!-- Main Content Area -->
      <div class="w-full max-w-[600px] mx-auto pb-32">
        <div class="font-body text-text-primary leading-[1.7] whitespace-pre-wrap" v-if="bookmark.Excerpt">
          {{ bookmark.Excerpt }}
        </div>
        <div class="font-body text-text-muted italic flex items-center gap-component justify-center py-break border border-border-hairline bg-surface mt-break" v-else>
          <span class="material-symbols-outlined">hourglass_empty</span>
          No excerpt available. Read original article.
        </div>
        
        <!-- Existing Highlights -->
        <div v-if="bookmark.Highlights && bookmark.Highlights.length" class="mt-break border-t border-border-hairline pt-break">
          <h3 class="font-headline text-text-primary mb-component text-sm uppercase tracking-widest font-semibold text-text-faint">Highlights ({{ bookmark.Highlights.length }})</h3>
          <div v-for="(h, i) in bookmark.Highlights" :key="i" class="bg-surface border border-border-hairline px-component py-component mb-component">
            <p class="font-body text-text-primary text-body leading-relaxed">{{ h.text }}</p>
            <p v-if="h.note" class="font-body text-text-muted text-mono-data mt-tight italic">{{ h.note }}</p>
          </div>
        </div>

        <!-- Highlighter UI -->
        <div class="mt-break border-t border-border-hairline pt-break flex flex-col gap-component">
          <h3 class="font-headline text-text-primary text-sm uppercase tracking-widest font-semibold text-text-faint">Add Highlight</h3>
          <UiField label="Highlight Text">
            <UiTextarea v-model="newHighlight" placeholder="Paste text to highlight..." rows="3" />
          </UiField>
          <UiField label="Note">
            <UiInput v-model="newNote" placeholder="Add a note to this highlight (optional)..." />
          </UiField>
          <div class="flex justify-end mt-tight">
            <UiButton
              @click="submitHighlight"
              :disabled="!newHighlight.trim()"
              variant="secondary"
              icon="edit_note"
            >
              Save Highlight
            </UiButton>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import type { BookmarkItemData } from './BookmarkItem.vue'
import UiButton from '~/components/ui/UiButton.vue'
import UiField from '~/components/ui/UiField.vue'
import UiInput from '~/components/ui/UiInput.vue'
import UiTextarea from '~/components/ui/UiTextarea.vue'

const props = defineProps<{
  bookmark: BookmarkItemData
}>()

const emit = defineEmits<{
  (e: 'highlight', data: { text: string, note?: string }): void
}>()

const newHighlight = ref('')
const newNote = ref('')

watch(() => props.bookmark.ID, () => {
  newHighlight.value = ''
  newNote.value = ''
})

const submitHighlight = () => {
  if (!newHighlight.value.trim()) return
  
  emit('highlight', {
    text: newHighlight.value.trim(),
    note: newNote.value.trim() || undefined
  })
  
  newHighlight.value = ''
  newNote.value = ''
}
</script>
