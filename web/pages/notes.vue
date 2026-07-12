<template>
  <div class="notes-shell" :class="{ 'notes-shell--collapsed': sidebarCollapsed }">
    <!-- Backdrop for the mobile overlay sidebar. -->
    <div
      v-if="!sidebarCollapsed"
      class="notes-shell__scrim"
      aria-hidden="true"
      @click="sidebarCollapsed = true"
    ></div>

    <aside class="vault" :class="{ 'vault--collapsed': sidebarCollapsed }">
     <div class="vault__inner">
      <div class="vault__tabs">
        <button
          v-for="tab in TABS"
          :key="tab.id"
          type="button"
          class="vault-tab"
          :class="{ 'vault-tab--active': activeTab === tab.id }"
          :aria-selected="activeTab === tab.id"
          @click="activeTab = tab.id"
        >
          <span class="material-symbols-outlined !text-[16px]" aria-hidden="true">{{ tab.icon }}</span>
          {{ tab.label }}
        </button>
        <div class="vault__tabs-grow"></div>
        <button type="button" class="vault__new" title="New note" aria-label="New note" @click="openNewNote">
          <span class="material-symbols-outlined !text-[18px]" aria-hidden="true">add</span>
        </button>
      </div>

      <div v-if="activeTab === 'files'" class="vault__search">
        <span class="material-symbols-outlined !text-[16px]" aria-hidden="true">search</span>
        <input
          v-model="query"
          class="vault__search-input"
          type="text"
          placeholder="Search notes"
          aria-label="Search notes"
        >
        <button v-if="query" type="button" class="vault__search-clear" aria-label="Clear search" @click="query = ''">
          <span class="material-symbols-outlined !text-[15px]" aria-hidden="true">close</span>
        </button>
      </div>

      <div class="vault__body">
        <NoteList
          v-show="activeTab === 'files'"
          ref="noteListRef"
          :selected="selectedFile"
          :query="query"
          @select="selectFile"
        />
        <div v-if="activeTab === 'tags'" class="vault__tags">
          <TagTree
            :tree="tagTree"
            :loading="tagsLoading"
            :error="tagsError"
            :selected="selectedTag"
            type="note"
            empty-text="No tags yet. Add tags in a note's properties."
            @select="selectedTag = $event"
          />
          <TagNodeList
            v-if="selectedTag"
            :tag="selectedTag"
            type="note"
            @open="(node) => selectFile(node.path)"
          />
        </div>
      </div>
     </div>
    </aside>

    <section class="workspace">
      <MarkdownEditor
        v-if="selectedFile"
        :path="selectedFile"
        :key="selectedFile"
        :sidebar-collapsed="sidebarCollapsed"
        @open-wikilink="openWikilink"
        @toggle-sidebar="toggleSidebar"
        @delete-note="showDeleteNote = true"
      />
      <div v-else class="workspace__empty">
        <div class="workspace__topbar">
          <button
            type="button"
            class="workspace__toggle"
            :title="sidebarCollapsed ? 'Show sidebar' : 'Hide sidebar'"
            :aria-label="sidebarCollapsed ? 'Show sidebar' : 'Hide sidebar'"
            @click="toggleSidebar"
          >
            <span class="material-symbols-outlined !text-[18px]" aria-hidden="true">
              {{ sidebarCollapsed ? 'left_panel_open' : 'left_panel_close' }}
            </span>
          </button>
        </div>
        <div class="workspace__empty-body">
          <span class="material-symbols-outlined workspace__empty-icon" aria-hidden="true">edit_note</span>
          <p class="workspace__empty-title">No note open</p>
          <p class="workspace__empty-hint">Pick a note from the vault, or create a new one.</p>
          <UiButton variant="primary" icon="add" @click="openNewNote">New note</UiButton>
        </div>
      </div>
    </section>

    <UiModal :open="showDeleteNote" title="Delete note" size="sm" @close="showDeleteNote = false">
      <p class="font-body text-body text-text-muted">
        Delete <span class="text-text-primary">{{ selectedFile }}</span>? The file will be moved to the system trash and its node will leave the graph.
      </p>
      <template #footer>
        <div class="flex justify-end gap-base">
          <UiButton variant="ghost" @click="showDeleteNote = false">Cancel</UiButton>
          <UiButton variant="primary" :loading="deleting" @click="confirmDeleteNote">Delete note</UiButton>
        </div>
      </template>
    </UiModal>

    <UiModal :open="showNewNote" :title="newNoteTag === 'telos' ? 'New Telos note' : 'New note'" size="sm" @close="closeNewNote">
      <UiField :hint="namePreview ? `Will create ${namePreview}` : ''">
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
            <UiButton variant="primary" :loading="creating" :disabled="!namePreview" @click="confirmNewNote">Create note</UiButton>
          </div>
        </div>
      </template>
    </UiModal>
  </div>
