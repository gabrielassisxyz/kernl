<template>
  <div class="editor-toolbar">
    <!-- Left: collapse the vault sidebar. -->
    <button
      type="button"
      class="tbtn"
      :title="sidebarCollapsed ? 'Show sidebar' : 'Hide sidebar'"
      :aria-label="sidebarCollapsed ? 'Show sidebar' : 'Hide sidebar'"
      @click="$emit('toggle-sidebar')"
    >
      <span class="material-symbols-outlined !text-[18px]" aria-hidden="true">
        {{ sidebarCollapsed ? 'left_panel_open' : 'left_panel_close' }}
      </span>
    </button>

    <!-- Autosave status: the editor has no Save button by design, so this chip
         is the user's only confirmation that edits reached disk (Ctrl+S flushes). -->
    <span
      v-if="saveState"
      class="save-chip"
      :class="`save-chip--${saveState}`"
      role="status"
      :title="saveState === 'dirty' ? 'Unsaved changes — autosaves shortly, Ctrl+S to save now' : undefined"
    >
      <span class="save-chip__dot" aria-hidden="true"></span>
      {{ SAVE_LABELS[saveState] }}
    </span>

    <div class="grow"></div>

    <!-- View mode: source / live preview / reading. -->
    <div class="seg" role="group" aria-label="View mode">
      <button
        v-for="mode in VIEW_MODES"
        :key="mode.id"
        type="button"
        class="seg-btn"
        :class="{ 'seg-btn--active': settings.viewMode === mode.id }"
        :title="mode.title"
        :aria-label="mode.title"
        :aria-pressed="settings.viewMode === mode.id"
        @click="setViewMode(mode.id)"
      >
        <span class="material-symbols-outlined !text-[17px]" aria-hidden="true">{{ mode.icon }}</span>
      </button>
    </div>

    <button
      type="button"
      class="tbtn"
      :class="{ 'tbtn--active': settings.typewriter }"
      title="Typewriter mode"
      aria-label="Typewriter mode"
      :aria-pressed="settings.typewriter"
      @click="settings.typewriter = !settings.typewriter"
    >
      <span class="material-symbols-outlined !text-[18px]" aria-hidden="true">keyboard</span>
    </button>

    <!-- Settings popover. -->
    <div ref="settingsAnchor" class="settings-anchor">
      <button
        type="button"
        class="tbtn"
        :class="{ 'tbtn--active': settingsOpen }"
        title="Editor settings"
        aria-label="Editor settings"
        :aria-expanded="settingsOpen"
        @click="settingsOpen = !settingsOpen"
      >
        <span class="material-symbols-outlined !text-[18px]" aria-hidden="true">tune</span>
      </button>

      <div v-if="settingsOpen" class="settings-pop z-dropdown" role="dialog" aria-label="Editor settings">
        <div class="settings-row">
          <span class="settings-label">
            <span class="material-symbols-outlined !text-[16px]" aria-hidden="true">format_list_numbered</span>
            Line numbers
          </span>
          <button
            type="button"
            class="switch"
            :class="{ 'switch--on': settings.lineNumbers }"
            role="switch"
            :aria-checked="settings.lineNumbers"
            aria-label="Line numbers"
            @click="settings.lineNumbers = !settings.lineNumbers"
          ><span class="switch-thumb"></span></button>
        </div>

        <div class="settings-row">
          <span class="settings-label">
            <span class="material-symbols-outlined !text-[16px]" aria-hidden="true">keyboard</span>
            Typewriter
          </span>
          <button
            type="button"
            class="switch"
            :class="{ 'switch--on': settings.typewriter }"
            role="switch"
            :aria-checked="settings.typewriter"
            aria-label="Typewriter mode"
            @click="settings.typewriter = !settings.typewriter"
          ><span class="switch-thumb"></span></button>
        </div>

        <div class="settings-row">
          <span class="settings-label">
            <span class="material-symbols-outlined !text-[16px]" aria-hidden="true">lock</span>
            Show note ID
          </span>
          <button
            type="button"
            class="switch"
            :class="{ 'switch--on': settings.showId }"
            role="switch"
            :aria-checked="settings.showId"
            aria-label="Show note ID"
            @click="settings.showId = !settings.showId"
          ><span class="switch-thumb"></span></button>
        </div>

        <div class="settings-divider"></div>

        <div class="settings-row settings-row--stack">
          <span class="settings-label">
            <span class="material-symbols-outlined !text-[16px]" aria-hidden="true">text_fields</span>
            Font
          </span>
          <div class="seg seg--wide" role="group" aria-label="Editor font">
            <button
              v-for="f in FONTS"
              :key="f.id"
              type="button"
              class="seg-btn seg-btn--text"
              :class="{ 'seg-btn--active': settings.font === f.id }"
              :aria-pressed="settings.font === f.id"
              @click="setFont(f.id)"
            >{{ f.label }}</button>
          </div>
        </div>

        <div class="settings-row">
          <span class="settings-label">Font size</span>
          <div class="stepper">
            <button type="button" class="step" aria-label="Decrease font size" @click="setFontSize(settings.fontSize - 1)">−</button>
            <span class="step-value">{{ settings.fontSize }}px</span>
            <button type="button" class="step" aria-label="Increase font size" @click="setFontSize(settings.fontSize + 1)">+</button>
          </div>
        </div>

        <div class="settings-row">
          <span class="settings-label">Heading size</span>
          <div class="stepper">
            <button type="button" class="step" aria-label="Decrease heading size" @click="setHeadingScale(settings.headingScale - 0.05)">−</button>
            <span class="step-value">{{ Math.round(settings.headingScale * 100) }}%</span>
            <button type="button" class="step" aria-label="Increase heading size" @click="setHeadingScale(settings.headingScale + 0.05)">+</button>
          </div>
        </div>

        <div class="settings-divider"></div>

        <button type="button" class="settings-reset" @click="reset">
          <span class="material-symbols-outlined !text-[15px]" aria-hidden="true">refresh</span>
          Reset to defaults
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { useEditorSettings, type EditorFont, type ViewMode } from '~/composables/useEditorSettings'

