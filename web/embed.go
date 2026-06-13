package web

import (
	"embed"
	"io/fs"
	"path"
)

//go:embed index.html monitor.js style.css
var rootEmbed embed.FS

//go:embed all:.output/public
var nuxtEmbed embed.FS

// FS is the merged filesystem serving both the orchestrator monitor
// (root-level files) and the Vue/Nuxt chat UI (from .output/public).
var FS fs.FS

func init() {
	nuxtFS, err := fs.Sub(nuxtEmbed, ".output/public")
	if err != nil {
		panic("failed to sub Nuxt output: " + err.Error())
	}
	FS = &mergedFS{root: rootEmbed, nuxt: nuxtFS}
}

// mergedFS serves the Nuxt shell (the main app) at the site root and keeps
// the legacy orchestrator monitor reachable at /monitor.html. Root-embedded
// assets (monitor.js, style.css) still resolve directly; everything else
// falls back to the Nuxt build output.
type mergedFS struct {
	root fs.FS
	nuxt fs.FS
}

func (m *mergedFS) Open(name string) (fs.File, error) {
	name = path.Clean(name)
	// Strip leading slash(es)
	for len(name) > 0 && name[0] == '/' {
		name = name[1:]
	}
	if name == "" {
		name = "."
	}
	// Legacy orchestrator monitor moved to /monitor.html so it no longer
	// shadows the Nuxt Home at "/". Its assets (/style.css, /monitor.js)
	// still resolve from the root embed below.
	if name == "monitor.html" || name == "monitor" {
		return m.root.Open("index.html")
	}
	// Serve the Nuxt shell's index.html at the site root instead of the
	// legacy monitor (which a root-first lookup would otherwise return).
	if name == "index.html" {
		return m.nuxt.Open("index.html")
	}
	// Root-embedded assets (monitor.js, style.css) first, then Nuxt output.
	if f, err := m.root.Open(name); err == nil {
		return f, nil
	}
	return m.nuxt.Open(name)
}
