<template>
  <div class="notes-editor-pane" :style="styleVars">
    <NoteEditorToolbar :sidebar-collapsed="sidebarCollapsed" @toggle-sidebar="$emit('toggle-sidebar')" />

    <div
      ref="scrollEl"
      class="notes-editor-scroll"
      :class="{ 'notes-editor-scroll--typewriter': settings.typewriter }"
    >
      <div class="notes-editor-measure" :class="{ 'is-reading': settings.viewMode === 'reading' }">
        <NoteProperties
          v-if="settings.viewMode !== 'source'"
          :data="frontmatterData"
          :parse-error="frontmatterError"
          :readonly="settings.viewMode === 'reading'"
          :show-id="settings.showId"
          @update:data="applyFrontmatterUpdate"
        />
        <div ref="editorContainer" class="notes-editor-cm"></div>
      </div>
    </div>

    <UiModal :open="conflict" title="Save Conflict" size="sm" @close="conflict = false">
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

    <!-- DA diff plumbing stays wired for note-scoped DA chat suggestions; it
         renders only when hunks arrive (none from this surface today). -->
    <DiffSuggest
      v-if="activeHunks.length > 0"
      :hunks="activeHunks"
      @accept="acceptHunk"
      @reject="rejectHunk"
    />
  </div>
</template>

<script setup>
import { ref, onMounted, onBeforeUnmount, watch } from 'vue'
import { EditorState, StateField, StateEffect, Compartment } from '@codemirror/state'
import { EditorView, lineNumbers, Decoration } from '@codemirror/view'
import { markdown } from '@codemirror/lang-markdown'
import { wikilinkExtensions } from '~/utils/wikilinkEditor'
import { livePreviewExtensions } from '~/utils/markdownPreview'
import { frontmatterConcealExtension } from '~/utils/frontmatterConceal'
import { typewriterExtension } from '~/utils/typewriterMode'
import { replaceFrontmatter, splitFrontmatter } from '~/utils/frontmatter'
import { useEditorSettings } from '~/composables/useEditorSettings'
import NoteProperties from './NoteProperties.vue'
import NoteEditorToolbar from './NoteEditorToolbar.vue'
import DiffSuggest from './DiffSuggest.vue'
import UiButton from '~/components/ui/UiButton.vue'
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
  initialContent: String,
  sidebarCollapsed: Boolean,
})

// open-wikilink: emitted when a wikilink pill is ctrl/cmd-clicked.
// toggle-sidebar: forwarded from the toolbar to the parent shell.
const emit = defineEmits(['open-wikilink', 'toggle-sidebar'])

const { settings, styleVars } = useEditorSettings()

const editorContainer = ref(null)
const scrollEl = ref(null)
let view = null

// Compartments let us swap mode/setting-driven extensions without tearing down
// the editor (preserves cursor, history, scroll on a mode flip).
const previewComp = new Compartment()
const concealComp = new Compartment()
const lineNumbersComp = new Compartment()
const editableComp = new Compartment()
const typewriterComp = new Compartment()

const frontmatterData = ref({})
const frontmatterError = ref('')
const rawContent = ref('')
const isDirty = ref(false)
const conflict = ref(false)
const lastModified = ref('')
const activeHunks = ref([])
const saveError = ref(false)
let saveTimer = null

const syncFrontmatter = (text) => {
  try {
    frontmatterData.value = splitFrontmatter(text).data
    frontmatterError.value = ''
  } catch (error) {
    frontmatterError.value = error instanceof Error ? error.message : 'Frontmatter is not valid YAML.'
  }
}

// --- Mode/setting → extension mapping -------------------------------------

const previewExtFor = (mode) => {
  if (mode === 'source') return []
  return livePreviewExtensions(mode === 'live') // reading → reveal=false (full conceal)
}
const concealExtFor = (mode) => (mode === 'source' ? [] : frontmatterConcealExtension())
const lineNumbersExtFor = (mode, on) => (on && mode !== 'reading' ? lineNumbers() : [])
const editableExtFor = (mode) => EditorView.editable.of(mode !== 'reading')
const typewriterExtFor = (on) => (on ? typewriterExtension() : [])

const reconfigure = () => {
  if (!view) return
  const mode = settings.viewMode
  view.dispatch({
    effects: [
      previewComp.reconfigure(previewExtFor(mode)),
      concealComp.reconfigure(concealExtFor(mode)),
      lineNumbersComp.reconfigure(lineNumbersExtFor(mode, settings.lineNumbers)),
      editableComp.reconfigure(editableExtFor(mode)),
      typewriterComp.reconfigure(typewriterExtFor(settings.typewriter)),
    ],
  })
}