defineProps<{ sidebarCollapsed?: boolean; saveState?: 'saved' | 'saving' | 'dirty' | 'conflict' }>()
defineEmits<{ (e: 'toggle-sidebar'): void }>()

const SAVE_LABELS: Record<string, string> = {
  saved: 'Saved',
  saving: 'Saving…',
  dirty: 'Unsaved',
  conflict: 'Conflict',
}

const { settings, setViewMode, setFont, setFontSize, setHeadingScale, reset } = useEditorSettings()

const VIEW_MODES: { id: ViewMode; icon: string; title: string }[] = [
  { id: 'source', icon: 'code', title: 'Source' },
  { id: 'live', icon: 'visibility', title: 'Live preview' },
  { id: 'reading', icon: 'chrome_reader_mode', title: 'Reading' },
]

const FONTS: { id: EditorFont; label: string }[] = [
  { id: 'sans', label: 'Sans' },
  { id: 'serif', label: 'Serif' },
  { id: 'mono', label: 'Mono' },
]

const settingsOpen = ref(false)
const settingsAnchor = ref<HTMLElement | null>(null)

// Close the popover on outside click / Escape.
function onPointerDown(event: PointerEvent) {
  if (!settingsOpen.value) return
  if (settingsAnchor.value && !settingsAnchor.value.contains(event.target as Node)) {
    settingsOpen.value = false
  }
}
function onKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape') settingsOpen.value = false
}

onMounted(() => {
  document.addEventListener('pointerdown', onPointerDown, true)
  document.addEventListener('keydown', onKeydown)
})
onBeforeUnmount(() => {
  document.removeEventListener('pointerdown', onPointerDown, true)
  document.removeEventListener('keydown', onKeydown)
})
</script>

<style scoped>
.editor-toolbar {
  display: flex;
  align-items: center;
  gap: 6px;
  height: 42px;
  flex-shrink: 0;
  padding: 0 10px;
  border-bottom: 1px solid var(--color-border-default);
  background-color: var(--color-surface);
}

.grow {
  flex: 1 1 auto;
}

/* Autosave status chip. */
.save-chip {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 0 8px;
  height: 22px;
  border-radius: var(--radius-full);
  font-family: var(--font-mono-data);
  font-size: 11px;
  color: var(--color-text-muted);
}

.save-chip__dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background-color: var(--color-text-faint);
}

.save-chip--saved .save-chip__dot {
  background-color: var(--color-status-passed);
}

.save-chip--saving .save-chip__dot,
.save-chip--dirty .save-chip__dot {
  background-color: var(--color-status-gate);
}

.save-chip--conflict {
  color: var(--color-status-failed-text);
}

.save-chip--conflict .save-chip__dot {
  background-color: var(--color-status-failed-text);
}

/* Toolbar icon button (ghost). */
.tbtn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  border-radius: var(--radius-lg);
  border: 1px solid transparent;
  color: var(--color-text-muted);
  cursor: pointer;
  transition: background-color 120ms ease, color 120ms ease, border-color 120ms ease;
}

