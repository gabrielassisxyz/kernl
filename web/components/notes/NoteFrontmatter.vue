<template>
  <div v-if="hasAnything" class="note-fm">
    <h1 v-if="showTitle" class="note-fm__title">{{ title }}</h1>

    <!-- Inline: the metadata reads as a byline under the title, and the
         editable panel is one chevron away rather than always on the page. -->
    <div v-if="showInlineHeader" class="note-fm__inline">
      <button
        type="button"
        class="note-fm__chevron-btn"
        title="Show properties"
        aria-label="Show properties"
        :aria-expanded="false"
        @click="inlineOpen = true"
      >
        <span class="material-symbols-outlined note-fm__chevron !text-[14px]" aria-hidden="true">expand_more</span>
      </button>

      <span v-for="tag in tags" :key="tag" class="note-fm__tag"># {{ tag }}</span>

      <template v-if="category">
        <span v-if="tags.length" class="note-fm__sep" aria-hidden="true"></span>
        <span class="note-fm__category">{{ category }}</span>
      </template>

      <template v-if="metaLine">
        <span v-if="tags.length || category" class="note-fm__sep" aria-hidden="true"></span>
        <span class="note-fm__meta">{{ metaLine }}</span>
      </template>
    </div>

    <div v-else class="note-fm__card">
      <div
        class="note-fm__card-head"
        role="button"
        tabindex="0"
        :aria-expanded="cardExpanded"
        @click="toggleCard"
        @keydown.enter.prevent="toggleCard"
        @keydown.space.prevent="toggleCard"
      >
        <span
          class="material-symbols-outlined note-fm__chevron !text-[14px]"
          :class="{ 'is-open': cardExpanded }"
          aria-hidden="true"
        >expand_more</span>
        <span class="note-fm__caption">properties</span>

        <span v-if="!cardExpanded" class="note-fm__summary-wrap">
          <span class="note-fm__summary">{{ summary }}</span>
          <span v-if="category" class="note-fm__category">{{ category }}</span>
        </span>
      </div>

      <div v-if="cardExpanded" class="note-fm__card-body">
        <NoteProperties
          :data="data"
          :parse-error="parseError"
          :readonly="readonly"
          :show-id="showId"
          @update:data="$emit('update:data', $event)"
        />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import NoteProperties from './NoteProperties.vue'
import { useEditorSettings } from '~/composables/useEditorSettings'
import type { VaultNote } from '~/composables/useVaultIndex'
import type { FrontmatterData } from '~/utils/frontmatter'
import { formatDayStamp, formatTimestamp } from '~/utils/time'

const props = withDefaults(defineProps<{
  data?: FrontmatterData | null
  parseError?: string
  readonly?: boolean
  showId?: boolean
  /** The vault-index entry for the open note: category and dates live there,
      not in the note's own frontmatter. Absent until the index resolves. */
  note?: VaultNote | null
}>(), {
  data: null,
  parseError: '',
  readonly: false,
  showId: false,
  note: null,
})

defineEmits<{ (e: 'update:data', data: FrontmatterData): void }>()

const { settings } = useEditorSettings()

// Both toggles are per-open-note: the editor is keyed by path, so switching
// notes remounts this and returns to the mode's resting state.
const inlineOpen = ref(false)
const panelExpanded = ref(true)

const isInline = computed(() => settings.frontmatter === 'inline')

// Broken YAML has to be visible: the notice lives inside the panel, so a
// collapsed block would hide the one thing the user needs to act on.
const forcedOpen = computed(() => Boolean(props.parseError))

const showTitle = computed(() => isInline.value)
const showInlineHeader = computed(() => isInline.value && !inlineOpen.value && !forcedOpen.value)
const cardExpanded = computed(() => (isInline.value || forcedOpen.value ? true : panelExpanded.value))

function toggleCard(): void {
  if (forcedOpen.value) return
  if (isInline.value) inlineOpen.value = false
  else panelExpanded.value = !panelExpanded.value
}

const title = computed(() => {
  const fromFrontmatter = props.data?.title
  if (typeof fromFrontmatter === 'string' && fromFrontmatter.trim()) return fromFrontmatter.trim()
  return props.note?.title || ''
})

