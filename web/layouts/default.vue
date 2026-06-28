<template>
  <div class="bg-bg-base text-text-primary h-screen flex flex-col overflow-hidden font-body selection:bg-da-accent selection:text-white">
    <!-- Top Layout Wrapper -->
    <div class="flex flex-1 overflow-hidden">
      <!-- Left SideNav (Icon Rail) -->
      <nav class="w-rail-width h-full bg-surface border-r border-border-hairline flex flex-col items-center py-base z-dropdown flex-shrink-0">
        <!-- Logo -->
        <NuxtLink to="/" class="mb-break flex flex-col items-center gap-1 cursor-pointer outline-none focus-visible:ring-1 focus-visible:ring-primary/30 rounded p-1 group">
          <div class="w-8 h-8 rounded-lg bg-surface-container flex items-center justify-center group-active:scale-95 transition-transform">
            <span class="material-symbols-outlined text-primary font-bold">terminal</span>
          </div>
          <span class="font-label-caps text-label-caps text-primary">Kernl</span>
        </NuxtLink>
        
        <!-- Nav Items (only built, reachable surfaces; ordered by the magic loop) -->
        <div class="flex flex-col gap-1 flex-grow w-full items-center px-1">
          <NuxtLink to="/" class="relative w-full h-[48px] rounded flex flex-col items-center justify-center text-text-muted hover:text-text-primary outline-none focus-visible:ring-1 focus-visible:ring-primary/30 transition-colors duration-150 cursor-pointer" active-class="text-primary bg-surface-hover">
            <span class="material-symbols-outlined font-bold text-[20px]">dashboard</span>
            <span class="font-label-caps text-label-caps mt-0.5 leading-none">Home</span>
          </NuxtLink>
          <NuxtLink to="/inbox" class="relative w-full h-[48px] rounded flex flex-col items-center justify-center text-text-muted hover:text-text-primary outline-none focus-visible:ring-1 focus-visible:ring-primary/30 transition-colors duration-150 cursor-pointer" active-class="text-primary bg-surface-hover">
            <span class="material-symbols-outlined text-[20px]">inbox</span>
            <span class="font-label-caps text-label-caps mt-0.5 leading-none">Inbox</span>
          </NuxtLink>
          <NuxtLink to="/notes" class="relative w-full h-[48px] rounded flex flex-col items-center justify-center text-text-muted hover:text-text-primary outline-none focus-visible:ring-1 focus-visible:ring-primary/30 transition-colors duration-150 cursor-pointer" active-class="text-primary bg-surface-hover">
            <span class="material-symbols-outlined text-[20px]">description</span>
            <span class="font-label-caps text-label-caps mt-0.5 leading-none">Notes</span>
          </NuxtLink>
          <NuxtLink to="/bookmarks" class="relative w-full h-[48px] rounded flex flex-col items-center justify-center text-text-muted hover:text-text-primary outline-none focus-visible:ring-1 focus-visible:ring-primary/30 transition-colors duration-150 cursor-pointer" active-class="text-primary bg-surface-hover">
            <span class="material-symbols-outlined text-[20px]">bookmark</span>
            <span class="font-label-caps text-label-caps mt-0.5 leading-none">Marks</span>
          </NuxtLink>
          <NuxtLink to="/memory" class="relative w-full h-[48px] rounded flex flex-col items-center justify-center text-text-muted hover:text-text-primary outline-none focus-visible:ring-1 focus-visible:ring-primary/30 transition-colors duration-150 cursor-pointer" active-class="text-primary bg-surface-hover">
            <span class="material-symbols-outlined text-[20px]">neurology</span>
            <span class="font-label-caps text-label-caps mt-0.5 leading-none">Memory</span>
          </NuxtLink>
          <NuxtLink to="/projects" class="relative w-full h-[48px] rounded flex flex-col items-center justify-center text-text-muted hover:text-text-primary outline-none focus-visible:ring-1 focus-visible:ring-primary/30 transition-colors duration-150 cursor-pointer" active-class="text-primary bg-surface-hover">
            <span class="material-symbols-outlined text-[20px]">folder_open</span>
            <span class="font-label-caps text-label-caps mt-0.5 leading-none">Projects</span>
          </NuxtLink>
          <NuxtLink to="/tasks" class="relative w-full h-[48px] rounded flex flex-col items-center justify-center text-text-muted hover:text-text-primary outline-none focus-visible:ring-1 focus-visible:ring-primary/30 transition-colors duration-150 cursor-pointer" active-class="text-primary bg-surface-hover">
            <span class="material-symbols-outlined text-[20px]">checklist</span>
            <span class="font-label-caps text-label-caps mt-0.5 leading-none">Tasks</span>
          </NuxtLink>
          <NuxtLink to="/graph" class="relative w-full h-[48px] rounded flex flex-col items-center justify-center text-text-muted hover:text-text-primary outline-none focus-visible:ring-1 focus-visible:ring-primary/30 transition-colors duration-150 cursor-pointer" active-class="text-primary bg-surface-hover">
            <span class="material-symbols-outlined text-[20px]">hub</span>
            <span class="font-label-caps text-label-caps mt-0.5 leading-none">Graph</span>
          </NuxtLink>
          <NuxtLink to="/orchestrator" class="relative w-full h-[48px] rounded flex flex-col items-center justify-center text-text-muted hover:text-text-primary outline-none focus-visible:ring-1 focus-visible:ring-primary/30 transition-colors duration-150 cursor-pointer" active-class="text-primary bg-surface-hover">
            <span class="material-symbols-outlined text-[20px]">account_tree</span>
            <span class="font-label-caps text-label-caps mt-0.5 leading-none">Orch</span>
          </NuxtLink>
          <NuxtLink to="/ingest" class="relative w-full h-[48px] rounded flex flex-col items-center justify-center text-text-muted hover:text-text-primary outline-none focus-visible:ring-1 focus-visible:ring-primary/30 transition-colors duration-150 cursor-pointer" active-class="text-primary bg-surface-hover">
            <span class="material-symbols-outlined text-[20px]">input</span>
            <span class="font-label-caps text-label-caps mt-0.5 leading-none">Ingest</span>
          </NuxtLink>
          <NuxtLink to="/audit" class="relative w-full h-[48px] rounded flex flex-col items-center justify-center text-text-muted hover:text-text-primary outline-none focus-visible:ring-1 focus-visible:ring-primary/30 transition-colors duration-150 cursor-pointer" active-class="text-primary bg-surface-hover">
            <span class="material-symbols-outlined text-[20px]">policy</span>
            <span class="font-label-caps text-label-caps mt-0.5 leading-none">Audit</span>
          </NuxtLink>
        </div>
        
        <!-- Footer Nav -->
        <div class="flex flex-col gap-component pb-base w-full items-center">
          <button class="w-full h-11 flex items-center justify-center text-text-muted hover:text-text-primary outline-none focus-visible:ring-1 focus-visible:ring-primary/30 active:scale-95 transition-[color,transform] duration-150 cursor-pointer" aria-label="Settings">
            <span class="material-symbols-outlined">settings</span>
          </button>
          <button class="w-full h-11 flex items-center justify-center text-text-muted hover:text-text-primary outline-none focus-visible:ring-1 focus-visible:ring-primary/30 active:scale-95 transition-[color,transform] duration-150 cursor-pointer" aria-label="Account">
            <span class="material-symbols-outlined">account_circle</span>
          </button>
        </div>
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

    <!-- Shell: Footer Status Bar -->
    <footer class="h-[26px] bg-surface-container-low text-text-muted border-t border-border-hairline flex items-center justify-between px-base z-dropdown divide-x divide-border-hairline shrink-0">
      <div class="flex items-center gap-component pr-component">
        <div class="flex items-center gap-tight">
          <span class="w-2 h-2 rounded-full bg-status-passed"></span>
          <span class="font-mono-data text-mono-data text-status-passed uppercase">system_online</span>
        </div>
        <div class="h-3 w-px bg-border-hairline mx-tight"></div>
        <span class="font-mono-data text-mono-data">{{ vaultLabel }}</span>
      </div>

      <div class="flex items-center gap-component pl-component">
        <div class="flex items-center gap-tight">
          <span class="material-symbols-outlined text-[14px]">sync</span>
          <span class="font-mono-data text-mono-data">synced</span>
        </div>
        <div class="h-3 w-px bg-border-hairline mx-tight"></div>
        <span class="font-mono-data text-mono-data">{{ currentTime }}</span>
      </div>
    </footer>
  </div>
</template>

<script setup>
import { ref, watch, onMounted, onUnmounted } from 'vue'

const route = useRoute()
const daRelevantRoutes = new Set(['/chat', '/config/da'])
const daOpen = ref(daRelevantRoutes.has(route.path))
const currentTime = ref(new Date().toISOString().slice(0, 19).replace('T', ' '))
const userPreference = ref(false)
const vaultLabel = ref('~/vault')

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
  const year = now.getFullYear()
  const month = String(now.getMonth() + 1).padStart(2, '0')
  const day = String(now.getDate()).padStart(2, '0')
  const hours = String(now.getHours()).padStart(2, '0')
  const minutes = String(now.getMinutes()).padStart(2, '0')
  const seconds = String(now.getSeconds()).padStart(2, '0')
  currentTime.value = `${year}-${month}-${day} ${hours}:${minutes}:${seconds}`
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
  
  if (!daRelevantRoutes.has(route.path)) {
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

<style>
.custom-caret {
  caret-color: var(--color-primary);
}
</style>
