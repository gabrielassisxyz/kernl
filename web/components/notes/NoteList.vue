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
      <button
        type="button"
        class="note-group__head"
        :aria-expanded="isGroupOpen(group.key)"
        @click="$emit('toggle-group', group.key)"
      >
        <span
          class="material-symbols-outlined note-group__chevron !text-[16px]"
          :class="{ 'is-open': isGroupOpen(group.key) }"
          aria-hidden="true"
        >expand_more</span>
        <span class="note-group__label">{{ group.label }}</span>
        <span class="note-group__count">{{ group.count }}</span>
      </button>

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

defineEmits<{ select: [string]; 'toggle-pin': [VaultNote]; 'toggle-group': [string] }>()
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
  width: 100%;
  padding: 12px 6px 6px 6px;
  text-align: left;
  cursor: pointer;
  user-select: none;
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

.note-group__head:focus-visible {
  outline: none;
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--color-primary) 70%, transparent);
  border-radius: var(--radius-lg);
}

@media (prefers-reduced-motion: reduce) {
  .note-group__chevron {
    transition: none;
  }
}
</style>
