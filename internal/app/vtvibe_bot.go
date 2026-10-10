package app

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/ini"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/vtvibe"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// ai:bot — the simplified bot mode of unxed/f4#1842 (docs/VTVIBE.md § 19a,
// step B2). One bot per f4 process; its rounds are logged into the AI chat.

var aiBot vtvibe.Bot

// aiBotCommand handles "ai:bot", "ai:bot stop" and "ai:bot <source> [pause]".
func aiBotCommand(pf *panel.PanelsFrame, arg string) {
	// The bot and the stop request run in the background; they post back to
	// the manager this command was given on, never the global read later.
	manager := vtui.FrameManager
	arg = strings.TrimSpace(arg)
	switch strings.ToLower(arg) {
	case "":
		vtui.ShowMessage(i18n.Msg("AI.Title"), aiBotStatusText(aiBot.Status()), []string{i18n.Msg("vtui.Ok")})
		return
	case "stop":
		go func() {
			stopped := aiBot.Stop()
			manager.PostTask(func() {
				if stopped {
					aiSession().Note("assistant", i18n.Msg("AI.BotStopped"))
					aiBotRefresh(pf)
				}
				vtui.ShowMessage(i18n.Msg("AI.Title"), aiBotStatusText(aiBot.Status()), []string{i18n.Msg("vtui.Ok")})
			})
		}()
		return
	}
	source, pause := parseBotArgs(arg)
	if aiBot.Status().Running {
		vtui.ShowMessage(i18n.Msg("AI.Title"), i18n.Msg("AI.BotAlready"), []string{i18n.Msg("vtui.Ok")})
		return
	}
	cfg, keySource, provider := vtvibeProviderConfig()
	if cfg.APIKey == "" && keySource == "" && provider.NeedsKey(cfg.BaseURL) {
		aiShowError(vtvibe.ErrNoKey)
		return
	}
	if cfg.Kind == vtvibe.KindAnthropic {
		// The agent loop speaks only the chat-completions tool protocol so far.
		aiShowError(vtvibe.ErrAgentProtocol)
		return
	}
	dir := aiBotDir(pf)
	question := fmt.Sprintf(i18n.Msg("AI.BotConfirm"), source, pause, dir, cfg.Model)
	dlg := vtui.ShowMessage(i18n.Msg("AI.Title"), question, []string{i18n.Msg("AI.BotStart"), i18n.Msg("vtui.Cancel")})
	dlg.OnResult = func(code int) {
		if code != 0 {
			return
		}
		config := func() vtvibe.Config { c, _ := vtvibeConfig(); return c }
		err := aiBot.Start(source, pause, dir, config,
			func() []vtvibe.Tool { return vtvibe.DialogTools(vtvibeDialogControls(manager, pf)) },
			func(n int) {
				manager.PostTask(func() {
					aiSession().Note("assistant", fmt.Sprintf(i18n.Msg("AI.BotRoundStart"), n, source))
					aiBotRefresh(pf)
				})
			},
			func(r vtvibe.BotRound) {
				text := aiBotRoundText(r)
				manager.PostTask(func() {
					aiSession().Note("assistant", text)
					aiBotRefresh(pf)
				})
			})
		if err != nil {
			aiShowError(err)
		}
	}
}

// parseBotArgs splits "source [pause]": a last word that parses as a Go
// duration (30m, 1h, 90s) is the pause.
func parseBotArgs(arg string) (string, time.Duration) {
	fields := strings.Fields(arg)
	if len(fields) > 1 {
		if d, err := time.ParseDuration(fields[len(fields)-1]); err == nil && d > 0 {
			return strings.Join(fields[:len(fields)-1], " "), d
		}
	}
	return arg, vtvibe.DefaultBotPause
}

// aiBotDir is where the bot's commands start: the active panel's folder when
// it is on the local disk, the home folder otherwise.
func aiBotDir(pf *panel.PanelsFrame) string {
	if fsp := pf.GetActivePanel(); fsp != nil {
		if _, local := fsp.Vfs.(*vfs.OSVFS); local {
			return fsp.Vfs.GetPath()
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		return home
	}
	return "."
}

func aiBotRoundText(r vtvibe.BotRound) string {
	var sb strings.Builder
	if r.Err != nil {
		fmt.Fprintf(&sb, i18n.Msg("AI.BotRoundFailed"), r.N, r.Err)
	} else {
		fmt.Fprintf(&sb, i18n.Msg("AI.BotRoundDone"), r.N, len(r.Steps), r.Usage.In, r.Usage.Out)
		sb.WriteString("\n\n")
		sb.WriteString(r.Report)
	}
	sb.WriteString("\n\n")
	fmt.Fprintf(&sb, i18n.Msg("AI.BotNext"), r.Next.Format("15:04"))
	return sb.String()
}

func aiBotStatusText(st vtvibe.BotStatus) string {
	if !st.Running {
		return i18n.Msg("AI.BotIdle")
	}
	return fmt.Sprintf(i18n.Msg("AI.BotRunning"), st.Source, st.Pause, st.Rounds)
}

func aiBotRefresh(pf *panel.PanelsFrame) {
	pf.RefreshAll()
	if pf.AltPanels[pf.ActiveIdx] != nil {
		if cp, ok := pf.AltPanels[pf.ActiveIdx].(*AIChatPanel); ok {
			cp.ScrollToBottom()
		}
	}
	vtui.FrameManager.Redraw()
}

// vtvibeDialogControls gives the model its handles on the dialog, each only
// while Settings → AI allows it (f4#1842, step B3). Called from the bot's
// goroutine; the UI is touched only through manager.
func vtvibeDialogControls(manager interface{ PostTask(func()) }, pf *panel.PanelsFrame) vtvibe.DialogControls {
	var c vtvibe.DialogControls
	if vtvibeAllowed("allow_model_switch") {
		c.SetModel = func(model string) error {
			if err := vtvibeSaveSetting("model", model); err != nil {
				return err
			}
			manager.PostTask(func() {
				vtvibeConfig()
				aiSession().Note("assistant", fmt.Sprintf(i18n.Msg("AI.ModelSwitched"), model))
				aiBotRefresh(pf)
			})
			return nil
		}
	}
	if vtvibeAllowed("allow_rename") {
		c.Rename = func(title string) error {
			aiSession().SetTitle(title)
			manager.PostTask(func() { aiBotRefresh(pf) })
			return nil
		}
	}
	return c
}

// vtvibeAllowed reads one of the model's permissions from vtvibe.ini; they
// are on unless the user switched them off.
func vtvibeAllowed(key string) bool {
	return ini.Load(vtvibeIniPath()).GetString("general", key, "true") != "false"
}
