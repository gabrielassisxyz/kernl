<template>
  <div class="flex h-full bg-bg-base font-sans">
    <div class="w-64 border-r border-border-default flex flex-col bg-bg-elevated">
      <div class="h-12 border-b border-border-default flex items-center justify-between px-4 shrink-0">
        <h1 class="font-medium text-text-primary text-[14px]">Notes Vault</h1>
        <button
          @click="openNewNote"
          title="New note"
          class="flex items-center justify-center w-6 h-6 rounded text-text-muted hover:text-text-primary hover:bg-surface-hover transition-colors"
        >
          <span class="material-symbols-outlined text-[18px]">add</span>
        </button>
      </div>
      <div class="flex-1 overflow-y-auto bg-bg-base">
        <TagHierarchy @select="selectFile" />
        <NoteList @select="selectFile" ref="noteListRef" />
      </div>
    </div>
    <div class="flex-1 relative flex flex-col bg-bg-base">
      <div class="h-12 border-b border-border-default flex items-center px-4 shrink-0">
        <div v-if="selectedFile" class="flex items-center gap-2">
          <span class="material-symbols-outlined text-[16px] text-text-faint">description</span>
          <span class="font-body text-text-primary text-[14px] font-medium">{{ selectedFile.replace(/\.md$/, '') }}</span>
          <span class="font-mono-data text-text-faint text-[10px] bg-surface-container border border-border-hairline px-1.5 py-0.5 rounded">{{ selectedFile }}</span>
        </div>
      </div>
      <div class="flex-1 overflow-hidden relative">
        <MarkdownEditor v-if="selectedFile" :path="selectedFile" :key="selectedFile" @open-wikilink="openWikilink" />
        <div v-else class="absolute inset-0 flex items-center justify-center text-text-faint text-[13px]">
          Select a file from tags or create a new note
        </div>
      </div>
    </div>

    <UiModal :open="showNewNote" title="New note" size="sm" @close="closeNewNote">
      <UiField :hint="slugPreview ? `Will create ${slugPreview}.md` : ''">
        <UiInput
          ref="titleInput"
          v-model="newTitle"
          placeholder="Untitled note"
          @keydown.enter="confirmNewNote"
          @keydown.esc="closeNewNote"
        />
      </UiField>

      <template #footer>
        <div class="flex items-center justify-between gap-component">
          <span class="font-mono-data text-mono-data text-text-muted">Enter creates · Esc cancels</span>
          <div class="flex items-center gap-base">
            <UiButton variant="ghost" @click="closeNewNote">Cancel</UiButton>
            <UiButton variant="primary" :loading="creating" :disabled="!slugPreview" @click="confirmNewNote">Create note</UiButton>
          </div>
        </div>
      </template>
    </UiModal>
  </div>
</template>

<script setup>
import { ref, computed, nextTick } from 'vue'
import MarkdownEditor from '~/components/notes/MarkdownEditor.vue'
import TagHierarchy from '~/components/notes/TagHierarchy.vue'
import NoteList from '~/components/notes/NoteList.vue'
import UiButton from '~/components/ui/UiButton.vue'
import UiField from '~/components/ui/UiField.vue'
import UiInput from '~/components/ui/UiInput.vue'
import UiModal from '~/components/ui/UiModal.vue'

const selectedFile = ref(null)
const noteListRef = ref(null)

const showNewNote = ref(false)
const newTitle = ref('')
const titleInput = ref(null)
const creating = ref(false)

const selectFile = (path) => {
  selectedFile.value = path
}

// Resolve a ctrl/cmd-clicked wikilink target to an actual vault file and
// select it the same way clicking a note in the list does.
const openWikilink = async (target) => {
  const slug = target.endsWith('.md') ? target : `${target}.md`
  try {
    const res = await fetch('/api/vault/list')
    if (res.ok) {
      const { files } = await res.json()
      const match = (files || []).find((f) => f === slug || f.endsWith(`/${slug}`))
      if (match) selectFile(match)
    }
  } catch (e) { /* best-effort wikilink navigation */ }
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
