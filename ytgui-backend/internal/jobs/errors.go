package jobs

import (
	"strings"
)

// rule maps a yt-dlp error substring (lowercase) to a plain-language message.
type rule struct {
	match         []string
	msg           string
	code          string
	suggestUpdate bool
	permanent     bool
}

var rules = []rule{
	{match: []string{"unsupported url"}, code: "unsupported",
		msg: "This website or link type isn't supported.", permanent: true},
	{match: []string{"private video"}, code: "private",
		msg: "This video is private, so it can't be downloaded.", permanent: true},
	{match: []string{"sign in to confirm your age", "age-restricted", "inappropriate for some users"}, code: "age",
		msg: "This video is age-restricted and needs a signed-in account, which this app doesn't support.", permanent: true},
	{match: []string{"members-only", "join this channel"}, code: "members",
		msg: "This video is for channel members only.", permanent: true},
	{match: []string{"not available in your country", "geo restrict", "geo-restrict"}, code: "geo",
		msg: "This video isn't available in your country.", permanent: true},
	{match: []string{"video unavailable", "this video has been removed", "this video is no longer available", "does not exist"}, code: "unavailable",
		msg: "This video is unavailable. It may have been removed or made private.", permanent: true},
	{match: []string{"premieres in", "this live event will begin"}, code: "upcoming",
		msg: "This video hasn't been released yet.", permanent: true},
	{match: []string{"sign in to confirm you", "not a bot"}, code: "bot_check",
		msg: "YouTube is asking to confirm you're not a bot. Wait a while and try again. If it keeps happening, updating the downloader may help.", suggestUpdate: true},
	{match: []string{"no space left", "disk full", "errno 28"}, code: "disk_full",
		msg: "Your disk is full. Free up some space and try again.", permanent: true},
	{match: []string{"permission denied", "access is denied", "errno 13"}, code: "permission",
		msg: "The app isn't allowed to save in the download folder. Choose a different folder in Settings.", permanent: true},
	{match: []string{"ffmpeg not found", "ffprobe not found", "ffmpeg is not installed"}, code: "no_ffmpeg",
		msg: "A helper tool (FFmpeg) is missing. Open Settings and reinstall the tools."},
	{match: []string{"requested format is not available"}, code: "format",
		msg: "That quality isn't available for this video any more. Pick another quality, or update the downloader.", suggestUpdate: true},
	{match: []string{"unable to extract", "nsig extraction failed", "signature extraction failed", "unable to decode", "please report this issue", "confirm you are on the latest version", "unable to download api page", "precondition check failed"}, code: "extraction",
		msg: "The downloader couldn't read this video. YouTube changes often, so updating the downloader usually fixes this.", suggestUpdate: true},
	{match: []string{"http error 403", "forbidden"}, code: "forbidden",
		msg: "YouTube refused the download. Updating the downloader often fixes this.", suggestUpdate: true},
	{match: []string{"http error 429", "too many requests"}, code: "rate_limited",
		msg: "YouTube is limiting downloads from your connection. Wait a while and try again."},
	{match: []string{"getaddrinfo failed", "name or service not known", "nodename nor servname", "failed to resolve", "network is unreachable", "timed out", "connection reset", "connection refused", "temporary failure in name resolution", "unable to connect"}, code: "network",
		msg: "Couldn't reach the internet. Check your connection and try again."},
}

// Classify turns yt-dlp's stderr into a UserError.
func Classify(stderr string) *UserError {
	low := strings.ToLower(stderr)
	for _, r := range rules {
		for _, m := range r.match {
			if strings.Contains(low, m) {
				return &UserError{Msg: r.msg, Code: r.code, SuggestUpdate: r.suggestUpdate, Permanent: r.permanent}
			}
		}
	}
	msg := "The download failed."
	if line := lastErrorLine(stderr); line != "" {
		msg += " Details: " + line
	}
	return &UserError{Msg: msg, Code: "unknown"}
}

func lastErrorLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		l := strings.TrimSpace(lines[i])
		if strings.HasPrefix(l, "ERROR:") {
			l = strings.TrimSpace(strings.TrimPrefix(l, "ERROR:"))
			if len(l) > 240 {
				l = l[:240] + "…"
			}
			return l
		}
	}
	return ""
}
