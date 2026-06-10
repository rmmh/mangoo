package main

import "embed"

//go:embed frontend/dist
var staticFS embed.FS

//go:embed mangoo.example.toml
var exampleConfig []byte
