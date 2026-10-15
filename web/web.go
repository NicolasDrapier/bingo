package web

import "embed"

//go:embed index.html style.css app.js assets
var FS embed.FS
