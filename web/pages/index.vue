<template>
  <div class="flex h-full w-full overflow-hidden">
    <!-- File List -->
    <div class="w-64 border-r border-base-300 bg-base-100 flex flex-col shrink-0">
      <div class="p-4 border-b border-base-300 font-bold flex justify-between items-center">
        <span>Files</span>
        <button class="btn btn-xs btn-ghost" @click="createNewFile">+</button>
      </div>
      <div class="flex-1 overflow-y-auto p-2 space-y-1">
        <button 
          v-for="file in files" 
          :key="file" 
          @click="openFile(file)"
          class="w-full text-left p-2 rounded hover:bg-base-200 text-sm truncate"
          :class="{ 'bg-primary text-primary-content hover:bg-primary': currentFile === file }"
        >
          {{ file }}
        </button>
      </div>
    </div>

    <!-- Editor -->
    <div class="flex-1 flex flex-col h-full bg-base-100 min-w-0">
      <div class="p-4 border-b border-base-300 flex justify-between items-center gap-2 shrink-0">
        <input 
          v-model="currentFile" 
          class="input input-sm input-ghost text-lg font-bold flex-1" 
          placeholder="Filename (e.g. note.md)" 
          :disabled="!currentFile"
        />
        <div class="text-sm text-base-content/50">{{ saveStatus }}</div>
        <button class="btn btn-sm btn-primary" @click="saveFile" :disabled="!currentFile">Save</button>
      </div>
      <div class="flex-1 p-0 relative">
        <textarea 
          v-model="content"
          @input="debouncedSave"
          class="absolute inset-0 w-full h-full p-6 font-mono text-base resize-none bg-base-100 focus:outline-none border-none" 
          placeholder="Select a file or start typing..."
          :disabled="!currentFile"
        ></textarea>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'

const files = ref([])
const { currentFile, currentContent: content } = useVaultContext()
const saveStatus = ref('')
let saveTimeout = null

const loadFiles = async () => {
  const res = await fetch('/api/vault/list')
  if (res.ok) {
    const data = await res.json()
    files.value = data.files || []
  }
}

const openFile = async (path) => {
  currentFile.value = path
  saveStatus.value = 'Loading...'
  const res = await fetch(`/api/vault/file?path=${encodeURIComponent(path)}`)
  if (res.ok) {
    content.value = await res.text()
    saveStatus.value = ''
  } else {
    saveStatus.value = 'Error loading'
  }
}

const createNewFile = () => {
  const name = prompt('Filename (e.g. new_note.md):')
  if (name) {
    let finalName = name
    if (!finalName.endsWith('.md')) finalName += '.md'
    files.value.push(finalName)
    currentFile.value = finalName
    content.value = `# ${finalName.replace('.md', '')}\n\n`
    saveFile()
  }
}

const saveFile = async () => {
  if (!currentFile.value) return
  saveStatus.value = 'Saving...'
  const res = await fetch(`/api/vault/file?path=${encodeURIComponent(currentFile.value)}`, {
    method: 'POST',
    body: content.value
  })
  if (res.ok) {
    saveStatus.value = 'Saved'
    setTimeout(() => { if (saveStatus.value === 'Saved') saveStatus.value = '' }, 2000)
    if (!files.value.includes(currentFile.value)) {
      loadFiles()
    }
  } else {
    saveStatus.value = 'Error saving'
  }
}

const debouncedSave = () => {
  saveStatus.value = 'Editing...'
  clearTimeout(saveTimeout)
  saveTimeout = setTimeout(saveFile, 1000)
}

onMounted(() => {
  loadFiles()
})
</script>
