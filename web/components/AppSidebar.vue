<template>
  <nav
    class="app-sidebar"
    :class="{ 'app-sidebar--collapsed': collapsed }"
    aria-label="Primary navigation"
    @keydown.esc="closeContext"
  >
    <div class="sidebar-header">
      <NuxtLink to="/" class="sidebar-brand" aria-label="Kernl home">
        <span class="sidebar-logo material-symbols-outlined" aria-hidden="true">terminal</span>
        <span v-if="!collapsed" class="sidebar-wordmark">Kernl</span>
      </NuxtLink>
      <button
        type="button"
        class="sidebar-collapse"
        :aria-label="collapsed ? 'Expand sidebar' : 'Collapse sidebar'"
        :title="collapsed ? 'Expand sidebar' : 'Collapse sidebar'"
        @click="toggleCollapsed"
      >
        <span class="material-symbols-outlined" aria-hidden="true">
          {{ collapsed ? 'left_panel_open' : 'left_panel_close' }}
        </span>
      </button>
    </div>

    <div class="sidebar-scroll">
      <section v-for="group in navigationGroups" :key="group.label" class="sidebar-group">
        <h2 v-if="!collapsed" class="sidebar-group-label">{{ group.label }}</h2>
        <div class="sidebar-group-items">
          <div v-for="item in group.items" :key="item.id" class="sidebar-entry">
            <div class="sidebar-row" :class="{ 'sidebar-row--active': isActive(item.to) }">
              <NuxtLink
                :to="item.to"
                class="sidebar-link"
                :aria-label="collapsed ? item.label : undefined"
                :title="collapsed ? item.label : undefined"
              >
                <span class="sidebar-icon material-symbols-outlined" aria-hidden="true">{{ item.icon }}</span>
                <span v-if="!collapsed" class="sidebar-label">{{ item.label }}</span>
              </NuxtLink>
              <button
                v-if="!collapsed && item.context"
                type="button"
                class="sidebar-disclosure"
                :aria-label="`${openContexts.has(item.id as ContextId) ? 'Hide' : 'Show'} ${item.label} shortcuts`"
                :aria-expanded="openContexts.has(item.id as ContextId)"
                :aria-controls="`${item.id}-shortcuts`"
                @click="toggleContext(item.id)"
              >
                <span class="material-symbols-outlined" aria-hidden="true">expand_more</span>
              </button>
            </div>

            <div
              v-if="!collapsed && item.context && openContexts.has(item.id as ContextId)"
              :id="`${item.id}-shortcuts`"
              class="sidebar-context"
            >
              <p v-if="loading[item.id]" class="sidebar-context-status">Loading…</p>
              <p v-else-if="errors[item.id]" class="sidebar-context-status sidebar-context-status--error">
                Could not load.
                <button type="button" @click="loadContext(item.id)">Retry</button>
              </p>
              <p v-else-if="contextItems(item.id).length === 0" class="sidebar-context-status">
                {{ emptyLabel(item.id) }}
              </p>
              <NuxtLink
                v-for="shortcut in contextItems(item.id)"
                v-else
                :key="shortcut.key"
                :to="shortcut.to"
                class="sidebar-shortcut"
                :title="shortcut.label"
              >
                <span class="sidebar-shortcut-label">{{ shortcut.label }}</span>
                <span v-if="shortcut.meta" class="sidebar-shortcut-meta">{{ shortcut.meta }}</span>
              </NuxtLink>
              <button
                v-if="remainingItems(item.id) > 0"
                type="button"
                class="sidebar-view-more"
                @click="revealMore(item.id)"
              >
                View more
              </button>
            </div>
          </div>
        </div>
      </section>
    </div>

    <div class="sidebar-footer">
      <NuxtLink
        to="/settings"
        class="sidebar-row sidebar-link sidebar-settings"
        :class="{ 'sidebar-row--active': isActive('/settings') }"
        :aria-label="collapsed ? 'Settings' : undefined"
        :title="collapsed ? 'Settings' : undefined"
      >
        <span class="sidebar-icon material-symbols-outlined" aria-hidden="true">settings</span>
        <span v-if="!collapsed" class="sidebar-label">Settings</span>
      </NuxtLink>
    </div>
  </nav>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import {
  activeProjects,
  recentBookmarks,
  recentNotes,
  sidebarTasks,
  type SidebarBookmark,
  type SidebarNote,
  type SidebarProject,
  type SidebarTask,
} from '~/utils/sidebarContent'

