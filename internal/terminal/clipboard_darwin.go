//go:build darwin

package terminal

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/unxed/goclip"
)

// goclip reads and writes text on macOS through pbpaste and pbcopy started
// with the environment of f4, and when that has no UTF-8 locale the text loses
// every letter outside ASCII (f4#2057, sogonov: f4 --gui started from the
// terminal of far2l, whose interactive non-login zsh sets no LANG, pasted
// "Лунобот" as "???????" and copied it out as mojibake; in f4 --tty the
// terminal pastes, so pbpaste is not used there). This driver takes the
// "darwin" slot of goclip's registry and runs both with a UTF-8 locale
// (utf8LocaleEnv); images stay goclip's.
type darwinTextClipboard struct {
	*goclip.DarwinDriver
}

func pbCommand(ctx context.Context, name string) (*exec.Cmd, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return nil, goclip.ErrUnavailable
	}
	cmd := exec.CommandContext(ctx, path) // #nosec G204 -- pbcopy or pbpaste with no arguments, the text goes through stdin and stdout.
	cmd.Env = utf8LocaleEnv(os.Environ())
	return cmd, nil
}

func (d darwinTextClipboard) ReadText() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd, err := pbCommand(ctx, "pbpaste")
	if err != nil {
		return "", err
	}
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func (d darwinTextClipboard) WriteText(text string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd, err := pbCommand(ctx, "pbcopy")
	if err != nil {
		return err
	}
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

func (d darwinTextClipboard) Clear() error { return d.WriteText("") }

func init() {
	goclip.RegisterDriverPriority(darwinTextClipboard{goclip.NewDarwinDriver()}, true)
}
