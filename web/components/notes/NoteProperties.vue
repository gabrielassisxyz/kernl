<template>
  <section
    v-if="visible"
    class="note-properties"
    :class="{ 'note-properties--readonly': readonly }"
    aria-label="Note properties"
  >
    <div v-if="parseError" class="note-properties__notice">
      <span class="material-symbols-outlined !text-[15px]" aria-hidden="true">help</span>
      <span>Frontmatter has invalid YAML — switch to source mode to fix it.</span>
    </div>

    <template v-else>
      <div
        v-for="row in rows"
        :key="row.key"
        class="note-properties__row group"
      >
        <span class="note-properties__key" :title="row.key">
          <span
            v-if="row.locked"
            class="material-symbols-outlined note-properties__lock !text-[13px]"
            aria-hidden="true"
            title="Managed by Kernl"
          >lock</span>
          {{ row.key }}
        </span>

        <!-- List / tags: inline pills + a borderless add field. -->
        <div v-if="row.type === 'list'" class="note-properties__tags">
          <span v-for="tag in listValue(row.value)" :key="tag" class="note-properties__tag">
            <span class="note-properties__tag-hash">#</span>{{ tag }}
            <button
              v-if="!readonly && !row.locked"
              type="button"
              class="note-properties__tag-x"
              :aria-label="`Remove ${tag}`"
              @click="removeTag(row.key, tag)"
            >
              <span class="material-symbols-outlined !text-[12px]" aria-hidden="true">close</span>
            </button>
          </span>
          <input
            v-if="!readonly && !row.locked"
            v-model="tagDrafts[row.key]"
            class="note-properties__input note-properties__tag-input"
            :aria-label="`Add value to ${row.key}`"
            :placeholder="listValue(row.value).length ? '' : 'Empty'"
            @keydown.enter.prevent="addTag(row.key)"
            @keydown="onTagSeparator(row.key, $event)"
            @blur="addTag(row.key)"
          >
        </div>

        <!-- Scalar (text / date): borderless input that reads as text until focused. -->
        <input
          v-else
          :type="row.type === 'date' ? 'date' : 'text'"
          :value="stringValue(row.value)"
          :readonly="readonly || row.locked"
          :tabindex="row.locked ? -1 : 0"
          class="note-properties__input"
          :class="{ 'note-properties__input--locked': row.locked }"
          :aria-label="`${row.key} value`"
          placeholder="Empty"
          @change="commitScalar(row.key, $event)"
          @keydown.enter="blurTarget($event)"
        >

        <button
          v-if="!readonly && !row.locked"
          type="button"
          class="note-properties__remove"
          :aria-label="`Remove property ${row.key}`"
          @click="removeProperty(row.key)"
        >
          <span class="material-symbols-outlined !text-[15px]" aria-hidden="true">close</span>
        </button>
      </div>

      <!-- Add-property affordance: a quiet trigger that expands to an inline form. -->
      <div v-if="!readonly" class="note-properties__add">
        <form v-if="adding" class="note-properties__add-form" @submit.prevent="confirmAdd">
          <input
            ref="addKeyInput"
            v-model="newKey"
            class="note-properties__input note-properties__add-key"
            placeholder="Property name"
            aria-label="New property name"
            @keydown.esc="cancelAdd"
          >
          <select v-model="newType" class="note-properties__add-type" aria-label="New property type">
            <option value="text">Text</option>
            <option value="list">List</option>
            <option value="date">Date</option>
          </select>
          <button type="submit" class="note-properties__add-confirm" :disabled="!canAdd">Add</button>
          <button type="button" class="note-properties__add-cancel" @click="cancelAdd">Cancel</button>
        </form>
        <button v-else type="button" class="note-properties__add-trigger" @click="startAdd">
          <span class="material-symbols-outlined !text-[15px]" aria-hidden="true">add</span>
          Add property
        </button>
      </div>
    </template>
  </section>
</template>

<script setup lang="ts">
import { computed, nextTick, ref } from 'vue'
import {
  inferPropertyType,
  normalizeTag,
  type FrontmatterData,
  type PropertyType,
} from '~/utils/frontmatter'

interface PropertyRow {
  key: string
  value: unknown
  type: PropertyType
  locked: boolean
}

const props = withDefaults(defineProps<{
  data?: FrontmatterData | null
  parseError?: string
  readonly?: boolean
  showId?: boolean
}>(), {
  data: null,
  parseError: '',
  readonly: false,
  showId: false,
})

const emit = defineEmits<{ (e: 'update:data', data: FrontmatterData): void }>()

const tagDrafts = ref<Record<string, string>>({})
const adding = ref(false)
const newKey = ref('')
const newType = ref<PropertyType>('text')
const addKeyInput = ref<HTMLInputElement | null>(null)

