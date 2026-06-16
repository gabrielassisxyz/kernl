<template>
  <div class="notes-editor-pane flex flex-col h-full bg-[#0F1217] text-[#D6DBE3]">
    <FrontmatterUI v-if="frontmatter" :data="frontmatter" />
    <div class="flex items-center gap-2 px-3 py-2 border-b border-[#242935] shrink-0">
      <input
        v-model="instruction"
        @keydown.enter="requestSuggestion"
        placeholder="Ask the DA to edit this note (e.g. summarize, fix grammar)…"
        class="flex-1 bg-[#141821] border border-[#242935] rounded px-2 py-1 text-[12px] text-[#D6DBE3] placeholder-[#666D7C] focus:outline-none focus:border-[#6B7BB0]"
      />
      <button
        @click="requestSuggestion"
        :disabled="suggesting || !instruction.trim()"
        class="text-[12px] px-3 py-1 rounded bg-[#6B7BB0] text-[#F0F4F8] disabled:opacity-40"
      >{{ suggesting ? 'Asking…' : 'Suggest' }}</button>
    </div>
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
import { EditorState, StateField, StateEffect } from '@codemirror/state'
import { EditorView, lineNumbers, Decoration } from '@codemirror/view'
import { markdown } from '@codemirror/lang-markdown'
import { wikilinkExtensions } from '~/utils/wikilinkEditor'
import FrontmatterUI from './FrontmatterUI.vue'
import DiffSuggest from './DiffSuggest.vue'

// "DA wrote here": regions written by an accepted DA suggestion are marked for
// the session so the user can see what the DA authored. Session-scoped (the
// mark maps through edits but is not persisted across reloads).
const addDaRegion = StateEffect.define()
const daMark = Decoration.mark({ class: 'da-authored', attributes: { title: 'DA wrote here' } })
const daRegionField = StateField.define({
  create() { return Decoration.none },
  update(deco, tr) {
    deco = deco.map(tr.changes)
    for (const e of tr.effects) {
      if (e.is(addDaRegion) && e.value.to > e.value.from) {
        deco = deco.update({ add: [daMark.range(e.value.from, e.value.to)] })
      }
    }
    return deco
  },
  provide: (f) => EditorView.decorations.from(f),
})

const props = defineProps({
  path: String,
  initialContent: String
})

// Best-effort navigation: emitted when a wikilink pill is ctrl/cmd-clicked, so
// the parent can open the target node.
const emit = defineEmits(['open-wikilink'])

const editorContainer = ref(null)
let view = null

const frontmatter = ref(null)
const rawContent = ref('')
const isDirty = ref(false)
const conflict = ref(false)
const lastModified = ref('')
const activeHunks = ref([])
const instruction = ref('')
const suggesting = ref(false)
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
        daRegionField,
        wikilinkExtensions((target) => emit('open-wikilink', target)),
        EditorView.updateListener.of((v) => {
          if (v.docChanged) {
            isDirty.value = true
            scheduleSave()
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

const requestSuggestion = async () => {
  if (!props.path || !instruction.value.trim() || suggesting.value) return
  // Diff is computed against the file on disk, so flush unsaved edits first to
  // keep hunk offsets aligned with the editor buffer.
  if (isDirty.value) await saveFile()
  suggesting.value = true
  try {
    const res = await fetch('/api/notes/suggest', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ path: props.path, instruction: instruction.value })
    })
    if (res.ok) {
      const data = await res.json()
      activeHunks.value = data.hunks || []
      instruction.value = ''
    } else if (res.status === 503) {
      console.warn('DA suggest unavailable: no LLM provider configured')
    }
  } catch (e) {
    console.error('suggest failed', e)
  } finally {
    suggesting.value = false
  }
}

const acceptHunk = (hunk) => {
  if (!view) return
  // Apply the suggested change and mark the inserted range as DA-authored.
  const insertedTo = hunk.from + (hunk.content ? hunk.content.length : 0)
  view.dispatch({
    changes: { from: hunk.from, to: hunk.to, insert: hunk.content },
    effects: addDaRegion.of({ from: hunk.from, to: insertedTo })
  })
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
.cm-editor .cm-content {
  caret-color: #FFFFFF !important;
}
.cm-cursor,
.cm-cursor-primary {
  border-left-color: #FFFFFF !important;
}
.da-authored {
  background-color: rgba(107, 123, 176, 0.12);
  border-bottom: 1px dotted #6B7BB0;
  cursor: help;
}
</style>
