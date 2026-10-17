// Package feed embeds the tools feed as a fallback for when the hosted
// copy (the same file, served from the repo) cannot be reached.
package feed

import _ "embed"

//go:embed tools.json
var Embedded []byte