type ContextId = 'notes' | 'bookmarks' | 'memory' | 'projects' | 'tasks'

const INITIAL_VISIBLE_COUNT = 5
const REVEAL_COUNT = 50

interface NavigationItem {
  id: string
  label: string
  icon: string
  to: string
  context?: boolean
}

interface Shortcut {
  key: string
  label: string
  meta?: string
  to: string | { path: string; query: Record<string, string> }
}

const route = useRoute()
const collapsed = ref(false)
const openContexts = reactive(new Set<ContextId>())
const notes = ref<SidebarNote[]>([])
const bookmarks = ref<SidebarBookmark[]>([])
const memoryTopics = ref<string[]>([])
const projects = ref<SidebarProject[]>([])
const tasks = ref<SidebarTask[]>([])

const loading = reactive<Record<ContextId, boolean>>({
  notes: false,
  bookmarks: false,
  memory: false,
  projects: false,
  tasks: false,
})
const errors = reactive<Record<ContextId, boolean>>({
  notes: false,
  bookmarks: false,
  memory: false,
  projects: false,
  tasks: false,
})
const visibleCount = reactive<Record<ContextId, number>>({
  notes: INITIAL_VISIBLE_COUNT,
  bookmarks: INITIAL_VISIBLE_COUNT,
  memory: INITIAL_VISIBLE_COUNT,
  projects: INITIAL_VISIBLE_COUNT,
  tasks: INITIAL_VISIBLE_COUNT,
})

const navigationGroups: { label: string; items: NavigationItem[] }[] = [
  {
    label: 'Overview',
    items: [
      { id: 'home', label: 'Home', icon: 'home', to: '/' },
      { id: 'inbox', label: 'Inbox', icon: 'inbox', to: '/inbox' },
    ],
  },
  {
    label: 'Knowledge',
    items: [
      { id: 'notes', label: 'Notes', icon: 'description', to: '/notes', context: true },
      { id: 'bookmarks', label: 'Bookmarks', icon: 'bookmark', to: '/bookmarks', context: true },
      { id: 'memory', label: 'Memory', icon: 'neurology', to: '/memory', context: true },
      { id: 'graph', label: 'Graph', icon: 'hub', to: '/graph' },
    ],
  },
  {
    label: 'Work',
    items: [
      { id: 'projects', label: 'Projects', icon: 'folder_open', to: '/projects', context: true },
      { id: 'tasks', label: 'Tasks', icon: 'task_alt', to: '/tasks', context: true },
    ],
  },
  {
    label: 'Operations',
    items: [
      { id: 'orchestrator', label: 'Orchestrator', icon: 'account_tree', to: '/orchestrator' },
      { id: 'ingest', label: 'Ingest', icon: 'input', to: '/ingest' },
      { id: 'audit', label: 'Audit', icon: 'policy', to: '/audit' },
    ],
  },
]

const noteShortcuts = computed<Shortcut[]>(() =>
  recentNotes(notes.value, notes.value.length).map((note) => ({
    key: note.id,
    label: note.title || note.path.replace(/\.md$/, ''),
    to: { path: '/notes', query: { path: note.path } },
  })),
)

const bookmarkShortcuts = computed<Shortcut[]>(() =>
  recentBookmarks(bookmarks.value, bookmarks.value.length).map((bookmark) => ({
    key: bookmark.id,
    label: bookmark.title || bookmark.url,
    meta: domain(bookmark.url),
    to: { path: '/bookmarks', query: { bookmark: bookmark.id } },
  })),
)

