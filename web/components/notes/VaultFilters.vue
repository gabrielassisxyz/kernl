<template>
  <div ref="root" class="vault-filters">
    <!-- Sort -->
    <div class="chip-anchor">
      <button
        type="button"
        class="chip"
        :class="{ 'chip--active': open === 'sort' }"
        :aria-expanded="open === 'sort'"
        @click="toggle('sort')"
      >
        <span
          class="material-symbols-outlined chip__icon !text-[13px]"
          :class="{ 'chip__icon--flipped': sortDir === 'asc' }"
          aria-hidden="true"
        >arrow_downward</span>
        {{ sortLabel }}
      </button>

      <div v-if="open === 'sort'" class="menu" role="menu">
        <p class="menu__head">Sort by</p>
        <button
          v-for="option in SORT_OPTIONS"
          :key="option.id"
          type="button"
          class="menu__item"
          :class="{ 'menu__item--active': sortField === option.id }"
          @click="pickSort(option.id)"
        >
          <span class="menu__label">{{ option.label }}</span>
          <span class="menu__hint">{{ option.hint }}</span>
        </button>
        <div class="menu__divider"></div>
        <button type="button" class="menu__item" @click="$emit('update:sortDir', sortDir === 'desc' ? 'asc' : 'desc')">
          <span
            class="material-symbols-outlined chip__icon !text-[13px]"
            :class="{ 'chip__icon--flipped': sortDir === 'asc' }"
            aria-hidden="true"
          >arrow_downward</span>
          <span class="menu__label">{{ sortDir === 'desc' ? 'Descending' : 'Ascending' }}</span>
        </button>
      </div>
    </div>

    <!-- Authorship -->
    <div class="chip-anchor">
      <button
        type="button"
        class="chip"
        :class="{ 'chip--active': open === 'source' || sourceMode !== 'merged' }"
        :aria-expanded="open === 'source'"
        @click="toggle('source')"
      >
        <span class="material-symbols-outlined chip__icon !text-[13px]" aria-hidden="true">join_inner</span>
        {{ sourceLabel }}
      </button>

      <div v-if="open === 'source'" class="menu menu--wide" role="menu">
        <p class="menu__head">Authorship</p>
        <button
          v-for="option in SOURCE_OPTIONS"
          :key="option.id"
          type="button"
          class="menu__item"
          :class="{ 'menu__item--active': sourceMode === option.id }"
          @click="pickSource(option.id)"
        >
          <span class="menu__label">{{ option.label }}</span>
          <span class="menu__hint">{{ option.hint }}</span>
        </button>
      </div>
    </div>

    <!-- Categories. Files only: the Tags tab groups by tag, so a second
         grouping there would fight the first. -->
    <div v-if="showCategories" class="chip-anchor">
      <button
        type="button"
        class="chip"
        :class="{ 'chip--active': open === 'category' || categoryFilter !== 'all' || groupByCategory }"
        :aria-expanded="open === 'category'"
        @click="toggle('category')"
      >
        <span class="material-symbols-outlined chip__icon !text-[13px]" aria-hidden="true">grid_view</span>
        {{ categoryLabel }}
      </button>

      <div v-if="open === 'category'" class="menu" role="menu">
        <p class="menu__head">Categories</p>
        <button
          type="button"
          class="menu__item"
          :class="{ 'menu__item--active': groupByCategory }"
          role="switch"
          :aria-checked="groupByCategory"
          @click="$emit('update:groupByCategory', !groupByCategory)"
        >
          <span class="menu__label">Group by category</span>
          <span class="menu__switch" :class="{ 'menu__switch--on': groupByCategory }"></span>
        </button>
        <div class="menu__divider"></div>
        <button
          v-for="category in categories"
          :key="category.id"
          type="button"
          class="menu__item"
          :class="{ 'menu__item--active': categoryFilter === category.id }"
          @click="pickCategory(category.id)"
        >
          <span class="menu__label">{{ category.label }}</span>
          <span class="menu__hint">{{ category.count }}</span>
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount } from 'vue'
import {
  SORT_OPTIONS,
  SOURCE_OPTIONS,
  type CategoryOption,
  type SortDir,
  type SortField,
  type SourceMode,
} from '~/composables/useVaultFilters'

defineProps<{
  sortField: SortField
  sortDir: SortDir
  sortLabel: string
  sourceMode: SourceMode
  sourceLabel: string
  categoryFilter: string
  categoryLabel: string
  groupByCategory: boolean
  categories: CategoryOption[]
  showCategories: boolean
}>()

const emit = defineEmits<{
  'update:sortField': [SortField]
  'update:sortDir': [SortDir]
  'update:sourceMode': [SourceMode]
  'update:categoryFilter': [string]
  'update:groupByCategory': [boolean]
}>()

const root = ref<HTMLElement | null>(null)
const open = ref<'sort' | 'source' | 'category' | null>(null)

