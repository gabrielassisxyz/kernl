---
name: Kernl
description: Local-first cognitive substrate for notes, tasks, graph context, and agent execution.
colors:
  on-surface-variant: "#c5c6d0"
  error-container: "#93000a"
  surface-hover: "#1D222D"
  primary-fixed-dim: "#b4c5fe"
  error: "#ffb4ab"
  on-primary: "#1d2e5e"
  on-error-container: "#ffdad6"
  border-hairline: "#1B2029"
  text-dim: "#444A57"
  bg-base: "#0F1217"
  background: "#0F1217"
  surface-bright: "#39393d"
  primary-fixed: "#dbe1ff"
  inverse-on-surface: "#303034"
  surface-container-lowest: "#0d0e11"
  on-secondary: "#2c303b"
  tertiary-container: "#ac8d41"
  status-passed: "#6D9A78"
  secondary-fixed: "#dfe2f0"
  inverse-surface: "#e4e2e6"
  on-error: "#690005"
  tertiary-fixed: "#ffdf97"
  surface-container-highest: "#343438"
  text-primary: "#D6DBE3"
  surface: "#131317"
  surface-dim: "#131317"
  primary: "#b4c5fe"
  primary-container: "#7f8fc5"
  outline: "#8f909a"
  secondary-fixed-dim: "#c3c6d4"
  surface-variant: "#343438"
  surface-container: "#1f1f23"
  text-muted: "#9098A7"
  on-primary-fixed-variant: "#344576"
  on-primary-fixed: "#031848"
  on-secondary-fixed: "#171b25"
  bg-elevated: "#141821"
  surface-overlay: "#181C26"
  on-tertiary-fixed-variant: "#5a4300"
  on-tertiary: "#3e2e00"
  surface-container-high: "#292a2d"
  on-secondary-fixed-variant: "#434752"
  tertiary-fixed-dim: "#e5c270"
  on-secondary-container: "#b5b8c5"
  inverse-primary: "#4c5c8f"
  on-tertiary-container: "#362800"
  on-primary-container: "#152757"
  secondary-container: "#454954"
  tertiary: "#e5c270"
  surface-container-low: "#1b1b1f"
  status-failed: "#C2675C"
  on-surface: "#e4e2e6"
  on-background: "#e4e2e6"
  status-running: "#8089A0"
  status-active: "#4FA89C"
  text-faint: "#8A93A6"
  on-tertiary-fixed: "#251a00"
  outline-variant: "#45464f"
  border-default: "#242935"
  status-gate: "#C99A4A"
  secondary: "#c3c6d4"
  da-accent: "#6B7BB0"
  node-note: "#7B8FE0"
  node-bookmark-list: "#D49A6A"
  node-memory-claim: "#B58BD4"
  node-chat-session: "#5FA8C4"
  node-capture: "#D98E73"
  node-memory-refutation: "#C76B7A"
  node-decision: "#5FB39A"
  da-accent-text: "#8E9ED2"
  status-failed-text: "#D98279"
typography:
  display:
    fontFamily: "\"IBM Plex Sans\", system-ui, -apple-system, BlinkMacSystemFont, Segoe UI, Roboto, Helvetica, Arial, sans-serif"
    fontSize: "20px"
    fontWeight: 600
    lineHeight: "28px"
    letterSpacing: "-0.02em"
  headline:
    fontFamily: "\"IBM Plex Sans\", system-ui, -apple-system, BlinkMacSystemFont, Segoe UI, Roboto, Helvetica, Arial, sans-serif"
    fontSize: "16px"
    fontWeight: 600
    lineHeight: "24px"
    letterSpacing: "-0.01em"
  body:
    fontFamily: "\"IBM Plex Sans\", system-ui, -apple-system, BlinkMacSystemFont, Segoe UI, Roboto, Helvetica, Arial, sans-serif"
    fontSize: "13px"
    fontWeight: 400
    lineHeight: "20px"
  label:
    fontFamily: "\"IBM Plex Sans\", system-ui, -apple-system, BlinkMacSystemFont, Segoe UI, Roboto, Helvetica, Arial, sans-serif"
    fontSize: "12px"
    fontWeight: 600
    lineHeight: "16px"
    letterSpacing: "0em"
  mono-data:
    fontFamily: "\"IBM Plex Mono\", ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, Liberation Mono, Courier New, monospace"
    fontSize: "12px"
    fontWeight: 500
    lineHeight: "16px"
  rail-label:
    fontFamily: "\"IBM Plex Sans\", system-ui, -apple-system, BlinkMacSystemFont, Segoe UI, Roboto, Helvetica, Arial, sans-serif"
    fontSize: "10px"
    fontWeight: 400
    lineHeight: "12px"
    letterSpacing: "0em"
  symbol:
    fontFamily: "\"Material Symbols Outlined\""
    fontSize: "24px"
    fontWeight: 300
    lineHeight: "1"
    fontVariation: "'FILL' 0, 'wght' 300, 'GRAD' -25, 'opsz' 24"
