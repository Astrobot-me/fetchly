package jobs

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"ytgui/internal/osutil"
	"ytgui/internal/store"
	"ytgui/internal/updater"
)

// Markers are literal text in yt-dlp's output templates, so we can pick
// our lines out of everything else it prints.
const (
	markDL   = "[ytgui-dl] "
	markPP   = "[ytgui-pp] "
	markFile = "[ytgui-file] "
)

var retryDelays = []time.Duration{3 * time.Second, 10 * time.Second}

// childEnv makes yt-dlp (Python) print UTF-8 on every platform.
func childEnv() []string {
	return append(os.Environ(), "PYTHONIOENCODING=utf-8", "PYTHONUTF8=1")
}

// baseArgs are shared by info lookups and downloads. The release func
// unpins the ffmpeg/deno versions.
func (m *Manager) baseArgs() ([]string, func(), error) {
	args := []string{"--ignore-config", "--no-playlist", "--encoding", "utf-8", "--color", "never"}
	var releases []func()
	release := func() {
		for _, r := range releases {
			r()
		}
	}
	ff, rel, err := m.upd.Acquire(updater.FFmpeg)
	if err != nil {
		return nil, nil, &UserError{Msg: "A helper tool (FFmpeg) isn't installed yet. Open Settings to install it.", Code: "not_ready", Permanent: true}
	}
	releases = append(releases, rel)
	args = append(args, "--ffmpeg-location", ff.Dir)

	// Deno is optional. Without it yt-dlp still works but may miss formats.
	if d, rel, err := m.upd.Acquire(updater.Deno); err == nil {
		releases = append(releases, rel)
		args = append(args, "--js-runtimes", "deno:"+d.Path)
	}
	return args, release, nil
}

func (m *Manager) runWithRetries(j *Job) {
	for attempt := 1; attempt <= MaxAttempts; attempt++ {
		m.update(j, false, func(j *Job) {
			j.State, j.Attempt, j.Phase, j.Percent = Running, attempt, "Starting", 0
			j.Speed, j.ETA, j.Error = nil, nil, nil
		})
		file, skipped, err := m.runOnce(j)

		m.mu.Lock()
		canceled := j.canceled
		m.mu.Unlock()
		if canceled {
			m.finish(j, Canceled, nil, nil)
			return
		}
		if err == nil {
			if skipped {
				m.finishWithPhase(j, file, "Already downloaded")
			} else {
				m.finish(j, Done, file, nil)
			}
			return
		}
		var ue *UserError
		if !errors.As(err, &ue) {
			ue = &UserError{Msg: "The download failed: " + err.Error(), Code: "internal"}
		}
		if ue.Permanent || attempt == MaxAttempts {
			m.finish(j, Failed, nil, ue)
			return
		}
		// Wait before retrying; a cancel during the wait stops it.
		delay := retryDelays[min(attempt-1, len(retryDelays)-1)]
		m.update(j, false, func(j *Job) {
			msg := ue.Msg
			j.Error = &msg
			j.Phase = "Problem, trying again…"
			j.Speed, j.ETA = nil, nil
		})
		if !m.sleepUnlessCanceled(j, delay) {
			m.finish(j, Canceled, nil, nil)
			return
		}
	}
}

func (m *Manager) sleepUnlessCanceled(j *Job, d time.Duration) bool {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	m.mu.Lock()
	if j.canceled || m.closed {
		m.mu.Unlock()
		return false
	}
	j.cancel = func() { cancel() }
	m.mu.Unlock()
	<-ctx.Done()
	m.mu.Lock()
	defer m.mu.Unlock()
	j.cancel = nil
	return !j.canceled && !m.closed
}

func (m *Manager) finish(j *Job, st State, file *string, ue *UserError) {
	m.finishAs(j, st, file, ue, "Finished")
}

// finishWithPhase marks a job done with a custom phase, e.g. when the
// file was already on disk and nothing was downloaded.
func (m *Manager) finishWithPhase(j *Job, file *string, phase string) {
	m.finishAs(j, Done, file, nil, phase)
}