const memoryShortcuts = computed<Shortcut[]>(() =>
  memoryTopics.value.map((topic) => ({
    key: topic,
    label: topic,
    to: { path: '/memory', query: { topic } },
  })),
)

const projectShortcuts = computed<Shortcut[]>(() =>
  activeProjects(projects.value, projects.value.length).map((project) => ({
    key: project.id,
    label: project.title,
    to: { path: '/tasks', query: { project: project.id } },
  })),
)

const taskShortcuts = computed<Shortcut[]>(() =>
  sidebarTasks(tasks.value, new Date(), tasks.value.length).map((task) => ({
    key: task.id,
    label: task.title,
    meta: taskMeta(task),
    to: { path: '/tasks', query: { task: task.id } },
  })),
)

function isActive(path: string): boolean {
  return path === '/' ? route.path === '/' : route.path.startsWith(path)
}

function toggleCollapsed(): void {
  collapsed.value = !collapsed.value
  if (collapsed.value) openContexts.clear()
  localStorage.setItem('kernl:sidebar-collapsed', collapsed.value ? '1' : '0')
}

function closeContext(): void {
  openContexts.clear()
}

async function toggleContext(id: string): Promise<void> {
  const contextId = id as ContextId
  if (openContexts.has(contextId)) {
    openContexts.delete(contextId)
    return
  }
  openContexts.add(contextId)
  await loadContext(contextId)
}

async function fetchJSON<T>(url: string): Promise<T> {
  const response = await fetch(url)
  if (!response.ok) throw new Error(`GET ${url} → ${response.status}`)
  return response.json()
}

async function loadContext(id: ContextId): Promise<void> {
  loading[id] = true
  errors[id] = false
  try {
    if (id === 'notes') notes.value = await fetchJSON<SidebarNote[]>('/api/vault/notes')
    if (id === 'bookmarks') bookmarks.value = await fetchJSON<SidebarBookmark[]>('/api/bookmarks')
    if (id === 'memory') {
      const payload = await fetchJSON<{ topics: string[] }>('/api/memory/topics')
      memoryTopics.value = payload.topics || []
    }
    if (id === 'projects') projects.value = await fetchJSON<SidebarProject[]>('/api/projects')
    if (id === 'tasks') tasks.value = await fetchJSON<SidebarTask[]>('/api/tasks')
  } catch {
    errors[id] = true
  } finally {
    loading[id] = false
  }
}

function contextItems(id: string): Shortcut[] {
  const contextId = id as ContextId
  return allContextItems(contextId).slice(0, visibleCount[contextId])
}

function allContextItems(id: ContextId): Shortcut[] {
  if (id === 'notes') return noteShortcuts.value
  if (id === 'bookmarks') return bookmarkShortcuts.value
  if (id === 'memory') return memoryShortcuts.value
  if (id === 'projects') return projectShortcuts.value
  return taskShortcuts.value
}

function remainingItems(id: string): number {
  const contextId = id as ContextId
  return Math.max(0, allContextItems(contextId).length - visibleCount[contextId])
}

function revealMore(id: string): void {
  visibleCount[id as ContextId] += REVEAL_COUNT
}

function emptyLabel(id: string): string {
  if (id === 'projects') return 'No active projects.'
  if (id === 'tasks') return 'No tasks need attention.'
  return `No ${id}.`
}

function domain(url: string): string {
  try {
    return new URL(url).hostname.replace(/^www\./, '')
  } catch {
    return ''
  }
}

function taskMeta(task: SidebarTask): string {
  if (task.dueDate) return task.dueDate
  if (task.tags.includes('next')) return 'Next'
  return ''
}

onMounted(() => {
  collapsed.value = localStorage.getItem('kernl:sidebar-collapsed') === '1'
})
</script>

<style scoped src="../assets/css/app-sidebar.css"></style>
