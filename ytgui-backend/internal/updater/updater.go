// Package updater installs, updates and rolls back the external tools
// (yt-dlp, ffmpeg, deno) in the app-data folder. The app always runs
// whatever version bin/<tool>/current.json names.
package updater

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"
	"time"

	"ytgui/internal/paths"
)

const (
	YtDlp  = "yt-dlp"
	FFmpeg = "ffmpeg"
	Deno   = "deno"
)

// Tools lists every managed tool, in install order.
var Tools = []string{YtDlp, FFmpeg, Deno}

// Required tools must be installed before downloads work. Deno is
// optional: yt-dlp uses it for YouTube's JavaScript challenges and loses
// some formats without it.
func Required(tool string) bool { return tool != Deno }

var (
	errUnsupported = errors.New("this computer's system is not supported")
	ErrBusy        = errors.New("an update for this tool is already running")
	ErrUnknownTool = errors.New("unknown tool")
)

// Progress reports install steps to the UI.
type Progress struct {
	Name    string  `json:"name"`
	Stage   string  `json:"stage"` // checking|downloading|verifying|testing|installing|done|error
	Percent float64 `json:"percent"`
	Message string  `json:"message"`
}

// Status is one tool's state as the UI sees it.
type Status struct {
	Name            string  `json:"name"`
	Installed       *string `json:"installed"`
	Previous        *string `json:"previous"`
	Latest          *string `json:"latest"`
	UpdateAvailable bool    `json:"updateAvailable"`
	Busy            bool    `json:"busy"`
	Required        bool    `json:"required"`
	Error           string  `json:"error,omitempty"`
}

type current struct {
	Version  string `json:"version"`
	Previous string `json:"previous,omitempty"`
}

type Manager struct {
	BinDir  string
	FeedURL string
	// AllowPrune is false in a secondary instance, which cannot see the
	// main instance's running jobs.
	AllowPrune bool
	OnProgress func(Progress)
	OnChange   func()
	Logger     *log.Logger

	http *http.Client

	mu     sync.Mutex
	busy   map[string]bool
	latest map[string]string
	errs   map[string]string
	inUse  map[string]int // "tool/version" -> running jobs
}

func New(binDir, feedURL string) *Manager {
	return &Manager{
		BinDir:     binDir,
		FeedURL:    feedURL,
		AllowPrune: true,
		http:       &http.Client{Timeout: 30 * time.Minute},
		busy:       map[string]bool{},
		latest:     map[string]string{},
		errs:       map[string]string{},
		inUse:      map[string]int{},
	}
}

func known(tool string) bool {
	for _, t := range Tools {
		if t == tool {
			return true
		}
	}
	return false
}

func (m *Manager) logf(format string, args ...any) {
	if m.Logger != nil {
		m.Logger.Printf(format, args...)
	}
}

func (m *Manager) progress(p Progress) {
	if m.OnProgress != nil {
		m.OnProgress(p)
	}
}

func (m *Manager) changed() {
	if m.OnChange != nil {
		m.OnChange()
	}
}

func (m *Manager) toolDir(tool string) string { return filepath.Join(m.BinDir, tool) }

func (m *Manager) readCurrent(tool string) current {
	var c current
	b, err := os.ReadFile(filepath.Join(m.toolDir(tool), "current.json"))
	if err == nil {
		_ = json.Unmarshal(b, &c)
	}
	return c
}

func (m *Manager) writeCurrent(tool string, c current) error {
	b, _ := json.MarshalIndent(c, "", "  ")
	return writeFileAtomic(filepath.Join(m.toolDir(tool), "current.json"), b)
}

func (m *Manager) versionDir(tool, version string) string {
	return filepath.Join(m.toolDir(tool), "versions", safeName(version))
}

// Installed returns the installed version, or "" if missing.
func (m *Manager) Installed(tool string) string {
	c := m.readCurrent(tool)
	if c.Version == "" {
		return ""
	}
	if _, err := os.Stat(m.exePath(tool, c.Version)); err != nil {
		return ""
	}
	return c.Version
}

func (m *Manager) exePath(tool, version string) string {
	return filepath.Join(m.versionDir(tool, version), paths.Exe(tool))
}

