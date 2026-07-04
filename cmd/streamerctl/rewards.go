package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"streamer-remote/internal/commands"
	"streamer-remote/internal/config"
	"streamer-remote/internal/supervisor"
	"streamer-remote/internal/twitch"
	"streamer-remote/internal/twitchauth"
)

// manageRewardActions is the interactive menu for gating actions (like
// Alt+F4 or locking the screen) behind Twitch Channel Points: the app
// creates the reward on Twitch itself, so the streamer never has to leave
// this menu or hand-edit config.yaml.
func manageRewardActions(configPath string, cfg *config.Config, console *bufio.Reader) {
	if err := cfg.Twitch.Validate(); err != nil {
		fmt.Println("Set up Twitch first (choose \"Start\" once) before managing Channel Points actions.")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	quietLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
	auth := twitchauth.New(cfg.Twitch.ClientID, supervisor.TokenCachePath, supervisor.TwitchAuthScopes, quietLogger)
	tok, err := auth.EnsureToken(ctx)
	if err != nil {
		fmt.Println("Could not authorize with Twitch:", err)
		return
	}
	helix := twitch.NewHelixClient(cfg.Twitch.ClientID, tok.AccessToken)
	broadcasterID, err := helix.GetOwnUserID(ctx)
	if err != nil {
		fmt.Println("Could not resolve your Twitch account:", err)
		return
	}

	for {
		fmt.Println()
		if len(cfg.RewardActions) == 0 {
			fmt.Println("No channel-points-only actions set up yet.")
		} else {
			fmt.Println("Channel-points-only actions:")
			for i, ra := range cfg.RewardActions {
				fmt.Printf("  %d) %-20s -> redeem \"%s\" (%d points)\n", i+1, ra.Action, ra.RewardTitle, ra.Cost)
			}
		}
		fmt.Println()
		fmt.Println("  a) Add a new one")
		fmt.Println("  r) Remove one")
		fmt.Println("  b) Back")
		fmt.Print("> ")

		switch strings.ToLower(readLine(console)) {
		case "a":
			addRewardAction(ctx, configPath, cfg, console, helix, broadcasterID)
		case "r":
			removeRewardAction(ctx, configPath, cfg, console, helix, broadcasterID)
		case "b", "":
			return
		default:
			fmt.Println("Please enter a, r, or b.")
		}
	}
}

func addRewardAction(ctx context.Context, configPath string, cfg *config.Config, console *bufio.Reader, helix *twitch.HelixClient, broadcasterID string) {
	fmt.Println()
	fmt.Println("What should this reward trigger? Same syntax as a chat command, without the prefix.")
	fmt.Println("Examples: alt+f4    lwin    hold:w:2000")
	fmt.Print("Action: ")
	action := readLine(console)
	if action == "" {
		fmt.Println("Cancelled.")
		return
	}
	if _, err := commands.ParseCombo(action, cfg); err != nil {
		fmt.Println("That action isn't valid:", err)
		return
	}

	fmt.Print("Reward title, as viewers will see it (max 45 characters): ")
	title := readLine(console)
	if title == "" || len(title) > 45 {
		fmt.Println("Title must be 1-45 characters. Cancelled.")
		return
	}

	fmt.Print("Point cost: ")
	costStr := readLine(console)
	cost, err := strconv.Atoi(costStr)
	if err != nil || cost <= 0 {
		fmt.Println("Cost must be a positive number. Cancelled.")
		return
	}

	rewardID, err := helix.CreateCustomReward(ctx, broadcasterID, title, cost)
	if err != nil {
		fmt.Println("Twitch rejected the reward:", err)
		fmt.Println("(Channel Points rewards require Affiliate or Partner status.)")
		return
	}

	ra := config.RewardAction{Action: strings.ToLower(action), RewardTitle: title, Cost: cost, RewardID: rewardID}
	if err := config.AddRewardAction(configPath, ra); err != nil {
		fmt.Println("Created the reward on Twitch, but failed to save it locally:", err)
		return
	}
	cfg.RewardActions = append(cfg.RewardActions, ra)
	fmt.Printf("Done. \"%s\" now only works via that reward, not by typing in chat (mods are exempt).\n", action)
}

func removeRewardAction(ctx context.Context, configPath string, cfg *config.Config, console *bufio.Reader, helix *twitch.HelixClient, broadcasterID string) {
	if len(cfg.RewardActions) == 0 {
		fmt.Println("Nothing to remove.")
		return
	}
	fmt.Print("Which number to remove? ")
	n, err := strconv.Atoi(readLine(console))
	if err != nil || n < 1 || n > len(cfg.RewardActions) {
		fmt.Println("Invalid number. Cancelled.")
		return
	}
	ra := cfg.RewardActions[n-1]

	if err := helix.DeleteCustomReward(ctx, broadcasterID, ra.RewardID); err != nil {
		fmt.Println("Could not delete the reward on Twitch (it may already be gone):", err)
	}
	if err := config.RemoveRewardAction(configPath, ra.RewardID); err != nil {
		fmt.Println("Failed to update the config file:", err)
		return
	}
	cfg.RewardActions = append(cfg.RewardActions[:n-1], cfg.RewardActions[n:]...)
	fmt.Printf("Removed. \"%s\" can now be typed directly in chat again.\n", ra.Action)
}
