<template>
  <div>
    <div
      class="flex flex-wrap items-center gap-tight min-h-9 w-full rounded border border-border-hairline bg-bg-base px-base py-tight transition-colors focus-within:border-primary/70"
      @click="inputEl?.focus()"
    >
      <span
        v-for="tag in modelValue"
        :key="tag"
        class="flex items-center gap-tight pl-base pr-tight py-[2px] rounded bg-surface-hover font-mono-data text-mono-data text-text-primary"
      >
        <span class="material-symbols-outlined !text-[13px] text-text-faint" aria-hidden="true">tag</span>
        {{ tag }}
        <button
          type="button"
          class="flex items-center text-text-faint hover:text-text-primary transition-colors cursor-pointer outline-none focus-visible:ring-1 focus-visible:ring-primary/30 rounded"
          :aria-label="`Remove tag ${tag}`"
          @click.stop="remove(tag)"
        >
          <span class="material-symbols-outlined !text-[13px]" aria-hidden="true">close</span>
        </button>
      </span>

      <input
        ref="inputEl"
        v-model="draft"
        type="text"
        class="flex-1 min-w-[100px] bg-transparent font-body text-body text-text-primary outline-none placeholder:text-text-muted"
        :placeholder="modelValue.length ? '' : placeholder"
        :aria-label="ariaLabel"
        @keydown="onKeydown"
        @blur="commit"
      />
    </div>

    <p v-if="error" class="mt-tight font-mono-data text-mono-data text-status-failed-text">{{ error }}</p>
    <p v-else class="mt-tight font-mono-data text-mono-data text-text-faint">
      Enter or comma adds a tag. Nest with <span class="text-text-muted">parent/child</span>.
    </p>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { normalizeTag } from '~/utils/tagName'

// Tags on a task or a project are graph-native: they are written straight through
// the API, with no vault file in between. That is why this is not shared with
// NoteProperties, which edits a note's YAML frontmatter and reaches the graph via
// the vault reconcile path.
withDefaults(defineProps<{
  placeholder?: string
  ariaLabel?: string
}>(), {
  placeholder: 'Add a tag…',
  ariaLabel: 'Tags',
})

const modelValue = defineModel<string[]>({ default: () => [] })

const inputEl = ref<HTMLInputElement | null>(null)
const draft = ref('')
const error = ref<string | null>(null)

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Enter' || e.key === ',') {
    // Enter would submit the surrounding modal form; a comma is a separator here,
    // not a character a tag name may contain.
    e.preventDefault()
    commit()
    return
  }
  if (e.key === 'Backspace' && draft.value === '' && modelValue.value.length) {
    modelValue.value = modelValue.value.slice(0, -1)
  }
}

function commit() {
  const raw = draft.value.trim()
  if (!raw) {
    error.value = null
    return
  }

  const { name, error: err } = normalizeTag(raw)
  if (!name) {
    error.value = err ?? null
    return
  }

  error.value = null
  draft.value = ''
  // Normalisation collapses case and padding, so a "new" tag can already be there.
  if (!modelValue.value.includes(name)) {
    modelValue.value = [...modelValue.value, name]
  }
}

function remove(tag: string) {
  modelValue.value = modelValue.value.filter((t) => t !== tag)
}

defineExpose({ focus: () => inputEl.value?.focus() })
</script>