// Ready reports whether every required tool is installed.
func (m *Manager) Ready() bool {
	for _, t := range Tools {
		if Required(t) && m.Installed(t) == "" {
			return false
		}
	}
	return true
}

// Lease pins the current version of a tool for the life of a job, so a
// prune cannot remove it. Paths are resolved at spawn time, so an update
// during a download is safe. Call release when the job ends.
type Lease struct {
	Path    string // the executable
	Dir     string // its folder (for --ffmpeg-location)
	Version string
}

func (m *Manager) Acquire(tool string) (*Lease, func(), error) {
	v := m.Installed(tool)
	if v == "" {
		return nil, nil, fmt.Errorf("%s is not installed", tool)
	}
	key := tool + "/" + v
	m.mu.Lock()
	m.inUse[key]++
	m.mu.Unlock()
	var once sync.Once
	release := func() {
		once.Do(func() {
			m.mu.Lock()
			m.inUse[key]--
			m.mu.Unlock()
		})
	}
	return &Lease{Path: m.exePath(tool, v), Dir: m.versionDir(tool, v), Version: v}, release, nil
}

func (m *Manager) Status() []Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Status, 0, len(Tools))
	for _, t := range Tools {
		c := m.readCurrent(t)
		s := Status{Name: t, Busy: m.busy[t], Required: Required(t), Error: m.errs[t]}
		if v := m.Installed(t); v != "" {
			s.Installed = &v
		}
		if c.Previous != "" {
			if _, err := os.Stat(m.exePath(t, c.Previous)); err == nil {
				p := c.Previous
				s.Previous = &p
			}
		}
		if l := m.latest[t]; l != "" {
			s.Latest = &l
			s.UpdateAvailable = s.Installed != nil && *s.Installed != l
		}
		out = append(out, s)
	}
	return out
}

// CheckLatest refreshes the latest known version of every tool.
func (m *Manager) CheckLatest(ctx context.Context) {
	var wg sync.WaitGroup
	for _, t := range Tools {
		wg.Add(1)
		go func(t string) {
			defer wg.Done()
			r, err := m.resolveLatest(ctx, t)
			m.mu.Lock()
			if err != nil {
				m.errs[t] = "Could not check for updates: " + err.Error()
			} else {
				m.latest[t] = r.Version
				delete(m.errs, t)
			}
			m.mu.Unlock()
		}(t)
	}
	wg.Wait()
	m.changed()
}

// Update installs the latest version of a tool if it is not already current.
func (m *Manager) Update(ctx context.Context, tool string) (err error) {
	if !known(tool) {
		return ErrUnknownTool
	}
	m.mu.Lock()
	if m.busy[tool] {
		m.mu.Unlock()
		return ErrBusy
	}
	m.busy[tool] = true
	delete(m.errs, tool)
	m.mu.Unlock()
	m.changed()

	defer func() {
		m.mu.Lock()
		m.busy[tool] = false
		if err != nil {
			m.errs[tool] = err.Error()
		}
		m.mu.Unlock()
		if err != nil {
			m.progress(Progress{Name: tool, Stage: "error", Message: err.Error()})
		}
		m.changed()
	}()

	m.progress(Progress{Name: tool, Stage: "checking", Message: "Checking for the newest version"})
	rel, err := m.resolveLatest(ctx, tool)
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.latest[tool] = rel.Version
	m.mu.Unlock()

	if m.Installed(tool) == rel.Version {
		m.progress(Progress{Name: tool, Stage: "done", Percent: 100, Message: "Already up to date (" + rel.Version + ")"})
		return nil
	}
	if err := m.install(ctx, tool, rel); err != nil {
		return err
	}
	m.progress(Progress{Name: tool, Stage: "done", Percent: 100, Message: "Installed " + rel.Version})
	return nil
}

// Setup installs every missing tool. Optional tools that fail do not fail setup.
func (m *Manager) Setup(ctx context.Context) error {
	var firstErr error
	for _, t := range Tools {
		if m.Installed(t) != "" {
			continue
		}
		if err := m.Update(ctx, t); err != nil && Required(t) && firstErr == nil {
			firstErr = fmt.Errorf("%s: %w", t, err)
		}
	}
	return firstErr
}

