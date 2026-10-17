package updater

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"ytgui/internal/osutil"
	"ytgui/internal/paths"
)

// fetchAsset downloads one asset into tmp, checks its hash and places its
// files into stage.
func (m *Manager) fetchAsset(ctx context.Context, tool string, a Asset, idx, total int, tmp, stage string) error {
	if !strings.HasPrefix(a.URL, "https://") {
		return fmt.Errorf("refusing non-https download %s", a.URL)
	}
	want := strings.ToLower(strings.TrimSpace(a.SHA256))
	if len(want) != 64 {
		return fmt.Errorf("no checksum for %s; refusing to install it", a.URL)
	}
	if len(a.Files) == 0 {
		return fmt.Errorf("feed entry for %s lists no files", a.URL)
	}

	file := filepath.Join(tmp, fmt.Sprintf("asset-%d", idx))
	got, err := m.download(ctx, tool, a.URL, file, idx, total)
	if err != nil {
		return err
	}
	m.progress(Progress{Name: tool, Stage: "verifying", Percent: 100, Message: "Checking the download is genuine"})
	if got != want {
		return fmt.Errorf("the download of %s did not match its checksum; it was not installed", tool)
	}

	switch a.Archive {
	case "":
		for name := range a.Files {
			return os.Rename(file, filepath.Join(stage, filepath.Base(name)))
		}
	case "zip":
		return extractZip(file, stage, a.Files)
	}
	return fmt.Errorf("unknown archive type %q", a.Archive)
}

// download streams url to dest and returns the SHA-256 of what it wrote.
func (m *Manager) download(ctx context.Context, tool, url, dest string, idx, total int) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := m.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("could not download %s: %w", tool, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("could not download %s (HTTP %d)", tool, resp.StatusCode)
	}
	f, err := os.Create(dest)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	size := resp.ContentLength
	var done int64
	last := time.Time{}
	buf := make([]byte, 256<<10)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, err := f.Write(buf[:n]); err != nil {
				return "", err
			}
			h.Write(buf[:n])
			done += int64(n)
			if time.Since(last) > 250*time.Millisecond {
				last = time.Now()
				pct := 0.0
				if size > 0 {
					// Spread several assets across one 0-100 bar.
					pct = (float64(idx) + float64(done)/float64(size)) / float64(total) * 100
				}
				m.progress(Progress{Name: tool, Stage: "downloading", Percent: pct,
					Message: fmt.Sprintf("Downloading %s (%.1f MB)", tool, float64(done)/1e6)})
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return "", fmt.Errorf("download of %s was interrupted: %w", tool, rerr)
		}
	}
	if size > 0 && done != size {
		return "", fmt.Errorf("download of %s was incomplete", tool)
	}
	return hex.EncodeToString(h.Sum(nil)), f.Close()
}

// extractZip copies the wanted files (matched by path suffix) out of a zip.
func extractZip(zipPath, stage string, files map[string]string) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("the downloaded archive is damaged: %w", err)
	}
	defer zr.Close()
	for dest, want := range files {
		want = path.Clean("/" + strings.ReplaceAll(want, "\\", "/"))
		var found *zip.File
		for _, f := range zr.File {
			name := path.Clean("/" + f.Name)
			if !f.FileInfo().IsDir() && (name == want || strings.HasSuffix(name, want)) {
				found = f
				break
			}
		}
		if found == nil {
			return fmt.Errorf("%s not found in the downloaded archive", want)
		}
		if err := copyZipFile(found, filepath.Join(stage, filepath.Base(dest))); err != nil {
			return err
		}
	}
	return nil
}

func copyZipFile(f *zip.File, dest string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, rc); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// prepareExecutables sets the exec bit and, on macOS, clears quarantine.
// Files fetched by Go are not quarantined, but clearing is harmless.
func prepareExecutables(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		p := filepath.Join(dir, e.Name())
		if runtime.GOOS != "windows" {
			if err := os.Chmod(p, 0o755); err != nil {
				return err
			}
		}
		if runtime.GOOS == "darwin" {
			_ = exec.Command("xattr", "-d", "com.apple.quarantine", p).Run()
		}
	}
	return nil
}

// smokeTest runs the new binary's version command and discards it on failure.
func smokeTest(ctx context.Context, tool, dir string) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute) // yt-dlp_macos unpacks itself on first run
	defer cancel()
	checks := map[string][]string{
		YtDlp:  {"--version"},
		FFmpeg: {"-version"},
		Deno:   {"--version"},
	}
	// ffprobe is not installed: the app only needs ffmpeg for stream-copy
	// merges, and audio is saved as-is instead of converted.
	cmd := exec.CommandContext(ctx, filepath.Join(dir, paths.Exe(tool)), checks[tool]...)
	osutil.Background(cmd)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(out.String())
		if len(msg) > 300 {
			msg = msg[:300]
		}
		return errors.New("the new " + tool + " would not start on this computer (" + err.Error() + ") " + msg)
	}
	return nil
}
