package osutil

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// OpenBrowser opens a URL in the default browser.
func OpenBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		// rundll32 avoids cmd.exe treating & in the URL as a separator.
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

// OpenFolder shows a folder in Explorer/Finder.
func OpenFolder(dir string) error {
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return errors.New("that folder doesn't exist")
	}
	switch runtime.GOOS {
	case "windows":
		return exec.Command("explorer.exe", dir).Start()
	case "darwin":
		return exec.Command("open", dir).Start()
	}
	return exec.Command("xdg-open", dir).Start()
}

// RevealFile opens the containing folder with the file selected.
func RevealFile(file string) error {
	if _, err := os.Stat(file); err != nil {
		return errors.New("that file is no longer there")
	}
	switch runtime.GOOS {
	case "windows":
		// explorer returns exit code 1 even on success, so don't wait on it.
		return exec.Command("explorer.exe", "/select,", file).Start()
	case "darwin":
		return exec.Command("open", "-R", file).Start()
	}
	return OpenFolder(file)
}

// PickFolder shows a native folder picker. It returns "" if canceled.
func PickFolder(ctx context.Context, start string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		// The start folder comes in through an environment variable, never
		// through the script text.
		script := `Add-Type -AssemblyName System.Windows.Forms
$f = New-Object System.Windows.Forms.FolderBrowserDialog
$f.Description = 'Choose where downloads are saved'
$f.UseDescriptionForTitle = $true
if ($env:YTGUI_START -and (Test-Path -LiteralPath $env:YTGUI_START)) { $f.SelectedPath = $env:YTGUI_START }
$owner = New-Object System.Windows.Forms.Form -Property @{TopMost = $true}
if ($f.ShowDialog($owner) -eq 'OK') { [Console]::OutputEncoding = [Text.Encoding]::UTF8; Write-Output $f.SelectedPath }`
		cmd = exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-STA", "-Command", script)
		Background(cmd)
	case "darwin":
		cmd = exec.CommandContext(ctx, "osascript",
			"-e", `on run argv`,
			"-e", `try`,
			"-e", `return POSIX path of (choose folder with prompt "Choose where downloads are saved" default location (POSIX file (item 1 of argv)))`,
			"-e", `on error number -128`,
			"-e", `return ""`,
			"-e", `end try`,
			"-e", `end run`, start)
	default:
		return "", errors.New("folder picker not supported on this system")
	}
	cmd.Env = append(os.Environ(), "YTGUI_START="+start)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return "", errors.New("the folder picker could not be opened")
	}
	p := strings.TrimSpace(out.String())
	if runtime.GOOS == "darwin" && len(p) > 1 {
		p = strings.TrimSuffix(p, "/")
	}
	return p, nil
}