rounded:
  none: "0px"
  sm: "2px"
  md: "4px"
  full: "6px"
spacing:
  tight: "4px"
  base: "8px"
  component: "16px"
  section: "24px"
  margin: "32px"
  break: "64px"
  rail-width: "60px"
components:
  button-primary:
    backgroundColor: "{colors.primary}"
    textColor: "{colors.on-primary}"
    typography: "{typography.body}"
    rounded: "{rounded.none}"
    padding: "0 16px"
    height: "36px"
  button-secondary:
    backgroundColor: "{colors.surface-container-low}"
    textColor: "{colors.text-muted}"
    typography: "{typography.body}"
    rounded: "{rounded.none}"
    padding: "0 16px"
    height: "36px"
  input-default:
    backgroundColor: "{colors.bg-base}"
    textColor: "{colors.text-primary}"
    typography: "{typography.body}"
    rounded: "{rounded.none}"
    padding: "0 16px"
    height: "36px"
  modal-panel:
    backgroundColor: "{colors.surface-overlay}"
    textColor: "{colors.text-primary}"
    rounded: "{rounded.none}"
  toast-default:
    backgroundColor: "{colors.surface-container-high}"
    textColor: "{colors.text-primary}"
    typography: "{typography.body}"
    rounded: "{rounded.none}"
    padding: "8px 16px"
  navigation-rail-item:
    backgroundColor: "transparent"
    textColor: "{colors.text-muted}"
    typography: "{typography.rail-label}"
    rounded: "{rounded.none}"
    width: "{spacing.rail-width}"
    height: "40px"
---

# Design System: Kernl

## Overview

**Creative North Star: "The Operator Console"**

Kernl is a dark, dense product interface for a solo developer working in a local-first desktop environment. The system should feel like a native execution console fused with a graph-aware note vault: direct, structured, low-ornament, and ready for repeated use over long sessions.

The design philosophy is restrained. Tokens, rails, panes, tables, editors, and modals all exist to keep judgment and execution visible without turning the product into a generic SaaS dashboard. The UI should carry the product promise from PRODUCT.md: "the interface should feel like a native, responsive tool that stays out of the way so complex data and workflows can shine."

Kernl explicitly rejects Jira's bureaucracy, Notion's soft document-first whitespace, and generic SaaS dashboard decoration. No decorative gradients, glassmorphism, oversized colorful cards, or exaggerated drop shadows. The design serves the data.

**Key Characteristics:**
- Dense dark workspace with a near-black base and tonal surface layering.
- Small radii, crisp borders, and minimal elevation.
- One quiet blue-violet DA/accent vocabulary for primary intent and assistant context.
- Keyboard-friendly thin icon rail with restrained micro-labels, persistent status bar, and compact workflow panes.
- Shared UI primitives in `web/components/ui/` for buttons, modals, fields, states, and toasts.

## Colors

The palette is near-black and cool-neutral, with a single blue-violet assistant/accent family and muted semantic status colors.

### Primary
- **DA Blue-Violet** (`da-accent`): The assistant/context accent. Use for DA chips, DA-authored editor marks, assistant affordances, and subtle contextual emphasis.
- **Primary Action Blue** (`primary`, `primary-container`, `on-primary`): The strongest interactive action color. Use sparingly for primary buttons, focus affordances, current selection, and graph/node emphasis.

### Secondary
- **Muted System Blue-Grey** (`secondary`, `secondary-container`): Supporting Material-derived roles. Use only where existing Material role mappings require it; do not introduce it as a second brand accent.

### Tertiary
- **Gate Gold** (`tertiary`, `tertiary-container`, `status-gate`): Human gate, waiting, and review states. It signals judgment required, not decoration.

### Neutral
- **Console Base** (`bg-base`, `background`): App background and deepest canvas.
- **Pane Surface** (`surface`, `surface-dim`, `bg-elevated`): Rails, headers, sidebars, editor gutters, and persistent panes.
- **Raised Surface** (`surface-overlay`, `surface-container-*`): Modals, toasts, cards, dropdowns, and selected row backgrounds.
- **Hover Surface** (`surface-hover`): Hover, active, selected, and pressed state backgrounds.
- **Text Stack** (`text-primary`, `text-muted`, `text-faint`, `text-dim`, `on-surface`): Primary copy, secondary labels, placeholders, faint metadata, and brightest text on dark surfaces.
- **Border Stack** (`border-hairline`, `border-default`, `outline`, `outline-variant`): Dividers, panel boundaries, control borders, and stronger outlines.

