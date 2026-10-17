// Package paths resolves the per-user folders the app uses.
package paths

import (
	"os"
	"path/filepath"
	"runtime"
)

const AppName = "Fetchly"

// AppData returns the app-data root, creating it if needed.
//
//	Windows: %LOCALAPPDATA%\Fetchly
//	macOS:   ~/Library/Application Support/Fetchly
//
// YTGUI_HOME overrides it (useful for tests and dev).
func AppData() (string, error) {
	dir := os.Getenv("YTGUI_HOME")
	if dir == "" {
		var base string
		switch runtime.GOOS {
		case "windows":
			base = os.Getenv("LOCALAPPDATA")
			if base == "" {
				var err error
				if base, err = os.UserCacheDir(); err != nil {
					return "", err
				}
			}
		default:
			var err error
			if base, err = os.UserConfigDir(); err != nil {
				return "", err
			}
		}
		dir = filepath.Join(base, AppName)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// Bin is the folder holding managed tools: <appdata>/bin.
func Bin(appData string) string { return filepath.Join(appData, "bin") }

// Instances holds one small file per running instance (port + token).
func Instances(appData string) string { return filepath.Join(appData, "instances") }

// DefaultDownloads is ~/Downloads.
func DefaultDownloads() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, "Downloads")
}

// Exe appends .exe on Windows.
func Exe(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

// Platform is "windows-amd64", "darwin-arm64", ...
func Platform() string { return runtime.GOOS + "-" + runtime.GOARCH }
