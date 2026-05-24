<template>
  <div class="border-t border-[#334155] p-3 flex items-center gap-2">
    <input
      v-model="text"
      type="text"
      placeholder="Type a message..."
      :disabled="disabled"
      class="flex-1 bg-[#0f172a] border border-[#334155] text-[#e2e8f0] p-2 rounded font-mono text-sm focus:outline-none focus:border-[#3b82f6] disabled:opacity-50"
      @keydown.enter="emitSend"
    />
    <button
      :disabled="disabled || !text.trim()"
      class="bg-[#1d4ed8] text-white px-4 py-2 rounded font-mono text-sm hover:bg-[#2563eb] transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
      @click="emitSend"
    >
      Send
    </button>
  </div>
</template>

<script setup lang="ts">
defineProps<{
  disabled: boolean;
}>();

const emit = defineEmits<{
  send: [content: string];
}>();

const text = ref('');

function emitSend() {
  const trimmed = text.value.trim();
  if (!trimmed) return;
  emit('send', trimmed);
  text.value = '';
}
</script>
