<template>
  <div class="flex h-screen bg-[#0F1217] font-sans">
    <div class="w-64 border-r border-[#242935] flex flex-col bg-[#141821]">
      <div class="h-12 border-b border-[#242935] flex items-center justify-between px-4 shrink-0">
        <h1 class="font-medium text-[#D6DBE3] text-[14px]">Notes Vault</h1>
        <button
          @click="openNewNote"
          title="New note"
          class="flex items-center justify-center w-6 h-6 rounded text-[#9098A7] hover:text-[#D6DBE3] hover:bg-[#1D222D] transition-colors"
        >
          <span class="material-symbols-outlined text-[18px]">add</span>
        </button>
      </div>
      <div class="flex-1 overflow-y-auto bg-[#0F1217]">
        <TagHierarchy @select="selectFile" />
        <NoteList @select="selectFile" ref="noteListRef" />
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

    <!-- New Note modal -->
    <Transition name="modal">
      <div
        v-if="showNewNote"
        class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm"
        @click.self="closeNewNote"
        @keydown.esc="closeNewNote"
      >
        <div class="modal-card w-[340px] bg-[#181C26] border border-[#242935] rounded-lg overflow-hidden shadow-[0_24px_64px_-16px_rgba(0,0,0,0.75)]">
          <div class="px-5 pt-4 pb-3">
            <span class="block text-[11px] uppercase tracking-[0.14em] text-[#666D7C] mb-3">New note</span>
            <input
              ref="titleInput"
              v-model="newTitle"
              @keydown.enter="confirmNewNote"
              @keydown.esc="closeNewNote"
              placeholder="Untitled note"
              class="w-full bg-[#0F1217] border border-[#242935] rounded px-3 py-2 text-[14px] text-[#D6DBE3] placeholder-[#666D7C] focus:outline-none focus:border-[#6B7BB0] transition-colors"
            />
            <p class="mt-2 h-4 text-[11px] font-mono text-[#666D7C] truncate">
              <span v-if="slugPreview">will create <span class="text-[#9098A7]">{{ slugPreview }}.md</span></span>
            </p>
          </div>
          <div class="flex items-center justify-between px-5 py-3 border-t border-[#242935] bg-[#141821]">
            <span class="text-[10px] font-mono text-[#666D7C] tracking-wide">↵ create · esc cancel</span>
            <div class="flex items-center gap-2">
              <button
                @click="closeNewNote"
                class="text-[12px] px-3 py-1 rounded text-[#9098A7] hover:text-[#D6DBE3] hover:bg-[#1D222D] transition-colors"
              >Cancel</button>
              <button
                @click="confirmNewNote"
                :disabled="!slugPreview || creating"
                class="text-[12px] px-3 py-1 rounded bg-[#6B7BB0] text-[#F0F4F8] disabled:opacity-40 disabled:cursor-not-allowed hover:bg-[#7c8bc0] transition-colors"
              >{{ creating ? 'Creating…' : 'Create' }}</button>
            </div>
          </div>
        </div>
      </div>
    </Transition>
  </div>
</template>

<script setup>
import { ref, computed, nextTick } from 'vue'
import MarkdownEditor from '~/components/notes/MarkdownEditor.vue'
import TagHierarchy from '~/components/notes/TagHierarchy.vue'
import NoteList from '~/components/notes/NoteList.vue'

const selectedFile = ref(null)
const noteListRef = ref(null)

const showNewNote = ref(false)
const newTitle = ref('')
const titleInput = ref(null)
const creating = ref(false)

const selectFile = (path) => {
  selectedFile.value = path
}

const slugify = (title) => title
  .toLowerCase()
  .trim()
  .replace(/\s+/g, '-')
  .replace(/[^a-z0-9-]/g, '')
  .replace(/-+/g, '-')
  .replace(/^-|-$/g, '')

const slugPreview = computed(() => slugify(newTitle.value || ''))

const openNewNote = async () => {
  newTitle.value = ''
  showNewNote.value = true
  await nextTick()
  titleInput.value?.focus()
}

const closeNewNote = () => {
  if (creating.value) return
  showNewNote.value = false
}

const confirmNewNote = async () => {
  const title = newTitle.value.trim()
  const slug = slugify(title)
  if (!slug || creating.value) return
  creating.value = true

  // Collision guard against the current disk-truth list.
  let existing = []
  try {
    const res = await fetch('/api/vault/list')
    if (res.ok) existing = (await res.json()).files || []
  } catch (e) { /* best-effort */ }

  let path = `${slug}.md`
  let n = 2
  while (existing.includes(path)) {
    path = `${slug}-${n}.md`
    n++
  }

  const body = `---\ntitle: ${title}\ntags: []\n---\n\n# ${title}\n\n`
  try {
    const res = await fetch(`/api/vault/file?path=${encodeURIComponent(path)}`, {
      method: 'POST',
      headers: { 'Content-Type': 'text/plain' },
      body
    })
    if (res.ok) {
      showNewNote.value = false
      selectedFile.value = path
      noteListRef.value?.refresh()
    }
  } finally {
    creating.value = false
  }
}
</script>

<style scoped>
.modal-enter-active,
.modal-leave-active {
  transition: opacity 0.18s ease;
}
.modal-enter-from,
.modal-leave-to {
  opacity: 0;
}
.modal-enter-active .modal-card,
.modal-leave-active .modal-card {
  transition: transform 0.18s cubic-bezier(0.22, 1, 0.36, 1), opacity 0.18s ease;
}
.modal-enter-from .modal-card,
.modal-leave-to .modal-card {
  transform: translateY(6px) scale(0.98);
  opacity: 0;
}
</style>
