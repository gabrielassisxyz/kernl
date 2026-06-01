<template>
  <aside 
    class="bg-surface h-full border-l border-border-hairline flex flex-col z-40 transition-all duration-300 ease-in-out overflow-hidden" 
    :class="isOpen ? 'w-[400px] opacity-100' : 'w-0 opacity-0 border-l-0'" 
  >
    <!-- DA Header -->
    <div class="h-12 border-b border-border-hairline flex items-center justify-between px-base flex-shrink-0">
      <div class="flex items-center gap-2">
        <span class="font-headline text-text-primary font-semibold">DA</span>
        <span class="font-mono-data text-text-faint text-[10px] tracking-wider uppercase bg-surface-container px-1.5 py-0.5 border border-border-hairline">scope · global</span>
      </div>
      <button @click="$emit('close')" class="h-8 w-8 flex items-center justify-center text-text-faint hover:text-text-primary transition-colors">
        <span class="material-symbols-outlined">close</span>
      </button>
    </div>

    <!-- Chat History -->
    <div class="flex-1 overflow-y-auto p-section flex flex-col gap-section">
      <p class="font-body text-text-muted mb-4">Ask me about anything in your graph.</p>
      
      <!-- System/existing messages from composable -->
      <div class="flex flex-col gap-section w-full">
        <div v-for="(msg, idx) in messages" :key="idx" class="flex flex-col gap-base">
          <!-- Assistant Message -->
          <template v-if="msg.role === 'assistant'">
            <div class="flex flex-col gap-base">
              <div class="flex items-center gap-2">
                <span class="font-label-caps text-label-caps text-primary uppercase opacity-60">Kernl DA</span>
                <span class="px-1.5 py-0.5 rounded-sm bg-surface-container-high border border-border-hairline text-[10px] font-mono-data text-text-faint">v2.4-stable</span>
              </div>
              <div class="font-body text-[13.5px] leading-relaxed text-text-primary space-y-4">
                <p>{{ msg.content }}</p>
              </div>
            </div>
          </template>
          
          <!-- User Message -->
          <template v-else>
            <div class="flex flex-col gap-base pl-base border-l-2 border-da-accent">
              <span class="font-label-caps text-label-caps text-da-accent uppercase opacity-60">You</span>
              <p class="font-body text-[13.5px] leading-relaxed text-text-primary">{{ msg.content }}</p>
            </div>
          </template>
        </div>
      </div>
      
      <!-- Streaming indicator -->
      <div v-if="isStreaming" class="flex flex-col gap-base">
        <div class="flex items-center gap-2">
          <span class="font-label-caps text-label-caps text-primary uppercase opacity-60">Kernl DA</span>
        </div>
        <div class="font-body text-[13.5px] leading-relaxed text-text-primary">
          <span class="inline-block w-[2px] h-[1.1em] bg-[#6B7BB0] align-middle ml-[2px] animate-blink"></span>
        </div>
      </div>

      <div v-if="error" class="bg-surface-container-low border border-border-hairline p-component rounded-lg mt-4">
        <p class="font-mono-data text-mono-data text-status-failed mb-tight">ERROR</p>
        <p class="font-body text-body text-status-failed">{{ error }}</p>
      </div>
    </div>
    
    <!-- DA Input -->
    <div class="p-base border-t border-border-hairline bg-background">
      <div class="relative flex items-end bg-[#181C26] border border-border-hairline rounded-lg focus-within:border-primary transition-all p-2 gap-2">
        <textarea 
          v-model="daInput" 
          @keydown.enter.prevent="handleSend"
          :disabled="isStreaming"
          class="w-full bg-transparent border-none focus:ring-0 text-body text-text-primary resize-none placeholder:text-text-faint custom-caret outline-none" 
          placeholder="ask, instruct, or paste a directive…" 
          rows="3"
        ></textarea>
        <button @click="handleSend" class="h-8 w-8 bg-primary text-on-primary rounded flex items-center justify-center hover:opacity-90 transition-opacity">
          <span class="material-symbols-outlined !text-[18px]">arrow_upward</span>
        </button>
      </div>
      <div class="mt-2 flex justify-between items-center px-1">
        <span class="font-mono-data text-[10px] text-text-faint">↵ to send</span>
        <span class="font-mono-data text-[10px] text-text-faint">Context: All Files</span>
      </div>
    </div>
  </aside>
</template>

<script setup>
import { ref } from 'vue'
import { useChatSession } from '~/composables/useChatSession'

defineProps({
  isOpen: Boolean
})
defineEmits(['close'])

const daInput = ref('')
const { messages, error, isStreaming, sendMessage } = useChatSession()

const handleSend = () => {
  if (daInput.value.trim() && !isStreaming.value) {
    sendMessage(daInput.value)
    daInput.value = ''
  }
}
</script>

<style scoped>
@keyframes blink {
  0%, 100% { opacity: 1; }
  50% { opacity: 0; }
}
.animate-blink {
  animation: blink 1s step-end infinite;
}
</style>
