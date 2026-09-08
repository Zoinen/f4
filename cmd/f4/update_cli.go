package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"golang.org/x/term"
)

// updateChannelName spells a channel the way the command line does.
func updateChannelName(channel int) string {
	if channel == updateChannelNightly {
		return "nightly"
	}
	return "stable"
}

// parseUpdateChannelArg turns the `--update` argument into a channel number.
// An empty argument means the configured channel.
func parseUpdateChannelArg(arg string, configured int) (channel int, explicit bool, err error) {
	switch strings.ToLower(strings.TrimSpace(arg)) {
	case "":
		return configured, false, nil
	case "stable", "latest":
		return updateChannelStable, true, nil
	case "nightly":
		return updateChannelNightly, true, nil
	}
	return 0, false, fmt.Errorf("unknown update channel %q (expected \"stable\" or \"nightly\")", arg)
}

// runUpdateCLI serves `f4 --update [stable|nightly]`: the machinery behind the
// update dialog, without the UI. Returns the process exit code.
//
// Everything goes to stdout, because vtui.SetupStderrLog has already sent
// stderr to the log file where the user would never see it.
func runUpdateCLI(channelArg string) int {
	channel, explicit, err := parseUpdateChannelArg(channelArg, AppConfig.UpdateChannel)
	if err != nil {
		fmt.Printf("f4: %v\n", err)
		return 2
	}

	if explicit && AppConfig.UpdateChannel != channel {
		// The named channel is configured before GitHub is asked: an "already
		// up to date" exit or a network failure would otherwise leave the
		// automatic checks on the old channel, calling the user back to it.
		AppConfig.UpdateChannel = channel
		SaveConfig()
	}

	// A nightly archive takes minutes over a slow link, so this timeout is far
	// wider than the ten seconds an API query gets.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	fmt.Printf("Checking the %s channel...\n", updateChannelName(channel))
	cand, err := fetchUpdateCandidate(ctx, channel)
	if err != nil {
		fmt.Printf("f4: update check failed: %v\n", err)
		return 1
	}
	if !cand.needsUpdate {
		fmt.Printf("Already up to date: %s\n", cand.displayVersion)
		return 0
	}

	if _, err := updateTargetDir(); err != nil {
		fmt.Printf("f4: %v\n", err)
		return 1
	}

	fmt.Printf("Installing %s\n", cand.displayVersion)
	// Percentages are redrawn with a carriage return, so a redirected run gets
	// none: in a file they pile into one unreadable line.
	showProgress := term.IsTerminal(int(os.Stdout.Fd()))
	lastPct := -1
	data, err := downloadUpdateArchive(ctx, cand.downloadURL, func(percent int) {
		if !showProgress || percent == lastPct {
			return
		}
		lastPct = percent
		fmt.Printf("\rDownloading... %d%%", percent)
	})
	if showProgress {
		fmt.Println()
	}
	if err != nil {
		fmt.Printf("f4: download failed: %v\n", err)
		return 1
	}

	if err := installUpdateArchive(data, cand.archiveKind); err != nil {
		fmt.Printf("f4: install failed: %v\n", err)
		return 1
	}

	AppConfig.LastUpdateVersion = cand.updateKey
	AppConfig.LastUpdateCheck = time.Now().Unix()
	SaveConfig()

	fmt.Printf("Installed %s. Restart f4 to use it.\n", cand.displayVersion)
	return 0
}
