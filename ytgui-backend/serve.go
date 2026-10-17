package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"ytgui/internal/api"
	"ytgui/internal/instance"
	"ytgui/internal/jobs"
	"ytgui/internal/osutil"
	"ytgui/internal/store"
	"ytgui/internal/updater"
	"ytgui/web"
)

// serve runs the app until the console window closes, Ctrl+C, or Quit.
func serve(appData string, upd *updater.Manager, openBrowser bool) error {
	inst, err := instance.Acquire(appData)
	if err != nil {
		return err
	}
	if inst.ExistingURL != "" {
		fmt.Println("The app is already running.")
		if !openBrowser || osutil.OpenBrowser(inst.ExistingURL) != nil {
			fmt.Println("Open this address in your browser:", inst.ExistingURL)
		}
		time.Sleep(time.Second)
		return nil
	}

	tok := make([]byte, 32)
	if _, err := rand.Read(tok); err != nil {
		return err
	}
	token := hex.EncodeToString(tok)
	unregister, err := instance.Register(appData, inst.Port, token)
	if err != nil {
		return fmt.Errorf("cannot record running instance: %w", err)
	}
	defer unregister()

	// Only the instance on the main port prunes old tool versions: a
	// second instance can't see the first one's running downloads.
	upd.AllowPrune = !inst.Secondary

	// SIGHUP: closing the macOS Terminal window. SIGTERM: closing the
	// Windows console (CTRL_CLOSE_EVENT). Both must stop downloads first,
	// because yt-dlp runs in its own process group and would outlive us.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()

	st := store.Open(appData)
	hub := api.NewHub()
	jm := jobs.NewManager(upd, st, hub.Publish)
	srv := api.New(ctx, hub, jm, upd, st)
	srv.Port, srv.Token, srv.Version, srv.UI = inst.Port, token, version, web.Dist()

	upd.OnProgress = func(p updater.Progress) { hub.Publish("tool-progress", p) }
	upd.OnChange = srv.PublishTools

	quit := make(chan struct{})
	var quitOnce sync.Once
	srv.Quit = func() { quitOnce.Do(func() { close(quit) }) }

	httpSrv := srv.Serve(inst.Listener)

	base := fmt.Sprintf("http://127.0.0.1:%d/", inst.Port)
	openURL := base + "?token=" + token
	fmt.Println()
	fmt.Println("  Fetchly is running.")
	if api.DevMode {
		fmt.Println("  DEV MODE. UI dev server: http://localhost:5173/?token=" + token)
	}
	fmt.Println("  If your browser didn't open, go to:")
	fmt.Println("  " + openURL)
	fmt.Println()
	fmt.Println("  Close this window to quit.")
	fmt.Println()
	if openBrowser {
		if err := osutil.OpenBrowser(openURL); err != nil {
			log.Printf("could not open the browser: %v", err)
		}
	}

	// Look for tool updates in the background. Nothing is installed
	// without the user pressing Update.
	go func() {
		cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		upd.CheckLatest(cctx)
	}()

	select {
	case <-ctx.Done():
		fmt.Println("Stopping…")
	case <-quit:
		fmt.Println("Quit pressed. Stopping…")
	}

	// Windows gives a closing console about 5 seconds, so stop children first.
	srv.Stop()
	jm.Shutdown()
	sctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(sctx)
	return nil
}
