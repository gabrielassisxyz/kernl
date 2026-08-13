<template>
  <div ref="root" class="sort-control">
    <button
      type="button"
      class="chip"
      :class="{ 'chip--active': open }"
      :aria-expanded="open"
      @click="open = !open"
    >
      <span
        class="material-symbols-outlined chip__icon !text-[13px]"
        :class="{ 'chip__icon--flipped': sortDir === 'asc' }"
        aria-hidden="true"
      >arrow_downward</span>
      {{ sortLabel }}
    </button>

    <div v-if="open" class="menu" role="menu">
      <p class="menu__head">Sort by</p>
      <button
        v-for="option in LIST_SORT_OPTIONS"
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
      <button type="button" class="menu__item" @click="$emit('toggle-direction')">
        <span
          class="material-symbols-outlined chip__icon !text-[13px]"
          :class="{ 'chip__icon--flipped': sortDir === 'asc' }"
          aria-hidden="true"
        >arrow_downward</span>
        <span class="menu__label">{{ sortDir === 'desc' ? 'Descending' : 'Ascending' }}</span>
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { LIST_SORT_OPTIONS, type ListSortDir, type ListSortField } from '~/composables/useListPreferences'

defineProps<{
  sortField: ListSortField
  sortDir: ListSortDir
  sortLabel: string
}>()

const emit = defineEmits<{
  'update:sortField': [ListSortField]
  'toggle-direction': []
}>()

const root = ref<HTMLElement | null>(null)
const open = ref(false)

const pickSort = (field: ListSortField): void => {
  emit('update:sortField', field)
  open.value = false
}
const onDocumentPointerDown = (event: MouseEvent): void => {
  if (open.value && root.value && !root.value.contains(event.target as Node)) open.value = false
}
const onEscape = (event: KeyboardEvent): void => {
  if (event.key === 'Escape') open.value = false
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
.sort-control { position: relative; }
.chip { display: flex; align-items: center; gap: 5px; height: 30px; padding: 0 8px; border: 1px solid var(--color-border-default); border-radius: var(--radius-lg); background-color: var(--color-bg-elevated); color: var(--color-text-secondary); font-size: 11.5px; white-space: nowrap; cursor: pointer; transition: background-color 120ms ease, border-color 120ms ease, color 120ms ease; }
.chip:hover { color: var(--color-text-primary); }
.chip--active { background-color: color-mix(in oklab, var(--color-primary) 14%, transparent); border-color: color-mix(in oklab, var(--color-primary) 40%, var(--color-border-default)); color: var(--color-primary); }
.chip__icon { transition: transform 150ms ease; }
.chip__icon--flipped { transform: rotate(180deg); }
.menu { position: absolute; z-index: 30; top: calc(100% + 6px); right: 0; min-width: 188px; padding: 5px; border: 1px solid var(--color-border-default); border-radius: var(--radius-lg); background: var(--color-bg-elevated); box-shadow: var(--shadow-popover); }
.menu__head { margin: 4px 7px 5px; color: var(--color-text-faint); font-size: 10px; font-weight: 600; letter-spacing: .08em; text-transform: uppercase; }
.menu__item { display: flex; align-items: center; gap: 7px; width: 100%; padding: 6px 7px; border: 0; border-radius: var(--radius-md); background: transparent; color: var(--color-text-secondary); cursor: pointer; font-size: 12px; text-align: left; }
.menu__item:hover, .menu__item--active { background: var(--color-bg-hover); color: var(--color-text-primary); }
.menu__label { flex: 1; }
.menu__hint { color: var(--color-text-faint); font-family: var(--font-mono-data); font-size: 10px; }
.menu__divider { height: 1px; margin: 5px 2px; background: var(--color-border-hairline); }
</style>
