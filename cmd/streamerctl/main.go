// Command streamerctl lets a streamer's Twitch chat trigger keyboard and
// mouse input on their machine in real time, configured and monitored
// through a local web dashboard.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"streamer-remote/internal/browser"
	"streamer-remote/internal/config"
	"streamer-remote/internal/logging"
	"streamer-remote/internal/msgbox"
	"streamer-remote/internal/supervisor"
	"streamer-remote/internal/tray"
	"streamer-remote/internal/update"
	"streamer-remote/internal/webui"
)

// version is set at build time via -ldflags "-X main.version=vX.Y.Z" by the
// release workflow. Local builds stay "dev", which disables update checks.
var version = "dev"

func main() {
	configPath := flag.String("config", "config.yaml", "path to the config file")
	localOnly := flag.Bool("local", false, "run without connecting to Twitch")
	debug := flag.Bool("debug", false, "enable verbose logging")
	doUpdate := flag.Bool("update", false, "check for and install an update, then exit")
	noBrowser := flag.Bool("no-browser", false, "don't automatically open the dashboard in a browser")
	flag.Parse()

	update.CleanupOldBinary()

	if *doUpdate {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		runHeadlessUpdate(ctx)
		return
	}

	cfg, err := config.Load(*configPath)
	if errors.Is(err, config.ErrDefaultCreated) {
		// First run: config.yaml now exists with an empty Twitch section.
		// No console prompts needed — the dashboard's Overview tab shows
		// a setup form when Twitch isn't configured yet.
		cfg, err = config.Load(*configPath)
	}
	if err != nil {
		fatal("Configuration error", err.Error())
		return
	}
	if *debug {
		cfg.LogDebug = true
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	hub := webui.NewHub()
	logOpts := logging.DefaultOptions()
	logOpts.Debug = cfg.LogDebug
	logger, closeLog := logging.New(logOpts, webui.NewLogHandler(hub))
	defer closeLog.Close()

	core := supervisor.NewCore(ctx, cfg, logger)
	server := webui.New(ctx, *configPath, core.Dispatcher, logger, version, *localOnly, hub)

	if *localOnly {
		logger.Info("running in local-only mode: no Twitch or Kick connection will be made")
	} else {
		server.StartExistingTwitchSession()
		server.StartExistingKickSession()
	}

	listener, dashboardURL, err := server.Bind()
	if err != nil {
		fatal("Startup error", err.Error())
		return
	}

	go func() {
		if err := server.Serve(ctx, listener); err != nil {
			logger.Error("dashboard server error", "error", err)
		}
	}()

	if !*noBrowser {
		if err := browser.Open(dashboardURL); err != nil {
			logger.Debug("could not auto-open browser", "error", err)
		}
	}

	go tray.Run(tray.Callbacks{
		OnOpenDashboard: func() { _ = browser.Open(dashboardURL) },
		OnTogglePause: func() bool {
			if core.Dispatcher.Enabled() {
				core.Dispatcher.Pause("tray")
				return true
			}
			core.Dispatcher.Resume("tray")
			return false
		},
		OnQuit: stop,
	})

	logger.Info("starting streamer-remote", "version", version, "local_only", *localOnly, "url", dashboardURL)

	<-ctx.Done()
	logger.Info("shutting down")
	tray.ForceQuit()
}

// fatal reports a startup error both to stderr (visible if launched from
// a terminal) and via a native dialog (visible even in the no-console
// release build, where stderr goes nowhere anyone can see).
func fatal(title, message string) {
	fmt.Fprintln(os.Stderr, title+":", message)
	msgbox.Error(title, message)
}

// runHeadlessUpdate supports `streamerctl.exe --update` for scripted use
// (e.g. a scheduled task): check, and if newer, install without prompting
// — the flag itself is the confirmation.
func runHeadlessUpdate(ctx context.Context) {
	const repo = "mohsenbostan/streamer-remote"

	rel, err := update.FetchLatest(ctx, repo)
	if err != nil {
		fmt.Println("Could not check for updates:", err)
		return
	}
	if !update.IsNewer(version, rel.TagName) {
		fmt.Printf("Already on the latest version (%s).\n", version)
		return
	}
	assetURL, ok := rel.AssetURL()
	if !ok {
		fmt.Printf("Update %s is available but has no Windows build attached.\n", rel.TagName)
		return
	}

	exePath, err := os.Executable()
	if err != nil {
		fmt.Println("Update failed: could not locate the running executable:", err)
		return
	}
	newPath := exePath + ".new"

	fmt.Printf("Updating %s -> %s...\n", version, rel.TagName)
	if err := update.Download(ctx, assetURL, newPath); err != nil {
		fmt.Println("Update failed:", err)
		return
	}
	if err := update.Apply(newPath); err != nil {
		fmt.Println("Update failed:", err)
	}
}
