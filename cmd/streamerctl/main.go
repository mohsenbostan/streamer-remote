// Command streamerctl lets a streamer's Twitch chat trigger keyboard and
// mouse input on their machine in real time.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"streamer-remote/internal/browser"
	"streamer-remote/internal/config"
	"streamer-remote/internal/logging"
	"streamer-remote/internal/supervisor"
	"streamer-remote/internal/update"
)

// version is set at build time via -ldflags "-X main.version=vX.Y.Z" by the
// release workflow. Local builds stay "dev", which disables update checks.
var version = "dev"

const updateRepo = "mohsenbostan/streamer-remote"

func main() {
	// No arguments at all means someone double-clicked the exe rather than
	// running it from a terminal with flags: guide them with prompts and
	// keep the window open instead of flashing and closing.
	interactive := len(os.Args) == 1

	configPath := flag.String("config", "config.yaml", "path to the config file")
	localOnly := flag.Bool("local", false, "run without connecting to Twitch; drive commands from the console instead")
	debug := flag.Bool("debug", false, "enable verbose logging")
	doUpdate := flag.Bool("update", false, "check for and install an update, then exit")
	flag.Parse()

	console := bufio.NewReader(os.Stdin)
	update.CleanupOldBinary()

	if *doUpdate {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		checkForUpdate(ctx, console, false, true)
		exit(interactive, console, 0)
		return
	}

	cfg, err := loadOrSetupConfig(*configPath, interactive, console)
	if err != nil {
		fmt.Fprintln(os.Stderr, "config error:", err)
		exit(interactive, console, 1)
	}
	if cfg == nil {
		return // first run, non-interactive: default config written, instructions printed
	}
	if *debug {
		cfg.LogDebug = true
	}

	logOpts := logging.DefaultOptions()
	logOpts.Debug = cfg.LogDebug
	logger, closeLog := logging.New(logOpts)
	defer closeLog.Close()

	mode := *localOnly
	if interactive {
		checkCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		checkForUpdate(checkCtx, console, true, false)
		cancel()

		choice, ok := runMenu(console,
			func() { manageRewardActions(*configPath, cfg, console) },
			func() {
				updateCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
				checkForUpdate(updateCtx, console, true, true)
				cancel()
			},
		)
		if !ok {
			return
		}
		mode = choice == menuTestLocally
		if !mode {
			if err := ensureTwitchConfigured(*configPath, cfg, console); err != nil {
				fmt.Fprintln(os.Stderr, "setup error:", err)
				exit(interactive, console, 1)
			}
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger.Info("starting streamer-remote", "local_only", mode)
	fmt.Println("Running. Press Ctrl+C to stop.")

	if err := supervisor.Run(ctx, supervisor.Options{
		Config:    cfg,
		Logger:    logger,
		LocalOnly: mode,
		Console:   console,
	}); err != nil {
		logger.Error("fatal startup error", "error", err)
		exit(interactive, console, 1)
	}

	logger.Info("shutting down")
	exit(interactive, console, 0)
}

// loadOrSetupConfig loads the config, creating and (in interactive mode)
// walking the user through filling in a default one if it doesn't exist
// yet. Returns (nil, nil) for the non-interactive first-run case, where
// the caller should just exit after the default file is written.
func loadOrSetupConfig(path string, interactive bool, console *bufio.Reader) (*config.Config, error) {
	cfg, err := config.Load(path)
	if errors.Is(err, config.ErrDefaultCreated) {
		if !interactive {
			fmt.Printf("No config found. Created %s with defaults.\nEdit it (Twitch channel/client ID, keybind limits) and run this again.\n", path)
			return nil, nil
		}
		fmt.Println("Welcome! This is your first run, so let's get you set up.")
		fmt.Println("(You can skip Twitch setup for now and just try local testing.)")
		if err := runSetupWizard(path, console); err != nil {
			return nil, err
		}
		return config.Load(path)
	}
	return cfg, err
}

// ensureTwitchConfigured runs the setup wizard if Twitch fields are still
// missing when the user picks "Start" from the menu.
func ensureTwitchConfigured(path string, cfg *config.Config, console *bufio.Reader) error {
	if cfg.Twitch.Validate() == nil {
		return nil
	}
	fmt.Println("Twitch isn't set up yet.")
	if err := runSetupWizard(path, console); err != nil {
		return err
	}
	reloaded, err := config.Load(path)
	if err != nil {
		return err
	}
	*cfg = *reloaded
	return cfg.Twitch.Validate()
}

func runSetupWizard(path string, console *bufio.Reader) error {
	fmt.Println()
	fmt.Println("To read your Twitch chat you need a free Twitch Developer app (takes a minute).")
	fmt.Print("Open the registration page in your browser now? [Y/n]: ")
	if answer := readLine(console); answer == "" || strings.EqualFold(answer, "y") {
		if err := browser.Open("https://dev.twitch.tv/console/apps"); err != nil {
			fmt.Println("Couldn't open a browser automatically. Go to https://dev.twitch.tv/console/apps manually.")
		}
	}
	fmt.Println()
	fmt.Println("On that page, click 'Register Your Application' and fill in:")
	fmt.Println("  Name:               anything, e.g. 'my-stream-remote'")
	fmt.Println("  OAuth Redirect URL: http://localhost")
	fmt.Println("  Category:           Chat Bot")
	fmt.Println("  Client Type:        Public")
	fmt.Println()

	fmt.Print("Enter your Twitch channel name (lowercase, no '#'), or leave blank to skip Twitch setup for now: ")
	channel := readLine(console)
	if channel == "" {
		fmt.Println("Skipped. You can run local tests now and set up Twitch later by editing config.yaml.")
		return nil
	}

	fmt.Print("Paste the Client ID shown on the app's page: ")
	clientID := readLine(console)
	if clientID == "" {
		fmt.Println("No Client ID entered. Skipped Twitch setup; edit config.yaml later to finish it.")
		return nil
	}

	if err := config.UpdateTwitchFields(path, channel, clientID); err != nil {
		return err
	}
	fmt.Println("Saved. Continuing...")
	fmt.Println()
	return nil
}

type menuChoice int

const (
	menuStart menuChoice = iota
	menuTestLocally
	menuExit
)

func runMenu(console *bufio.Reader, onManageRewards, onCheckUpdates func()) (menuChoice, bool) {
	for {
		fmt.Println()
		fmt.Println("What would you like to do?")
		fmt.Println("  1) Start - connect to Twitch chat")
		fmt.Println("  2) Test locally - try out keybinds without Twitch")
		fmt.Println("  3) Manage channel-points-only actions")
		fmt.Println("  4) Check for updates")
		fmt.Println("  5) Exit")
		fmt.Print("> ")
		switch readLine(console) {
		case "1":
			return menuStart, true
		case "2":
			return menuTestLocally, true
		case "3":
			onManageRewards()
		case "4":
			onCheckUpdates()
		case "5", "":
			return menuExit, false
		default:
			fmt.Println("Please enter 1, 2, 3, 4, or 5.")
		}
	}
}

// checkForUpdate looks for a newer release on GitHub and, if found, offers
// (or in headless --update mode, proceeds) to download and install it.
// It relies purely on the GitHub REST API and a plain HTTPS download, no
// git required. On a successful install it relaunches the new binary and
// exits the current process; it does not return in that case.
func checkForUpdate(ctx context.Context, console *bufio.Reader, promptBeforeApplying, announce bool) {
	rel, err := update.FetchLatest(ctx, updateRepo)
	if err != nil {
		if announce {
			fmt.Println("Could not check for updates:", err)
		}
		return
	}
	if !update.IsNewer(version, rel.TagName) {
		if announce {
			fmt.Printf("You're already on the latest version (%s).\n", version)
		}
		return
	}
	assetURL, ok := rel.AssetURL()
	if !ok {
		if announce {
			fmt.Printf("Update %s is available but has no Windows build attached; skipping.\n", rel.TagName)
		}
		return
	}

	fmt.Printf("\nUpdate available: %s -> %s\n", version, rel.TagName)
	if promptBeforeApplying {
		fmt.Print("Update now? [Y/n]: ")
		if ans := readLine(console); ans != "" && !strings.EqualFold(ans, "y") {
			fmt.Println("Skipped for now.")
			return
		}
	}

	exePath, err := os.Executable()
	if err != nil {
		fmt.Println("Update failed: could not locate the running executable:", err)
		return
	}
	newPath := exePath + ".new"

	fmt.Println("Downloading update...")
	if err := update.Download(ctx, assetURL, newPath); err != nil {
		fmt.Println("Update failed:", err)
		return
	}
	fmt.Println("Installing update and restarting...")
	if err := update.Apply(newPath); err != nil {
		fmt.Println("Update failed:", err)
		return
	}
}

func readLine(console *bufio.Reader) string {
	line, _ := console.ReadString('\n')
	return strings.TrimSpace(line)
}

// exit prints a closing message and waits for a keypress in interactive
// mode, so a double-clicked window doesn't vanish before it can be read.
func exit(interactive bool, console *bufio.Reader, code int) {
	if interactive {
		fmt.Println("\nPress Enter to close this window...")
		readLine(console)
	}
	if code != 0 {
		os.Exit(code)
	}
}
