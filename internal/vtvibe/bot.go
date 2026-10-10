package vtvibe

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// The simplified bot mode (unxed/f4#1842, docs/VTVIBE.md § 19a, step B2): an
// instruction from a file or a URL is run by the agent loop in a clean
// dialog, then the bot waits and runs it again, each round from scratch. It is
// made first for the Lunobot instruction.

// DefaultBotPause is the wait between rounds when none is given.
const DefaultBotPause = 30 * time.Minute

const maxInstruction = 1 << 20

// LoadInstruction reads the bot's instruction from a local file or an
// http(s) URL. It is read again every round, so an edited instruction takes
// effect without restarting the bot.
func LoadInstruction(ctx context.Context, source string) (string, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return "", errors.New("no instruction given")
	}
	var data []byte
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
		if err != nil {
			return "", err
		}
		resp, err := httpClient.Do(req)
		if err != nil {
			return "", err
		}
		defer func() { _ = resp.Body.Close() }() // Read-only body; nothing to report.
		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("%s: HTTP %d", source, resp.StatusCode)
		}
		data, err = io.ReadAll(io.LimitReader(resp.Body, maxInstruction+1))
		if err != nil {
			return "", err
		}
	} else {
		var err error
		data, err = os.ReadFile(source) // #nosec G304 -- the user names the instruction file
		if err != nil {
			return "", err
		}
	}
	if len(data) > maxInstruction {
		return "", fmt.Errorf("%s is larger than %d bytes", source, maxInstruction)
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		return "", fmt.Errorf("%s is empty", source)
	}
	return text, nil
}

// BotSystemPrompt tells the model where it runs and on which model: the
// model's name is always part of the context (f4#1842).
func BotSystemPrompt(model, dir string, now time.Time) string {
	return fmt.Sprintf(`You are a bot run by the f4 file manager. You are running on the model %q.
Each round you get the same instruction in a fresh dialog: earlier rounds are
not in your context, so keep whatever must survive in files or in the systems
the instruction names. Carry out the instruction now, using the tools: shell
runs commands, read_file and write_file work with files. The working directory
is %s. The current time is %s. When the round is done, answer with a short
report of what you did; that report is all the user sees of the round.`, model, dir, now.UTC().Format(time.RFC3339))
}

// BotRound is what one round produced.
type BotRound struct {
	N      int
	Report string
	Steps  []AgentStep
	Usage  Usage
	Err    error
	Next   time.Time
}

// BotStatus is a snapshot for the host's status message.
type BotStatus struct {
	Running bool
	Source  string
	Pause   time.Duration
	Rounds  int
	Next    time.Time
}

// Bot runs one instruction round after round until stopped.
type Bot struct {
	mu      sync.Mutex
	cancel  context.CancelFunc
	done    chan struct{}
	source  string
	pause   time.Duration
	rounds  int
	next    time.Time
	running bool
}

// ErrBotRunning is returned by Start while a bot is already running.
var ErrBotRunning = errors.New("vtvibe: the bot is already running")

// Start launches the rounds in the background. config is asked for the
// current settings every round, so a model switched in between is used.
// onRound is called from the bot's goroutine after every round, and
// onStart before it.
//
// extra, when not nil, gives tools offered besides WorkTools; it is asked
// every round, so a tool switched off in Settings is gone from the next one.
func (b *Bot) Start(source string, pause time.Duration, dir string, config func() Config,
	extra func() []Tool, onStart func(n int), onRound func(BotRound)) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.running {
		return ErrBotRunning
	}
	if pause <= 0 {
		pause = DefaultBotPause
	}
	ctx, cancel := context.WithCancel(context.Background())
	b.cancel, b.done, b.source, b.pause, b.rounds, b.running = cancel, make(chan struct{}), source, pause, 0, true
	go b.loop(ctx, dir, config, extra, onStart, onRound)
	return nil
}

func (b *Bot) loop(ctx context.Context, dir string, config func() Config, extra func() []Tool, onStart func(int), onRound func(BotRound)) {
	defer func() {
		b.mu.Lock()
		b.running = false
		close(b.done)
		b.mu.Unlock()
	}()
	for n := 1; ; n++ {
		if onStart != nil {
			onStart(n)
		}
		tools := WorkTools(dir)
		if extra != nil {
			tools = append(tools, extra()...)
		}
		round := b.round(ctx, n, dir, config(), tools)
		b.mu.Lock()
		b.rounds = n
		b.next = time.Now().Add(b.pause)
		round.Next = b.next
		pause := b.pause
		b.mu.Unlock()
		if ctx.Err() != nil {
			return
		}
		if onRound != nil {
			onRound(round)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(pause):
		}
	}
}

func (b *Bot) round(ctx context.Context, n int, dir string, cfg Config, tools []Tool) BotRound {
	round := BotRound{N: n}
	instruction, err := LoadInstruction(ctx, b.source)
	if err != nil {
		round.Err = err
		return round
	}
	msgs := []Message{
		{Role: "system", Content: BotSystemPrompt(cfg.Model, dir, time.Now())},
		{Role: "user", Content: instruction},
	}
	round.Report, round.Usage, round.Err = cfg.RunAgent(ctx, msgs, tools, AgentOptions{
		MaxSteps: 200,
		OnStep:   func(s AgentStep) { round.Steps = append(round.Steps, s) },
	})
	return round
}

// Stop cancels the running round and the waiting, and returns when the bot
// has stopped. It reports whether a bot was running.
func (b *Bot) Stop() bool {
	b.mu.Lock()
	if !b.running {
		b.mu.Unlock()
		return false
	}
	cancel, done := b.cancel, b.done
	b.mu.Unlock()
	cancel()
	<-done
	return true
}

// Status returns a snapshot of the bot.
func (b *Bot) Status() BotStatus {
	b.mu.Lock()
	defer b.mu.Unlock()
	return BotStatus{Running: b.running, Source: b.source, Pause: b.pause, Rounds: b.rounds, Next: b.next}
}
