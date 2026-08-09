---
name: Kernl
description: Local-first cognitive substrate for notes, tasks, graph context, and agent execution.
colors:
  on-surface-variant: "#c5c6d0"
  error-container: "#93000a"
  surface-hover: "#191d22"
  primary-fixed-dim: "#b4c5fe"
  error: "#ffb4ab"
  on-primary: "#10161a"
  on-error-container: "#ffdad6"
  border-hairline: "#22262b"
  text-dim: "#4f555c"
  bg-base: "#121417"
  background: "#121417"
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
  text-primary: "#e6e8ea"
  surface: "#16181c"
  surface-dim: "#131317"
  primary: "#4e9e93"
  primary-container: "#54ab9f"
  outline: "#8f909a"
  secondary-fixed-dim: "#c3c6d4"
  surface-variant: "#343438"
  surface-container: "#1f1f23"
  text-muted: "#7d848c"
  on-primary-fixed-variant: "#344576"
  on-primary-fixed: "#031848"
  on-secondary-fixed: "#171b25"
  bg-elevated: "#191c21"
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
  status-failed: "#c2705f"
  on-surface: "#e4e2e6"
  on-background: "#e4e2e6"
  status-running: "#8089A0"
  status-active: "#4f9d7f"
  text-faint: "#5f666e"
  on-tertiary-fixed: "#251a00"
  outline-variant: "#45464f"
  border-default: "#282d34"
  status-gate: "#b3903f"
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
  text-heading: "#f0f2f4"
  text-secondary: "#b6bcc3"
  text-label: "#868d95"
  surface-nav-hover: "#1c2026"
  surface-control-hover: "#262b31"
  surface-chip: "#1d2126"
  border-row: "#1c2025"
  status-done: "#6b7a8c"
  status-archived: "#565d65"
typography:
  display:
    fontFamily: "\"IBM Plex Sans\", system-ui, -apple-system, BlinkMacSystemFont, Segoe UI, Roboto, Helvetica, Arial, sans-serif"
    fontSize: "25px"
    fontWeight: 600
    lineHeight: "32px"
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
  label-caps:
    fontFamily: "\"IBM Plex Sans\", system-ui, -apple-system, BlinkMacSystemFont, Segoe UI, Roboto, Helvetica, Arial, sans-serif"
    fontSize: "10.5px"
    fontWeight: 500
    lineHeight: "16px"
    letterSpacing: "0.1em"
  mono-data:
    fontFamily: "\"IBM Plex Mono\", ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, Liberation Mono, Courier New, monospace"
    fontSize: "11px"
    fontWeight: 400
    lineHeight: "16px"
  symbol:
    fontFamily: "\"Material Symbols Outlined\""
    fontSize: "24px"
    fontWeight: 300
    lineHeight: "1"
    fontVariation: "'FILL' 0, 'wght' 300, 'GRAD' -25, 'opsz' 24"
rounded:
  xs: "2px"
  sm: "3px"
  default: "4px"
  lg: "5px"
  xl: "6px"
  full: "9999px"
spacing:
  tight: "4px"
  base: "8px"
  component: "16px"
  section: "24px"
  margin: "32px"
  break: "64px"
  rail-width: "184px"
components:
  button-primary:
    backgroundColor: "{colors.primary}"
    textColor: "{colors.on-primary}"
    typography: "{typography.body}"
    rounded: "{rounded.default}"
    padding: "0 16px"
    height: "36px"
  button-secondary:
    backgroundColor: "{colors.surface-container-low}"
    textColor: "{colors.text-muted}"
    typography: "{typography.body}"
    rounded: "{rounded.default}"
    padding: "0 16px"
    height: "36px"
  input-default:
    backgroundColor: "{colors.bg-base}"
    textColor: "{colors.text-primary}"
    typography: "{typography.body}"
    rounded: "{rounded.default}"
    padding: "0 16px"
    height: "36px"
  panel-field:
    backgroundColor: "{colors.bg-elevated}"
    textColor: "{colors.text-secondary}"
    typography: "{typography.mono-data}"
    rounded: "{rounded.lg}"
    padding: "0 8px"
    height: "28px"
  list-row:
    backgroundColor: "transparent"
    textColor: "{colors.text-primary}"
    typography: "{typography.body}"
    rounded: "{rounded.default}"
    padding: "8px 10px"
    borderColor: "{colors.border-row}"
    hoverBackgroundColor: "{colors.surface-hover}"
  side-panel:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.text-primary}"
    borderColor: "{colors.border-hairline}"
    width: "384px"
  modal-panel:
    backgroundColor: "{colors.surface-overlay}"
    textColor: "{colors.text-primary}"
    rounded: "{rounded.default}"
  toast-default:
    backgroundColor: "{colors.surface-container-high}"
    textColor: "{colors.text-primary}"
    typography: "{typography.body}"
    rounded: "{rounded.default}"
    padding: "8px 16px"
  navigation-row:
    backgroundColor: "transparent"
    textColor: "{colors.text-secondary}"
    typography: "{typography.body}"
    rounded: "{rounded.lg}"
    padding: "6px 8px"
    height: "29px"
    hoverBackgroundColor: "{colors.surface-nav-hover}"
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
- One muted teal accent for primary intent, spent sparingly.
- A grouped, labelled sidebar, a persistent status bar, and screens built from sections and rows rather than cards.
- Shared UI primitives in `web/components/ui/` for buttons, modals, fields, states, and toasts.

