<template>
  <aside v-if="isOpen" class="absolute right-0 top-0 bottom-0 w-80 bg-[#181C26] border-l border-[#1B2029] flex flex-col shrink-0 text-[#D6DBE3] z-20 shadow-2xl transition-transform">
    <div class="p-4 border-b border-[#1B2029] font-medium flex items-center justify-between text-[13px]">
      <span class="text-[#6B7BB0]">DA Chat</span>
      <button @click="$emit('close')" class="text-[#666D7C] hover:text-[#D6DBE3]">ESC</button>
    </div>
    <div class="flex-1 overflow-y-auto flex flex-col relative p-4 space-y-4">
      <div v-for="(msg, idx) in messages" :key="idx" class="text-[13px]">
        <div :class="msg.role === 'assistant' ? 'text-[#9098A7]' : 'text-[#D6DBE3]'">
          {{ msg.content }}
        </div>
      </div>
      <div v-if="isStreaming" class="text-[#6B7BB0] text-[13px] animate-pulse">_</div>
      <div v-if="error" class="text-[#C2675C] text-[13px]">{{ error }}</div>
    </div>
    <div class="p-4 border-t border-[#1B2029]">
      <input 
        v-model="daInput" 
        @keyup.enter="handleSend"
        :disabled="isStreaming"
        class="w-full bg-[#0F1217] border border-[#242935] focus:border-[#6B7BB0] rounded px-3 py-2 text-[13px] outline-none text-[#D6DBE3]"
        placeholder="Ask DA..."
      />
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