// Tags come from the live frontmatter rather than the index entry: editing them
// in the panel has to be reflected the moment the block is collapsed again,
// and the index only catches up after the vault reconciles.
const tags = computed<string[]>(() => {
  const value = props.data?.tags
  if (Array.isArray(value)) return value.map((item) => String(item)).filter(Boolean)
  if (typeof value === 'string' && value.trim()) {
    return value.split(',').map((item) => item.trim()).filter(Boolean)
  }
  return []
})

const category = computed(() => props.note?.category || '')

const metaLine = computed(() => {
  const created = formatTimestamp(props.note?.createdAt)
  const updated = formatTimestamp(props.note?.updatedAt)
  if (!created && !updated) return ''
  if (!created) return `updated ${updated}`
  if (!updated) return `created ${created}`
  return `created ${created}  ·  updated ${updated}`
})

const summary = computed(() => {
  const count = `${tags.value.length} ${tags.value.length === 1 ? 'tag' : 'tags'}`
  const stamp = formatDayStamp(props.note?.updatedAt)
  return stamp ? `${count} · ${stamp}` : count
})

// Reading a note that carries no frontmatter should show no chrome at all -
// neither an empty card nor a header with nothing in it.
const hasAnything = computed(() => {
  if (props.parseError) return true
  if (!props.readonly) return true
  return Object.keys(props.data || {}).length > 0
})
</script>

<style scoped>
.note-fm {
  margin-bottom: 22px;
}

/* Matches .cm-md-h1 so a note that also opens with its own `# Title` shows two
   headings of one size rather than two of different ones. */
.note-fm__title {
  margin: 0 0 20px 0;
  font-size: calc(var(--notes-font-size, 15px) * 1.6 * var(--notes-heading-scale, 1));
  font-weight: 600;
  letter-spacing: -0.02em;
  line-height: 1.2;
  color: var(--color-text-heading);
}

/* --- Inline header --- */
.note-fm__inline {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  padding-bottom: 14px;
  border-bottom: 1px solid var(--color-border-hairline);
}

.note-fm__chevron-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  margin: -2px -4px -2px -2px;
  padding: 2px;
  border-radius: var(--radius-sm);
  color: var(--color-text-faint);
  cursor: pointer;
  transition: color 120ms ease;
}

.note-fm__chevron-btn:hover {
  color: var(--color-text-secondary);
}

.note-fm__chevron-btn:focus-visible {
  outline: none;
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--color-primary) 70%, transparent);
}

.note-fm__chevron {
  color: var(--color-text-faint);
  transform: rotate(-90deg);
  transition: transform 140ms ease-out;
}

.note-fm__chevron.is-open {
  transform: rotate(0deg);
}

.note-fm__tag {
  font-family: var(--font-mono-data);
  font-size: 11px;
  color: var(--color-text-muted);
}

.note-fm__sep {
  width: 3px;
  height: 3px;
  border-radius: var(--radius-full);
  background-color: var(--color-text-dim);
}

.note-fm__meta {
  font-family: var(--font-mono-data);
  font-size: 11px;
  color: var(--color-text-faint);
}

/* --- Properties card --- */
.note-fm__card {
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-xl);
  background-color: var(--color-surface);
  overflow: hidden;
}

.note-fm__card-head {
  display: flex;
  align-items: center;
  gap: 9px;
  padding: 10px 14px;
  cursor: pointer;
  user-select: none;
}

.note-fm__card-head:focus-visible {
  outline: none;
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--color-primary) 70%, transparent);
}

.note-fm__caption {
  font-family: var(--font-mono-data);
  font-size: 11px;
  letter-spacing: 0.06em;
  color: var(--color-text-faint);
}

.note-fm__summary-wrap {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  margin-left: auto;
}

.note-fm__summary {
  font-family: var(--font-mono-data);
  font-size: 10.5px;
  color: var(--color-text-faint);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* One muted grey for every category pill, wherever it renders. */
.note-fm__category {
  flex: 0 0 auto;
  padding: 1px 6px;
  border-radius: var(--radius-sm);
  background-color: color-mix(in srgb, var(--color-text-muted) 13%, transparent);
  color: var(--color-text-muted);
  font-family: var(--font-mono-data);
  font-size: 10px;
  letter-spacing: 0.03em;
}

.note-fm__card-body {
  padding: 0 14px 14px 14px;
}

/* The panel's own frame is the card's now: its standalone rule (a bottom
   hairline plus outer spacing) would draw a second edge inside this one. */
.note-fm__card-body :deep(.note-properties) {
  padding: 0;
  border-bottom: none;
  margin-bottom: 0;
}
</style>
