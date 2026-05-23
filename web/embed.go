package web

import "embed"

//go:embed index.html app.js style.css chat.html chat.js scope-selector.js da-config.html da-config.js
var FS embed.FS
