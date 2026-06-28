<template>
  <div class="overflow-y-auto p-4 space-y-2" ref="scrollContainer">
    <div v-if="!messages.length && !error" class="p-component font-body text-body text-text-muted">
      Start a conversation.
    </div>
    <div
      v-for="(msg, idx) in messages"
      :key="idx"
      class="px-base py-1.5 rounded text-body leading-relaxed whitespace-pre-wrap break-words"
      :class="msg.role === 'user'
        ? 'bg-da-accent/10 text-primary self-end ml-8'
        : msg.role === 'error'
          ? 'bg-status-failed/10 text-status-failed-text'
          : 'bg-surface-container text-text-primary mr-8'"
    >
      {{ msg.content }}
    </div>
    <div v-if="error" class="px-base py-1.5 rounded text-body bg-status-failed/10 text-status-failed-text">
      {{ error }}
    </div>
    <div v-if="isStreaming" class="text-primary text-body animate-pulse px-base py-1">
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
