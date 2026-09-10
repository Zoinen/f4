package update

import (
	"context"
	"fmt"
	"golang.org/x/term"
	"os"
	"strings"
	"time"
)

// ChannelName spells a channel the way the command line does.
func ChannelName(channel int) string {
	if channel == ChannelNightly {
		return "nightly"
	}
	return "stable"
}

// ParseChannelArg turns the `--update` argument into a channel number.
// An empty argument means the configured channel.
func ParseChannelArg(arg string, configured int) (channel int, explicit bool, err error) {
	switch strings.ToLower(strings.TrimSpace(arg)) {
	case "":
		return configured, false, nil
	case "stable", "latest":
		return ChannelStable, true, nil
	case "nightly":
		return ChannelNightly, true, nil
	}
	return 0, false, fmt.Errorf("unknown update channel %q (expected \"stable\" or \"nightly\")", arg)
}

// RunCLI serves `f4 --update [stable|nightly]`: the machinery behind the
// update dialog, without the UI. Returns the process exit code. save persists
// the settings it changes; the caller decides where they live.
//
// Everything goes to stdout, because vtui.SetupStderrLog has already sent
// stderr to the log file where the user would never see it.
func RunCLI(channelArg string, cfg Settings, b Build, save func(Settings)) int {
	channel, explicit, err := ParseChannelArg(channelArg, cfg.Channel)
	if err != nil {
		fmt.Printf("f4: %v\n", err)
		return 2
	}

	if explicit && cfg.Channel != channel {
		// The named channel is configured before GitHub is asked: an "already
		// up to date" exit or a network failure would otherwise leave the
		// automatic checks on the old channel, calling the user back to it.
		cfg.Channel = channel
		save(cfg)
	}

	// A nightly archive takes minutes over a slow link, so this timeout is far
	// wider than the ten seconds an API query gets.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	fmt.Printf("Checking the %s channel...\n", ChannelName(channel))
	cand, err := Check(ctx, cfg, b)
	if err != nil {
		fmt.Printf("f4: update check failed: %v\n", err)
		return 1
	}
	if !cand.NeedsUpdate {
		fmt.Printf("Already up to date: %s\n", cand.DisplayVersion)
		return 0
	}

	if _, err := TargetDir(); err != nil {
		fmt.Printf("f4: %v\n", err)
		return 1
	}

	fmt.Printf("Installing %s\n", cand.DisplayVersion)
	// Percentages are redrawn with a carriage return, so a redirected run gets
	// none: in a file they pile into one unreadable line.
	showProgress := term.IsTerminal(int(os.Stdout.Fd()))
	lastPct := -1
	data, err := Download(ctx, cand.DownloadURL, func(percent int) {
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

	if err := Install(data, cand.ArchiveKind); err != nil {
		fmt.Printf("f4: install failed: %v\n", err)
		return 1
	}

	cfg.LastVersion = cand.UpdateKey
	cfg.LastCheck = time.Now().Unix()
	save(cfg)

	fmt.Printf("Installed %s. Restart f4 to use it.\n", cand.DisplayVersion)
	return 0
}
