// Live design-token panel (live-tweaks), DEV ONLY.
//
// Mounts a floating panel that edits kernl's CSS custom properties live, so
// colors and type can be judged in the running app instead of guessed at in a
// stylesheet. Saving copies a before/after diff that the `/tweaks` skill writes
// back to source.
//
// The `import.meta.dev` guard is what keeps this out of production builds: Vite
// statically evaluates it to `false` in a production build, so the dynamic
// import below is dropped entirely and the dependency never enters the bundle.
// That is also why live-tweaks belongs in devDependencies — nothing in a
// shipped build references it.
//
// The panel reads `LiveTweaksConfig` once at mount, so it must be assigned
// BEFORE the import. The allowlist is not optional here: daisyUI floods `:root`
// with hundreds of its own custom properties, many sharing kernl's `--color-*`
// naming, which would bury the tokens that actually belong to the app. Entries
// come from .live-tweaks/design-tokens.md — regenerate both together by
// rerunning `/tweaks`, or the panel silently drops tokens the app has since
// added.
//
// Ordering is meaningful: the panel renders tokens in allow-entry order, so
// this list runs surfaces → text → brand accents → everything else, by visual
// prominence rather than alphabetically.
export default defineNuxtPlugin(() => {
  if (!import.meta.dev) return
  ;(window as any).LiveTweaksConfig = {
    allow: [
      "--color-background",
      "--color-bg-base",
      "--color-bg-elevated",
      "--color-surface",
      "--color-surface-bright",
      "--color-surface-container",
      "--color-surface-container-high",
      "--color-surface-container-highest",
      "--color-surface-container-low",
      "--color-surface-container-lowest",
      "--color-surface-dim",
      "--color-surface-hover",
      "--color-surface-overlay",
      "--color-surface-variant",
      "--color-on-background",
      "--color-on-surface",
      "--color-on-surface-variant",
      "--color-text-dim",
      "--color-text-faint",
      "--color-text-muted",
      "--color-text-primary",
      "--color-primary",
      "--color-secondary",
      "--color-tertiary",
      "--color-border-default",
      "--color-border-hairline",
      "--color-da-accent",
      "--color-error",
      "--color-error-container",
      "--color-inverse-on-surface",
      "--color-inverse-primary",
      "--color-inverse-surface",
      "--color-node-bookmark-list",
      "--color-node-capture",
      "--color-node-chat-session",
      "--color-node-decision",
      "--color-node-memory-claim",
      "--color-node-memory-refutation",
      "--color-node-note",
      "--color-on-error",
      "--color-on-error-container",
      "--color-on-primary",
      "--color-on-primary-container",
      "--color-on-primary-fixed",
      "--color-on-primary-fixed-variant",
      "--color-on-secondary",
      "--color-on-secondary-container",
      "--color-on-secondary-fixed",
      "--color-on-secondary-fixed-variant",
      "--color-on-tertiary",
      "--color-on-tertiary-container",
      "--color-on-tertiary-fixed",
      "--color-on-tertiary-fixed-variant",
      "--color-outline",
      "--color-outline-variant",
      "--color-primary-container",
      "--color-primary-fixed",
      "--color-primary-fixed-dim",
      "--color-secondary-container",
      "--color-secondary-fixed",
      "--color-secondary-fixed-dim",
      "--color-status-active",
      "--color-status-failed",
      "--color-status-failed-text",
      "--color-status-gate",
      "--color-status-passed",
      "--color-status-running",
      "--color-tertiary-container",
      "--color-tertiary-fixed",
      "--color-tertiary-fixed-dim",
      "--font-body",
      "--font-display",
      "--font-headline",
      "--font-label-caps",
      "--font-mono-data",
      "--text-body--font-weight",
      "--text-display--font-weight",
      "--text-headline--font-weight",
      "--text-label-caps--font-weight",
      "--text-mono-data--font-weight",
      "--radius",
      "--radius-full",
      "--radius-lg",
      "--radius-xl",
      "--spacing-base",
      "--spacing-break",
      "--spacing-component",
      "--spacing-margin",
      "--spacing-rail-width",
      "--spacing-section",
      "--spacing-tight",
      "--text-body",
      "--text-body--line-height",
      "--text-display",
      "--text-display--letter-spacing",
      "--text-display--line-height",
      "--text-headline",
      "--text-headline--letter-spacing",
      "--text-headline--line-height",
      "--text-label-caps",
      "--text-label-caps--letter-spacing",
      "--text-label-caps--line-height",
      "--text-mono-data",
      "--text-mono-data--line-height",
    ],
  }
  // Side-effect import: the module mounts the panel itself on load.
  import("live-tweaks")
})