### Named Rules

**The Accent Scarcity Rule.** `primary` and `da-accent` are functional signals, not decoration. If more than roughly ten percent of a screen is accented, the screen is shouting.

**The Token Exception Rule.** Notes/editor styling, SVG graph presentation attributes, and node-type colors must use design tokens or CSS variables. Document any future literal-color exception with the reason it cannot use CSS variables.

**The Status Semantics Rule.** Use `status-passed`, `status-failed`, `status-active`, `status-running`, and `status-gate` only for state. Never use semantic colors to make a card look lively.

## Typography

**Display Font:** IBM Plex Sans with system UI fallback.
**Body Font:** IBM Plex Sans with system UI fallback.
**Label/Mono Font:** IBM Plex Sans for labels; IBM Plex Mono with `ui-monospace` fallback for metadata, IDs, timestamps, counts, and command-like hints.
**Icon Font:** Material Symbols Outlined, self-hosted as a subset and rendered with low optical weight.

**Character:** The type is product-native and unobtrusive. It should feel like a serious desktop tool: compact, readable, and stable under dense information rather than expressive or editorial.

### Hierarchy

- **Display** (600, 20px, 28px, -0.02em): Page titles and major route headers. Use fixed sizing; no fluid hero typography.
- **Headline** (600, 16px, 24px, -0.01em): Modal titles, panel titles, section titles, and card headings.
- **Title** (600, 13px-16px, 20px-24px): Dense item titles where `headline` would be too loud.
- **Body** (400, 13px, 20px): Main UI copy, state descriptions, row text, and compact prose. Keep long explanatory prose near 65-75ch.
- **Label** (600, 12px, 16px, 0em): Form labels, panel labels, and workflow section labels. Use uppercase only where the surrounding UI is already metadata-heavy.
- **Rail Label** (400, 10px, 12px, 0em): Micro-labels in the 60px navigation rail. Keep them light; the rail should orient without becoming a bold text column.
- **Mono Data** (500, 12px, 16px): IDs, paths, status bar text, shortcuts, counters, and structured agent logs.
- **Symbols** (300, 24px, `FILL 0`, `wght 300`, `GRAD -25`, `opsz 24`): Material Symbols are thin by default. Rail icons scope down to roughly 19px and weight 260 so the shell matches the fine-line graph and editor aesthetic.

### Named Rules

**The Product Type Rule.** Fixed rem/px product typography only. Do not add clamp-based display scales or landing-page hero type inside the app.

**The Metadata Voice Rule.** Use monospace for data and machine context; use body sans for human-facing explanation and recovery text.

**The Thin Symbol Rule.** Never use `font-bold` on Material Symbols in persistent chrome. If an icon needs more emphasis, use color, state background, or a larger touch target before increasing stroke weight.

## Elevation

Kernl is flat by default. Depth is conveyed through tonal surface changes, 1px borders, modal backdrops, selected row backgrounds, and z-index layering. Decorative shadows are not part of the core system.

### Shadow Vocabulary

- **Status Glow** (`0 0 8px rgba(...)`): Tiny status-dot glow only, used for live/connected indicators.
- **No Card Shadow**: Cards, buttons, modals, and panels do not pair borders with large soft shadows.

### Named Rules

**The Tonal Layer Rule.** Move through `bg-base`, `surface`, `bg-elevated`, `surface-overlay`, and `surface-container-*` before reaching for shadow.

**The Semantic Z Rule.** Use the semantic z-index utilities from `web/assets/css/tailwind.css`: `z-dropdown`, `z-modal`, `z-toast`, `z-tooltip`. Do not introduce arbitrary `999` values.

## Components

### Buttons

