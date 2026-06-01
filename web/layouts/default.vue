<template>
  <div class="flex flex-col h-screen w-full bg-[#0F1217] text-[#D6DBE3] font-sans overflow-hidden">
    <div class="flex flex-1 overflow-hidden relative">
      <!-- Left Sidebar: Navigation -->
      <aside class="w-[60px] bg-[#0F1217] border-r border-[#1B2029] flex flex-col items-center py-4 shrink-0 z-10">
        <nav class="flex-1 flex flex-col items-center space-y-4 w-full">
          <!-- Home -->
          <NuxtLink to="/" class="flex flex-col items-center justify-center w-10 h-10 rounded hover:bg-[#1D222D] text-[#9098A7] hover:text-[#6B7BB0]" active-class="text-[#6B7BB0] border-l-2 border-[#6B7BB0] bg-[#181C26]">
            <span class="text-[10px]">Home</span>
          </NuxtLink>
          <NuxtLink to="/projects" class="flex flex-col items-center justify-center w-10 h-10 rounded hover:bg-[#1D222D] text-[#9098A7] hover:text-[#6B7BB0]" active-class="text-[#6B7BB0] border-l-2 border-[#6B7BB0] bg-[#181C26]">
            <span class="text-[10px]">Proj</span>
          </NuxtLink>
          <NuxtLink to="/notes" class="flex flex-col items-center justify-center w-10 h-10 rounded hover:bg-[#1D222D] text-[#9098A7] hover:text-[#6B7BB0]" active-class="text-[#6B7BB0] border-l-2 border-[#6B7BB0] bg-[#181C26]">
            <span class="text-[10px]">Note</span>
          </NuxtLink>
          <NuxtLink to="/tasks" class="flex flex-col items-center justify-center w-10 h-10 rounded hover:bg-[#1D222D] text-[#9098A7] hover:text-[#6B7BB0]" active-class="text-[#6B7BB0] border-l-2 border-[#6B7BB0] bg-[#181C26]">
            <span class="text-[10px]">Task</span>
          </NuxtLink>
          <NuxtLink to="/orchestrator" class="flex flex-col items-center justify-center w-10 h-10 rounded hover:bg-[#1D222D] text-[#9098A7] hover:text-[#6B7BB0]" active-class="text-[#6B7BB0] border-l-2 border-[#6B7BB0] bg-[#181C26]">
            <span class="text-[10px]">Orch</span>
          </NuxtLink>
          <NuxtLink to="/inbox" class="flex flex-col items-center justify-center w-10 h-10 rounded hover:bg-[#1D222D] text-[#9098A7] hover:text-[#6B7BB0]" active-class="text-[#6B7BB0] border-l-2 border-[#6B7BB0] bg-[#181C26]">
            <span class="text-[10px]">Inbx</span>
          </NuxtLink>
          <NuxtLink to="/bookmarks" class="flex flex-col items-center justify-center w-10 h-10 rounded hover:bg-[#1D222D] text-[#9098A7] hover:text-[#6B7BB0]" active-class="text-[#6B7BB0] border-l-2 border-[#6B7BB0] bg-[#181C26]">
            <span class="text-[10px]">Mark</span>
          </NuxtLink>
          <NuxtLink to="/chat" class="flex flex-col items-center justify-center w-10 h-10 rounded hover:bg-[#1D222D] text-[#9098A7] hover:text-[#6B7BB0]" active-class="text-[#6B7BB0] border-l-2 border-[#6B7BB0] bg-[#181C26]">
            <span class="text-[10px]">Chat</span>
          </NuxtLink>
          <NuxtLink to="/graph" class="flex flex-col items-center justify-center w-10 h-10 rounded hover:bg-[#1D222D] text-[#9098A7] hover:text-[#6B7BB0]" active-class="text-[#6B7BB0] border-l-2 border-[#6B7BB0] bg-[#181C26]">
            <span class="text-[10px]">Grph</span>
          </NuxtLink>
          <NuxtLink to="/dashboard" class="flex flex-col items-center justify-center w-10 h-10 rounded hover:bg-[#1D222D] text-[#9098A7] hover:text-[#6B7BB0]" active-class="text-[#6B7BB0] border-l-2 border-[#6B7BB0] bg-[#181C26]">
            <span class="text-[10px]">Dash</span>
          </NuxtLink>
        </nav>
        <button @click="toggleDa" class="mt-auto flex flex-col items-center justify-center w-10 h-10 rounded hover:bg-[#1D222D] text-[#9098A7]">
          <span class="text-[10px]">DA</span>
        </button>
      </aside>

      <!-- Center: Main Content -->
      <main class="flex-1 flex flex-col relative overflow-hidden bg-[#0F1217]">
        <slot />
      </main>

      <!-- Right Sidebar: DA Chat (Overlay) -->
      <DaChatSurface :isOpen="daOpen" @close="daOpen = false" />
    </div>

    <!-- Bottom Status Bar -->
    <footer class="h-[26px] border-t border-[#1B2029] bg-[#0F1217] flex items-center px-4 text-[#666D7C] text-xs font-mono shrink-0 gap-6">
      <div class="flex items-center gap-2">
        <span class="w-1.5 h-1.5 rounded-full bg-[#8089A0]"></span>
        <span class="text-[11px] font-sans">Ready</span>
      </div>
      <div>kernl-ws/main</div>
      <div>0 active</div>
      <div class="ml-auto">12ms</div>
    </footer>
  </div>
</template>

<script setup>
import { ref, onMounted, onUnmounted } from 'vue'

const daOpen = ref(false)

const toggleDa = () => {
  daOpen.value = !daOpen.value
}

const handleKeydown = (e) => {
  if (e.key === 'Escape' && daOpen.value) {
    daOpen.value = false
  }
  if (e.key === '.' && e.metaKey) {
    toggleDa()
  }
}

onMounted(() => {
  window.addEventListener('keydown', handleKeydown)
})

onUnmounted(() => {
  window.removeEventListener('keydown', handleKeydown)
})
</script>
