package jobs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"sort"
	"strings"
	"time"

	"ytgui/internal/osutil"
	"ytgui/internal/updater"
)

type QualityOption struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Height      *int   `json:"height"`
	ApproxBytes *int64 `json:"approxBytes"`
}

type VideoInfo struct {
	URL         string          `json:"url"`
	ID          string          `json:"id"`
	Title       string          `json:"title"`
	Uploader    *string         `json:"uploader"`
	DurationSec *float64        `json:"durationSec"`
	Thumbnail   *string         `json:"thumbnail"`
	Qualities   []QualityOption `json:"qualities"`
}

// UserError carries a plain-language message for the UI.
type UserError struct {
	Msg           string
	Code          string
	SuggestUpdate bool
	Permanent     bool // retrying will not help
}

func (e *UserError) Error() string { return e.Msg }

// ValidateURL accepts only http(s) URLs.
func ValidateURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", &UserError{Msg: "That doesn't look like a web link. Copy the address from your browser and paste it here.", Code: "bad_url", Permanent: true}
	}
	return u.String(), nil
}

// ladder is the list of standard heights offered. A video's real height is
// rounded up to the next step, so an 800-pixel-high widescreen film shows
// as 1080p, and the filter height<=1080 still picks it.
var ladder = []int{4320, 2160, 1440, 1080, 720, 480, 360, 240, 144}

type ytFormat struct {
	FormatID       string   `json:"format_id"`
	Ext            string   `json:"ext"`
	VCodec         string   `json:"vcodec"`
	ACodec         string   `json:"acodec"`
	Height         *int     `json:"height"`
	TBR            *float64 `json:"tbr"`
	Filesize       *int64   `json:"filesize"`
	FilesizeApprox *int64   `json:"filesize_approx"`
}

type ytInfo struct {
	Type       string     `json:"_type"`
	ID         string     `json:"id"`
	Title      string     `json:"title"`
	Uploader   *string    `json:"uploader"`
	Channel    *string    `json:"channel"`
	Duration   *float64   `json:"duration"`
	Thumbnail  *string    `json:"thumbnail"`
	WebpageURL string     `json:"webpage_url"`
	IsLive     bool       `json:"is_live"`
	Formats    []ytFormat `json:"formats"`
}

// FetchInfo runs `yt-dlp -J` and summarizes the result.
func (m *Manager) FetchInfo(ctx context.Context, rawURL string) (*VideoInfo, error) {
	u, err := ValidateURL(rawURL)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	args, release, err := m.baseArgs()
	if err != nil {
		return nil, err
	}
	defer release()
	ytdlp, releaseY, err := m.upd.Acquire(updater.YtDlp)
	if err != nil {
		return nil, &UserError{Msg: "The downloader isn't installed yet.", Code: "not_ready"}
	}
	defer releaseY()

	args = append(args, "-J", "--", u)
	cmd := exec.CommandContext(ctx, ytdlp.Path, args...)
	osutil.Background(cmd)
	cmd.Env = childEnv()
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	cmd.Cancel = func() error { return osutil.KillTree(cmd) }
	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, &UserError{Msg: "Looking up the video took too long. Check your internet connection and try again.", Code: "timeout"}
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, Classify(stderr.String())
	}

	var info ytInfo
	if err := json.Unmarshal(stdout.Bytes(), &info); err != nil {
		return nil, fmt.Errorf("could not read the video details: %w", err)
	}
	if info.Type == "playlist" || info.Type == "multi_video" {
		return nil, &UserError{Msg: "This link is a playlist. For now, open a single video and copy its link.", Code: "playlist", Permanent: true}
	}
	if info.IsLive {
		return nil, &UserError{Msg: "Live streams can't be downloaded. Try again after the stream has ended.", Code: "live", Permanent: true}
	}
	if info.WebpageURL == "" {
		info.WebpageURL = u
	}
	up := info.Uploader
	if up == nil {
		up = info.Channel
	}
	return &VideoInfo{
		URL: info.WebpageURL, ID: info.ID, Title: info.Title, Uploader: up,
		DurationSec: info.Duration, Thumbnail: info.Thumbnail,
		Qualities: qualities(info.Formats),
	}, nil
}

