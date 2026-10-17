package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"runtime"
	"strings"

	"ytgui/feed"
	"ytgui/internal/paths"
)

// Asset is one file to download for a release.
type Asset struct {
	URL     string `json:"url"`
	SHA256  string `json:"sha256"`
	Archive string `json:"archive,omitempty"` // "" (plain file) or "zip"
	// Files maps the installed file name to its path inside the archive.
	// A path is matched as a suffix, so "bin/ffmpeg.exe" finds
	// "ffmpeg-n9.0.2-win64-gpl-9.0/bin/ffmpeg.exe". For a plain file, the
	// single key is the installed name and the value is ignored.
	Files map[string]string `json:"files"`
}

// Release is a resolved version of a tool for this platform.
type Release struct {
	Version string  `json:"version"`
	Assets  []Asset `json:"assets"`
}

// Feed is the hosted JSON for tools without a clean "latest" endpoint (ffmpeg).
type Feed struct {
	Schema int                 `json:"schema"`
	Tools  map[string]FeedTool `json:"tools"`
}

type FeedTool struct {
	Version string             `json:"version"`
	Builds  map[string][]Asset `json:"builds"` // key: paths.Platform()
}

var hex64 = regexp.MustCompile(`(?i)\b[0-9a-f]{64}\b`)

func (m *Manager) resolveLatest(ctx context.Context, tool string) (*Release, error) {
	switch tool {
	case YtDlp:
		return m.latestYtDlp(ctx)
	case Deno:
		return m.latestDeno(ctx)
	case FFmpeg:
		return m.latestFromFeed(ctx, tool)
	}
	return nil, fmt.Errorf("unknown tool %q", tool)
}

func (m *Manager) latestYtDlp(ctx context.Context) (*Release, error) {
	var asset string
	switch paths.Platform() {
	case "windows-amd64":
		asset = "yt-dlp.exe"
	case "darwin-arm64", "darwin-amd64":
		asset = "yt-dlp_macos"
	default:
		return nil, errUnsupported
	}
	tag, err := m.githubLatestTag(ctx, "yt-dlp/yt-dlp")
	if err != nil {
		return nil, err
	}
	base := "https://github.com/yt-dlp/yt-dlp/releases/download/" + tag + "/"
	sums, err := m.fetchText(ctx, base+"SHA2-256SUMS")
	if err != nil {
		return nil, fmt.Errorf("could not get yt-dlp checksums: %w", err)
	}
	hash := ""
	for _, line := range strings.Split(sums, "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && strings.TrimPrefix(f[1], "*") == asset {
			hash = f[0]
		}
	}
	if hash == "" {
		return nil, fmt.Errorf("no checksum published for %s", asset)
	}
	return &Release{Version: tag, Assets: []Asset{{
		URL: base + asset, SHA256: hash, Files: map[string]string{paths.Exe("yt-dlp"): ""},
	}}}, nil
}

func (m *Manager) latestDeno(ctx context.Context) (*Release, error) {
	triple := map[string]string{
		"windows-amd64": "x86_64-pc-windows-msvc",
		"darwin-arm64":  "aarch64-apple-darwin",
		"darwin-amd64":  "x86_64-apple-darwin",
	}[paths.Platform()]
	if triple == "" {
		return nil, errUnsupported
	}
	tag, err := m.githubLatestTag(ctx, "denoland/deno")
	if err != nil {
		return nil, err
	}
	url := "https://github.com/denoland/deno/releases/download/" + tag + "/deno-" + triple + ".zip"
	// The .sha256sum file is "hash  name" on macOS and Get-FileHash output on Windows.
	sum, err := m.fetchText(ctx, url+".sha256sum")
	if err != nil {
		return nil, fmt.Errorf("could not get deno checksum: %w", err)
	}
	hash := hex64.FindString(sum)
	if hash == "" {
		return nil, fmt.Errorf("deno checksum file has no hash")
	}
	exe := paths.Exe("deno")
	return &Release{Version: strings.TrimPrefix(tag, "v"), Assets: []Asset{{
		URL: url, SHA256: strings.ToLower(hash), Archive: "zip", Files: map[string]string{exe: exe},
	}}}, nil
}

func (m *Manager) latestFromFeed(ctx context.Context, tool string) (*Release, error) {
	f, err := m.loadFeed(ctx)
	if err != nil {
		return nil, err
	}
	t, ok := f.Tools[tool]
	if !ok {
		return nil, fmt.Errorf("feed has no entry for %s", tool)
	}
	assets, ok := t.Builds[paths.Platform()]
	if !ok || len(assets) == 0 {
		return nil, errUnsupported
	}
	return &Release{Version: t.Version, Assets: assets}, nil
}

// loadFeed reads the hosted feed. YTGUI_FEED may name a local file or URL.
// If the hosted copy is unreachable, the copy embedded at build time is used.
func (m *Manager) loadFeed(ctx context.Context) (*Feed, error) {
	src := os.Getenv("YTGUI_FEED")
	if src == "" {
		src = m.FeedURL
	}
	var data []byte
	var err error
	switch {
	case src == "":
		// No hosted feed set at build time: the embedded copy is the feed.
		data = feed.Embedded
	case strings.HasPrefix(src, "https://"):
		var s string
		s, err = m.fetchText(ctx, src)
		data = []byte(s)
	default:
		data, err = os.ReadFile(src)
	}
	if err != nil {
		m.logf("tool list at %s could not be read (%v); using the built-in copy", src, err)
		data = feed.Embedded
	}
	var f Feed
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("feed is not valid JSON: %w", err)
	}
	if f.Schema != 1 {
		return nil, fmt.Errorf("feed schema %d not supported; update the app", f.Schema)
	}
	return &f, nil
}

// githubLatestTag reads the tag from the /releases/latest redirect, which,
// unlike the REST API, is not rate limited for anonymous clients.
func (m *Manager) githubLatestTag(ctx context.Context, repo string) (string, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodHead, "https://github.com/"+repo+"/releases/latest", nil)
	req.Header.Set("User-Agent", userAgent)
	client := *m.http
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	resp.Body.Close()
	loc := resp.Header.Get("Location")
	i := strings.LastIndex(loc, "/tag/")
	if i < 0 {
		return "", fmt.Errorf("could not find latest release of %s (HTTP %d)", repo, resp.StatusCode)
	}
	tag := loc[i+len("/tag/"):]
	if tag == "" || strings.ContainsAny(tag, "/\\?#") {
		return "", fmt.Errorf("unexpected release tag %q", tag)
	}
	return tag, nil
}

func (m *Manager) fetchText(ctx context.Context, url string) (string, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	req.Header.Set("User-Agent", userAgent)
	resp, err := m.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return string(b), err
}

var userAgent = "ytgui-updater (" + runtime.GOOS + "/" + runtime.GOARCH + ")"
