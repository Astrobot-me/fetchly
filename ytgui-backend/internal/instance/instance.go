// Package instance picks the port and finds already-running copies.
//
// Rules: bind the main port if free. If this app already holds it, bind
// the fallback port as a second instance. If both are held by this app,
// open the browser on the existing one instead of starting a third. A port
// held by some other program is skipped.
package instance

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"ytgui/internal/paths"
)

const (
	MainPort     = 8765
	FallbackPort = 8766
	appID        = "ytgui"
)

type Result struct {
	Listener  net.Listener
	Port      int
	Secondary bool // another instance of this app is running
	// ExistingURL is set instead of Listener when no new instance should start.
	ExistingURL string
}

// Info is written to instances/<port>.json so later launches can open
// the running instance with its token. Only the current user can read it.
type Info struct {
	PID   int    `json:"pid"`
	Port  int    `json:"port"`
	Token string `json:"token"`
}

func Acquire(appData string) (*Result, error) {
	ports := []int{MainPort, FallbackPort}
	if p, err := strconv.Atoi(os.Getenv("YTGUI_PORT")); err == nil && p > 0 {
		ports = []int{p, p + 1}
	}
	var ours []int
	for _, port := range ports {
		ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
		if err == nil {
			return &Result{Listener: ln, Port: port, Secondary: len(ours) > 0}, nil
		}
		if isOurs(port) {
			ours = append(ours, port)
		}
	}
	if len(ours) == 0 {
		return nil, fmt.Errorf("ports %d and %d are both used by other programs; close them and try again", ports[0], ports[1])
	}
	return &Result{ExistingURL: URLFor(appData, ours[0])}, nil
}

// isOurs asks the program on port whether it is this app.
func isOurs(port int) bool {
	c := http.Client{Timeout: 2 * time.Second}
	resp, err := c.Get("http://127.0.0.1:" + strconv.Itoa(port) + "/api/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	var h struct {
		App string `json:"app"`
	}
	return json.NewDecoder(resp.Body).Decode(&h) == nil && h.App == appID
}

// URLFor builds the address of a running instance, with its token if known.
func URLFor(appData string, port int) string {
	u := "http://127.0.0.1:" + strconv.Itoa(port) + "/"
	b, err := os.ReadFile(file(appData, port))
	if err != nil {
		return u
	}
	var info Info
	if json.Unmarshal(b, &info) == nil && info.Token != "" {
		u += "?token=" + info.Token
	}
	return u
}

func file(appData string, port int) string {
	return filepath.Join(paths.Instances(appData), strconv.Itoa(port)+".json")
}

// Register records this instance; the returned func removes the record.
func Register(appData string, port int, token string) (func(), error) {
	dir := paths.Instances(appData)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	b, _ := json.Marshal(Info{PID: os.Getpid(), Port: port, Token: token})
	p := file(appData, port)
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return nil, err
	}
	if err := os.Rename(tmp, p); err != nil {
		return nil, err
	}
	return func() { _ = os.Remove(p) }, nil
}
