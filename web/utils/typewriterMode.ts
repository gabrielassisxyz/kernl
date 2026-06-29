// Typewriter mode: keeps the line being edited vertically centered in the
// viewport, so the caret stays put while text scrolls past it. Implemented as a
// CM6 update listener that recenters the active line on selection/doc changes.
// The editor's scroll container does the actual scrolling via scrollIntoView,
// which walks up to whichever ancestor is scrollable (the editor pane here).

import { EditorView } from '@codemirror/view'

export function typewriterExtension() {
  return EditorView.updateListener.of((update) => {
    if (!update.selectionSet && !update.docChanged && !update.geometryChanged) return

    const view = update.view
    requestAnimationFrame(() => {
      const head = view.state.selection.main.head
      const node = view.domAtPos(head).node
      const el = node instanceof Element ? node : node.parentElement
      const line = el?.closest('.cm-line')
      // 'auto' (not 'smooth'): typewriter recentering must be instant, otherwise
      // it lags behind fast typing and fights itself — and it respects reduced
      // motion by construction.
      line?.scrollIntoView({ block: 'center', behavior: 'auto' })
    })
  })
}
