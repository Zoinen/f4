package intchecker

import (
	"fmt"
	"time"

	"github.com/unxed/f4/internal/numeric"
	"github.com/unxed/vtui"
)

// progressSpeed renders the "Time: hh:mm:ss  Remaining: hh:mm:ss  speed/s"
// line the file-copy/move progress dialog already shows (internal/fileops,
// getGlobalStats), reused here so hashing and verifying speak the same
// language as the rest of the app instead of inventing a second format.
//
// A caller creates one at the start of a hashing/verifying run and calls
// text with the cumulative bytes processed so far on every progress update;
// it tracks elapsed time and a smoothed instantaneous speed itself.
type progressSpeed struct {
	start           time.Time
	lastSpeedUpdate time.Time
	lastDone        int64
	speed           float64 // bytes/sec, refreshed at most once a second
}

// newProgressSpeed starts a tracker whose clock begins now.
func newProgressSpeed() *progressSpeed {
	now := time.Now()
	return &progressSpeed{start: now, lastSpeedUpdate: now}
}

// text is the elapsed/remaining/speed line for done bytes out of total
// (both cumulative over the whole run, not just the current file). total<=0
// or done<=0 leaves the ETA as "unknown"; the speed is blank until the first
// full second of data.
func (p *progressSpeed) text(done, total int64) string {
	now := time.Now()
	elapsed := now.Sub(p.start)

	if speedDur := now.Sub(p.lastSpeedUpdate).Seconds(); speedDur >= 1.0 {
		p.speed = float64(done-p.lastDone) / speedDur
		p.lastSpeedUpdate = now
		p.lastDone = done
	}

	elapsedStr := fmt.Sprintf(vtui.Msg("IntChecker.ProgressElapsed"),
		int(elapsed.Hours()), int(elapsed.Minutes())%60, int(elapsed.Seconds())%60)

	etaStr := vtui.Msg("IntChecker.ProgressRemainingUnknown")
	if total > 0 && done > 0 && elapsed.Seconds() > 0.5 {
		ratio := float64(done) / float64(total)
		etaSecs := elapsed.Seconds()/ratio - elapsed.Seconds()
		if etaSecs < 0 {
			etaSecs = 0
		}
		switch {
		case etaSecs > 359999:
			etaStr = vtui.Msg("IntChecker.ProgressRemainingLong")
		default:
			etaDur := time.Duration(etaSecs * float64(time.Second))
			etaStr = fmt.Sprintf(vtui.Msg("IntChecker.ProgressRemaining"),
				int(etaDur.Hours()), int(etaDur.Minutes())%60, int(etaDur.Seconds())%60)
		}
	}

	speedStr := ""
	if p.speed > 0 {
		speedStr = numeric.FormatSize(int64(p.speed)) + "/s"
	}

	return fmt.Sprintf("%-16s %-21s %15s", elapsedStr, etaStr, speedStr)
}
