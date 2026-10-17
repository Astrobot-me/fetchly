//go:build !dev

package api

// DevMode relaxes the Origin check for the Vite dev server. Release builds
// never include it; build with -tags dev to enable.
const DevMode = false
