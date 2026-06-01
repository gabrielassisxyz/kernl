<template>
  <div class="flex h-screen bg-[#0F1217] font-sans">
    <div class="w-64 border-r border-[#242935] flex flex-col bg-[#141821]">
      <div class="h-12 border-b border-[#242935] flex items-center px-4 shrink-0">
        <h1 class="font-medium text-[#D6DBE3] text-[14px]">Notes Vault</h1>
      </div>
      <div class="flex-1 overflow-hidden">
        <TagHierarchy @select="selectFile" />
      </div>
    </div>
    <div class="flex-1 relative flex flex-col bg-[#0F1217]">
      <div class="h-12 border-b border-[#242935] flex items-center px-4 shrink-0">
        <span v-if="selectedFile" class="font-mono text-[#9098A7] text-[12px]">{{ selectedFile }}</span>
      </div>
      <div class="flex-1 overflow-hidden relative">
        <MarkdownEditor v-if="selectedFile" :path="selectedFile" :key="selectedFile" />
        <div v-else class="absolute inset-0 flex items-center justify-center text-[#666D7C] text-[13px]">
          Select a file from tags or create a new note
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import MarkdownEditor from '~/components/notes/MarkdownEditor.vue'
import TagHierarchy from '~/components/notes/TagHierarchy.vue'

const selectedFile = ref(null)

const selectFile = (path) => {
  selectedFile.value = path
}
</script>
