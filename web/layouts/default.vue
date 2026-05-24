<template>
  <div class="min-h-screen bg-[#0f172a] text-[#e2e8f0] font-mono">
    <header class="flex items-center gap-4 p-6 pb-0 flex-wrap">
      <h1 class="text-2xl text-[#38bdf8] font-bold">Kernl</h1>
      <nav class="flex gap-3 text-sm text-[#94a3b8]">
        <NuxtLink to="/chat" class="hover:text-[#38bdf8] transition-colors">Chat</NuxtLink>
        <NuxtLink to="/chat/config/da" class="hover:text-[#38bdf8] transition-colors">Persona</NuxtLink>
      </nav>
    </header>
    <main class="p-6 pt-4">
      <slot />
    </main>
  </div>
</template>

<script setup lang="ts">
</script>
