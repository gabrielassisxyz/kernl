# Kernl Design System

The current design system as it stands, extracted so it can be iterated on
externally and re-applied centrally. Source of truth for tokens:
`web/assets/css/tailwind.css` (`@theme` block). Tailwind v4 generates utility
classes from each `--color-*` / `--radius-*` / `--spacing-*` / `--font-*` token,
so screens reference roles (`bg-bg-base`, `text-text-primary`) rather than raw hex.

**Aesthetic in one line:** dark, near-black, cool blue-grey UI; very square
(small radii); IBM Plex type; a single blue-violet accent; muted Material-style
status colors. Calm and information-dense, not flashy.

To change the look: edit token *values* in `tailwind.css` (and keep this file in
sync). Because every rebuilt surface is on tokens, a palette change is a central
edit, not a per-screen sweep.

---

## Color

### Backgrounds & surfaces (darkest → lightest)
| Token | Hex | Role |
| --- | --- | --- |
| `bg-base` / `background` | `#0F1217` | App background |
| `surface` / `surface-dim` | `#131317` | Base surface |
| `bg-elevated` | `#141821` | Elevated panels, headers, rails |
| `surface-overlay` | `#181C26` | Modals & cards (raised above panels) |
| `surface-container-lowest` | `#0d0e11` | |
| `surface-container-low` | `#1b1b1f` | |
| `surface-container` | `#1f1f23` | |
| `surface-hover` | `#1D222D` | Hover / pressed state |
| `surface-container-high` | `#292a2d` | |
| `surface-container-highest` | `#343438` | |
| `surface-variant` | `#343438` | |
| `surface-bright` | `#39393d` | |

### Text (on dark)
| Token | Hex | Role |
| --- | --- | --- |
| `on-surface` / `on-background` | `#e4e2e6` | Brightest text |
| `text-primary` | `#D6DBE3` | Primary body text |
| `text-muted` | `#9098A7` | Secondary / muted |
| `text-faint` | `#666D7C` | Faint, placeholders |
| `text-dim` | `#444A57` | Dimmest readable |

### Borders & outlines
| Token | Hex | Role |
| --- | --- | --- |
| `border-hairline` | `#1B2029` | Subtle dividers |
| `border-default` | `#242935` | Default borders |
| `outline-variant` | `#45464f` | |
| `outline` | `#8f909a` | Stronger outline |

### Accent (blue-violet)
| Token | Hex | Role |
| --- | --- | --- |
| `da-accent` | `#6B7BB0` | The DA accent — primary interactive accent |
| `primary` | `#b4c5fe` | Material primary |
| `primary-container` | `#7f8fc5` | Accent container / hover |
| `on-primary` | `#1d2e5e` | Text on primary |

Full Material `primary/secondary/tertiary` fixed-variant ramps also exist in
`tailwind.css` (`primary-fixed`, `secondary-*`, `tertiary-*`, `inverse-*`).

### Status (muted)
| Token | Hex | Role |
| --- | --- | --- |
| `status-passed` | `#6D9A78` | Success / accept (green) |
| `status-failed` | `#C2675C` | Failure (red) |
| `status-active` | `#4FA89C` | Active (teal) |
| `status-running` | `#8089A0` | Running (blue-grey) |
| `status-gate` | `#C99A4A` | Gate / waiting (amber) |

### Tertiary & error
`tertiary #e5c270` / `tertiary-container #ac8d41` (gold); `error #ffb4ab`,
`error-container #93000a`, `on-error #690005`, `on-error-container #ffdad6`.

---

## Radius

Deliberately square. `radius: 0px`, `radius-lg: 2px`, `radius-xl: 4px`,
`radius-full: 6px`.

## Spacing

`tight 4px` · `base 8px` · `component 16px` · `section 24px` · `margin 32px`
· `break 64px` · `rail-width 60px`.

## Typography

All IBM Plex. `font-headline` / `font-body` / `font-label-caps` / `font-display`
= **IBM Plex Sans**; `font-mono-data` = **IBM Plex Mono** (metadata, counts, IDs).

| Style | Size / line-height | Weight | Notes |
| --- | --- | --- | --- |
| `display` | 24 / 32 | 600 | -0.02em tracking |
| `headline` | 18 / 24 | 500 | -0.01em tracking |
| `body` | 13 / 20 | 400 | |
| `label-caps` | 11.5 / 16 | 600 | 0.12em tracking, UPPERCASE |
| `mono-data` | 12 / 16 | 450 | monospace |

---

## Component patterns

- **Rails / headers:** `bg-bg-elevated` + `border-border-hairline`, fixed height
  `rail-width`.
- **Cards / modals:** `bg-surface-overlay` + `border-border-default`, square radius,
  soft long shadow.
- **Hover / pressed:** `bg-surface-hover`; muted text brightens toward `text-primary`.
- **Accent actions:** `bg-da-accent` with `on-surface` text; focus rings use
  `da-accent` / `primary-container`.
- **Section labels:** `label-caps` (uppercase, tracked) in `text-text-muted`.
- **Metadata / counts / IDs:** `mono-data` in `text-text-faint` or `text-text-dim`.

---

## Graph-specific colors (not tokenized)

`web/pages/graph.vue` renders an SVG force-graph. SVG presentation attributes
(`fill`/`stroke`) do not reliably resolve CSS `var()`, so these stay as literals
and are documented here instead of tokenized:

| Use | Value | Notes |
| --- | --- | --- |
| Idle edge | `#454A52` | Neutral grey at-rest connection (user-tuned) |
| Node core stroke | `#0F1217` | = `bg-base`, cuts the node from the background |
| Selected ring / specular highlight | `#fff` | Intentional pure-white affordance |
| Soft shadow | `#000` | Black at low alpha |

Active edges and node cores take their color from `colorFor(type)` (per node-type
palette in `graph.vue`). When the palette is re-themed, update these literals to
match.

---

## Notes on extraction

Two values had no exact token and were mapped to the nearest existing one
(imperceptible shift; recorded for transparency):

- `#7c8bc0` (a focus-border accent) → `primary-container` (`#7f8fc5`).
- `#F0F4F8` (a near-white hover text) → `on-surface` (`#e4e2e6`).

If a future palette wants these exact values, add dedicated tokens.