</template>

<script setup>
import { ref, computed, nextTick, onMounted, defineAsyncComponent } from 'vue'
import TagNodeList from '~/components/tags/TagNodeList.vue'
import TagTree from '~/components/tags/TagTree.vue'
import NoteList from '~/components/notes/NoteList.vue'
import { useTags } from '~/composables/useTags'
import UiButton from '~/components/ui/UiButton.vue'
import UiField from '~/components/ui/UiField.vue'
import UiInput from '~/components/ui/UiInput.vue'
import UiModal from '~/components/ui/UiModal.vue'

// Lazy-loaded so CodeMirror (a ~620 KB cohesive chunk) is fetched only when a
// note is actually opened, keeping the vault index paint light.
const MarkdownEditor = defineAsyncComponent(() => import('~/components/notes/MarkdownEditor.vue'))

const TABS = [
  { id: 'files', label: 'Files', icon: 'description' },
  { id: 'tags', label: 'Tags', icon: 'tag' },
]

const selectedFile = ref(null)
const noteListRef = ref(null)

// The Tags tab browses the universal tag tree narrowed to notes; the tags page
// (/tags) is the same tree across every node type.
const { tree: tagTree, loading: tagsLoading, error: tagsError, loadTree } = useTags()
const selectedTag = ref('')
const activeTab = ref('files')
const query = ref('')
const sidebarCollapsed = ref(false)

const showNewNote = ref(false)
const newTitle = ref('')
const newNoteTag = ref('') // when set, the new note is created pre-tagged (e.g. 'telos')
const titleInput = ref(null)
const creating = ref(false)

// Deep links from other surfaces: ?path= opens an existing note (e.g. Memory's
// "Edit" on a Telos note); ?new=<tag> opens the create dialog pre-tagged.
const route = useRoute()
onMounted(() => {
  const path = typeof route.query.path === 'string' ? route.query.path : ''
  if (path) {
    activeTab.value = 'files'
    selectFile(path)
    return
  }
  const tag = typeof route.query.new === 'string' ? route.query.new : ''
  if (tag) openNewNote(tag)
})

onMounted(loadTree)

const toggleSidebar = () => {
  sidebarCollapsed.value = !sidebarCollapsed.value
}

const selectFile = (path) => {
  selectedFile.value = path
  // On narrow screens the sidebar overlays the editor; get out of the way once
  // a note is chosen.
  if (typeof window !== 'undefined' && window.innerWidth < 768) {
    sidebarCollapsed.value = true
  }
}

// Resolve a clicked wikilink target to an actual vault file and select it the
// same way clicking a note in the list does. Autocomplete inserts
// [[<uuid>|title]], so try the graph id→path mapping first; hand-typed
// [[Some Note]] links fall back to filename-slug matching.
const openWikilink = async (target) => {
  try {
    const res = await fetch('/api/vault/notes')
    if (res.ok) {
      const notes = (await res.json()) || []
      const byId = notes.find((n) => n.id === target)
      if (byId?.path) {
        selectFile(byId.path)
        return
      }
      const byTitle = notes.find((n) => (n.title || '').toLowerCase() === target.toLowerCase())
      if (byTitle?.path) {
        selectFile(byTitle.path)
        return
      }
    }
    const slug = target.endsWith('.md') ? target : `${target}.md`
    const listRes = await fetch('/api/vault/list')
    if (listRes.ok) {
      const { files } = await listRes.json()
      const match = (files || []).find((f) => f === slug || f.endsWith(`/${slug}`))
      if (match) {
        selectFile(match)
        return
      }
    }

    // Note not found; create it automatically.
    // (If the target is a UUID, we don't want to create a file named "019f...md",
    // but wikilinks typed manually without autocomplete just use the title as target).
    const isUUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(target)
    if (!isUUID) {
      const title = target.replace(/\.md$/, '')
      const path = `${title}.md`
      const body = `---\ntitle: ${title}\ntags: []\n---\n\n# ${title}\n\n`

      const createRes = await fetch(`/api/vault/file?path=${encodeURIComponent(path)}`, {
        method: 'POST',
        headers: { 'Content-Type': 'text/plain' },
        body
      })
      if (createRes.ok) {
        // Wait briefly for the backend vault watcher to index the new note
        await new Promise((r) => setTimeout(r, 250))
        // Refresh the file list so the sidebar knows about it
        if (noteListRef.value) noteListRef.value.refresh()
        selectFile(path)
      }
    }
  } catch (e) { /* best-effort wikilink navigation */ }
}