func (m *Manager) finishAs(j *Job, st State, file *string, ue *UserError, donePhase string) {
	now := time.Now()
	pause := false
	m.update(j, false, func(j *Job) {
		j.State, j.FinishedAt, j.Speed, j.ETA, j.cancel = st, &now, nil, nil, nil
		switch st {
		case Done:
			j.Phase, j.Percent, j.FilePath, j.Error = donePhase, 100, file, nil
		case Canceled:
			j.Phase, j.Error = "Canceled", nil
		case Failed:
			msg := ue.Msg
			j.Phase, j.Error, j.SuggestUpdate = "Failed", &msg, ue.SuggestUpdate
			// The queue stops after a failure, so the user notices it.
			if !m.paused {
				m.paused, pause = true, true
			}
		}
	})
	if pause {
		m.emit("queue", map[string]bool{"queuePaused": true})
	}
	if st == Done || st == Failed {
		e := store.HistoryEntry{ID: j.ID, URL: j.URL, Title: j.Title, Quality: j.Quality,
			FilePath: file, State: string(st), FinishedAt: now}
		if ue != nil {
			e.Error = &ue.Msg
		}
		m.store.AddHistory(e)
	}
}

type progressJSON struct {
	Status             string   `json:"status"`
	DownloadedBytes    *float64 `json:"downloaded_bytes"`
	TotalBytes         *float64 `json:"total_bytes"`
	TotalBytesEstimate *float64 `json:"total_bytes_estimate"`
	Speed              *float64 `json:"speed"`
	ETA                *float64 `json:"eta"`
	Filename           string   `json:"filename"`
	TmpFilename        string   `json:"tmpfilename"`
	FragmentIndex      *float64 `json:"fragment_index"`
	FragmentCount      *float64 `json:"fragment_count"`
}

// CacheDirName holds a download's pieces (separate video and audio
// streams, .part files) while it runs; only the finished file is moved
// out into the download folder.
const CacheDirName = ".cache"

// runOnce spawns yt-dlp for one attempt. It returns the final file path
// and whether yt-dlp skipped the download because the file already existed.
func (m *Manager) runOnce(j *Job) (*string, bool, error) {
	settings := m.store.Settings()
	if err := os.MkdirAll(settings.DownloadDir, 0o755); err != nil {
		return nil, false, &UserError{Msg: "The download folder can't be used. Choose a different folder in Settings.", Code: "permission", Permanent: true}
	}
	cacheDir := filepath.Join(settings.DownloadDir, CacheDirName)
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return nil, false, &UserError{Msg: "The download folder can't be used. Choose a different folder in Settings.", Code: "permission", Permanent: true}
	}
	osutil.HideFile(cacheDir)
	cleanStale(cacheDir, 24*time.Hour)

	fmtArgs, err := formatArgs(j.Quality)
	if err != nil {
		return nil, false, &UserError{Msg: err.Error(), Code: "bad_quality", Permanent: true}
	}
	args, release, err := m.baseArgs()
	if err != nil {
		return nil, false, err
	}
	defer release()
	ytdlp, releaseY, err := m.upd.Acquire(updater.YtDlp)
	if err != nil {
		return nil, false, &UserError{Msg: "The downloader isn't installed yet. Open Settings to install it.", Code: "not_ready", Permanent: true}
	}
	defer releaseY()

	args = append(args,
		"--newline", "--progress", "--quiet", "--no-warnings",
		"--progress-template", "download:"+markDL+"%(info.vcodec)s %(progress)j",
		"--progress-template", "postprocess:"+markPP+"%(progress.status)s %(progress.postprocessor)s",
		"--print", "after_move:"+markFile+"%(filepath)s",
		"--windows-filenames", "--no-mtime",
		"-P", settings.DownloadDir,
		"-P", "temp:"+cacheDir,
		"-o", "%(title).150B [%(id)s] "+fileSuffix(j.Quality)+".%(ext)s",
	)
	args = append(args, fmtArgs...)
	args = append(args, "--", j.URL)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.CommandContext(ctx, ytdlp.Path, args...)
	osutil.Background(cmd)
	cmd.Env = childEnv()
	cmd.Dir = settings.DownloadDir
	cmd.Cancel = func() error { return osutil.KillTree(cmd) }
	cmd.WaitDelay = 5 * time.Second

	m.mu.Lock()
	if j.canceled || m.closed {
		m.mu.Unlock()
		return nil, false, context.Canceled
	}
	j.cancel = cancel
	m.mu.Unlock()

	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		return nil, false, &UserError{Msg: "The downloader could not be started: " + err.Error(), Code: "spawn"}
	}

	var (
		mu         sync.Mutex
		final      string
		downloaded bool // any progress line means something was fetched
		errLines   []string
		partials   = map[string]bool{}
	)
	handle := func(line string) {
		line = strings.TrimRight(line, "\r\n")
		switch {
		case strings.HasPrefix(line, markDL):
			mu.Lock()
			downloaded = true
			mu.Unlock()
			m.onProgress(j, strings.TrimPrefix(line, markDL), &mu, partials)
		case strings.HasPrefix(line, markPP):
			m.onPostprocess(j, strings.TrimPrefix(line, markPP))
		case strings.HasPrefix(line, markFile):
			mu.Lock()
			final = strings.TrimSpace(strings.TrimPrefix(line, markFile))
			mu.Unlock()
		case strings.TrimSpace(line) != "":
			mu.Lock()
			if len(errLines) < 200 {
				errLines = append(errLines, line)
			}
			mu.Unlock()
		}
	}
	var wg sync.WaitGroup
	for _, r := range []io.Reader{stdout, stderr} {
		wg.Add(1)
		go func(r io.Reader) {
			defer wg.Done()
			br := bufio.NewReaderSize(r, 64<<10)
			for {
				line, err := br.ReadString('\n')
				if line != "" {
					handle(line)
				}
				if err != nil {
					return
				}
			}
		}(r)
	}
	wg.Wait()
	waitErr := cmd.Wait()

	m.mu.Lock()
	j.cancel = nil
	canceled := j.canceled
	m.mu.Unlock()
	if canceled {
		removePartials(settings.DownloadDir, partials)
		return nil, false, context.Canceled
	}
	if waitErr != nil {
		return nil, false, Classify(strings.Join(errLines, "\n"))
	}
	if final == "" {
		return nil, false, &UserError{Msg: "The download finished but the file could not be found.", Code: "no_file"}
	}
	return &final, !downloaded, nil
}

