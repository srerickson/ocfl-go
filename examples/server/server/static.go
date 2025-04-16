package server

import "embed"

//go:embed all:static
var staticFS embed.FS
