package main

import "embed"

//go:embed frontend/dist
var staticFS embed.FS