// `uuid` is an internal join key with no human meaning — always hidden. `id` is
// the node identity: hidden by default (toggle in editor settings), and shown
// locked (with the lock icon) when revealed.
const LOCKED_KEYS = new Set(['id'])

const rows = computed<PropertyRow[]>(() =>
  Object.entries(props.data || {})
    .filter(([key]) => key !== 'uuid' && (key !== 'id' || props.showId))
    .map(([key, value]) => ({
      key,
      value,
      type: inferPropertyType(key, value),
      locked: LOCKED_KEYS.has(key),
    })),
)

// Render nothing when there's nothing to show and nothing to add (reading mode
// over a note with no frontmatter shouldn't leave an empty band).
const visible = computed(() => Boolean(props.parseError) || rows.value.length > 0 || !props.readonly)

const canAdd = computed(() => {
  const key = normalizeKey(newKey.value)
  return Boolean(key) && !Object.prototype.hasOwnProperty.call(props.data || {}, key)
})

function emitUpdate(next: FrontmatterData): void {
  emit('update:data', next)
}

function commitScalar(key: string, event: Event): void {
  if (props.readonly || LOCKED_KEYS.has(key)) return
  const value = (event.target as HTMLInputElement).value
  setTimeout(() => {
    emitUpdate({ ...(props.data || {}), [key]: value })
  }, 0)
}

function addTag(key: string): void {
  // Defer execution so that if this blur was triggered by a click on the
  // CodeMirror editor, we don't dispatch a state change while CM is handling
  // its mousedown/focus event.
  setTimeout(() => {
    const tag = normalizeTag(tagDrafts.value[key] || '')
    if (!tag) return
    const values = listValue((props.data || {})[key])
    if (!values.includes(tag)) emitUpdate({ ...(props.data || {}), [key]: [...values, tag] })
    tagDrafts.value[key] = ''
  }, 0)
}

// Comma or space commits the current tag, matching how people type tag lists.
function onTagSeparator(key: string, event: KeyboardEvent): void {
  if (event.key === ',' || event.key === ' ') {
    event.preventDefault()
    addTag(key)
  }
}

function removeTag(key: string, tag: string): void {
  emitUpdate({
    ...(props.data || {}),
    [key]: listValue((props.data || {})[key]).filter((item) => item !== tag),
  })
}

function removeProperty(key: string): void {
  const next = { ...(props.data || {}) }
  delete next[key]
  emitUpdate(next)
}

function startAdd(): void {
  adding.value = true
  nextTick(() => addKeyInput.value?.focus())
}

function cancelAdd(): void {
  adding.value = false
  newKey.value = ''
  newType.value = 'text'
}

function confirmAdd(): void {
  if (!canAdd.value) return
  const key = normalizeKey(newKey.value)
  emitUpdate({ ...(props.data || {}), [key]: initialValueForType(newType.value) })
  cancelAdd()
}

function blurTarget(event: Event): void {
  ;(event.target as HTMLInputElement).blur()
}

function listValue(value: unknown): string[] {
  if (Array.isArray(value)) return value.map((item) => String(item)).filter(Boolean)
  if (typeof value === 'string' && value.trim()) {
    return value.split(',').map((item) => normalizeTag(item)).filter(Boolean)
  }
  return []
}

function stringValue(value: unknown): string {
  if (value === null || value === undefined) return ''
  if (typeof value === 'string') return value
  if (typeof value === 'number' || typeof value === 'boolean') return String(value)
  return JSON.stringify(value)
}

function normalizeKey(value: string): string {
  return value.trim().replace(/\s+/g, '-')
}

function initialValueForType(type: PropertyType): unknown {
  if (type === 'list') return []
  if (type === 'date') return new Date().toISOString().slice(0, 10)
  return ''
}
</script>

<style scoped>
.note-properties {
  display: flex;
  flex-direction: column;
  gap: 1px;
  padding: var(--spacing-base) 0;
  border-bottom: 1px solid var(--color-border-hairline);
  margin-bottom: var(--spacing-component);
  font-size: 13px;
}

.note-properties__notice {
  display: flex;
  align-items: center;
  gap: var(--spacing-base);
  padding: var(--spacing-base) 0;
  color: var(--color-status-failed-text);
}

.note-properties__row {
  display: grid;
  grid-template-columns: minmax(96px, 168px) minmax(0, 1fr) auto;
  align-items: start;
  gap: var(--spacing-component);
  border-radius: var(--radius-lg);
  padding: 3px var(--spacing-base);
  margin: 0 calc(-1 * var(--spacing-base));
  transition: background-color 120ms ease;
}

.note-properties__row:hover,
.note-properties__row:focus-within {
  background-color: color-mix(in srgb, var(--color-surface-hover) 55%, transparent);
}

