<template>
  <div class="note-list">
    <div v-if="loading" class="note-list__status">Loading…</div>

    <p v-else-if="!hasAnyNote" class="note-list__status">
      No notes yet. Create your first note with the
      <span class="material-symbols-outlined !text-[14px] align-text-bottom">add</span> above.
    </p>

    <div v-else-if="groups.length === 0" class="note-list__status">
      No note matches “{{ query }}”.
    </div>

    <div v-for="group in groups" v-else :key="group.key" class="note-group">
      <div class="note-group__head">
        <button
          type="button"
          class="note-group__toggle"
          :aria-expanded="isGroupOpen(group.key)"
          @click="$emit('toggle-group', group.key)"
        >
          <span
            class="material-symbols-outlined note-group__chevron !text-[16px]"
            :class="{ 'is-open': isGroupOpen(group.key) }"
            aria-hidden="true"
          >expand_more</span>
          <span class="note-group__label">{{ group.label }}</span>
        </button>
        <button
          v-if="group.category"
          type="button"
          class="note-group__pin"
          :class="{ 'note-group__pin--on': group.pinned }"
          :title="group.pinned ? `Unpin ${group.label} category` : `Pin ${group.label} category`"
          :aria-label="group.pinned ? `Unpin ${group.label} category` : `Pin ${group.label} category`"
          :aria-pressed="group.pinned"
          @click.stop="$emit('toggle-category-pin', group.category)"
        >
          <span class="material-symbols-outlined !text-[13px]" aria-hidden="true">keep</span>
        </button>
        <span class="note-group__count">{{ group.count }}</span>
      </div>

      <VaultNoteRow
        v-for="note in group.notes"
        :key="note.id"
        :note="note"
        :active="note.path === selected"
        variant="file"
        @open="$emit('select', $event.path)"
        @toggle-pin="$emit('toggle-pin', $event)"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import VaultNoteRow from '~/components/notes/VaultNoteRow.vue'
import type { VaultNote } from '~/composables/useVaultIndex'
import type { NoteGroup } from '~/composables/useVaultFilters'

defineProps<{
  groups: NoteGroup[]
  selected: string | null
  query: string
  loading: boolean
  hasAnyNote: boolean
  isGroupOpen: (key: string) => boolean
}>()

defineEmits<{
  select: [string]
  'toggle-pin': [VaultNote]
  'toggle-group': [string]
  'toggle-category-pin': [string]
}>()
</script>

<style scoped>
.note-list {
  padding: 0 8px 24px 8px;
}

.note-list__status {
  padding: 28px 10px;
  font-family: var(--font-body);
  font-size: 12.5px;
  line-height: 1.6;
  color: var(--color-text-muted);
}

.note-group__head {
  display: flex;
  align-items: center;
  gap: 7px;
  padding: 12px 6px 6px 6px;
  user-select: none;
}

.note-group__toggle {
  display: flex;
  align-items: center;
  gap: 7px;
  flex: 1 1 auto;
  min-width: 0;
  text-align: left;
  cursor: pointer;
}

.note-group__chevron {
  flex: 0 0 auto;
  color: var(--color-text-faint);
  transform: rotate(-90deg);
  transition: transform 140ms ease-out;
}

.note-group__chevron.is-open {
  transform: rotate(0deg);
}

.note-group__label {
  flex: 1 1 auto;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 10px;
  font-weight: 500;
  letter-spacing: 0.11em;
  text-transform: uppercase;
  color: var(--color-text-muted);
}

.note-group__count {
  flex: 0 0 auto;
  font-family: var(--font-mono-data);
  font-size: 10px;
  color: var(--color-text-faint);
}

.note-group__pin {
  flex: 0 0 auto;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 2px;
  border-radius: var(--radius);
  color: var(--color-text-muted);
  opacity: 0;
  cursor: pointer;
  transition: opacity 120ms ease-out, color 120ms ease;
}

.note-group__head:hover .note-group__pin,
.note-group__head:focus-within .note-group__pin,
.note-group__pin--on {
  opacity: 1;
}

.note-group__pin--on {
  color: var(--color-primary);
}

.note-group__pin:hover {
  color: var(--color-text-primary);
}

.note-group__pin--on:hover {
  color: var(--color-primary);
}

.note-group__pin:focus-visible {
  outline: none;
  opacity: 1;
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--color-primary) 70%, transparent);
}

.note-group__toggle:focus-visible {
  outline: none;
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--color-primary) 70%, transparent);
  border-radius: var(--radius-lg);
}

@media (pointer: coarse) {
  .note-group__pin {
    opacity: 1;
  }
}

@media (prefers-reduced-motion: reduce) {
  .note-group__chevron {
    transition: none;
  }

  .note-group__pin {
    transition: none;
  }
}
</style>
