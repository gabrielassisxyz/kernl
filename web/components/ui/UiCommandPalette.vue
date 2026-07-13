<template>
  <Teleport to="body">
    <div class="fixed inset-0 z-modal flex justify-center bg-black/55" @pointerdown.self="$emit('close')">
      <div class="mt-[18vh] h-fit w-full max-w-[420px] flex flex-col rounded border border-border-default bg-surface-overlay">
        <div class="flex items-center gap-base border-b border-border-hairline px-component py-base">
          <span class="font-mono-data text-mono-data text-text-faint shrink-0">{{ label }}</span>
          <input
            ref="inputEl"
            v-model="query"
            class="flex-1 min-w-0 bg-transparent font-body text-body text-text-primary placeholder:text-text-muted outline-none"
            :placeholder="placeholder"
            @keydown.down.prevent="move(1)"
            @keydown.up.prevent="move(-1)"
            @keydown.enter.prevent="pick"
            @keydown.escape.prevent.stop="$emit('close')"
          />
        </div>

        <ul v-if="matches.length > 0" class="max-h-[42vh] overflow-y-auto p-tight">
          <li v-for="(option, i) in matches" :key="option.id">
            <button
              class="w-full flex items-baseline gap-base rounded px-base py-tight text-left transition-colors cursor-pointer"
              :class="i === active ? 'bg-surface-hover text-text-primary' : 'text-text-muted hover:bg-surface-hover'"
              @pointermove="active = i"
              @click="$emit('pick', option.id)"
            >
              <span class="font-body text-body truncate">{{ option.label }}</span>
              <span v-if="option.hint" class="ml-auto shrink-0 font-mono-data text-mono-data text-text-faint">{{ option.hint }}</span>
            </button>
          </li>
        </ul>

        <p v-else class="px-component py-base font-body text-body text-text-muted">
          {{ allowRaw ? 'No match — ⏎ takes what you typed.' : 'No match.' }}
        </p>

        <div class="flex items-center gap-component border-t border-border-hairline px-component py-tight font-mono-data text-mono-data text-text-faint">
          <span>↑↓ move</span><span>⏎ select</span><span>esc cancel</span>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'

export interface PaletteOption {
  id: string
  label: string
  hint?: string
}

const props = withDefaults(defineProps<{
  /** what is being chosen — sits as a prompt to the left of the query */
  label: string
  options: PaletteOption[]
  placeholder?: string
  /** accept the raw query when nothing matches (a typed date, a new name) */
  allowRaw?: boolean
}>(), {
  placeholder: '',
  allowRaw: false,
})

const emit = defineEmits<{
  (e: 'pick', id: string): void
  (e: 'close'): void
}>()

const query = ref('')
const active = ref(0)
const inputEl = ref<HTMLInputElement | null>(null)

const matches = computed(() => {
  const q = query.value.trim().toLowerCase()
  if (!q) return props.options
  return props.options.filter(o => o.label.toLowerCase().includes(q))
})

watch(matches, () => (active.value = 0))

function move(delta: number) {
  const n = matches.value.length
  if (n === 0) return
  active.value = (active.value + delta + n) % n
}

// A raw query only wins when nothing matched: otherwise typing "tom" would send
// the literal string rather than the Tomorrow the list is already offering.
function pick() {
  const option = matches.value[active.value]
  if (option) { emit('pick', option.id); return }
  const raw = query.value.trim()
  if (props.allowRaw && raw) emit('pick', raw)
}

onMounted(async () => {
  await nextTick()
  inputEl.value?.focus()
})
</script>
