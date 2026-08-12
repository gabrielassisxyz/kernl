<template>
  <div class="tag-hierarchy">
    <div v-if="loading" class="tag-hierarchy__status">Loading…</div>

    <p v-else-if="!hasAnyNote" class="tag-hierarchy__status">
      No tags yet. Add tags in a note's properties.
    </p>

    <div v-else-if="groups.length === 0" class="tag-hierarchy__status">
      No tag holds a note matching “{{ query }}”.
    </div>

    <div v-for="group in groups" v-else :key="group.key" class="tag-section">
      <!-- The Pinned heading only earns its place once something is pinned, so
           an untouched vault shows one unlabelled list rather than two. -->
      <div v-if="showHeader(group)" class="tag-section__head">
        <span class="tag-section__label">{{ group.label }}</span>
        <span class="tag-section__count">{{ group.count }}</span>
      </div>

      <div v-for="tag in group.tags" :key="tag.name" class="tag-group">
        <div
          class="tag-row"
          :class="{ 'tag-row--open': isTagOpen(tag.name) }"
          role="button"
          tabindex="0"
          :aria-expanded="isTagOpen(tag.name)"
          @click="$emit('toggle-tag', tag.name)"
          @keydown.enter.prevent="$emit('toggle-tag', tag.name)"
          @keydown.space.prevent="$emit('toggle-tag', tag.name)"
        >
          <span
            class="material-symbols-outlined tag-row__chevron !text-[14px]"
            :class="{ 'is-open': isTagOpen(tag.name) }"
            aria-hidden="true"
          >expand_more</span>
          <span class="tag-row__hash">#</span>
          <span class="tag-row__name" :class="{ 'tag-row__name--pinned': tag.pinned }">{{ tag.name }}</span>
          <button
            type="button"
            class="tag-row__pin"
            :class="{ 'tag-row__pin--on': tag.pinned }"
            :title="tag.pinned ? 'Unpin tag' : 'Pin tag'"
            :aria-label="tag.pinned ? 'Unpin tag' : 'Pin tag'"
            :aria-pressed="tag.pinned"
            @click.stop="$emit('toggle-tag-pin', tag)"
          >
            <span class="material-symbols-outlined !text-[13px]" aria-hidden="true">keep</span>
          </button>
          <span class="tag-row__count">{{ tag.count }}</span>
        </div>

        <VaultNoteRow
          v-for="note in tag.notes"
          :key="`${tag.name}:${note.id}`"
          :note="note"
          :active="note.path === selected"
          variant="child"
          @open="$emit('select', $event.path)"
          @toggle-pin="$emit('toggle-pin', $event)"
        />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import VaultNoteRow from '~/components/notes/VaultNoteRow.vue'
import type { VaultNote } from '~/composables/useVaultIndex'
import type { TagEntry, TagGroup } from '~/composables/useVaultFilters'

const props = defineProps<{
  groups: TagGroup[]
  selected: string | null
  query: string
  loading: boolean
  hasAnyNote: boolean
  isTagOpen: (name: string) => boolean
}>()

defineEmits<{
  select: [string]
  'toggle-tag': [string]
  'toggle-tag-pin': [TagEntry]
  'toggle-pin': [VaultNote]
}>()

const showHeader = (group: TagGroup): boolean =>
  group.key === 'pinned' || props.groups.length > 1
</script>

<style scoped>
.tag-hierarchy {
  padding: 0 8px 24px 8px;
}

.tag-hierarchy__status {
  padding: 28px 10px;
  font-family: var(--font-body);
  font-size: 12.5px;
  line-height: 1.6;
  color: var(--color-text-muted);
}

.tag-section__head {
  display: flex;
  align-items: baseline;
  gap: 8px;
  padding: 12px 6px 6px 6px;
}

.tag-section__label {
  font-size: 10px;
  font-weight: 500;
  letter-spacing: 0.11em;
  text-transform: uppercase;
  color: var(--color-text-muted);
}

.tag-section__count {
  font-family: var(--font-mono-data);
  font-size: 10px;
  color: var(--color-text-faint);
}

.tag-row {
  display: flex;
  align-items: center;
  gap: 7px;
  width: 100%;
  padding: 6px 8px;
  border-radius: var(--radius-lg);
  cursor: pointer;
  transition: background-color 120ms ease;
}

.tag-row:hover,
/* An open tag keeps its background: with its notes listed underneath, the row
   is a heading for them, and a heading that looks like every other row leaves
   the indentation to carry the whole structure. */
.tag-row--open {
  background-color: var(--color-surface-hover);
}

.tag-row:focus-visible {
  outline: none;
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--color-primary) 70%, transparent);
}

.tag-row__chevron {
  flex: 0 0 auto;
  color: var(--color-text-faint);
  transform: rotate(-90deg);
  transition: transform 140ms ease-out;
}

.tag-row__chevron.is-open {
  transform: rotate(0deg);
}

.tag-row__hash {
  flex: 0 0 auto;
  font-family: var(--font-mono-data);
  font-size: 11px;
  color: var(--color-text-faint);
}

.tag-row__name {
  flex: 1 1 auto;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 12.5px;
  color: var(--color-text-secondary);
}

.tag-row__name--pinned {
  color: var(--color-text-primary);
}

.tag-row__pin {
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

.tag-row:hover .tag-row__pin,
.tag-row:focus-within .tag-row__pin,
.tag-row__pin--on {
  opacity: 1;
  pointer-events: auto;
}

.tag-row__pin--on {
  color: var(--color-primary);
}

.tag-row__pin:hover {
  color: var(--color-text-primary);
}

.tag-row__pin--on:hover {
  color: var(--color-primary);
}

.tag-row__pin:focus-visible {
  outline: none;
  opacity: 1;
  pointer-events: auto;
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--color-primary) 70%, transparent);
}

.tag-row__count {
  flex: 0 0 auto;
  font-family: var(--font-mono-data);
  font-size: 10.5px;
  color: var(--color-text-faint);
}

@media (pointer: coarse) {
  .tag-row__pin {
    opacity: 1;
    pointer-events: auto;
  }
}

@media (prefers-reduced-motion: reduce) {
  .tag-row,
  .tag-row__chevron,
  .tag-row__pin {
    transition: none;
  }
}
</style>
