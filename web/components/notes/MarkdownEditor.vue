<template>
  <div class="notes-editor-pane flex flex-col h-full bg-bg-base text-text-primary">
    <FrontmatterUI v-if="frontmatter" :data="frontmatter" />
    <div class="flex items-center gap-base px-component py-base border-b border-border-default shrink-0">
      <UiInput
        v-model="instruction"
        @keydown.enter="requestSuggestion"
        placeholder="How should the AI edit this note? (e.g. summarize, fix grammar)…"
        classes="h-8 flex-1 rounded border border-border-default bg-bg-elevated px-base font-body text-body text-text-primary outline-none transition-colors placeholder:text-text-muted focus:border-da-accent disabled:cursor-not-allowed disabled:opacity-50"
      />
      <UiButton
        @click="requestSuggestion"
        :disabled="suggesting || !instruction.trim()"
        :loading="suggesting"
        variant="accent"
        size="sm"
      >Suggest</UiButton>
    </div>
    <div class="flex-1 relative">
      <div ref="editorContainer" class="h-full w-full"></div>
      
      <UiModal
        :open="conflict"
        title="Save Conflict"
        size="sm"
        @close="conflict = false"
      >
        <p class="font-body text-body text-text-muted">
          This note was modified outside the editor while you were working. If you overwrite the file, you'll erase those outside changes. If you discard your edits, you'll lose the work you just did here.
        </p>

        <template #footer>
          <div class="flex justify-end gap-base">
            <UiButton variant="secondary" @click="resolveConflict('reload')">Discard my edits</UiButton>
            <UiButton variant="primary" @click="resolveConflict('keep')">Overwrite file</UiButton>
          </div>
        </template>
      </UiModal>
      
      <UiToast 
        v-if="saveError" 
        message="We couldn't save your changes." 
        actionLabel="Retry" 
        @action="saveFile" 
      />
      
      <UiToast 
        v-if="suggestError" 
        :message="suggestError" 
        actionLabel="Dismiss" 
        @action="suggestError = ''" 
      />
      
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
import { livePreviewExtensions } from '~/utils/markdownPreview'
import FrontmatterUI from './FrontmatterUI.vue'
import DiffSuggest from './DiffSuggest.vue'
import UiButton from '~/components/ui/UiButton.vue'
import UiInput from '~/components/ui/UiInput.vue'
import UiModal from '~/components/ui/UiModal.vue'
import UiToast from '~/components/ui/UiToast.vue'

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
const saveError = ref(false)
const suggestError = ref('')
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
        livePreviewExtensions(),
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
  // Flush any pending autosave before tearing down. A plain clearTimeout would
  // silently discard edits made within the 5s debounce window. saveFile() must
  // run before view.destroy() so view.state.doc still holds the outgoing content.
  // props.path is still the outgoing file's path here (new instance not yet mounted).
  if (saveTimer && isDirty.value && view) {
    clearTimeout(saveTimer)
    saveTimer = null
    saveFile() // fire-and-forget; saves outgoing path + content, not the new file
  }
  if (view) view.destroy()
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
  
  try {
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
      saveError.value = false
    } else {
      saveError.value = true
    }
  } catch (e) {
    saveError.value = true
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
  suggestError.value = ''
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
      suggestError.value = "AI provider not configured. Add an LLM provider in settings."
    } else {
      suggestError.value = "We couldn't generate a suggestion. Please try again."
    }
  } catch (e) {
    suggestError.value = "We couldn't generate a suggestion. Please check your connection and try again."
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
  color: var(--color-text-primary) !important;
}
.cm-gutters {
  background-color: var(--color-bg-elevated) !important;
  color: var(--color-text-muted) !important;
  border-right: 1px solid var(--color-border-hairline) !important;
}
.cm-activeLineGutter {
  background-color: var(--color-surface-hover) !important;
  color: var(--color-text-primary) !important;
}
.cm-activeLine {
  background-color: var(--color-surface-hover) !important;
}
.cm-editor .cm-content {
  caret-color: var(--color-on-surface) !important;
}
.cm-cursor,
.cm-cursor-primary {
  border-left-color: var(--color-on-surface) !important;
}
.da-authored {
  background-color: color-mix(in srgb, var(--color-da-accent) 12%, transparent);
  border-bottom: 1px dotted var(--color-da-accent);
  cursor: help;
}
</style>
