<template>
  <div class="w-full mb-section shrink-0 z-10">
    <div class="bg-surface-overlay border border-border-default focus-within:border-primary/70 transition-colors p-component flex flex-col gap-2 relative rounded">
      <div class="flex items-start gap-3">
        <span class="text-text-faint font-mono-data text-mono-data pt-[2px]">&gt;</span>
        <textarea
          ref="inputEl"
          v-model="text"
          @keydown="onKeydown"
          rows="1"
          placeholder="Capture thought..."
          class="w-full bg-transparent border-none text-text-primary font-mono-data text-mono-data focus:ring-0 resize-none h-[40px] p-0 placeholder:text-text-faint leading-relaxed outline-none"
        ></textarea>
        <span v-show="!text" class="blinking-cursor absolute top-[18px] left-[32px] pointer-events-none font-mono-data text-mono-data h-[14px]">_</span>
      </div>
      <div class="flex justify-between items-center mt-1">
        <div class="flex gap-4 text-text-faint font-mono-data text-mono-data tracking-wide">
          <span>[ESC] cancel</span>
          <span>[SHIFT+ENTER] new line</span>
          <span>[ENTER] save</span>
        </div>
        <span
          class="flex items-center gap-1 text-text-faint font-mono-data text-mono-data transition-opacity duration-300"
          :class="captured ? 'opacity-100' : 'opacity-0'"
        >captured</span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'

const emit = defineEmits<{ submit: [body: string]; cancel: [] }>()

const text = ref('')
const captured = ref(false)
const inputEl = ref<HTMLTextAreaElement | null>(null)
let capturedTimer: ReturnType<typeof setTimeout>

const onKeydown = (e: KeyboardEvent) => {
  if (e.key === 'Escape') {
    e.preventDefault()
    text.value = ''
    inputEl.value?.blur()
    emit('cancel')
  } else if (e.key === 'Enter' && !e.shiftKey) {
    // Plain Enter saves; Shift+Enter falls through to insert a newline.
    e.preventDefault()
    save()
  }
}

const save = async () => {
  const body = text.value.trim()
  if (!body) return
  text.value = ''
  try {
    await $fetch('/api/inbox', { method: 'POST', body: { body } })
    captured.value = true
    clearTimeout(capturedTimer)
    capturedTimer = setTimeout(() => { captured.value = false }, 1500)
    emit('submit', body)
  } catch {
    // swallow; keep the input quiet on failure
  }
}
</script>

<style scoped>
.blinking-cursor {
  font-weight: bold;
  font-size: 1.2em;
  color: var(--color-text-muted);
  animation: 1s blink step-end infinite;
}

@keyframes blink {
  from, to { color: transparent; }
  50% { color: var(--color-text-muted); }
}
</style>
