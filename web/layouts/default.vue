<template>
  <div class="bg-bg-base text-text-primary h-screen flex flex-col overflow-hidden font-body selection:bg-da-accent selection:text-white">
    <!-- Top Layout Wrapper -->
    <div class="flex flex-1 overflow-hidden">
      <!-- Left SideNav (Icon Rail) -->
      <nav class="w-rail-width h-full bg-surface border-r border-border-hairline flex flex-col items-center py-base z-50 flex-shrink-0">
        <!-- Logo -->
        <div class="mb-break flex flex-col items-center gap-1 cursor-pointer">
          <div class="w-8 h-8 rounded-lg bg-surface-container flex items-center justify-center">
            <span class="material-symbols-outlined text-primary font-bold">terminal</span>
          </div>
          <span class="font-display text-[10px] tracking-widest text-primary uppercase">Kernl</span>
        </div>
        
        <!-- Nav Items (only built, reachable surfaces; ordered by the magic loop) -->
        <div class="flex flex-col gap-component flex-grow w-full items-center">
          <NuxtLink to="/" class="relative w-full h-10 flex items-center justify-center text-text-muted hover:text-text-primary transition-colors duration-150 cursor-pointer" active-class="border-l-2 border-primary text-primary bg-surface-hover" title="Home">
            <span class="material-symbols-outlined font-bold">dashboard</span>
          </NuxtLink>
          <NuxtLink to="/inbox" class="relative w-full h-10 flex items-center justify-center text-text-muted hover:text-text-primary transition-colors duration-150 cursor-pointer" active-class="border-l-2 border-primary text-primary bg-surface-hover" title="Inbox">
            <span class="material-symbols-outlined">inbox</span>
          </NuxtLink>
          <NuxtLink to="/notes" class="relative w-full h-10 flex items-center justify-center text-text-muted hover:text-text-primary transition-colors duration-150 cursor-pointer" active-class="border-l-2 border-primary text-primary bg-surface-hover" title="Notes">
            <span class="material-symbols-outlined">description</span>
          </NuxtLink>
          <NuxtLink to="/bookmarks" class="relative w-full h-10 flex items-center justify-center text-text-muted hover:text-text-primary transition-colors duration-150 cursor-pointer" active-class="border-l-2 border-primary text-primary bg-surface-hover" title="Bookmarks">
            <span class="material-symbols-outlined">bookmark</span>
          </NuxtLink>
          <NuxtLink to="/memory" class="relative w-full h-10 flex items-center justify-center text-text-muted hover:text-text-primary transition-colors duration-150 cursor-pointer" active-class="border-l-2 border-primary text-primary bg-surface-hover" title="Memory">
            <span class="material-symbols-outlined">neurology</span>
          </NuxtLink>
          <NuxtLink to="/projects" class="relative w-full h-10 flex items-center justify-center text-text-muted hover:text-text-primary transition-colors duration-150 cursor-pointer" active-class="border-l-2 border-primary text-primary bg-surface-hover" title="Projects">
            <span class="material-symbols-outlined">folder_open</span>
          </NuxtLink>
          <NuxtLink to="/tasks" class="relative w-full h-10 flex items-center justify-center text-text-muted hover:text-text-primary transition-colors duration-150 cursor-pointer" active-class="border-l-2 border-primary text-primary bg-surface-hover" title="Tasks">
            <span class="material-symbols-outlined">checklist</span>
          </NuxtLink>
          <NuxtLink to="/orchestrator" class="relative w-full h-10 flex items-center justify-center text-text-muted hover:text-text-primary transition-colors duration-150 cursor-pointer" active-class="border-l-2 border-primary text-primary bg-surface-hover" title="Orchestrator">
            <span class="material-symbols-outlined">account_tree</span>
          </NuxtLink>
          <NuxtLink to="/ingest" class="relative w-full h-10 flex items-center justify-center text-text-muted hover:text-text-primary transition-colors duration-150 cursor-pointer" active-class="border-l-2 border-primary text-primary bg-surface-hover" title="Ingest">
            <span class="material-symbols-outlined">input</span>
          </NuxtLink>
          <NuxtLink to="/audit" class="relative w-full h-10 flex items-center justify-center text-text-muted hover:text-text-primary transition-colors duration-150 cursor-pointer" active-class="border-l-2 border-primary text-primary bg-surface-hover" title="Audit">
            <span class="material-symbols-outlined">policy</span>
          </NuxtLink>
        </div>
        
        <!-- Footer Nav -->
        <div class="flex flex-col gap-component pb-base w-full items-center">
          <button class="w-full h-10 flex items-center justify-center text-text-muted hover:text-text-primary transition-colors duration-150 cursor-pointer">
            <span class="material-symbols-outlined">settings</span>
          </button>
          <button class="w-full h-10 flex items-center justify-center text-text-muted hover:text-text-primary transition-colors duration-150 cursor-pointer">
            <span class="material-symbols-outlined">account_circle</span>
          </button>
        </div>
      </nav>

      <!-- Main Content Area -->
      <main class="flex-1 flex flex-col min-w-0 relative bg-bg-base overflow-y-auto">
        <slot />
      </main>

      <!-- Right DA Panel overlay inside flex wrapper -->
      <DaChatSurface :isOpen="daOpen" @close="daOpen = false" />

      <!-- Reopen tab: pinned to right edge, only when DA is closed -->
      <button
        v-if="!daOpen"
        @click="daOpen = true"
        title="Open DA (⌘.)"
        class="absolute top-1/2 right-0 -translate-y-1/2 w-6 flex flex-col items-center justify-center gap-1.5 py-3 bg-surface border-y border-l border-border-hairline rounded-l text-text-muted hover:text-text-primary transition-colors duration-150 cursor-pointer z-40"
      >
        <span class="material-symbols-outlined !text-[18px]">smart_toy</span>
        <span class="font-label-caps text-[10px] tracking-widest [writing-mode:vertical-rl]">DA</span>
      </button>
    </div>

    <!-- Shell: Footer Status Bar -->
    <footer class="h-[26px] bg-surface-container-low text-text-dim border-t border-border-hairline flex items-center justify-between px-base z-50 divide-x divide-border-hairline shrink-0">
      <div class="flex items-center gap-component pr-component">
        <div class="flex items-center gap-tight">
          <span class="w-2 h-2 rounded-full bg-status-passed shadow-[0_0_8px_rgba(109,154,120,0.4)]"></span>
          <span class="font-mono-data text-mono-data text-status-passed uppercase">connected</span>
        </div>
        <div class="h-3 w-px bg-border-hairline mx-tight"></div>
        <span class="font-mono-data text-mono-data">~/vault</span>
      </div>
      
      <div class="flex-grow flex items-center justify-center font-mono-data text-mono-data gap-section">
        <span class="hover:bg-surface-hover px-2 transition-colors cursor-default">UTF-8</span>
        <span class="hover:bg-surface-hover px-2 transition-colors cursor-default">system_ready</span>
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
import { ref, onMounted, onUnmounted } from 'vue'

const daOpen = ref(true)
const currentTime = ref(new Date().toISOString().slice(0, 19).replace('T', ' '))

const toggleDa = () => {
  daOpen.value = !daOpen.value
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
    daOpen.value = false
  }
  if (e.key === '.' && e.metaKey) {
    e.preventDefault()
    toggleDa()
  }
}

onMounted(() => {
  window.addEventListener('keydown', handleKeydown)
  updateTime()
  timer = setInterval(updateTime, 1000)
})

onUnmounted(() => {
  window.removeEventListener('keydown', handleKeydown)
  clearInterval(timer)
})
</script>

<style>
::-webkit-scrollbar {
  width: 4px;
  height: 4px;
}
::-webkit-scrollbar-track {
  background: transparent;
}
::-webkit-scrollbar-thumb {
  background: var(--color-border-hairline);
  border-radius: 2px;
}
::-webkit-scrollbar-thumb:hover {
  background: var(--color-border-default);
}
.custom-caret {
  caret-color: var(--color-primary);
}
</style>
