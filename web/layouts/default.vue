<template>
  <div class="bg-bg-base text-text-primary h-screen flex flex-col overflow-hidden font-body selection:bg-primary selection:text-on-primary">
    <!-- Top Layout Wrapper -->
    <div class="flex flex-1 overflow-hidden">
      <!-- Left sidebar: labelled destinations, grouped -->
      <nav class="side-nav w-rail-width shrink-0 h-full bg-surface border-r border-border-hairline flex flex-col px-3 pb-3 overflow-y-auto z-dropdown">
        <div class="flex items-center justify-between pt-4 pb-[18px] px-2">
          <NuxtLink to="/" class="flex items-center gap-[9px] rounded outline-none focus-visible:ring-1 focus-visible:ring-primary/30">
            <span class="w-[22px] h-[22px] shrink-0 flex items-center justify-center rounded-lg bg-accent-tint-strong">
              <span class="material-symbols-outlined nav-logo-icon text-primary">hub</span>
            </span>
            <span class="font-headline text-[14px] font-semibold tracking-[-0.01em] text-text-primary">Kernl</span>
          </NuxtLink>
          <button
            class="flex p-1 rounded text-text-muted hover:bg-surface-nav-hover hover:text-text-secondary outline-none focus-visible:ring-1 focus-visible:ring-primary/30 transition-colors duration-150 cursor-pointer"
            aria-label="Collapse sidebar"
            title="Collapse sidebar"
            @click="collapse"
          >
            <span class="material-symbols-outlined nav-icon">left_panel_close</span>
          </button>
        </div>

        <div v-for="(group, i) in NAV_GROUPS" :key="group.caption" class="contents">
          <!-- Group separation is the sidebar's only vertical rhythm, so it is a
               larger step than the gap between rows. The first caption already
               sits under the wordmark's padding and does not need it. -->
          <div
            class="font-label-caps text-label-caps font-normal uppercase text-text-faint px-2 mb-1.5"
            :class="i === 0 ? 'mt-0.5' : 'mt-[18px]'"
          >
            {{ group.caption }}
          </div>
          <div class="flex flex-col gap-px">
            <NuxtLink
              v-for="item in group.items"
              :key="item.to"
              :to="item.to"
              class="nav-row flex items-center gap-2.5 px-2 py-1.5 rounded-lg outline-none focus-visible:ring-1 focus-visible:ring-primary/30 transition-colors duration-150 cursor-pointer"
              :class="isActive(item)
                ? 'bg-accent-tint text-text-primary'
                : 'text-text-secondary hover:bg-surface-nav-hover'"
            >
              <span class="material-symbols-outlined nav-icon" :class="isActive(item) ? 'text-primary' : ''">{{ item.icon }}</span>
              <span>{{ item.label }}</span>
            </NuxtLink>
          </div>
        </div>

        <div class="flex-1 min-h-6"></div>

        <NuxtLink
          to="/settings"
          class="nav-row flex items-center gap-2.5 px-2 py-1.5 rounded-lg outline-none focus-visible:ring-1 focus-visible:ring-primary/30 transition-colors duration-150 cursor-pointer"
          :class="isActive(SETTINGS_ITEM)
            ? 'bg-accent-tint text-text-primary'
            : 'text-text-secondary hover:bg-surface-nav-hover'"
        >
          <span class="material-symbols-outlined nav-icon" :class="isActive(SETTINGS_ITEM) ? 'text-primary' : ''">settings</span>
          <span>Settings</span>
        </NuxtLink>
      </nav>

      <!-- Main Content Area -->
      <main class="flex-1 flex flex-col min-w-0 relative bg-bg-base overflow-y-auto">
        <slot />
      </main>

      <!-- Right DA Panel overlay inside flex wrapper -->
      <DaChatSurface :isOpen="daOpen" @close="closeDa" />

      <!-- Reopen tab: pinned to right edge, only when DA is closed -->
      <button
        v-if="!daOpen"
        @click="openDa"
        class="absolute top-1/2 right-0 -translate-y-1/2 w-6 flex flex-col items-center justify-center gap-1.5 py-3 bg-surface border-y border-l border-border-hairline rounded-l text-text-muted hover:text-text-primary outline-none focus-visible:border-primary/70 focus-visible:ring-1 focus-visible:ring-primary/30 active:scale-x-95 origin-right transition-[color,transform,border-color] duration-150 cursor-pointer z-dropdown group"
        aria-label="Open DA (⌘.)"
      >
        <span class="material-symbols-outlined !text-[18px]">smart_toy</span>
        <div class="flex flex-col items-center gap-1">
          <span class="font-label-caps text-label-caps font-semibold [writing-mode:vertical-rl]">DA</span>
          <span class="font-mono-data text-mono-data font-medium [writing-mode:vertical-rl] text-text-faint group-hover:text-text-muted transition-colors">⌘.</span>
        </div>
      </button>
    </div>

    <!-- Shell footer: only state that is real, which is where the vault is and
         the time. The system/sync indicators it replaces reported neither. -->
    <footer class="h-7 shrink-0 border-t border-border-hairline flex items-center justify-between px-4 font-mono-data text-mono-data text-text-faint">
      <span>{{ vaultLabel }}</span>
      <span>{{ currentTime }}</span>
    </footer>
  </div>
