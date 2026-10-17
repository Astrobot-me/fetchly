// Package jobs runs yt-dlp: info lookups and a one-at-a-time download queue.
package jobs

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"ytgui/internal/store"
	"ytgui/internal/updater"
)

type State string

const (
	Queued   State = "queued"
	Running  State = "running"
	Done     State = "done"
	Failed   State = "failed"
	Canceled State = "canceled"
)

// MaxAttempts is the first try plus 2 retries.
const MaxAttempts = 3

// Job is what the UI sees. Only the manager mutates it, under m.mu.
type Job struct {
	ID            string     `json:"id"`
	URL           string     `json:"url"`
	Title         string     `json:"title"`
	Thumbnail     *string    `json:"thumbnail"`
	Quality       string     `json:"quality"`
	State         State      `json:"state"`
	Phase         string     `json:"phase"`
	Percent       float64    `json:"percent"`
	Speed         *float64   `json:"speed"`
	ETA           *float64   `json:"eta"`
	Attempt       int        `json:"attempt"`
	Error         *string    `json:"error"`
	SuggestUpdate bool       `json:"suggestUpdate"`
	FilePath      *string    `json:"filePath"`
	CreatedAt     time.Time  `json:"createdAt"`
	FinishedAt    *time.Time `json:"finishedAt"`

	cancel   func()
	canceled bool
}

type Snapshot struct {
	Jobs        []Job `json:"jobs"`
	QueuePaused bool  `json:"queuePaused"`
}

// Emit sends an event to the UI ("job", "job-removed", "queue").
type Emit func(event string, data any)

type Manager struct {
	upd   *updater.Manager
	store *store.Store
	emit  Emit

	mu       sync.Mutex
	jobs     []*Job
	paused   bool
	wake     chan struct{}
	lastEmit map[string]time.Time
	closed   bool
	wg       sync.WaitGroup
}

var ErrNotFound = errors.New("no such download")

func NewManager(upd *updater.Manager, st *store.Store, emit Emit) *Manager {
	if emit == nil {
		emit = func(string, any) {}
	}
	m := &Manager{upd: upd, store: st, emit: emit, wake: make(chan struct{}, 1), lastEmit: map[string]time.Time{}}
	m.wg.Add(1)
	go m.loop()
	return m
}

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (m *Manager) Snapshot() Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.snapshotLocked()
}

func (m *Manager) snapshotLocked() Snapshot {
	s := Snapshot{Jobs: make([]Job, 0, len(m.jobs)), QueuePaused: m.paused}
	for _, j := range m.jobs {
		s.Jobs = append(s.Jobs, *j)
	}
	return s
}

// Add queues a download.
func (m *Manager) Add(rawURL, quality, title string, thumb *string) (Job, error) {
	u, err := ValidateURL(rawURL)
	if err != nil {
		return Job{}, err
	}
	if _, err := formatArgs(quality); err != nil {
		return Job{}, &UserError{Msg: "Pick a quality first.", Code: "bad_quality"}
	}
	if title == "" {
		title = u
	}
	j := &Job{ID: newID(), URL: u, Title: title, Thumbnail: thumb, Quality: quality,
		State: Queued, Phase: "Waiting", CreatedAt: time.Now()}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return Job{}, errors.New("the app is shutting down")
	}
	m.jobs = append(m.jobs, j)
	cp := *j
	m.mu.Unlock()
	m.emit("job", cp)
	m.kick()
	return cp, nil
}

// Cancel stops an active job, or removes a finished one from the list.
func (m *Manager) Cancel(id string) error {
	m.mu.Lock()
	j, i := m.find(id)
	if j == nil {
		m.mu.Unlock()
		return ErrNotFound
	}
	switch j.State {
	case Queued:
		j.State, j.Phase = Canceled, "Canceled"
		now := time.Now()
		j.FinishedAt = &now
		cp := *j
		m.mu.Unlock()
		m.emit("job", cp)
		return nil
	case Running:
		j.canceled = true
		if j.cancel != nil {
			j.cancel()
		}
		m.mu.Unlock()
		return nil
	}
	m.jobs = append(m.jobs[:i], m.jobs[i+1:]...)
	m.mu.Unlock()
	m.emit("job-removed", map[string]string{"id": id})
	return nil
}

// Retry re-queues a failed or canceled job.
func (m *Manager) Retry(id string) (Job, error) {
	m.mu.Lock()
	j, _ := m.find(id)
	if j == nil {
		m.mu.Unlock()
		return Job{}, ErrNotFound
	}
	if j.State != Failed && j.State != Canceled {
		cp := *j
		m.mu.Unlock()
		return cp, nil
	}
	j.State, j.Phase, j.Percent, j.Attempt = Queued, "Waiting", 0, 0
	j.Error, j.Speed, j.ETA, j.FinishedAt, j.SuggestUpdate, j.canceled = nil, nil, nil, nil, false, false
	cp := *j
	// Retrying by hand also resumes a paused queue.
	resumed := m.paused
	m.paused = false
	m.mu.Unlock()
	m.emit("job", cp)
	if resumed {
		m.emit("queue", map[string]bool{"queuePaused": false})
	}
	m.kick()
	return cp, nil
}

func (m *Manager) Resume() Snapshot {
	m.mu.Lock()
	m.paused = false
	s := m.snapshotLocked()
	m.mu.Unlock()
	m.emit("queue", map[string]bool{"queuePaused": false})
	m.kick()
	return s
}

// ClearFinished drops done/failed/canceled jobs from the list.
func (m *Manager) ClearFinished() Snapshot {
	m.mu.Lock()
	var keep []*Job
	var removed []string
	for _, j := range m.jobs {
		if j.State == Queued || j.State == Running {
			keep = append(keep, j)
		} else {
			removed = append(removed, j.ID)
		}
	}
	m.jobs = keep
	s := m.snapshotLocked()
	m.mu.Unlock()
	for _, id := range removed {
		m.emit("job-removed", map[string]string{"id": id})
	}
	return s
}

// Shutdown cancels everything and waits for the running process to exit.
func (m *Manager) Shutdown() {
	m.mu.Lock()
	m.closed = true
	for _, j := range m.jobs {
		if j.State == Running && j.cancel != nil {
			j.canceled = true
			j.cancel()
		}
	}
	m.mu.Unlock()
	m.kick()
	done := make(chan struct{})
	go func() { m.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
	}
}

func (m *Manager) find(id string) (*Job, int) {
	for i, j := range m.jobs {
		if j.ID == id {
			return j, i
		}
	}
	return nil, -1
}

func (m *Manager) kick() {
	select {
	case m.wake <- struct{}{}:
	default:
	}
}

// loop runs queued jobs one at a time.
func (m *Manager) loop() {
	defer m.wg.Done()
	for {
		m.mu.Lock()
		if m.closed {
			m.mu.Unlock()
			return
		}
		var next *Job
		if !m.paused {
			for _, j := range m.jobs {
				if j.State == Queued {
					next = j
					break
				}
			}
		}
		m.mu.Unlock()
		if next == nil {
			<-m.wake
			continue
		}
		m.runWithRetries(next)
	}
}

// update mutates a job under the lock and emits it. Progress-only updates
// are throttled to ~4 per second per job.
func (m *Manager) update(j *Job, progressOnly bool, f func(*Job)) {
	m.mu.Lock()
	f(j)
	if progressOnly {
		if time.Since(m.lastEmit[j.ID]) < 250*time.Millisecond {
			m.mu.Unlock()
			return
		}
	}
	m.lastEmit[j.ID] = time.Now()
	cp := *j
	m.mu.Unlock()
	m.emit("job", cp)
}
