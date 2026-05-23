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

// mergedFS tries the root embedded files first, then falls back to
// the Nuxt build output. This allows the orchestrator monitor
// (index.html, app.js, style.css) to coexist with the chat SPA
// served under /chat/.
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
	// Try root-embedded files first (monitor)
	f, err := m.root.Open(name)
	if err == nil {
		return f, nil
	}
	// Fall back to Nuxt output
	return m.nuxt.Open(name)
}
