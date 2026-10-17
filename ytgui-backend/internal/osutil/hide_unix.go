//go:build !windows

package osutil

// HideFile is a no-op: a leading dot already hides the folder.
func HideFile(string) {}