## Colors

The palette is near-black and cool-neutral, with a single blue-violet assistant/accent family and muted semantic status colors.

### Primary
- **Accent Teal** (`primary`, `primary-container`, `on-primary`): The strongest interactive color. Use sparingly for primary buttons, focus affordances, current selection, progress fills, and graph/node emphasis.
- **DA Blue-Violet** (`da-accent`): The assistant accent. Use for DA chips, DA-authored editor marks, and assistant affordances.

The two are not interchangeable, and the line between them is what the token means,
not how it looks. `da-accent` says "the assistant is involved here": its chat
surface, its briefings, its proposed memory writes, its authored regions in the
editor. Everything else that simply needs the interface's accent takes `primary`,
focus rings and switch states included. `da-accent` had drifted into being a second
generic accent, which was invisible while the primary was itself blue; reach for
`primary` unless the surface is genuinely about the assistant.

**Derived accents.** `accent-tint`, `accent-tint-strong`, and `accent-edge` are
`color-mix()` expressions over `primary`, never literals, so they follow the accent
if it changes. They are deliberately absent from the token map above: a resolved hex
copied out of a running page would stop tracking the accent the moment it moved.

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

**Display Font:** IBM Plex Sans with system UI fallback. **Body Font:** IBM Plex Sans with system UI fallback. **Label/Mono Font:** IBM Plex Sans for labels; IBM Plex Mono with `ui-monospace` fallback for metadata, IDs, timestamps, counts, and command-like hints. **Icon Font:** Material Symbols Outlined, self-hosted as a subset and rendered with low optical weight.

**Character:** The type is product-native and unobtrusive. It should feel like a serious desktop tool: compact, readable, and stable under dense information rather than expressive or editorial.

### Hierarchy

- **Display** (600, 25px, 32px, -0.02em): Page titles and major route headers. Use fixed sizing; no fluid hero typography.
- **Headline** (600, 16px, 24px, -0.01em): Modal titles, panel titles, section titles, and card headings.
- **Body** (400, 13px, 20px): Main UI copy, state descriptions, row text, and compact prose. Keep long explanatory prose near 65-75ch.
- **Label Caps** (500, 10.5px, 16px, 0.1em): Section captions and workflow labels, always uppercase. The tracking is what makes it read as a caption rather than as small text.
- **Nav Caption** (`label-caps` at weight 400): The group captions in the sidebar. Keep them light; the sidebar should orient without becoming a bold text column.
- **Mono Data** (400, 11px, 16px): IDs, paths, status bar text, shortcuts, counters, chips, dates, and structured agent logs.
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

- **Shape:** `rounded` (4px), the step that owns buttons and list rows.
- **Primary:** `UiButton variant="primary"` uses `primary` background, `on-primary` text, a 1px `primary/40` border, 36px default height, and 16px horizontal padding.
- **Secondary:** `surface-container-low` background, hairline border, muted text, and hover transition to `surface-hover` + `text-primary`.
- **Ghost:** Transparent at rest, surface-hover on hover. Use for low-commitment actions and modal cancel actions.
- **Danger / Success / Accent / DA:** Tinted backgrounds with full borders. `accent` is the affirmative action tint over `primary`; `da` is the same shape over `da-accent` and belongs only to assistant actions. Use for actual semantic actions only.
- **Hover / Focus:** 150ms color transitions. Focus uses a visible primary border/ring. Loading uses an inline Material Symbols spinner inside the button.

### Chips

- **Style:** Compact bordered chips with muted text or semantic text. Use full borders and background tints.
- **State:** Selected chips use the relevant semantic tint. Unselected chips stay neutral. Never use a thick side stripe to signal selection.

### Cards / Containers

- **Corner Style:** Every step of the scale has one owner, so a value can be judged against what carries it: `xs` (2px) progress bars, `sm` (3px) tag chips, `rounded` (4px) buttons and list rows, `lg` (5px) fields, panels and nav rows, `xl` (6px) cards. `full` is a real pill and belongs only to shapes that are actually round.
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

- **Sidebar:** 184px fixed sidebar, `surface` background, hairline right border. Routes are grouped under uppercase captions and carry a label beside a 16px Material Symbol. A row is 29px at `lg` radius, 6px/8px padding, 10px gap. Default state is `text-secondary`; hover uses `surface-nav-hover`; the active route uses `accent-tint` and `primary`.
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

### Lists, Rows and the Inline Panel

