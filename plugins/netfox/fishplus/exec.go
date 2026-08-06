package fishplus

import (
	"context"
	"errors"
	"strconv"
)

// CanRun reports whether the remote host will run a command for us. It needs
// nothing but the job machinery: the command is the shell's own business.
func (c *Client) CanRun() bool { return c.CanRunJobs() }

// StartCommand starts a command line in a directory and returns the job id.
// An empty directory means wherever the session started.
func (c *Client) StartCommand(ctx context.Context, dir, command string) (int, error) {
	if command == "" {
		return 0, errors.New("fishplus: empty command")
	}
	if !c.CanRun() {
		return 0, ErrNoJobs
	}
	return c.StartJob(ctx, "exec", []string{dir, command})
}

// FollowCommand polls a command job and hands each line to cb as it comes,
// returning the exit status of the command. Output and errors arrive
// together, in the order the command produced them, because that is the
// order somebody reading it expects and the only one a single stream can
// carry.
//
// A non-zero exit status is not an error here: a command that failed still
// ran, and what it printed is usually the answer the caller wanted.
func (c *Client) FollowCommand(ctx context.Context, id int, cb func(line string)) (int, error) {
	reqCtx := context.WithoutCancel(ctx)
	wait := jobPollMin
	for {
		st, err := c.PollJob(reqCtx, id, DefaultPollLines)
		if err != nil {
			return 0, err
		}
		fresh := len(st.Lines) > 0
		if cb != nil {
			for _, line := range st.Lines {
				cb(line)
			}
		}
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		if st.More {
			continue
		}
		switch st.State {
		case JobKilled:
			return 0, ErrJobKilled
		case JobDone:
			if st.Msg != "" && st.Exit != 0 && len(st.Lines) == 0 {
				// The helper refused the job outright rather than the
				// command failing, which is a different thing entirely.
				return st.Exit, &RemoteError{Cmd: "exec", Msg: st.Msg}
			}
			return st.Exit, nil
		}
		if fresh {
			wait = jobPollMin
		}
		if err := sleepCtx(ctx, wait); err != nil {
			return 0, err
		}
		wait *= 2
		if wait > jobPollMax {
			wait = jobPollMax
		}
	}
}

// Run starts a command, follows it to the end and cleans up after it. A
// caller that wants the command to outlive it builds the same thing out of
// StartCommand and FollowCommand, which kill nothing.
func (c *Client) Run(ctx context.Context, dir, command string, cb func(line string)) (int, error) {
	id, err := c.StartCommand(ctx, dir, command)
	if err != nil {
		return 0, err
	}
	code, err := c.FollowCommand(ctx, id, cb)
	c.dropJobQuietly(id)
	return code, err
}

// RunOutput is Run for a command whose output is small enough to keep.
func (c *Client) RunOutput(ctx context.Context, dir, command string) ([]string, int, error) {
	var lines []string
	code, err := c.Run(ctx, dir, command, func(line string) { lines = append(lines, line) })
	return lines, code, err
}

// KillCommand stops a running command. It is a plain job kill, named for
// what it does to a caller who thinks in commands.
func (c *Client) KillCommand(ctx context.Context, id int) error {
	return c.KillJob(ctx, id)
}

var _ = strconv.Itoa