// cleanStale removes leftovers in the cache folder (from a crash or a
// killed app) that are older than age. Fresh files may belong to a
// download running in another instance, so they are left alone.
func cleanStale(dir string, age time.Duration) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		info, err := e.Info()
		if err == nil && time.Since(info.ModTime()) > age {
			_ = os.RemoveAll(filepath.Join(dir, e.Name()))
		}
	}
}

func (m *Manager) onProgress(j *Job, rest string, mu *sync.Mutex, partials map[string]bool) {
	vcodec, raw, ok := strings.Cut(rest, " ")
	if !ok {
		return
	}
	var p progressJSON
	if json.Unmarshal([]byte(raw), &p) != nil {
		return
	}
	mu.Lock()
	for _, f := range []string{p.TmpFilename, p.Filename} {
		if f != "" {
			partials[f] = true
		}
	}
	mu.Unlock()

	phase := "Downloading video"
	if vcodec == "none" {
		phase = "Downloading audio"
	}
	pct := 0.0
	total := p.TotalBytes
	if total == nil {
		total = p.TotalBytesEstimate
	}
	switch {
	case p.Status == "finished":
		pct = 100
	case total != nil && *total > 0 && p.DownloadedBytes != nil:
		pct = *p.DownloadedBytes / *total * 100
	case p.FragmentCount != nil && *p.FragmentCount > 0 && p.FragmentIndex != nil:
		pct = *p.FragmentIndex / *p.FragmentCount * 100
	}
	m.update(j, p.Status != "finished", func(j *Job) {
		j.Phase, j.Percent, j.Speed, j.ETA = phase, min(pct, 100), p.Speed, p.ETA
	})
}

func (m *Manager) onPostprocess(j *Job, rest string) {
	status, pp, _ := strings.Cut(rest, " ")
	if status != "started" {
		return
	}
	phase := "Finishing up"
	switch strings.TrimSpace(pp) {
	case "Merger":
		phase = "Joining video and audio"
	case "ExtractAudio":
		phase = "Converting audio"
	}
	m.update(j, false, func(j *Job) {
		j.Phase, j.Percent, j.Speed, j.ETA = phase, 100, nil, nil
	})
}

// removePartials deletes leftovers of a canceled download, but only files
// inside the download folder.
func removePartials(dir string, files map[string]bool) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return
	}
	for f := range files {
		p := f
		if !filepath.IsAbs(p) {
			p = filepath.Join(absDir, p)
		}
		if rel, err := filepath.Rel(absDir, p); err != nil || strings.HasPrefix(rel, "..") {
			continue
		}
		for _, cand := range []string{p, p + ".part", p + ".ytdl"} {
			_ = os.Remove(cand)
		}
		// Fragmented downloads leave numbered pieces next to the .part file.
		if frags, _ := filepath.Glob(p + ".part-Frag*"); len(frags) > 0 {
			for _, fr := range frags {
				_ = os.Remove(fr)
			}
		}
	}
}