- **Shape:** Square product controls with minimal rounding (`radius: 0px`; `rounded` maps to the project's small radius).
- **Primary:** `UiButton variant="primary"` uses `primary` background, `on-primary` text, a 1px `primary/40` border, 36px default height, and 16px horizontal padding.
- **Secondary:** `surface-container-low` background, hairline border, muted text, and hover transition to `surface-hover` + `text-primary`.
- **Ghost:** Transparent at rest, surface-hover on hover. Use for low-commitment actions and modal cancel actions.
- **Danger / Success / Accent:** Tinted semantic backgrounds with full borders. Use for actual semantic actions only.
- **Hover / Focus:** 150ms color transitions. Focus uses a visible primary border/ring. Loading uses an inline Material Symbols spinner inside the button.

### Chips

- **Style:** Compact bordered chips with muted text or semantic text. Use full borders and background tints.
- **State:** Selected chips use the relevant semantic tint. Unselected chips stay neutral. Never use a thick side stripe to signal selection.

### Cards / Containers

- **Corner Style:** Square-to-subtle corners only (`0px`-`4px`; `6px` only for pill-like compact elements).
- **Background:** `surface`, `surface-container-low`, or `surface-overlay` depending on depth.
- **Shadow Strategy:** No decorative card shadows. Use tonal layering and borders.
- **Border:** `border-hairline` for subtle containers, `border-default` for raised panels and modal cards.
- **Internal Padding:** `component` (16px) for cards and rows; `section` (24px) for major modal/panel bodies.

### Inputs / Fields

- **Style:** `UiInput`, `UiTextarea`, and `UiSelect` use `bg-base`, hairline border, `text-primary`, `text-muted` placeholders, and 36px default control height.
- **Focus:** Border shifts to `primary/70` or `da-accent` for DA/editor contexts. Do not add glow-heavy focus treatments.
- **Error / Disabled:** Disabled controls reduce opacity and show a disabled cursor. Error text uses `status-failed-text` through `UiField` or contextual error states.
- **Field Wrapper:** `UiField` owns labels, hints, and field-level errors.

### Navigation

- **Icon Rail:** 60px fixed rail, `surface` background, hairline right border, 40px route items, thin 19px Material Symbols, and 10px micro-labels. Default state is `text-muted`; hover moves to `text-primary`; active state uses `surface-hover` + `primary`.
- **Status Bar:** 26px bottom bar, `surface-container-low`, mono data typography, hairline top border, compact live/sync/vault information.
- **DA Panel:** Closed by default; auto-opens only on DA-specific routes (`/chat`, `/config/da`). It overlays the right edge instead of permanently shrinking the workspace.

### Modals / Sheets

- **Shell:** `UiModal` owns backdrop, escape handling, centered/top alignment, max width, reduced-motion-safe transitions, and semantic modal z-index.
- **Surface:** `surface-overlay`, `border-default`, no decorative shadow.
- **Content:** Header, scrollable body, and footer are separated by hairline borders. Footer uses `surface-container-low`.

### Loading, Empty, Error, Toast

- **Loading:** Use `UiSkeleton` for content surfaces. Use spinners only inside controls or tiny inline status indicators.
- **Empty:** `UiEmptyState` must include one clear explanation and, when useful, one next action.
- **Error:** `UiErrorState` must distinguish failure from absence and include retry when recovery is possible.
- **Toast:** `UiToast` appears bottom-left or bottom-center, uses `surface-container-high`, hairline border, body text, and an optional ghost action.

### Signature Components

- **Graph Canvas:** Full-bleed SVG graph surface whose SVG presentation attributes and node-type colors reference design-token CSS variables.
- **CodeMirror Notes Editor:** Tokenized dark editor shell; editor theme uses CSS variables and `color-mix()` for DA-authored regions and wikilink pills.
- **Agent Log Pane:** Dense monospace event stream with semantic success/failure coloring and expandable tool results.

## Do's and Don'ts

### Do:

- **Do** use `web/components/ui/` before writing a new local button, modal, input, select, toast, skeleton, empty state, or error state.
- **Do** keep surfaces dark, dense, and structurally quiet; the UI exists so complex data and workflows can shine.
- **Do** use skeletons for content loading and retry-capable error states for API failures.
- **Do** keep DA contextual: available everywhere, visually dominant only when summoned or on DA-specific routes.
- **Do** keep persistent navigation visually light: thin symbols, 10px micro-labels, and state backgrounds instead of heavy icon weight.
- **Do** document any future literal color exceptions with the reason they cannot use CSS variables.
- **Do** preserve keyboard-first affordances and visible focus on rows, buttons, fields, and modal actions.

### Don't:

- **Don't** make Kernl feel like Jira: bureaucratic, cluttered, slow, or enterprise-heavy.
- **Don't** make Kernl feel like Notion: too much whitespace, too document-centric, too soft, or generic.
- **Don't** use generic SaaS dashboard patterns: decorative gradients, glassmorphism, oversized colorful cards, exaggerated drop shadows, or hero-metric layouts.
- **Don't** add border-left or border-right accents greater than 1px on cards, callouts, rows, or alerts. Use full borders, tints, icons, or state text instead.
- **Don't** pair a 1px border with a large soft shadow on the same card/button/modal.
- **Don't** make the side rail feel like a bold menu. Avoid `font-bold`, oversized labels, or heavy Material Symbols in persistent navigation.
- **Don't** add arbitrary z-index values. Use semantic z-index utilities.
- **Don't** introduce raw hex in Vue templates, editor styles, or SVG graph presentation unless it is a documented literal-color exception.
- **Don't** add new typography families or fluid display scales without a deliberate design-system revision.
