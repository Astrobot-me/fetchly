//go:build windows

package osutil

import "syscall"

// HideFile sets the Hidden attribute, so the cache folder stays out of
// sight in Explorer like a dot-folder does on macOS.
func HideFile(path string) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return
	}
	attrs, err := syscall.GetFileAttributes(p)
	if err != nil || attrs&syscall.FILE_ATTRIBUTE_HIDDEN != 0 {
		return
	}
	_ = syscall.SetFileAttributes(p, attrs|syscall.FILE_ATTRIBUTE_HIDDEN)
}
