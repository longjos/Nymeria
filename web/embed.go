package web

import "embed"

// Build contains the compiled SvelteKit frontend assets.
//
//go:embed all:build
var Build embed.FS
