<template>
  <div class="border-t border-border-hairline p-component flex items-center gap-base">
    <UiInput
      v-model="text"
      placeholder="Type a message..."
      :disabled="disabled"
      classes="h-9 flex-1 rounded border border-border-hairline bg-bg-base px-component font-mono-data text-mono-data text-text-primary outline-none transition-colors placeholder:text-text-muted focus:border-primary/70 disabled:cursor-not-allowed disabled:opacity-50"
      @keydown.enter="emitSend"
    />
    <UiButton
      :disabled="disabled || !text.trim()"
      variant="primary"
      @click="emitSend"
    >
      Send
    </UiButton>
  </div>
</template>

<script setup lang="ts">
import UiButton from '~/components/ui/UiButton.vue'
import UiInput from '~/components/ui/UiInput.vue'

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
