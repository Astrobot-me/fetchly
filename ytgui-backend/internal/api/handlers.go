package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ytgui/internal/jobs"
	"ytgui/internal/osutil"
	"ytgui/internal/store"
	"ytgui/internal/updater"
)

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"app": AppID, "version": s.Version})
}

func (s *Server) handleTools(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.toolsResponse())
}

func (s *Server) handleToolsCheck(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	s.upd.CheckLatest(ctx)
	writeJSON(w, http.StatusOK, s.toolsResponse())
}

func (s *Server) handleToolsSetup(w http.ResponseWriter, r *http.Request) {
	go func() { _ = s.upd.Setup(s.bg) }()
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) handleToolUpdate(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !isTool(name) {
		writeErr(w, http.StatusNotFound, "unknown_tool", "Unknown tool.")
		return
	}
	go func() { _ = s.upd.Update(s.bg, name) }()
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) handleToolRollback(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !isTool(name) {
		writeErr(w, http.StatusNotFound, "unknown_tool", "Unknown tool.")
		return
	}
	if err := s.upd.Rollback(name); err != nil {
		code := http.StatusConflict
		writeErr(w, code, "rollback_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.toolsResponse())
}

func isTool(name string) bool {
	for _, t := range updater.Tools {
		if t == name {
			return true
		}
	}
	return false
}

func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL string `json:"url"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	info, err := s.jobs.FetchInfo(r.Context(), body.URL)
	if err != nil {
		writeJobErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

func writeJobErr(w http.ResponseWriter, err error) {
	var ue *jobs.UserError
	switch {
	case errors.As(err, &ue):
		writeErr(w, http.StatusUnprocessableEntity, ue.Code, ue.Msg)
	case errors.Is(err, jobs.ErrNotFound):
		writeErr(w, http.StatusNotFound, "not_found", "That download is no longer in the list.")
	case errors.Is(err, context.Canceled):
		writeErr(w, 499, "canceled", "Canceled.")
	default:
		writeErr(w, http.StatusInternalServerError, "internal", "Something went wrong: "+err.Error())
	}
}

func (s *Server) handleJobs(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.jobs.Snapshot())
}

func (s *Server) handleJobAdd(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL       string  `json:"url"`
		Quality   string  `json:"quality"`
		Title     string  `json:"title"`
		Thumbnail *string `json:"thumbnail"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	j, err := s.jobs.Add(body.URL, body.Quality, body.Title, body.Thumbnail)
	if err != nil {
		writeJobErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, j)
}

func (s *Server) handleJobCancel(w http.ResponseWriter, r *http.Request) {
	if err := s.jobs.Cancel(r.PathValue("id")); err != nil {
		writeJobErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleJobRetry(w http.ResponseWriter, r *http.Request) {
	j, err := s.jobs.Retry(r.PathValue("id"))
	if err != nil {
		writeJobErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, j)
}

func (s *Server) handleJobsClear(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.jobs.ClearFinished())
}

func (s *Server) handleQueueResume(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.jobs.Resume())
}

func (s *Server) handleSettingsGet(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.store.Settings())
}

func (s *Server) handleSettingsPut(w http.ResponseWriter, r *http.Request) {
	var body store.Settings
	if !readJSON(w, r, &body) {
		return
	}
	saved, err := s.store.SaveSettings(body)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, "bad_settings", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.store.History())
}

func (s *Server) handleHistoryClear(w http.ResponseWriter, r *http.Request) {
	if err := s.store.ClearHistory(); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "History could not be cleared.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleOpenFolder opens the downloads folder, or reveals a file. Only
// paths inside the download folder are allowed, so a page cannot use this
// to open arbitrary locations.
func (s *Server) handleOpenFolder(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
	}
	if r.ContentLength != 0 {
		if err := decodeOptional(r, &body); err != nil {
			writeErr(w, http.StatusBadRequest, "bad_request", "The request could not be read.")
			return
		}
	}
	dir := s.store.Settings().DownloadDir
	var err error
	if body.Path == "" {
		_ = os.MkdirAll(dir, 0o755)
		err = osutil.OpenFolder(dir)
	} else {
		if !inside(dir, body.Path) {
			writeErr(w, http.StatusForbidden, "outside_downloads", "Only files in the download folder can be shown.")
			return
		}
		err = osutil.RevealFile(body.Path)
	}
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, "open_failed", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func decodeOptional(r *http.Request, v any) error {
	b, err := io.ReadAll(r.Body)
	if err != nil || len(b) == 0 {
		return err
	}
	return json.Unmarshal(b, v)
}

// inside reports whether p is dir itself or somewhere below it.
func inside(dir, p string) bool {
	absDir, err1 := filepath.Abs(dir)
	absP, err2 := filepath.Abs(p)
	if err1 != nil || err2 != nil {
		return false
	}
	rel, err := filepath.Rel(absDir, absP)
	return err == nil && !filepath.IsAbs(rel) && rel != ".." &&
		!strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func (s *Server) handlePickFolder(w http.ResponseWriter, r *http.Request) {
	p, err := osutil.PickFolder(r.Context(), s.store.Settings().DownloadDir)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, "picker_failed", err.Error())
		return
	}
	var out *string
	if p != "" {
		out = &p
	}
	writeJSON(w, http.StatusOK, map[string]*string{"path": out})
}

func (s *Server) handleQuit(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	if s.Quit != nil {
		go s.Quit()
	}
}
