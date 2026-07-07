<template>
  <div :class="wrapperClass">
    <div class="bg-surface-overlay border border-border-default focus-within:border-primary/70 transition-colors p-component flex flex-col gap-2 relative rounded">
      <div class="flex items-start gap-3">
        <span class="text-text-faint font-mono-data text-mono-data pt-[2px]">&gt;</span>
        <textarea
          ref="inputEl"
          v-model="text"
          @keydown="onKeydown"
          @input="resizeTextarea"
          rows="1"
          :placeholder="placeholder"
          class="w-full bg-transparent border-none text-text-primary font-mono-data text-mono-data focus:ring-0 resize-none min-h-[40px] p-0 placeholder:text-text-faint leading-relaxed outline-none"
        ></textarea>
        <span v-show="!text" class="blinking-cursor absolute top-[18px] left-[32px] pointer-events-none font-mono-data text-mono-data h-[14px]">_</span>
      </div>
      <div class="flex justify-between items-center gap-component mt-1">
        <div class="flex items-center gap-4 text-text-faint font-mono-data text-mono-data tracking-wide min-w-0 flex-wrap">
          <template v-if="uploadEnabled">
            <input ref="fileInput" type="file" :accept="uploadAccept" class="hidden" @change="onFilePicked" />
            <span v-if="uploadStatus" class="text-text-muted truncate">{{ uploadStatus }}</span>
          </template>
          <span>[ESC] cancel</span>
          <span>[SHIFT+ENTER] new line</span>
          <span>{{ enterHint }}</span>
          <span v-if="helpText" class="text-text-muted truncate">{{ helpText }}</span>
        </div>
        <div v-if="actionLabel || uploadEnabled" class="shrink-0 flex items-center gap-base">
          <button
            v-if="uploadEnabled"
            class="inline-flex items-center justify-center gap-tight rounded border border-border-hairline bg-surface-container-low px-component h-7 font-body text-body text-text-muted transition-colors duration-150 cursor-pointer outline-none hover:border-border-default hover:bg-surface-hover hover:text-text-primary focus-visible:border-primary/70 focus-visible:ring-1 focus-visible:ring-primary/30"
            @click="fileInput?.click()"
          >
            <span class="material-symbols-outlined !text-[16px]">upload_file</span>
            {{ uploadLabel }}
          </button>
          <button
            v-if="actionLabel"
            class="inline-flex items-center justify-center gap-tight rounded border border-primary/40 bg-primary px-component h-7 font-body text-body text-on-primary transition-colors duration-150 cursor-pointer outline-none hover:bg-primary-container focus-visible:border-primary/70 focus-visible:ring-1 focus-visible:ring-primary/30 disabled:cursor-not-allowed disabled:opacity-45"
            :disabled="loading || !text.trim()"
            @click="save"
          >
            <span v-if="loading" class="material-symbols-outlined animate-spin !text-[16px]" aria-hidden="true">progress_activity</span>
            <span v-else-if="actionIcon" class="material-symbols-outlined !text-[16px]" aria-hidden="true">{{ actionIcon }}</span>
            {{ actionLabel }}
          </button>
        </div>
        <span
          v-else
          class="flex items-center gap-1 text-text-faint font-mono-data text-mono-data transition-opacity duration-300"
          :class="captured ? 'opacity-100' : 'opacity-0'"
        >captured</span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, onMounted, watch } from 'vue'

const emit = defineEmits<{ submit: [body: string]; cancel: [] }>()
const props = withDefaults(defineProps<{
  embedded?: boolean
  placeholder?: string
  manualSubmit?: boolean
  saveOnEnter?: boolean
  actionLabel?: string
  actionIcon?: string
  helpText?: string
  loading?: boolean
  uploadEnabled?: boolean
  uploadLabel?: string
  uploadAccept?: string
  maxRows?: number
}>(), {
  embedded: false,
  placeholder: 'Capture thought...',
  manualSubmit: false,
  saveOnEnter: true,
  actionLabel: '',
  actionIcon: '',
  helpText: '',
  loading: false,
  uploadEnabled: false,
  uploadLabel: 'Upload file',
  uploadAccept: '.txt,.md,.csv,.json,text/*',
  maxRows: 4,
})

const text = defineModel<string>({ default: '' })
const captured = ref(false)
const inputEl = ref<HTMLTextAreaElement | null>(null)
const fileInput = ref<HTMLInputElement | null>(null)
const uploadStatus = ref('')
let capturedTimer: ReturnType<typeof setTimeout>

const wrapperClass = computed(() => [
  'w-full shrink-0 z-10',
  props.embedded ? '' : 'mb-section',
])
const enterHint = computed(() => props.saveOnEnter ? '[ENTER] save' : '[ENTER] new line')

// Capture is the whole point of opening the inbox — focus the box so the user
// can type immediately instead of clicking into it first.
onMounted(() => {
  inputEl.value?.focus()
  resizeTextarea()
})

watch(text, async () => {
  await nextTick()
  resizeTextarea()
})

function resizeTextarea() {
  const el = inputEl.value
  if (!el) return
  const lineHeight = Number.parseFloat(getComputedStyle(el).lineHeight) || 20
  const maxHeight = Math.max(1, props.maxRows) * lineHeight
  el.style.height = 'auto'
  el.style.height = `${Math.max(40, Math.min(el.scrollHeight, maxHeight))}px`
  el.style.overflowY = el.scrollHeight > maxHeight ? 'auto' : 'hidden'
}

const onKeydown = (e: KeyboardEvent) => {
  if (e.key === 'Escape') {
    e.preventDefault()
    text.value = ''
    inputEl.value?.blur()
    emit('cancel')
  } else if (props.saveOnEnter && e.key === 'Enter' && !e.shiftKey) {
    // Plain Enter saves; Shift+Enter falls through to insert a newline.
    e.preventDefault()
    save()
  }
}

const save = async () => {
  const body = text.value.trim()
  if (!body) return
  if (props.manualSubmit) {
    emit('submit', body)
    return
  }
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

const onFilePicked = async (e: Event) => {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  uploadStatus.value = `Loaded ${file.name}`
  try {
    const body = await file.text()
    text.value = text.value ? `${text.value.trimEnd()}\n${body}` : body
    await nextTick()
    resizeTextarea()
  } catch {
    uploadStatus.value = 'Upload failed'
  } finally {
    input.value = ''
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
