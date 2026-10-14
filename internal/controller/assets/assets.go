// Package assets embeds the SOP Controller templates and static files.
package assets

import "embed"

// FS holds the server-rendered templates and static assets shipped with the
// binary, so the dashboard runs from anywhere without a Node toolchain.
//
//go:embed templates static
var FS embed.FS