// Rollback flips current and previous.
func (m *Manager) Rollback(tool string) error {
	if !known(tool) {
		return ErrUnknownTool
	}
	m.mu.Lock()
	if m.busy[tool] {
		m.mu.Unlock()
		return ErrBusy
	}
	m.busy[tool] = true
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		m.busy[tool] = false
		m.mu.Unlock()
		m.changed()
	}()

	c := m.readCurrent(tool)
	if c.Previous == "" {
		return errors.New("there is no earlier version to go back to")
	}
	if _, err := os.Stat(m.exePath(tool, c.Previous)); err != nil {
		return errors.New("the earlier version is no longer on this computer")
	}
	return m.writeCurrent(tool, current{Version: c.Previous, Previous: c.Version})
}

func (m *Manager) install(ctx context.Context, tool string, rel *Release) error {
	if err := os.MkdirAll(filepath.Join(m.toolDir(tool), "versions"), 0o755); err != nil {
		return err
	}
	// The temp folder is on the same filesystem so the final move is a rename.
	tmp, err := os.MkdirTemp(m.toolDir(tool), ".tmp-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	stage := filepath.Join(tmp, "stage")
	if err := os.Mkdir(stage, 0o755); err != nil {
		return err
	}
	for i, a := range rel.Assets {
		if err := m.fetchAsset(ctx, tool, a, i, len(rel.Assets), tmp, stage); err != nil {
			return err
		}
	}

	m.progress(Progress{Name: tool, Stage: "testing", Percent: 100, Message: "Testing it runs"})
	if err := prepareExecutables(stage); err != nil {
		return err
	}
	if err := smokeTest(ctx, tool, stage); err != nil {
		return err
	}

	m.progress(Progress{Name: tool, Stage: "installing", Percent: 100, Message: "Finishing up"})
	dest := m.versionDir(tool, rel.Version)
	if _, err := os.Stat(dest); err == nil {
		// A folder for this version exists (an earlier, possibly broken copy).
		// Replace it unless a running job is using it.
		if m.leased(tool, rel.Version) {
			return errors.New("this version is in use by a download; try again when it finishes")
		}
		if err := os.RemoveAll(dest); err != nil {
			return err
		}
	}
	if err := os.Rename(stage, dest); err != nil {
		return err
	}
	old := m.readCurrent(tool)
	next := current{Version: rel.Version, Previous: old.Previous}
	if old.Version != "" && old.Version != rel.Version {
		next.Previous = old.Version
	}
	if err := m.writeCurrent(tool, next); err != nil {
		return err
	}
	if m.AllowPrune {
		m.prune(tool, next)
	}
	return nil
}

func (m *Manager) leased(tool, version string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.inUse[tool+"/"+version] > 0
}

// prune keeps current, previous, and anything a running job uses.
func (m *Manager) prune(tool string, c current) {
	dir := filepath.Join(m.toolDir(tool), "versions")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	keep := map[string]bool{safeName(c.Version): true, safeName(c.Previous): true}
	m.mu.Lock()
	for k, n := range m.inUse {
		if n > 0 && filepath.Dir(k) == tool {
			keep[safeName(filepath.Base(k))] = true
		}
	}
	m.mu.Unlock()
	for _, e := range entries {
		if !keep[e.Name()] {
			if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
				m.logf("prune %s/%s: %v", tool, e.Name(), err)
			}
		}
	}
	// Leftover temp folders from a crash.
	if tmps, _ := filepath.Glob(filepath.Join(m.toolDir(tool), ".tmp-*")); len(tmps) > 0 {
		sort.Strings(tmps)
		for _, t := range tmps {
			if fi, err := os.Stat(t); err == nil && time.Since(fi.ModTime()) > time.Hour {
				os.RemoveAll(t)
			}
		}
	}
}

// Versions lists installed versions for each tool (for `fetchly versions`).
func (m *Manager) Versions(tool string) []string {
	entries, _ := os.ReadDir(filepath.Join(m.toolDir(tool), "versions"))
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	return out
}

var unsafeChars = regexp.MustCompile(`[^A-Za-z0-9._-]`)

func safeName(v string) string {
	s := unsafeChars.ReplaceAllString(v, "_")
	if s == "" || s == "." || s == ".." {
		return "_"
	}
	return s
}

func writeFileAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
