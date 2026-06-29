import { reactive, computed, watch } from 'vue'

// Per-user notes-editor preferences. These used to be hardcoded in the editor /
// preview modules; surfacing them here lets the toolbar settings popover drive
// them and persists the choice in localStorage (frontend-only — there's no
// server-side notion of editor chrome). A single shared reactive object backs
// every editor instance and the popover so they stay in lockstep.

export type ViewMode = 'source' | 'live' | 'reading'
export type EditorFont = 'sans' | 'serif' | 'mono'

export interface EditorSettings {
  viewMode: ViewMode
  lineNumbers: boolean
  typewriter: boolean
  showId: boolean
  font: EditorFont
  fontSize: number
  headingScale: number
}

const STORAGE_KEY = 'kernl.notes.editor-settings'

const DEFAULTS: EditorSettings = {
  viewMode: 'live',
  lineNumbers: false,
  typewriter: false,
  showId: false,
  font: 'sans',
  fontSize: 15,
  headingScale: 1,
}

export const FONT_SIZE_MIN = 13
export const FONT_SIZE_MAX = 22
export const HEADING_SCALE_MIN = 0.85
export const HEADING_SCALE_MAX = 1.4

// Resolved font stacks per choice. Serif is a system stack (no extra webfont):
// the body/mono families are already self-hosted IBM Plex.
const FONT_STACKS: Record<EditorFont, string> = {
  sans: 'var(--font-body)',
  serif: '"Iowan Old Style", "Palatino Linotype", "Source Serif 4", Georgia, ui-serif, serif',
  mono: 'var(--font-mono-data)',
}

function clamp(value: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, value))
}

function load(): EditorSettings {
  if (typeof window === 'undefined') return { ...DEFAULTS }
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY)
    if (!raw) return { ...DEFAULTS }
    const parsed = JSON.parse(raw) as Partial<EditorSettings>
    return {
      viewMode: parsed.viewMode ?? DEFAULTS.viewMode,
      lineNumbers: parsed.lineNumbers ?? DEFAULTS.lineNumbers,
      typewriter: parsed.typewriter ?? DEFAULTS.typewriter,
      showId: parsed.showId ?? DEFAULTS.showId,
      font: parsed.font ?? DEFAULTS.font,
      fontSize: clamp(Number(parsed.fontSize) || DEFAULTS.fontSize, FONT_SIZE_MIN, FONT_SIZE_MAX),
      headingScale: clamp(Number(parsed.headingScale) || DEFAULTS.headingScale, HEADING_SCALE_MIN, HEADING_SCALE_MAX),
    }
  } catch {
    return { ...DEFAULTS }
  }
}

// Module-level singleton: one source of truth for all editor instances.
const settings = reactive<EditorSettings>(load())

let persisting = false
function startPersisting() {
  if (persisting || typeof window === 'undefined') return
  persisting = true
  watch(
    settings,
    (value) => {
      try {
        window.localStorage.setItem(STORAGE_KEY, JSON.stringify(value))
      } catch {
        // Ignore quota/availability errors — settings just won't persist.
      }
    },
    { deep: true },
  )
}

export function useEditorSettings() {
  startPersisting()

  // CSS custom properties consumed by the editor container + preview theme, so
  // restyling is centralized (no per-rule JS recompute).
  const styleVars = computed(() => ({
    '--notes-font': FONT_STACKS[settings.font],
    '--notes-font-size': `${settings.fontSize}px`,
    '--notes-heading-scale': String(settings.headingScale),
  }))

  function setViewMode(mode: ViewMode) {
    settings.viewMode = mode
  }
  function setFont(font: EditorFont) {
    settings.font = font
  }
  function setFontSize(size: number) {
    settings.fontSize = clamp(Math.round(size), FONT_SIZE_MIN, FONT_SIZE_MAX)
  }
  function setHeadingScale(scale: number) {
    settings.headingScale = clamp(Number(scale.toFixed(2)), HEADING_SCALE_MIN, HEADING_SCALE_MAX)
  }
  function reset() {
    Object.assign(settings, DEFAULTS)
  }

  return { settings, styleVars, setViewMode, setFont, setFontSize, setHeadingScale, reset }
}
