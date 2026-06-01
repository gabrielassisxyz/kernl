<template>
  <div class="notes-editor-pane flex flex-col h-full bg-[#0F1217] text-[#D6DBE3]">
    <FrontmatterUI v-if="frontmatter" :data="frontmatter" />
    <div class="flex-1 relative">
      <div ref="editorContainer" class="h-full w-full"></div>
      
      <div v-if="conflict" class="absolute inset-0 bg-black/50 flex items-center justify-center z-50">
        <div class="bg-[#181C26] border border-[#242935] p-6 rounded-md shadow-lg max-w-sm">
          <h3 class="text-xl font-bold mb-4 text-[#D6DBE3]">File Changed on Disk</h3>
          <p class="mb-6 text-[#9098A7] text-sm">The file was modified externally. What do you want to do?</p>
          <div class="flex gap-4">
            <button class="px-4 py-2 bg-[#6B7BB0] text-[#F0F4F8] rounded text-sm font-medium" @click="resolveConflict('keep')">Keep Editor</button>
            <button class="px-4 py-2 bg-[#1D222D] border border-[#242935] text-[#D6DBE3] rounded text-sm font-medium" @click="resolveConflict('reload')">Reload Disk</button>
          </div>
        </div>
      </div>
      
      <DiffSuggest 
        v-if="activeHunks.length > 0" 
        :hunks="activeHunks" 
        @accept="acceptHunk" 
        @reject="rejectHunk" 
      />
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, onBeforeUnmount, watch } from 'vue'
import { EditorState } from '@codemirror/state'
import { EditorView, lineNumbers } from '@codemirror/view'
import { markdown } from '@codemirror/lang-markdown'
import FrontmatterUI from './FrontmatterUI.vue'
import DiffSuggest from './DiffSuggest.vue'

const props = defineProps({
  path: String,
  initialContent: String
})

const editorContainer = ref(null)
let view = null

const frontmatter = ref(null)
const rawContent = ref('')
const isDirty = ref(false)
const conflict = ref(false)
const lastModified = ref('')
const activeHunks = ref([])
let saveTimer = null

const extractFrontmatter = (text) => {
  if (text.startsWith('---\n')) {
    const end = text.indexOf('\n---\n', 4)
    if (end !== -1) {
      const fmText = text.substring(4, end)
      const lines = fmText.split('\n')
      const data = {}
      lines.forEach(line => {
        const colon = line.indexOf(':')
        if (colon !== -1) {
          data[line.substring(0, colon).trim()] = line.substring(colon + 1).trim()
        }
      })
      return { data, length: end + 5 }
    }
  }
  return { data: null, length: 0 }
}

const loadFile = async (path) => {
  if (!path) return
  const res = await fetch(`/api/vault/file?path=${encodeURIComponent(path)}`)
  if (res.ok) {
    const text = await res.text()
    rawContent.value = text
    
    const fm = extractFrontmatter(text)
    frontmatter.value = fm.data
    
    if (view) view.destroy()
    
    const docText = text
    const state = EditorState.create({
      doc: docText,
      extensions: [
        lineNumbers(),
        markdown(),
        EditorView.updateListener.of((v) => {
          if (v.docChanged) {
            isDirty.value = true
            scheduleSave()
          }
        }),
        EditorView.domEventHandlers({
          click: (event, view) => {
            if (event.ctrlKey || event.metaKey) {
              const pos = view.posAtCoords({ x: event.clientX, y: event.clientY })
              if (pos) {
                const line = view.state.doc.lineAt(pos)
                const text = line.text
                const linkMatch = text.match(/\[\[(.*?)\]\]/)
                if (linkMatch) {
                  console.log('Wikilink clicked:', linkMatch[1])
                }
              }
            }
          }
        })
      ]
    })
    
    view = new EditorView({
      state,
      parent: editorContainer.value
    })
    
    lastModified.value = res.headers.get('Last-Modified') || new Date().toISOString()
  }
}

watch(() => props.path, (newPath) => {
  loadFile(newPath)
})

onMounted(() => {
  loadFile(props.path)
})

onBeforeUnmount(() => {
  if (view) view.destroy()
  if (saveTimer) clearTimeout(saveTimer)
})

const scheduleSave = () => {
  if (saveTimer) clearTimeout(saveTimer)
  saveTimer = setTimeout(() => {
    saveFile()
  }, 5000)
}

const saveFile = async () => {
  if (!isDirty.value || conflict.value || !props.path) return
  
  const content = view.state.doc.toString()
  
  const res = await fetch(`/api/notes/save?path=${encodeURIComponent(props.path)}`, {
    method: 'POST',
    headers: {
      'If-Match': lastModified.value,
      'Content-Type': 'text/plain'
    },
    body: content
  })
  
  if (res.status === 409) {
    conflict.value = true
    return
  }
  
  if (res.ok) {
    const data = await res.json()
    lastModified.value = data.last_modified
    isDirty.value = false
  }
}

const resolveConflict = (action) => {
  conflict.value = false
  if (action === 'keep') {
    lastModified.value = ''
    saveFile()
  } else {
    loadFile(props.path)
  }
}

const acceptHunk = (hunk) => {
  if (!view) return
  const tr = view.state.update({
    changes: { from: hunk.from, to: hunk.to, insert: hunk.content }
  })
  view.dispatch(tr)
  activeHunks.value = activeHunks.value.filter(h => h.id !== hunk.id)
}

const rejectHunk = (hunk) => {
  activeHunks.value = activeHunks.value.filter(h => h.id !== hunk.id)
}
</script>

<style>
.cm-editor {
  height: 100%;
  background-color: transparent !important;
  color: #D6DBE3 !important;
}
.cm-gutters {
  background-color: #141821 !important;
  color: #666D7C !important;
  border-right: 1px solid #1B2029 !important;
}
.cm-activeLineGutter {
  background-color: #1D222D !important;
  color: #D6DBE3 !important;
}
.cm-activeLine {
  background-color: #1D222D !important;
}
.cm-cursor {
  border-left-color: #6B7BB0 !important;
}
</style>
