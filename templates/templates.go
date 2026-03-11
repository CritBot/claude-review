// Package templates provides embedded agent prompt templates.
package templates

import "embed"

//go:embed *.md
var FS embed.FS
