// Package api is the HTTP layer: security middleware, REST handlers, SSE,
// and the embedded UI.
package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"net"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"ytgui/internal/jobs"
	"ytgui/internal/store"
	"ytgui/internal/updater"
)

const (
	AppID       = "ytgui"
	TokenHeader = "X-Ytgui-Token"
	maxBody     = 64 << 10
)

type Server struct {
	Port    int
	Token   string
	Version string
	UI      fs.FS
	// Quit is called once when the user presses Quit.
	Quit func()

	hub   *Hub
	jobs  *jobs.Manager
	upd   *updater.Manager
	store *store.Store

	// bg is the app's lifetime context for background updates.
	bg       context.Context
	quitting chan struct{}
	quitOnce sync.Once
}

func New(bg context.Context, hub *Hub, jm *jobs.Manager, upd *updater.Manager, st *store.Store) *Server {
	return &Server{hub: hub, jobs: jm, upd: upd, store: st, bg: bg, quitting: make(chan struct{})}
}

// ToolsResponse matches the UI's ToolsResponse type.
type ToolsResponse struct {
	Tools      []updater.Status `json:"tools"`
	Ready      bool             `json:"ready"`
	AppVersion string           `json:"appVersion"`
}

func (s *Server) toolsResponse() ToolsResponse {
	return ToolsResponse{Tools: s.upd.Status(), Ready: s.upd.Ready(), AppVersion: s.Version}
}

// PublishTools pushes the current tool status to every tab.
func (s *Server) PublishTools() { s.hub.Publish("tools", s.toolsResponse()) }

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.handleHealth)

	api := http.NewServeMux()
	api.HandleFunc("GET /api/tools", s.handleTools)
	api.HandleFunc("POST /api/tools/check", s.handleToolsCheck)
	api.HandleFunc("POST /api/tools/setup", s.handleToolsSetup)
	api.HandleFunc("POST /api/tools/{name}/update", s.handleToolUpdate)
	api.HandleFunc("POST /api/tools/{name}/rollback", s.handleToolRollback)
	api.HandleFunc("POST /api/info", s.handleInfo)
	api.HandleFunc("GET /api/jobs", s.handleJobs)
	api.HandleFunc("POST /api/jobs", s.handleJobAdd)
	api.HandleFunc("POST /api/jobs/clear", s.handleJobsClear)
	api.HandleFunc("DELETE /api/jobs/{id}", s.handleJobCancel)
	api.HandleFunc("POST /api/jobs/{id}/retry", s.handleJobRetry)
	api.HandleFunc("POST /api/queue/resume", s.handleQueueResume)
	api.HandleFunc("GET /api/settings", s.handleSettingsGet)
	api.HandleFunc("PUT /api/settings", s.handleSettingsPut)
	api.HandleFunc("GET /api/history", s.handleHistory)
	api.HandleFunc("DELETE /api/history", s.handleHistoryClear)
	api.HandleFunc("POST /api/open-folder", s.handleOpenFolder)
	api.HandleFunc("POST /api/pick-folder", s.handlePickFolder)
	api.HandleFunc("POST /api/quit", s.handleQuit)
	api.HandleFunc("GET /api/events", s.handleEvents)
	mux.Handle("/api/", s.requireToken(api))

	mux.Handle("/", s.uiHandler())
	return s.secure(mux)
}

// secure applies to every request: Host allow-list (blocks DNS rebinding),
// Origin check on state-changing methods, and safe default headers.
func (s *Server) secure(next http.Handler) http.Handler {
	port := strconv.Itoa(s.Port)
	hosts := map[string]bool{"127.0.0.1:" + port: true, "localhost:" + port: true}
	origins := map[string]bool{"http://127.0.0.1:" + port: true, "http://localhost:" + port: true}
	if DevMode {
		origins["http://localhost:5173"] = true
		origins["http://127.0.0.1:5173"] = true
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !hosts[strings.ToLower(r.Host)] {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			// Browsers always send Origin on these; non-browser callers
			// without one still need the token.
			if o := r.Header.Get("Origin"); o != "" && !origins[o] {
				writeErr(w, http.StatusForbidden, "forbidden_origin", "Request blocked.")
				return
			}
			if site := r.Header.Get("Sec-Fetch-Site"); site == "cross-site" {
				writeErr(w, http.StatusForbidden, "forbidden_origin", "Request blocked.")
				return
			}
		}
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Content-Security-Policy",
			"default-src 'self'; img-src 'self' https: data:; style-src 'self' 'unsafe-inline'; "+
				"connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) requireToken(next http.Handler) http.Handler {
	want := []byte(s.Token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := []byte(r.Header.Get(TokenHeader))
		if len(got) == 0 || subtle.ConstantTimeCompare(got, want) != 1 {
			writeErr(w, http.StatusUnauthorized, "bad_token", "Please reopen the app from its launcher window.")
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		r.Body = http.MaxBytesReader(w, r.Body, maxBody)
		next.ServeHTTP(w, r)
	})
}

// uiHandler serves the embedded build. Unknown paths get index.html.
func (s *Server) uiHandler() http.Handler {
	files := http.FileServerFS(s.UI)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p != "" {
			if fi, err := fs.Stat(s.UI, p); err == nil && !fi.IsDir() {
				if strings.HasPrefix(p, "assets/") {
					// Vite puts a content hash in these file names.
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				files.ServeHTTP(w, r)
				return
			}
		}
		b, err := fs.ReadFile(s.UI, "index.html")
		if err != nil {
			http.Error(w, "UI not built: copy ytgui-web/dist into web/dist and rebuild", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(b)
	})
}

// Serve runs until Shutdown.
func (s *Server) Serve(ln net.Listener) *http.Server {
	srv := &http.Server{
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
		ErrorLog:          log.New(discard{}, "", 0),
	}
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("server stopped: %v", err)
		}
	}()
	return srv
}

// Stop ends SSE streams so tabs notice the app has stopped.
func (s *Server) Stop() {
	s.quitOnce.Do(func() { close(s.quitting) })
	s.hub.CloseAll()
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }

type apiError struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, apiError{Error: msg, Code: code})
}

func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if r.Body == nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "Missing request body.")
		return false
	}
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(v); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "The request could not be read.")
		return false
	}
	return true
}