The pattern Tasks and Projects are built from, and the one a new index screen
should follow before inventing another.

- **Sections, not cards.** An index is a stack of `<section>`s under `label-caps` captions with a count. An empty section is dropped rather than rendered as a heading over blank space, which reads as a failed load. Terminal sections (finished work, called-off work) arrive collapsed and remember that independently of each other.
- **Rows.** A fixed grid, so metadata lines up down the page and only the title flexes. `border-row` divides them, which is fainter than a hairline. The whole row is the target for opening the panel; a link inside it keeps its own destination and stops the event.
- **Hover cluster.** Row actions live at the right edge and appear on hover and on keyboard focus. A control that exists only under a pointer is a control a keyboard cannot reach. State markers are the exception: a lit pin says the row is pinned and is also the control that unpins it.
- **Inline panel.** An `aside` of 384px on `surface`, not a dialog: the list beside it stays visible and usable, so trapping focus or claiming `aria-modal` would both be lies. Focus moves in on open, Escape leaves. Editing autosaves per field on blur; Escape and Ctrl/Cmd+Enter flush the field being edited before closing, because with autosave everywhere a key that discarded only the focused field would be a rule nobody could predict.
- **Compact rows.** While the panel is open the rows give up their metadata columns rather than truncating every title to nothing.
- **Destructive actions are two-step and inline.** The row asks in place of its action cluster instead of opening a dialog over the list.

### Signature Components

- **Graph Canvas:** Full-bleed SVG graph surface whose SVG presentation attributes and node-type colors reference design-token CSS variables.
- **CodeMirror Notes Editor:** Tokenized dark editor shell; editor theme uses CSS variables and `color-mix()` for DA-authored regions and wikilink pills.
- **Agent Log Pane:** Dense monospace event stream with semantic success/failure coloring and expandable tool results.
- **DA Learned Card:** The human-in-the-loop memory write surface inside the DA panel. A quiet bordered callout (`border-default`, `surface-overlay`, 4px corners) led by a `DA · learned` mono kicker (`9.5px`, `0.08em` tracking, `text-faint`, no decorative icon); the proposed memory sits in 12px body. Three right-aligned `UiButton` actions carry their function icons: `Keep` (`da` + `check`), `Edit` (`secondary` + `edit`), `Discard` (`ghost` + `close`). Only `Keep` takes the `da-accent` tint, honoring the Accent Scarcity Rule. The `da` variant exists for exactly this: `accent` is the interface's own tinted affirmative, used by Save and Approve, and the assistant needs one that still says whose action it is.

## Coverage

The token layer is applied everywhere: every screen already renders on the palette,
type scale, and radius scale above. The structural half is not. Tasks and Projects
are the two screens built to the pattern described under Lists, Rows and the Inline
Panel; the shell around them is the sidebar described under Navigation.

Every other screen still carries its previous structure on the new tokens: Home,
Notes, Inbox, Bookmarks, Memory, Graph, Ingest, Audit, Settings, Orchestrator, and
the two redirects into Home. They are not wrong, and they are not the pattern.
Read this section before treating one of them as a precedent: a screen that predates
the pattern is evidence of what the product used to do, not of what it should do
next. Porting each one is tracked as its own task in the graph rather than here.

## Do's and Don'ts

### Do:

- **Do** use `web/components/ui/` before writing a new local button, modal, input, select, toast, skeleton, empty state, or error state.
- **Do** keep surfaces dark, dense, and structurally quiet; the UI exists so complex data and workflows can shine.
- **Do** use skeletons for content loading and retry-capable error states for API failures.
- **Do** keep DA contextual: available everywhere, visually dominant only when summoned or on DA-specific routes.
- **Do** keep persistent navigation visually light: thin symbols, quiet captions, and state backgrounds instead of heavy icon weight.
- **Do** document any future literal color exceptions with the reason they cannot use CSS variables.
- **Do** preserve keyboard-first affordances and visible focus on rows, buttons, fields, and modal actions.

### Don't:

- **Don't** make Kernl feel like Jira: bureaucratic, cluttered, slow, or enterprise-heavy.
- **Don't** make Kernl feel like Notion: too much whitespace, too document-centric, too soft, or generic.
- **Don't** use generic SaaS dashboard patterns: decorative gradients, glassmorphism, oversized colorful cards, exaggerated drop shadows, or hero-metric layouts.
- **Don't** add border-left or border-right accents greater than 1px on cards, callouts, rows, or alerts. Use full borders, tints, icons, or state text instead.
- **Don't** pair a 1px border with a large soft shadow on the same card/button/modal.
- **Don't** make the sidebar feel like a bold menu. Avoid `font-bold`, oversized labels, or heavy Material Symbols in persistent navigation.
- **Don't** add arbitrary z-index values. Use semantic z-index utilities.
- **Don't** introduce raw hex in Vue templates, editor styles, or SVG graph presentation unless it is a documented literal-color exception.
- **Don't** add new typography families or fluid display scales without a deliberate design-system revision.