const loadFile = async (path) => {
  if (!path) return
  const res = await fetch(`/api/vault/file?path=${encodeURIComponent(path)}`)
  if (res.ok) {
    const text = await res.text()
    rawContent.value = text
    syncFrontmatter(text)

    if (view) view.destroy()

    const mode = settings.viewMode
    const state = EditorState.create({
      doc: text,
      extensions: [
        lineNumbersComp.of(lineNumbersExtFor(mode, settings.lineNumbers)),
        markdown(),
        daRegionField,
        previewComp.of(previewExtFor(mode)),
        concealComp.of(concealExtFor(mode)),
        editableComp.of(editableExtFor(mode)),
        typewriterComp.of(typewriterExtFor(settings.typewriter)),
        wikilinkExtensions((target) => emit('open-wikilink', target)),
        EditorView.lineWrapping,
        EditorView.updateListener.of((v) => {
          if (v.docChanged) {
            isDirty.value = true
            syncFrontmatter(v.state.doc.toString())
            scheduleSave()
          }
        }),
      ],
    })

    view = new EditorView({ state, parent: editorContainer.value })

    lastModified.value = res.headers.get('Last-Modified') || new Date().toISOString()
    isDirty.value = false
    saveError.value = false
  }
}

const applyFrontmatterUpdate = (nextData) => {
  if (!view || frontmatterError.value) return
  const nextContent = replaceFrontmatter(view.state.doc.toString(), nextData)
  view.dispatch({
    changes: { from: 0, to: view.state.doc.length, insert: nextContent },
  })
  frontmatterData.value = nextData
}

watch(() => props.path, (newPath) => {
  loadFile(newPath)
})

// React to view-mode + line-number + typewriter changes from the toolbar.
watch(
  () => [settings.viewMode, settings.lineNumbers, settings.typewriter],
  () => reconfigure(),
)

onMounted(() => {
  loadFile(props.path)
})

onBeforeUnmount(() => {
  // Flush any pending autosave before tearing down. A plain clearTimeout would
  // silently discard edits made within the 5s debounce window. saveFile() must
  // run before view.destroy() so view.state.doc still holds the outgoing content.
  if (saveTimer && isDirty.value && view) {
    clearTimeout(saveTimer)
    saveTimer = null
    saveFile() // fire-and-forget; saves outgoing path + content
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
        'Content-Type': 'text/plain',
      },
      body: content,
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

const acceptHunk = (hunk) => {
  if (!view) return
  // Apply the suggested change and mark the inserted range as DA-authored.
  const insertedTo = hunk.from + (hunk.content ? hunk.content.length : 0)
  view.dispatch({
    changes: { from: hunk.from, to: hunk.to, insert: hunk.content },
    effects: addDaRegion.of({ from: hunk.from, to: insertedTo }),
  })
  activeHunks.value = activeHunks.value.filter(h => h.id !== hunk.id)
}

const rejectHunk = (hunk) => {
  activeHunks.value = activeHunks.value.filter(h => h.id !== hunk.id)
}
</script>

<style scoped>
.notes-editor-pane {
  position: relative;
  display: flex;
  flex-direction: column;
  height: 100%;
  min-width: 0;
  background-color: var(--color-bg-base);
  color: var(--color-text-primary);
}

/* The single scroll surface: the properties block and the editor scroll as one
   document, so properties scroll away with content (Obsidian-like). */
.notes-editor-scroll {
  flex: 1 1 auto;
  min-height: 0;
  overflow-y: auto;
  overflow-x: hidden;
}

.notes-editor-measure {
  width: 100%;
  max-width: 760px;
  margin: 0 auto;
  padding: 28px 40px 25vh;
}

.notes-editor-measure.is-reading {
  max-width: 720px;
}

/* Extra bottom room so the last lines can reach the vertical center. */
.notes-editor-scroll--typewriter .notes-editor-measure {
  padding-bottom: 55vh;
}
</style>

<style>
/* CodeMirror grows to its content; the pane (.notes-editor-scroll) owns scroll,
   so the properties block and document share one scrollbar. */
.notes-editor-cm .cm-editor {
  height: auto;
  background-color: transparent !important;
  color: var(--color-text-primary) !important;
}
/* No focus ring on the editor surface — it's a page-like document, not a field. */
.notes-editor-cm .cm-editor.cm-focused {
  outline: none !important;
}
.notes-editor-cm .cm-scroller {
  overflow: visible;
  font-family: var(--notes-font, var(--font-body)) !important;
  font-size: var(--notes-font-size, 15px);
  line-height: 1.7;
}
.notes-editor-cm .cm-content {
  padding: 0;
  caret-color: var(--color-on-surface) !important;
}
/* Line-number gutter sits flush; transparent so it blends into the page. */
.notes-editor-cm .cm-gutters {
  background-color: transparent !important;
  color: var(--color-text-dim) !important;
  border-right: none !important;
}
.notes-editor-cm .cm-activeLineGutter {
  background-color: transparent !important;
  color: var(--color-text-muted) !important;
}
.notes-editor-cm .cm-activeLine {
  background-color: color-mix(in srgb, var(--color-surface-hover) 40%, transparent) !important;
}
/* Reading mode reads as a document, not an editor: no active-line tint, no caret. */
.is-reading .cm-activeLine {
  background-color: transparent !important;
}
.is-reading .cm-cursor,
.is-reading .cm-cursor-primary {
  display: none !important;
}
.notes-editor-cm .cm-cursor,
.notes-editor-cm .cm-cursor-primary {
  border-left-color: var(--color-on-surface) !important;
}
.da-authored {
  background-color: color-mix(in srgb, var(--color-da-accent) 12%, transparent);
  border-bottom: 1px dotted var(--color-da-accent);
  cursor: help;
}
</style>