.note-properties__key {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding-top: 4px;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-family: var(--font-mono-data);
  font-size: 12px;
  color: var(--color-text-faint);
  transition: color 120ms ease;
}

.note-properties__row:hover .note-properties__key {
  color: var(--color-text-muted);
}

.note-properties__lock {
  color: var(--color-text-dim);
}

/* Inputs read as plain text until focused — no boxy chrome at rest. */
.note-properties__input {
  width: 100%;
  min-width: 0;
  background: transparent;
  border: none;
  outline: none;
  padding: 4px 6px;
  margin: 0 -6px;
  border-radius: var(--radius-lg);
  font-family: inherit;
  font-size: 13px;
  line-height: 1.5;
  color: var(--color-text-primary);
  transition: background-color 120ms ease;
}

.note-properties__input::placeholder {
  color: var(--color-text-dim);
}

.note-properties__input:focus {
  background-color: var(--color-bg-base);
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--color-primary) 45%, transparent);
}

.note-properties__input--locked {
  color: var(--color-text-muted);
  cursor: default;
}

.note-properties__input[type='date'] {
  color-scheme: dark;
}

.note-properties__tags {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 5px;
  padding-top: 2px;
}

.note-properties__tag {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  height: 21px;
  padding: 0 7px;
  border-radius: var(--radius-full);
  background-color: color-mix(in srgb, var(--color-node-note) 14%, transparent);
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--color-node-note) 26%, transparent);
  color: var(--color-text-primary);
  font-size: 12px;
  line-height: 1;
  max-width: 100%;
}

.note-properties__tag-hash {
  color: color-mix(in srgb, var(--color-node-note) 80%, var(--color-text-faint));
}

.note-properties__tag-x {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 14px;
  height: 14px;
  margin-left: 1px;
  border-radius: var(--radius-full);
  color: var(--color-text-faint);
  cursor: pointer;
  transition: color 120ms ease, background-color 120ms ease;
}

.note-properties__tag-x:hover {
  background-color: var(--color-surface-hover);
  color: var(--color-text-primary);
}

.note-properties__tag-input {
  width: auto;
  flex: 1 1 60px;
  min-width: 60px;
  margin: 0;
  padding: 2px 4px;
}

.note-properties__remove {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  margin-top: 2px;
  border-radius: var(--radius-lg);
  color: var(--color-text-faint);
  opacity: 0;
  cursor: pointer;
  transition: opacity 120ms ease, color 120ms ease, background-color 120ms ease;
}

.note-properties__row:hover .note-properties__remove,
.note-properties__row:focus-within .note-properties__remove,
.note-properties__remove:focus-visible {
  opacity: 1;
}

.note-properties__remove:hover {
  background-color: var(--color-surface-hover);
  color: var(--color-status-failed-text);
}

.note-properties__add {
  margin-top: 2px;
}

.note-properties__add-trigger {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 4px 6px;
  margin-left: -6px;
  border-radius: var(--radius-lg);
  color: var(--color-text-faint);
  font-family: var(--font-mono-data);
  font-size: 12px;
  cursor: pointer;
  transition: color 120ms ease;
}

.note-properties__add-trigger:hover,
.note-properties__add-trigger:focus-visible {
  color: var(--color-text-primary);
}

.note-properties__add-form {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--spacing-base);
  padding-top: var(--spacing-base);
}

.note-properties__add-key {
  width: auto;
  flex: 1 1 140px;
  background-color: var(--color-bg-base);
  box-shadow: inset 0 0 0 1px var(--color-border-hairline);
}

.note-properties__add-type {
  height: 28px;
  padding: 0 var(--spacing-base);
  border-radius: var(--radius-lg);
  border: 1px solid var(--color-border-hairline);
  background-color: var(--color-bg-base);
  color: var(--color-text-primary);
  font-family: var(--font-body);
  font-size: 12px;
  outline: none;
}

.note-properties__add-confirm,
.note-properties__add-cancel {
  height: 28px;
  padding: 0 12px;
  border-radius: var(--radius-lg);
  border: 1px solid transparent;
  font-size: 12px;
  cursor: pointer;
  transition: background-color 120ms ease, color 120ms ease, border-color 120ms ease;
}

.note-properties__add-confirm {
  border-color: color-mix(in srgb, var(--color-primary) 40%, transparent);
  background-color: var(--color-primary);
  color: var(--color-on-primary);
}

.note-properties__add-confirm:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}

.note-properties__add-cancel {
  color: var(--color-text-muted);
}

.note-properties__add-cancel:hover {
  background-color: var(--color-surface-hover);
  color: var(--color-text-primary);
}

/* Touch: keep the remove + add affordances reachable without hover. */
@media (pointer: coarse) {
  .note-properties__remove,
  .note-properties__add-trigger {
    opacity: 1;
  }
}
</style>
