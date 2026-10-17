package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"

	"ytgui/internal/jobs"
	"ytgui/internal/paths"
	"ytgui/internal/store"
	"ytgui/internal/updater"
)

// Set at build time with -ldflags "-X main.version=... -X main.feedURL=...".
var (
	version = "dev"
	feedURL = ""
)

func main() {
	log.SetFlags(log.Ltime)
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	appData, err := paths.AppData()
	if err != nil {
		return fmt.Errorf("cannot find app-data folder: %w", err)
	}
	upd := updater.New(paths.Bin(appData), feedURL)
	upd.Logger = log.Default()

	cmd := ""
	if len(args) > 0 {
		cmd = args[0]
	}
	switch cmd {
	case "", "serve", "--no-browser":
		openBrowser := true
		for _, a := range args {
			if a == "--no-browser" {
				openBrowser = false
			}
		}
		return serve(appData, upd, openBrowser)
	case "update":
		return cliUpdate(upd, args[1:])
	case "rollback":
		if len(args) < 2 {
			return fmt.Errorf("usage: ytgui rollback <tool>")
		}
		if err := upd.Rollback(args[1]); err != nil {
			return err
		}
		fmt.Printf("%s is now %s\n", args[1], upd.Installed(args[1]))
		return nil
	case "versions":
		fmt.Printf("ytgui %s\napp data: %s\n", version, appData)
		for _, t := range updater.Tools {
			cur := upd.Installed(t)
			if cur == "" {
				cur = "(not installed)"
			}
			fmt.Printf("  %-7s current %-14s installed: %s\n", t, cur, strings.Join(upd.Versions(t), ", "))
		}
		return nil
	case "version", "--version":
		fmt.Println(version)
		return nil
	case "info", "get":
		if len(args) < 2 {
			return fmt.Errorf("usage: ytgui %s <url> [quality]", cmd)
		}
		return cliJob(upd, store.Open(appData), cmd, args[1:])
	}
	return fmt.Errorf("unknown command %q (try: update [tool], rollback <tool>, versions, info <url>, get <url> [quality])", cmd)
}

// cliJob looks up a video or downloads it from the terminal (for testing).
func cliJob(upd *updater.Manager, st *store.Store, cmd string, args []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if cmd == "info" {
		mgr := jobs.NewManager(upd, st, nil)
		defer mgr.Shutdown()
		info, err := mgr.FetchInfo(ctx, args[0])
		if err != nil {
			return err
		}
		b, _ := json.MarshalIndent(info, "", "  ")
		fmt.Println(string(b))
		return nil
	}

	quality := st.Settings().DefaultQuality
	if len(args) > 1 {
		quality = args[1]
	}
	done := make(chan jobs.Job, 1)
	last := ""
	mgr := jobs.NewManager(upd, st, func(event string, data any) {
		j, ok := data.(jobs.Job)
		if event != "job" || !ok {
			return
		}
		line := fmt.Sprintf("[try %d] %s %.0f%%", j.Attempt, j.Phase, j.Percent)
		if j.Error != nil && j.State == jobs.Running {
			line += " (" + *j.Error + ")"
		}
		if line != last {
			fmt.Println(line)
			last = line
		}
		if j.State == jobs.Done || j.State == jobs.Failed || j.State == jobs.Canceled {
			done <- j
		}
	})
	defer mgr.Shutdown()
	j, err := mgr.Add(args[0], quality, "", nil)
	if err != nil {
		return err
	}
	select {
	case j = <-done:
	case <-ctx.Done():
		_ = mgr.Cancel(j.ID)
		j = <-done
	}
	switch j.State {
	case jobs.Done:
		fmt.Println("Saved:", *j.FilePath)
		return nil
	case jobs.Failed:
		return fmt.Errorf("%s", *j.Error)
	}
	return fmt.Errorf("canceled")
}

func cliUpdate(upd *updater.Manager, args []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	last := ""
	upd.OnProgress = func(p updater.Progress) {
		line := fmt.Sprintf("[%s] %s", p.Name, p.Message)
		if line != last {
			fmt.Println(line)
			last = line
		}
	}
	tools := updater.Tools
	if len(args) > 0 {
		tools = args
	}
	var failed bool
	for _, t := range tools {
		if err := upd.Update(ctx, t); err != nil {
			fmt.Fprintf(os.Stderr, "[%s] failed: %v\n", t, err)
			failed = true
		}
	}
	if failed {
		return fmt.Errorf("some updates failed")
	}
	return nil
}