// Delete the open note: the toolbar button asks, this confirms and executes.
// The backend removes the file; the vault watcher reconciles the node away.
const showDeleteNote = ref(false)
const deleting = ref(false)

const confirmDeleteNote = async () => {
  if (!selectedFile.value || deleting.value) return
  deleting.value = true
  try {
    const res = await fetch(`/api/vault/file?path=${encodeURIComponent(selectedFile.value)}`, {
      method: 'DELETE',
    })
    if (res.ok || res.status === 404) {
      showDeleteNote.value = false
      selectedFile.value = null
      noteListRef.value?.refresh()
    }
  } finally {
    deleting.value = false
  }
}

const namePreview = computed(() => newTitle.value ? `${newTitle.value.trim()}.md` : '')

const openNewNote = async (tag = '') => {
  newTitle.value = ''
  // Guard against the DOM click event being passed as a positional arg.
  newNoteTag.value = typeof tag === 'string' ? tag : ''
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
  if (!title || creating.value) return
  creating.value = true

  // Collision guard against the current disk-truth list.
  let existing = []
  try {
    const res = await fetch('/api/vault/list')
    if (res.ok) existing = (await res.json()).files || []
  } catch (e) { /* best-effort */ }

  let path = `${title}.md`
  let n = 2
  while (existing.includes(path)) {
    path = `${title} ${n}.md`
    n++
  }

  const tagsLine = newNoteTag.value ? `tags: [${newNoteTag.value}]` : 'tags: []'
  const body = `---\ntitle: ${title}\n${tagsLine}\n---\n\n# ${title}\n\n`
  try {
    const res = await fetch(`/api/vault/file?path=${encodeURIComponent(path)}`, {
      method: 'POST',
      headers: { 'Content-Type': 'text/plain' },
      body
    })
    if (res.ok) {
      showNewNote.value = false
      newNoteTag.value = ''
      activeTab.value = 'files'
      selectFile(path)
      await new Promise((r) => setTimeout(r, 250))
      noteListRef.value?.refresh()
    }
  } finally {
    creating.value = false
  }
}
</script>

<style scoped>
.notes-shell {
  position: relative;
  display: grid;
  grid-template-columns: 272px minmax(0, 1fr);
  height: 100%;
  background-color: var(--color-bg-base);
  font-family: var(--font-body);
  /* Collapse animates the grid track, not the panel's own width/margin, so the
     fixed-width inner content slides behind overflow:hidden without reflowing. */
  transition: grid-template-columns 200ms cubic-bezier(0.22, 1, 0.36, 1);
}

.notes-shell--collapsed {
  grid-template-columns: 0 minmax(0, 1fr);
}

/* --- Vault sidebar --- */
.vault {
  min-width: 0;
  border-right: 1px solid var(--color-border-default);
  background-color: var(--color-surface);
  overflow: hidden;
}

.vault--collapsed {
  border-right-color: transparent;
}

/* Fixed width so the content keeps its layout while the track shrinks to 0. */
.vault__inner {
  display: flex;
  flex-direction: column;
  width: 272px;
  height: 100%;
}

.vault__tabs {
  display: flex;
  align-items: center;
  gap: 2px;
  height: 42px;
  flex-shrink: 0;
  padding: 0 8px;
  border-bottom: 1px solid var(--color-border-hairline);
}

.vault-tab {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 28px;
  padding: 0 10px;
  border-radius: var(--radius-lg);
  color: var(--color-text-muted);
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
  position: relative;
  transition: color 120ms ease, background-color 120ms ease;
}

.vault-tab:hover {
  color: var(--color-text-primary);
  background-color: color-mix(in srgb, var(--color-surface-hover) 50%, transparent);
}

.vault-tab--active {
  color: var(--color-text-primary);
}

