<template>
  <div class="overflow-y-auto p-4 space-y-2" ref="scrollContainer">
    <div v-if="!messages.length && !error" class="text-[#475569] italic p-4">
      Start a conversation.
    </div>
    <div
      v-for="(msg, idx) in messages"
      :key="idx"
      class="px-3 py-1.5 rounded text-sm leading-relaxed whitespace-pre-wrap break-words"
      :class="msg.role === 'user'
        ? 'bg-[#1d4ed8]/20 text-[#93c5fd] self-end ml-8'
        : msg.role === 'error'
          ? 'bg-[#7f1d1d]/30 text-[#fca5a5]'
          : 'bg-[#334155]/50 text-[#e2e8f0] mr-8'"
    >
      {{ msg.content }}
    </div>
    <div v-if="error" class="px-3 py-1.5 rounded text-sm bg-[#7f1d1d]/30 text-[#fca5a5]">
      {{ error }}
    </div>
    <div v-if="isStreaming" class="text-[#38bdf8] text-sm animate-pulse px-3 py-1">
      Thinking...
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, nextTick } from 'vue';

const props = defineProps<{
  messages: { role: string; content: string }[];
  error: string | null;
  isStreaming: boolean;
}>();

const scrollContainer = ref<HTMLElement | null>(null);

watch(
  () => [props.messages.length, props.error],
  () => {
    nextTick(() => {
      if (scrollContainer.value) {
        scrollContainer.value.scrollTop = scrollContainer.value.scrollHeight;
      }
    });
  }
);
</script>