const toggle = (name: 'sort' | 'source' | 'category'): void => {
  open.value = open.value === name ? null : name
}

// Picking a value closes the menu; toggling the direction or the grouping
// switch does not, because those are the two a user flips to compare.
const pickSort = (id: SortField): void => {
  emit('update:sortField', id)
  open.value = null
}
const pickSource = (id: SourceMode): void => {
  emit('update:sourceMode', id)
  open.value = null
}
const pickCategory = (id: string): void => {
  emit('update:categoryFilter', id)
  open.value = null
}

const onDocumentPointerDown = (event: MouseEvent): void => {
  if (!open.value) return
  if (root.value && !root.value.contains(event.target as Node)) open.value = null
}
const onEscape = (event: KeyboardEvent): void => {
  if (event.key === 'Escape') open.value = null
}

onMounted(() => {
  document.addEventListener('pointerdown', onDocumentPointerDown)
  document.addEventListener('keydown', onEscape)
})
onBeforeUnmount(() => {
  document.removeEventListener('pointerdown', onDocumentPointerDown)
  document.removeEventListener('keydown', onEscape)
})
</script>

<style scoped>
.vault-filters {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
  padding: 0 12px 10px 12px;
  position: relative;
  z-index: 20;
}

.chip-anchor {
  position: relative;
}

.chip {
  display: flex;
  align-items: center;
  gap: 5px;
  height: 26px;
  padding: 0 8px;
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-lg);
  background-color: var(--color-bg-elevated);
  color: var(--color-text-secondary);
  font-size: 11.5px;
  white-space: nowrap;
  cursor: pointer;
  transition: background-color 120ms ease, border-color 120ms ease, color 120ms ease;
}

.chip:hover {
  color: var(--color-text-primary);
}

.chip--active {
  background-color: color-mix(in oklab, var(--color-primary) 14%, transparent);
  border-color: color-mix(in oklab, var(--color-primary) 40%, var(--color-border-default));
  color: var(--color-primary);
}

.chip:focus-visible {
  outline: none;
  border-color: color-mix(in srgb, var(--color-primary) 70%, transparent);
}

.chip__icon {
  flex: 0 0 auto;
  transition: transform 140ms ease-out;
}

.chip__icon--flipped {
  transform: rotate(180deg);
}

.menu {
  position: absolute;
  top: calc(100% + 4px);
  left: 0;
  width: 178px;
  padding: 4px;
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-xl);
  background-color: var(--color-surface-overlay);
  /* Matches the editor's settings popover rather than the prototype's 12px
     offset, so every floating surface in the app casts one shadow. */
  box-shadow: 0 8px 28px rgba(0, 0, 0, 0.45);
}

.menu--wide {
  width: 214px;
}

.menu__head {
  padding: 6px 8px 4px;
  font-size: var(--text-label-caps);
  letter-spacing: var(--text-label-caps--letter-spacing);
  text-transform: uppercase;
  color: var(--color-text-faint);
}

.menu__item {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  padding: 6px 8px;
  border-radius: var(--radius);
  color: var(--color-text-secondary);
  font-size: 12px;
  cursor: pointer;
  transition: background-color 120ms ease, color 120ms ease;
}

.menu__item:hover {
  background-color: var(--color-surface-hover);
  color: var(--color-text-primary);
}

.menu__item--active {
  background-color: color-mix(in oklab, var(--color-primary) 13%, transparent);
  color: var(--color-primary);
}

.menu__item:focus-visible {
  outline: none;
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--color-primary) 70%, transparent);
}

.menu__label {
  flex: 1 1 auto;
  min-width: 0;
  text-align: left;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.menu__hint {
  flex: 0 0 auto;
  font-family: var(--font-mono-data);
  font-size: 10px;
  color: var(--color-text-faint);
}

.menu__divider {
  height: 1px;
  margin: 4px 0;
  background-color: var(--color-border-hairline);
}

.menu__switch {
  flex: 0 0 auto;
  position: relative;
  width: 22px;
  height: 13px;
  border-radius: var(--radius-full);
  background-color: var(--color-surface-container-high);
  box-shadow: inset 0 0 0 1px var(--color-border-default);
  transition: background-color 150ms ease;
}

.menu__switch::after {
  content: '';
  position: absolute;
  top: 2px;
  left: 2px;
  width: 9px;
  height: 9px;
  border-radius: 50%;
  background-color: var(--color-text-muted);
  transition: transform 150ms cubic-bezier(0.22, 1, 0.36, 1), background-color 150ms ease;
}

.menu__switch--on {
  background-color: var(--color-primary);
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--color-primary) 60%, transparent);
}

.menu__switch--on::after {
  background-color: var(--color-on-primary);
  transform: translateX(9px);
}

@media (prefers-reduced-motion: reduce) {
  .chip__icon,
  .menu__switch,
  .menu__switch::after {
    transition: none;
  }
}
</style>
