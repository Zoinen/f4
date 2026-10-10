package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"

	"github.com/unxed/f4/internal/vtvibe"
)

// f4 --ai "question" (unxed/f4#1842, docs/VTVIBE.md § 19a.9, item 12): one
// question to the model of Settings → AI without starting the UI, the answer
// on stdout — for scripts, like claude -p or opencode run. Each run is a new
// dialog of its own; the panel's dialog is not touched.

// aiCLIArgs is what the command line says about a headless question.
type aiCLIArgs struct {
	question string
	model    string
	files    []string
}

var errAICLIUsage = errors.New(`usage: f4 --ai "question" [--ai-file PATH]... [--ai-model NAME]`)

// parseAICLIArgs finds --ai among args; found is false when it is not there.
func parseAICLIArgs(args []string) (parsed aiCLIArgs, found bool, err error) {
	asked := false
	for i := 0; i < len(args); i++ {
		name, value, hasValue := strings.Cut(args[i], "=")
		if name != "--ai" && name != "--ai-file" && name != "--ai-model" {
			continue
		}
		found = true
		if !hasValue {
			if i+1 >= len(args) {
				return parsed, true, errAICLIUsage
			}
			i++
			value = args[i]
		}
		switch name {
		case "--ai":
			parsed.question, asked = value, true
		case "--ai-file":
			parsed.files = append(parsed.files, value)
		case "--ai-model":
			parsed.model = value
		}
	}
	if found && (!asked || strings.TrimSpace(parsed.question) == "") {
		return parsed, true, errAICLIUsage
	}
	return parsed, found, nil
}

// runAICLI answers the question; handled is false when the command line has
// no --ai. Piped stdin is attached to the dialog as stdin.txt, so
// `git diff | f4 --ai "review this"` works.
func runAICLI(args []string, stdin io.Reader, stdout, stderr io.Writer, readConfig func() (vtvibe.Config, string, vtvibe.Provider)) (code int, handled bool) {
	parsed, found, err := parseAICLIArgs(args)
	if !found {
		return 0, false
	}
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 2, true
	}
	cfg, _, _ := readConfig()
	if parsed.model != "" {
		cfg.Model = parsed.model
	}
	session := vtvibe.NewSession()
	for _, path := range parsed.files {
		data, err := os.ReadFile(path) // #nosec G304 -- the user names the file to send on the command line
		if err == nil {
			err = vtvibeWriteContextFile(session, filepath.Base(path), data)
		}
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "f4 --ai: %s: %v\n", path, err)
			return 2, true
		}
	}
	if stdin != nil {
		data, err := io.ReadAll(stdin)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "f4 --ai: stdin: %v\n", err)
			return 2, true
		}
		if len(data) > 0 {
			if err := vtvibeWriteContextFile(session, "stdin.txt", data); err != nil {
				_, _ = fmt.Fprintf(stderr, "f4 --ai: stdin: %v\n", err)
				return 2, true
			}
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	answer, err := session.AskOnce(ctx, cfg, parsed.question)
	if err != nil {
		if errors.Is(err, vtvibe.ErrNoKey) {
			_, _ = fmt.Fprintln(stderr, "f4 --ai: no API key: set the key variable of the provider or the key in Settings → AI")
		} else {
			_, _ = fmt.Fprintf(stderr, "f4 --ai: %v\n", err)
		}
		return 1, true
	}
	if !strings.HasSuffix(answer, "\n") {
		answer += "\n"
	}
	if _, err := io.WriteString(stdout, answer); err != nil {
		return 1, true
	}
	return 0, true
}

// pipedStdin is stdin when something is piped into f4, nil at a terminal.
func pipedStdin() io.Reader {
	info, err := os.Stdin.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice != 0 {
		return nil
	}
	return os.Stdin
}
