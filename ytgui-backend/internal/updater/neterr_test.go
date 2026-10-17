package updater

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestNetErrIsShort(t *testing.T) {
	long := "https://release-assets.githubusercontent.com/github-production-release-asset/1/abc?sig=" + strings.Repeat("x", 400)
	m := New(t.TempDir(), "")
	m.http = &http.Client{Timeout: time.Nanosecond}
	_, err := m.fetchText(context.Background(), long)
	if err == nil {
		t.Fatal("expected an error")
	}
	msg := err.Error()
	if strings.Contains(msg, "sig=") || len(msg) > 200 {
		t.Fatalf("message still contains the URL or is too long: %q", msg)
	}
	if !strings.HasPrefix(msg, "couldn't reach release-assets.githubusercontent.com:") {
		t.Fatalf("unexpected message: %q", msg)
	}
	t.Log(msg)
}
