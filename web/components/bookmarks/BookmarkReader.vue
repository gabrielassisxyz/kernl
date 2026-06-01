<template>
  <div class="flex flex-col h-full bg-surface-container-low">
    <!-- Reader Header -->
    <div class="px-section py-break border-b border-border-hairline bg-surface">
      <a :href="bookmark.URL" target="_blank" rel="noopener noreferrer" class="group flex items-center gap-tight mb-component text-text-muted hover:text-primary transition-colors w-fit">
        <span class="font-mono-data text-[12px] truncate max-w-lg">{{ bookmark.URL }}</span>
        <span class="material-symbols-outlined text-[14px] opacity-0 group-hover:opacity-100 transition-opacity">open_in_new</span>
      </a>
      <h1 class="font-headline text-[24px] text-text-primary leading-tight mb-component">{{ bookmark.Title || 'Untitled Bookmark' }}</h1>
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
        
        <!-- Highlighter UI -->
        <div class="mt-break border-t border-border-hairline pt-break">
          <h3 class="font-headline text-text-primary mb-component text-sm uppercase tracking-widest font-semibold text-text-faint">Add Highlight</h3>
          <div class="bg-surface border border-border-hairline p-component rounded-sm mb-component focus-within:border-primary transition-colors">
            <textarea 
              v-model="newHighlight" 
              placeholder="Paste text to highlight..." 
              class="w-full bg-transparent border-none outline-none font-body text-text-primary resize-none custom-caret text-[13.5px]"
              rows="3"
            ></textarea>
          </div>
          <div class="bg-surface border border-border-hairline p-component rounded-sm mb-component focus-within:border-primary transition-colors">
            <input 
              v-model="newNote" 
              type="text" 
              placeholder="Add a note to this highlight (optional)..." 
              class="w-full bg-transparent border-none outline-none font-body text-text-primary custom-caret text-[13.5px]"
            />
          </div>
          <div class="flex justify-end">
            <button 
              @click="submitHighlight"
              :disabled="!newHighlight.trim()"
              class="bg-surface-hover border border-border-hairline hover:border-primary hover:bg-surface text-text-primary px-section py-component text-[13px] transition-colors flex items-center gap-tight disabled:opacity-50 disabled:cursor-not-allowed font-medium rounded-sm"
            >
              <span class="material-symbols-outlined text-[16px]">edit_note</span>
              Save Highlight
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import type { BookmarkItemData } from './BookmarkItem.vue'

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
