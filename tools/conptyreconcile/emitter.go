package main

import (
	"fmt"
	"os"
	"time"
)

// emitProbeWorkload is deliberately a dumb child: every byte below is
// authored by the probe, while repaint bytes, chunking, cursor movement and
// resize effects are produced by the pinned OpenConsole process.  Keeping the
// payload here makes the expected logical markers independent of any parser
// implementation that will eventually consume the captured stream.
func emitProbeWorkload() error {
	return emitProbeWorkloadWidth(80)
}

func emitProbeWorkloadWidth(width int) error {
	input := []byte(probeWorkloadForWidth(width))
	for offset := 0; offset < len(input); {
		end := offset + 1
		if end < len(input) {
			end += (offset * 17) % 31
			if end > len(input) {
				end = len(input)
			}
		}
		if _, err := os.Stdout.Write(input[offset:end]); err != nil {
			return fmt.Errorf("emit native probe workload: %w", err)
		}
		offset = end
		time.Sleep(time.Duration((offset*13)%400) * time.Microsecond)
	}
	return nil
}

func emitSeedWorkload(seed uint64, width int) error {
	input := seedWorkload(seed, width)
	for offset := 0; offset < len(input); {
		end := offset + 1 + int(seed%31)
		if end > len(input) {
			end = len(input)
		}
		if _, err := os.Stdout.Write(input[offset:end]); err != nil {
			return fmt.Errorf("emit seed %016x: %w", seed, err)
		}
		offset = end
		seed = seed*6364136223846793005 + 1
		time.Sleep(time.Duration(seed%200) * time.Microsecond)
	}
	return nil
}

func emitPartialWorkload(width int) error {
	input := partialProbeWorkload(width)
	cut := len(input) / 2
	if _, err := os.Stdout.Write(input[:cut]); err != nil {
		return fmt.Errorf("emit partial probe prefix: %w", err)
	}
	time.Sleep(100 * time.Millisecond)
	if _, err := os.Stdout.Write(input[cut:]); err != nil {
		return fmt.Errorf("emit partial probe suffix: %w", err)
	}
	return nil
}

func emitQuirkWorkload() error {
	input := quirkProbeWorkload()
	cut := 4096
	if cut > len(input) {
		cut = len(input)
	}
	if _, err := os.Stdout.Write(input[:cut]); err != nil {
		return fmt.Errorf("emit quirk probe prefix: %w", err)
	}
	time.Sleep(150 * time.Millisecond)
	for offset := cut; offset < len(input); {
		end := offset + 1 + (offset*7)%29
		if end > len(input) {
			end = len(input)
		}
		if _, err := os.Stdout.Write(input[offset:end]); err != nil {
			return fmt.Errorf("emit quirk probe: %w", err)
		}
		offset = end
		time.Sleep(500 * time.Microsecond)
	}
	return nil
}

func emitAlternateWorkload(width int) error {
	input := []byte(alternateProbeWorkload(width))
	if _, err := os.Stdout.Write(input); err != nil {
		return fmt.Errorf("emit alternate probe: %w", err)
	}
	return nil
}

func emitControlWorkload() error {
	input := []byte(controlProbeWorkload())
	if _, err := os.Stdout.Write(input); err != nil {
		return fmt.Errorf("emit control probe: %w", err)
	}
	return nil
}

func emitReflowWorkload(width int) error {
	input := reflowProbeWorkload(width)
	for offset := 0; offset < len(input); {
		end := offset + 1 + (offset*11)%23
		if end > len(input) {
			end = len(input)
		}
		if _, err := os.Stdout.Write(input[offset:end]); err != nil {
			return fmt.Errorf("emit reflow probe: %w", err)
		}
		offset = end
		time.Sleep(time.Duration((offset*7)%200) * time.Microsecond)
	}
	return nil
}
