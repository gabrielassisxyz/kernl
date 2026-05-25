<template>
  <div class="flex h-screen w-full bg-base-100 text-base-content font-sans overflow-hidden">
    <!-- Left Sidebar: Navigation -->
    <aside class="w-64 bg-base-200 border-r border-base-300 flex flex-col shrink-0">
      <div class="p-4 text-xl font-bold border-b border-base-300">Kernl</div>
      <nav class="flex-1 p-4 space-y-2">
        <NuxtLink to="/" class="btn btn-ghost w-full justify-start gap-3">
          <svg xmlns="http://www.w3.org/2000/svg" class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10" /></svg>
          Vault
        </NuxtLink>
        <NuxtLink to="/inbox" class="btn btn-ghost w-full justify-start gap-3">
          <svg xmlns="http://www.w3.org/2000/svg" class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M20 13V6a2 2 0 00-2-2H6a2 2 0 00-2 2v7m16 0v5a2 2 0 01-2 2H6a2 2 0 01-2-2v-5m16 0h-2.586a1 1 0 00-.707.293l-2.414 2.414a1 1 0 01-.707.293h-3.172a1 1 0 01-.707-.293l-2.414-2.414A1 1 0 006.586 13H4" /></svg>
          Inbox
        </NuxtLink>
        <NuxtLink to="/projects" class="btn btn-ghost w-full justify-start gap-3">
          <svg xmlns="http://www.w3.org/2000/svg" class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2m-3 7h3m-3 4h3m-6-4h.01M9 16h.01" /></svg>
          Projects
        </NuxtLink>
      </nav>
    </aside>

    <!-- Center: Main Content -->
    <main class="flex-1 flex flex-col relative overflow-hidden bg-base-100">
      <slot />
    </main>

    <!-- Right Sidebar: DA Chat -->
    <aside class="w-80 bg-[#0f172a] border-l border-[#334155] flex flex-col shrink-0 text-[#e2e8f0]">
      <div class="p-4 border-b border-[#334155] font-bold flex items-center gap-2">
        <svg xmlns="http://www.w3.org/2000/svg" class="h-5 w-5 text-[#3b82f6]" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 10h.01M12 10h.01M16 10h.01M9 16H5a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v8a2 2 0 01-2 2h-5l-5 5v-5z" /></svg>
        DA Chat
      </div>
      <div class="flex-1 overflow-y-auto flex flex-col relative">
        <ChatMessageList :messages="messages" :error="error" :isStreaming="isStreaming" class="absolute inset-0" />
      </div>
      <ChatInput :disabled="isStreaming" @send="sendMessage" />
    </aside>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'

const { currentFile, currentContent } = useVaultContext()
const sessionId = ref('')
const messages = ref([])
const error = ref(null)
const isStreaming = ref(false)

const createSession = async () => {
  const res = await fetch('/api/chat/sessions', { method: 'POST' })
  if (res.ok) {
    const data = await res.json()
    sessionId.value = data.id
    connectSSE(data.id)
  }
}

const connectSSE = (id) => {
  const source = new EventSource(`/api/chat/sessions/${id}/events`)
  source.onmessage = (e) => {
    try {
      const data = JSON.parse(e.data)
      if (data.event === 'delta' || data.event === 'message') {
        const lastMsg = messages.value[messages.value.length - 1]
        if (lastMsg && lastMsg.role === 'assistant') {
          lastMsg.content += data.content || ''
        } else {
          messages.value.push({ role: 'assistant', content: data.content || '' })
        }
      } else if (data.event === 'error') {
        error.value = data.message || 'Stream error'
        isStreaming.value = false
      }
    } catch(err) {
      //
    }
  }
  source.onerror = () => {
    isStreaming.value = false
  }
}

const sendMessage = async (text) => {
  if (!sessionId.value) {
    await createSession()
  }
  
  // Inject context into the prompt
  let fullPrompt = text
  if (currentFile.value) {
    fullPrompt = `[Context: Viewing ${currentFile.value}]\n\nFile Content:\n\`\`\`\n${currentContent.value}\n\`\`\`\n\nUser: ${text}`
  }

  messages.value.push({ role: 'user', content: text })
  isStreaming.value = true
  error.value = null

  const res = await fetch(`/api/chat/sessions/${sessionId.value}/messages`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ content: fullPrompt })
  })
  
  if (!res.ok) {
    error.value = 'Failed to send message'
    isStreaming.value = false
  }
}

onMounted(() => {
  if (!sessionId.value) {
    createSession()
  }
})
</script>