func qualities(formats []ytFormat) []QualityOption {
	size := func(f ytFormat) int64 {
		if f.Filesize != nil {
			return *f.Filesize
		}
		if f.FilesizeApprox != nil {
			return *f.FilesizeApprox
		}
		return 0
	}
	// Largest audio-only stream approximates "best audio"; the largest m4a
	// is what "Audio only" saves.
	var audio, m4a int64
	for _, f := range formats {
		if f.VCodec == "none" && f.ACodec != "none" && f.ACodec != "" {
			s := size(f)
			audio = max(audio, s)
			if f.Ext == "m4a" {
				m4a = max(m4a, s)
			}
		}
	}
	// For each ladder step, the biggest video stream that fits.
	best := map[int]int64{}
	present := map[int]bool{}
	for _, f := range formats {
		if f.VCodec == "none" || f.VCodec == "" || f.Height == nil || *f.Height <= 0 {
			continue
		}
		step := 0
		for i := len(ladder) - 1; i >= 0; i-- {
			if *f.Height <= ladder[i] {
				step = ladder[i]
				break
			}
		}
		if step == 0 {
			step = ladder[0]
		}
		present[step] = true
		if s := size(f); s > best[step] {
			best[step] = s
		}
	}
	steps := make([]int, 0, len(present))
	for s := range present {
		steps = append(steps, s)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(steps)))

	var out []QualityOption
	for _, s := range steps {
		h := s
		q := QualityOption{ID: fmt.Sprint(s), Label: label(s), Height: &h}
		if best[s] > 0 {
			total := best[s] + audio
			q.ApproxBytes = &total
		}
		out = append(out, q)
	}
	if len(out) == 0 {
		// No height info (some non-YouTube sites): offer best only.
		out = append(out, QualityOption{ID: "best", Label: "Best available"})
	}
	aq := QualityOption{ID: "audio", Label: "Audio only"}
	if a := m4a; a > 0 || audio > 0 {
		if a == 0 {
			a = audio
		}
		aq.ApproxBytes = &a
	}
	return append(out, aq)
}

func label(h int) string {
	switch h {
	case 4320:
		return "8K (4320p)"
	case 2160:
		return "4K (2160p)"
	case 1440:
		return "1440p"
	}
	return fmt.Sprintf("%dp", h)
}

// fileSuffix names the chosen quality in the file name, so downloading
// the same video in another quality makes a new file instead of being
// skipped as "already downloaded".
func fileSuffix(quality string) string {
	switch quality {
	case "best", "":
		return "best"
	case "audio":
		return "audio"
	}
	return quality + "p"
}

// formatArgs maps a quality id to yt-dlp selection arguments.
// -S res,vcodec:h264,ext:mp4:m4a keeps the highest resolution first and,
// among equals, prefers H.264 and mp4/m4a so the file plays in any player.
// YouTube has no H.264 above 1080p, so 1440p and 4K fall back to VP9/AV1.
func formatArgs(quality string) ([]string, error) {
	switch quality {
	case "best", "":
		return []string{"-f", "bv*+ba/b", "-S", "res,vcodec:h264,ext:mp4:m4a", "--merge-output-format", "mp4"}, nil
	case "audio":
		// Saved as delivered, no conversion, so ffprobe and encoders are
		// not needed. YouTube always offers an m4a (AAC) stream.
		return []string{"-f", "ba[ext=m4a]/ba"}, nil
	}
	for _, h := range ladder {
		if quality == fmt.Sprint(h) {
			f := fmt.Sprintf("bv*[height<=%d]+ba/b[height<=%d]", h, h)
			return []string{"-f", f, "-S", "res,vcodec:h264,ext:mp4:m4a", "--merge-output-format", "mp4"}, nil
		}
	}
	return nil, errors.New("unknown quality " + quality)
}