.tbtn:hover {
  background-color: var(--color-surface-hover);
  color: var(--color-text-primary);
}

.tbtn--active {
  background-color: var(--color-surface-hover);
  color: var(--color-primary);
}

.tbtn:focus-visible {
  outline: none;
  border-color: color-mix(in srgb, var(--color-primary) 70%, transparent);
}

/* Segmented control. */
.seg {
  display: inline-flex;
  align-items: center;
  padding: 2px;
  gap: 2px;
  border-radius: var(--radius-xl);
  border: 1px solid var(--color-border-hairline);
  background-color: var(--color-surface-container-low);
}

.seg-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  height: 24px;
  min-width: 30px;
  padding: 0 6px;
  border-radius: var(--radius-lg);
  color: var(--color-text-muted);
  cursor: pointer;
  transition: background-color 120ms ease, color 120ms ease;
}

.seg-btn--text {
  font-family: var(--font-body);
  font-size: 12px;
  font-weight: 500;
}

.seg-btn:hover {
  color: var(--color-text-primary);
}

.seg-btn--active {
  background-color: var(--color-surface-hover);
  color: var(--color-text-primary);
}

.seg-btn:focus-visible {
  outline: none;
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--color-primary) 70%, transparent);
}

/* Settings popover. */
.settings-anchor {
  position: relative;
}

.settings-pop {
  position: absolute;
  top: calc(100% + 6px);
  right: 0;
  width: 256px;
  padding: 8px;
  border-radius: var(--radius-xl);
  border: 1px solid var(--color-border-default);
  background-color: var(--color-surface-overlay);
  box-shadow: 0 8px 28px rgba(0, 0, 0, 0.45);
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.settings-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  min-height: 32px;
  padding: 2px 6px;
}

.settings-row--stack {
  flex-direction: column;
  align-items: stretch;
  gap: 6px;
}

.settings-label {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  font-size: 13px;
  color: var(--color-text-primary);
}

.settings-label .material-symbols-outlined {
  color: var(--color-text-faint);
}

.settings-divider {
  height: 1px;
  margin: 6px 0;
  background-color: var(--color-border-hairline);
}

.seg--wide {
  width: 100%;
}

.seg--wide .seg-btn {
  flex: 1 1 0;
}

/* Toggle switch. */
.switch {
  position: relative;
  width: 34px;
  height: 18px;
  flex-shrink: 0;
  border-radius: var(--radius-full);
  background-color: var(--color-surface-container-high);
  box-shadow: inset 0 0 0 1px var(--color-border-default);
  cursor: pointer;
  transition: background-color 150ms ease, box-shadow 150ms ease;
}

.switch--on {
  background-color: var(--color-primary);
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--color-primary) 60%, transparent);
}

.switch-thumb {
  position: absolute;
  top: 2px;
  left: 2px;
  width: 14px;
  height: 14px;
  border-radius: 50%;
  background-color: var(--color-text-primary);
  transition: transform 150ms cubic-bezier(0.22, 1, 0.36, 1);
}

.switch--on .switch-thumb {
  transform: translateX(16px);
  background-color: var(--color-on-primary);
}

.switch:focus-visible {
  outline: none;
  box-shadow: inset 0 0 0 1px var(--color-border-default), 0 0 0 2px color-mix(in srgb, var(--color-primary) 35%, transparent);
}

/* Stepper. */
.stepper {
  display: inline-flex;
  align-items: center;
  gap: 2px;
}

.step {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  border-radius: var(--radius-lg);
  border: 1px solid var(--color-border-hairline);
  background-color: var(--color-surface-container-low);
  color: var(--color-text-muted);
  font-size: 15px;
  line-height: 1;
  cursor: pointer;
  transition: background-color 120ms ease, color 120ms ease;
}

.step:hover {
  background-color: var(--color-surface-hover);
  color: var(--color-text-primary);
}

.step-value {
  min-width: 42px;
  text-align: center;
  font-family: var(--font-mono-data);
  font-size: 12px;
  color: var(--color-text-primary);
}

.settings-reset {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 30px;
  padding: 0 8px;
  border-radius: var(--radius-lg);
  color: var(--color-text-muted);
  font-size: 12px;
  cursor: pointer;
  transition: background-color 120ms ease, color 120ms ease;
}

.settings-reset:hover {
  background-color: var(--color-surface-hover);
  color: var(--color-text-primary);
}

@media (prefers-reduced-motion: reduce) {
  .switch-thumb {
    transition: none;
  }
}

@media (pointer: coarse) {
  .tbtn { width: 36px; height: 36px; }
  .seg-btn { height: 30px; }
}
</style>
