<template>
  <div
    class="note-row"
    :class="[`note-row--${variant}`, { 'note-row--active': active }]"
    role="button"
    tabindex="0"
    :title="note.path"
    @click="$emit('open', note)"
    @keydown.enter.prevent="$emit('open', note)"
    @keydown.space.prevent="$emit('open', note)"
  >
    <span class="note-row__title">{{ note.title }}</span>

    <!-- Only the DA's notes are badged. "me" is the unmarked default, so the
         badge means "not yours" rather than being half of a symmetric pair. -->
    <span v-if="isAgentAuthored(note)" class="note-row__source">DA</span>

    <span v-if="variant === 'file'" class="note-row__category">{{ note.category }}</span>

    <button
      type="button"
      class="note-row__pin"
      :class="{ 'note-row__pin--on': note.pinned }"
      :title="note.pinned ? 'Unpin note' : 'Pin note'"
      :aria-label="note.pinned ? 'Unpin note' : 'Pin note'"
      :aria-pressed="note.pinned"
      @click.stop="$emit('toggle-pin', note)"
    >
      <span class="material-symbols-outlined !text-[13px]" aria-hidden="true">keep</span>
    </button>
  </div>
</template>

<script setup lang="ts">
import { isAgentAuthored, type VaultNote } from '~/composables/useVaultIndex'

withDefaults(
  defineProps<{
    note: VaultNote
    active: boolean
    /** `file` carries a category pill; `child` sits indented under its tag. */
    variant?: 'file' | 'child'
  }>(),
  { variant: 'file' }
)

defineEmits<{ open: [VaultNote]; 'toggle-pin': [VaultNote] }>()
</script>

<style scoped>
.note-row {
  display: flex;
  align-items: center;
  width: 100%;
  border-radius: var(--radius-lg);
  cursor: pointer;
  transition: background-color 120ms ease;
}

.note-row--file {
  gap: 9px;
  padding: 7px 8px;
}

.note-row--child {
  gap: 8px;
  padding: 5px 8px 5px 28px;
}

.note-row:hover {
  background-color: var(--color-surface-hover);
}

.note-row--active {
  background-color: color-mix(in oklab, var(--color-primary) 11%, transparent);
}

.note-row:focus-visible {
  outline: none;
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--color-primary) 70%, transparent);
}

.note-row__title {
  flex: 1 1 auto;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 12.5px;
  line-height: 1.35;
  color: var(--color-text-secondary);
}

.note-row--active .note-row__title {
  color: var(--color-text-primary);
}

.note-row__source {
  flex: 0 0 auto;
  padding: 0 4px;
  border: 1px solid color-mix(in oklab, var(--color-primary) 30%, transparent);
  border-radius: var(--radius-sm);
  background-color: color-mix(in oklab, var(--color-primary) 14%, transparent);
  color: var(--color-primary);
  font-family: var(--font-mono-data);
  font-size: 9.5px;
  letter-spacing: 0.06em;
}

/* Every category is the same muted grey, as the prototype has it: the pill
   says which kind of thing this is, and colour-coding five kinds in a list
   that is already dense buys nothing the word does not. */
.note-row__category {
  flex: 0 0 auto;
  padding: 1px 6px;
  border-radius: var(--radius-sm);
  background-color: color-mix(in srgb, var(--color-text-muted) 13%, transparent);
  color: var(--color-text-muted);
  font-family: var(--font-mono-data);
  font-size: 10px;
  letter-spacing: 0.03em;
}

/* Hidden until the row is hovered, but never hidden while it is pinned -
   otherwise the only way to find a pin is to hover every row looking for it. */
.note-row__pin {
  flex: 0 0 auto;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 2px;
  border-radius: var(--radius);
  color: var(--color-text-muted);
  opacity: 0;
  pointer-events: none;
  cursor: pointer;
  transition: opacity 120ms ease-out, color 120ms ease;
}

.note-row:hover .note-row__pin,
.note-row:focus-within .note-row__pin,
.note-row__pin--on {
  opacity: 1;
  pointer-events: auto;
}

.note-row__pin--on {
  color: var(--color-primary);
}

.note-row__pin:hover {
  color: var(--color-text-primary);
}

.note-row__pin--on:hover {
  color: var(--color-primary);
}

.note-row__pin:focus-visible {
  outline: none;
  opacity: 1;
  pointer-events: auto;
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--color-primary) 70%, transparent);
}

@media (pointer: coarse) {
  .note-row__pin {
    opacity: 1;
    pointer-events: auto;
  }
}

@media (prefers-reduced-motion: reduce) {
  .note-row,
  .note-row__pin {
    transition: none;
  }
}
</style>