</template>

<script setup>
import { ref, watch, onMounted, onUnmounted } from 'vue'

// Grouped destinations. Home is the only exact match: every path starts with
// "/", so a prefix test would light it up on every page.
const NAV_GROUPS = [
  {
    caption: 'Overview',
    items: [
      { to: '/', label: 'Home', icon: 'dashboard', exact: true },
      { to: '/inbox', label: 'Inbox', icon: 'inbox' },
    ],
  },
  {
    caption: 'Knowledge',
    items: [
      { to: '/notes', label: 'Notes', icon: 'description' },
      { to: '/bookmarks', label: 'Bookmarks', icon: 'bookmark' },
      { to: '/memory', label: 'Memory', icon: 'neurology' },
      { to: '/graph', label: 'Graph', icon: 'hub' },
    ],
  },
  {
    caption: 'Work',
    items: [
      { to: '/projects', label: 'Projects', icon: 'folder_open' },
      { to: '/tasks', label: 'Tasks', icon: 'checklist' },
    ],
  },
  {
    caption: 'Operations',
    items: [
      { to: '/orchestrator', label: 'Orchestrator', icon: 'account_tree' },
      { to: '/ingest', label: 'Ingest', icon: 'input' },
      { to: '/audit', label: 'Audit', icon: 'policy' },
    ],
  },
]

const SETTINGS_ITEM = { to: '/settings' }

const route = useRoute()
const daRelevantRoutes = new Set(['/chat'])
const daOpen = ref(daRelevantRoutes.has(route.path))
const currentTime = ref('--:--')
const userPreference = ref(false)
const vaultLabel = ref('~/vault')

// Nuxt's static build serves /tasks/ with a trailing slash, so the raw path is
// normalised before it is compared with the link target.
function isActive(item) {
  const path = route.path.replace(/\/$/, '') || '/'
  if (item.exact) return path === item.to
  return path === item.to || path.startsWith(`${item.to}/`)
}

// The collapsed icon rail is not built. The control ships anyway because the
// expanded default has to be reversible to be judged, and it says what it is
// rather than silently doing nothing.
function collapse() {
  window.alert('The collapsed icon rail is not built yet.')
}

const loadVaultLabel = async () => {
  try {
    const res = await fetch('/api/health')
    if (!res.ok) return
    const data = await res.json()
    const label = data.vaultLabel || data.vaultRoot
    if (label) vaultLabel.value = label
  } catch {
    // Leave the placeholder if the server is unreachable.
  }
}

const closeDa = () => {
  daOpen.value = false
  userPreference.value = false
  if (typeof localStorage !== 'undefined') {
    localStorage.setItem('kernl:da-open', '0')
  }
}

const openDa = () => {
  daOpen.value = true
  userPreference.value = true
  if (typeof localStorage !== 'undefined') {
    localStorage.setItem('kernl:da-open', '1')
  }
}

const toggleDa = () => {
  if (daOpen.value) closeDa()
  else openDa()
}

let timer;

const updateTime = () => {
  const now = new Date()
  const hours = String(now.getHours()).padStart(2, '0')
  const minutes = String(now.getMinutes()).padStart(2, '0')
  currentTime.value = `${hours}:${minutes}`
}

const handleKeydown = (e) => {
  if (e.key === 'Escape' && daOpen.value) {
    closeDa()
  }
  if (e.key === '.' && e.metaKey) {
    e.preventDefault()
    toggleDa()
  }
}

onMounted(() => {
  if (typeof localStorage !== 'undefined') {
    const saved = localStorage.getItem('kernl:da-open')
    if (saved !== null) {
      userPreference.value = saved === '1'
    }
  }
  if (daRelevantRoutes.has(route.path)) {
    daOpen.value = true
  } else {
    daOpen.value = userPreference.value
  }

  window.addEventListener('keydown', handleKeydown)
  updateTime()
  timer = setInterval(updateTime, 1000)
  loadVaultLabel()
})

watch(() => route.path, (path) => {
  if (daRelevantRoutes.has(path)) {
    daOpen.value = true
  } else {
    daOpen.value = userPreference.value
  }
})

onUnmounted(() => {
  window.removeEventListener('keydown', handleKeydown)
  clearInterval(timer)
})
</script>

<style scoped>
/* A lighter optical weight than the body set: at the default grade these read
   heavy next to a 13px label. `line-height: 1` keeps the glyph from setting the
   row height, which the label should own. */
.side-nav .material-symbols-outlined {
  font-size: 16px;
  line-height: 1;
  font-variation-settings: 'FILL' 0, 'wght' 260, 'GRAD' -25, 'opsz' 20;
}

.nav-logo-icon {
  font-size: 15px;
}

/* `normal` rather than the body's 20px: the extra leading turns a 29px row into
   a 32px one, and the sidebar's density is what buys it eleven visible
   destinations without scrolling. */
.nav-row {
  font-size: 13px;
  line-height: normal;
}
</style>

<style>
.custom-caret {
  caret-color: var(--color-primary);
}
</style>
