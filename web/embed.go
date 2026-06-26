package web

import (
	"embed"
	"io/fs"
)

//go:embed all:.output/public
var nuxtEmbed embed.FS

// FS serves the Vue/Nuxt UI built into .output/public.
var FS fs.FS

func init() {
	nuxtFS, err := fs.Sub(nuxtEmbed, ".output/public")
	if err != nil {
		panic("failed to sub Nuxt output: " + err.Error())
	}
	FS = nuxtFS
}