/* Underline indicator for the active tab. */
.vault-tab--active::after {
  content: '';
  position: absolute;
  left: 10px;
  right: 10px;
  bottom: -8px;
  height: 2px;
  border-radius: 2px;
  background-color: var(--color-primary);
}

.vault-tab:focus-visible {
  outline: none;
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--color-da-accent) 70%, transparent);
}

.vault__tabs-grow {
  flex: 1 1 auto;
}

.vault__new {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border-radius: var(--radius-lg);
  color: var(--color-text-muted);
  cursor: pointer;
  transition: color 120ms ease, background-color 120ms ease;
}

.vault__new:hover {
  color: var(--color-text-primary);
  background-color: var(--color-surface-hover);
}

.vault__new:focus-visible {
  outline: none;
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--color-da-accent) 70%, transparent);
}

.vault__search {
  display: flex;
  align-items: center;
  gap: 6px;
  height: 34px;
  margin: 8px;
  padding: 0 8px;
  border-radius: var(--radius-lg);
  border: 1px solid var(--color-border-hairline);
  background-color: var(--color-bg-base);
  color: var(--color-text-faint);
  transition: border-color 120ms ease;
}

.vault__search:focus-within {
  border-color: color-mix(in srgb, var(--color-da-accent) 70%, transparent);
}

.vault__search-input {
  flex: 1 1 auto;
  min-width: 0;
  background: transparent;
  border: none;
  outline: none;
  font-size: 13px;
  color: var(--color-text-primary);
}

.vault__search-input::placeholder {
  color: var(--color-text-faint);
}

.vault__search-clear {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  border-radius: var(--radius-full);
  color: var(--color-text-faint);
  cursor: pointer;
}

.vault__search-clear:hover {
  color: var(--color-text-primary);
  background-color: var(--color-surface-hover);
}

.vault__body {
  flex: 1 1 auto;
  min-height: 0;
  overflow-y: auto;
}

/* The tag tree on top, the notes under the picked tag below it. */
.vault__tags {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-base);
  padding: var(--spacing-base);
}

/* --- Workspace --- */
.workspace {
  display: flex;
  flex-direction: column;
  flex: 1 1 auto;
  min-width: 0;
  background-color: var(--color-bg-base);
}

.workspace__empty {
  display: flex;
  flex-direction: column;
  height: 100%;
}

.workspace__topbar {
  display: flex;
  align-items: center;
  height: 42px;
  flex-shrink: 0;
  padding: 0 10px;
  border-bottom: 1px solid var(--color-border-default);
  background-color: var(--color-surface);
}

.workspace__toggle {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  border-radius: var(--radius-lg);
  color: var(--color-text-muted);
  cursor: pointer;
  transition: color 120ms ease, background-color 120ms ease;
}

.workspace__toggle:hover {
  color: var(--color-text-primary);
  background-color: var(--color-surface-hover);
}

.workspace__empty-body {
  flex: 1 1 auto;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 10px;
  padding: 24px;
  text-align: center;
}

.workspace__empty-icon {
  font-size: 40px;
  color: var(--color-text-dim);
}

.workspace__empty-title {
  font-size: 15px;
  font-weight: 600;
  color: var(--color-text-primary);
}

.workspace__empty-hint {
  margin-bottom: 6px;
  font-size: 13px;
  color: var(--color-text-muted);
}

.notes-shell__scrim {
  display: none;
}

/* Mobile: the sidebar overlays the editor rather than squeezing it. */
@media (max-width: 767px) {
  .notes-shell {
    grid-template-columns: minmax(0, 1fr);
  }

  .notes-shell--collapsed {
    grid-template-columns: minmax(0, 1fr);
  }

  .vault {
    position: absolute;
    top: 0;
    bottom: 0;
    left: 0;
    width: 272px;
    z-index: 50;
    box-shadow: 0 12px 32px rgba(0, 0, 0, 0.5);
    transition: transform 200ms cubic-bezier(0.22, 1, 0.36, 1);
  }

  .vault--collapsed {
    transform: translateX(-100%);
  }

  .notes-shell__scrim {
    display: block;
    position: absolute;
    inset: 0;
    z-index: 40;
    background-color: rgba(0, 0, 0, 0.5);
  }
}

@media (prefers-reduced-motion: reduce) {
  .notes-shell,
  .vault {
    transition: none;
  }
}
</style>
