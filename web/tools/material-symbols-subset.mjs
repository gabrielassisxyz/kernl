// Regenerates the self-hosted Material Symbols Outlined subset.
//
// We ship only the glyphs the app actually uses (a ~41 KB woff2) instead of the
// full ~3 MB variable font. To add an icon: append its name to ICONS (keep the
// list sorted — the Google Fonts API requires alphabetical icon_names), then run
//   node tools/material-symbols-subset.mjs
// which refetches public/fonts/material-symbols-outlined.woff2 and prints the
// updated header to paste into assets/css/fonts.css.
//
// No build-step dependency: run it by hand when the icon set changes.

import { writeFile } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'
import { dirname, resolve } from 'node:path'

// Sorted alphabetically (API requirement). Each name is a Material Symbol used
// somewhere in the app; grep the codebase before removing one.
const ICONS = [
  'account_circle', 'account_tree', 'add', 'analytics', 'arrow_forward',
  'arrow_upward', 'auto_awesome', 'bookmark', 'check', 'check_circle',
  'checklist', 'chrome_reader_mode', 'close', 'cloud_off', 'code', 'dashboard',
  'delete', 'description', 'edit', 'edit_note', 'expand_more', 'explore', 'filter_list',
  'fit_screen', 'folder_open', 'format_list_numbered', 'help', 'history',
  'hourglass_empty', 'hub', 'inbox', 'input', 'keyboard', 'left_panel_close',
  'left_panel_open', 'link_off', 'lock', 'memory', 'neurology', 'open_in_new',
  'play_arrow', 'policy', 'progress_activity', 'queue', 'refresh', 'save', 'search',
  'settings', 'smart_toy', 'source', 'swap_vert', 'sync', 'tag', 'task_alt',
  'terminal', 'text_fields', 'tune', 'upload_file', 'view_kanban', 'view_list', 'visibility',
  'warning',
]

// A modern browser UA so the CSS2 endpoint returns a woff2 src, not legacy ttf.
const UA =
  'Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36'

const here = dirname(fileURLToPath(import.meta.url))
const outFile = resolve(here, '../public/fonts/material-symbols-outlined.woff2')

// Axis ranges are pinned to what the app actually renders, which keeps the
// woff2 small: opsz 20 (rail) → 24 (body), wght 260/280 (rail) → 300 (body),
// FILL and GRAD never vary, so they're pinned to single values.
const cssUrl =
  'https://fonts.googleapis.com/css2?family=Material+Symbols+Outlined:' +
  'opsz,wght,FILL,GRAD@20..24,260..300,0,-25' +
  `&icon_names=${ICONS.join(',')}&display=swap`

const cssRes = await fetch(cssUrl, { headers: { 'User-Agent': UA } })
if (!cssRes.ok) {
  throw new Error(`CSS2 request failed (${cssRes.status}): ${await cssRes.text()}`)
}
const css = await cssRes.text()

// The subset src is a /l/font?kit=… URL (no .woff2 suffix); key off the format.
const match = css.match(/url\((https:\/\/[^)]+)\)\s*format\('woff2'\)/)
if (!match) {
  throw new Error(`No woff2 src found in CSS response:\n${css}`)
}

const fontRes = await fetch(match[1], { headers: { 'User-Agent': UA } })
if (!fontRes.ok) {
  throw new Error(`Font download failed (${fontRes.status})`)
}
const buf = Buffer.from(await fontRes.arrayBuffer())
await writeFile(outFile, buf)

const kb = (buf.length / 1024).toFixed(1)
console.log(`Wrote ${outFile} (${kb} KB, ${ICONS.length} glyphs)`)
console.log('\nUpdate the header comment in assets/css/fonts.css to:')
console.log(`  Subset icon names (${ICONS.length} glyphs used in this project):`)
console.log('   ' + ICONS.join(', '))
