package app

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/unxed/f4/internal/dialog"
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
	dir := aiBotDir(pf)
	question := fmt.Sprintf(i18n.Msg("AI.BotConfirm"), source, pause, dir, cfg.Model)
	dlg := vtui.ShowMessage(i18n.Msg("AI.Title"), question, []string{i18n.Msg("AI.BotStart"), i18n.Msg("vtui.Cancel")})
	dlg.OnResult = func(code int) {
		if code != 0 {
			return
		}
		config := aiAgentConfig(aiSession())
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

// aiAgentConfig is the configuration of the bot's and the workers' requests:
// the chat's, with the GitHub token for the commands they run (f4#1842,
// stage H6). It is read again for each round, so a token set meanwhile
// counts from the next one.
func aiAgentConfig(session *vtvibe.Session) func() vtvibe.Config {
	return func() vtvibe.Config {
		c, _ := vtvibeConfig()
		token, _ := aiGitHubToken(session)
		c.ToolEnv = vtvibe.GitHubEnv(token)
		return c
	}
}

// aiGitHubToken is the token the dialog's commands get and where it comes
// from: the dialog's own, else the one in Settings → AI; empty when neither
// is set, and the environment's GH_TOKEN, if any, stays as it is.
func aiGitHubToken(session *vtvibe.Session) (token, source string) {
	if t := session.GitHubToken(); t != "" {
		return t, i18n.Msg("AI.TokenFromDialog")
	}
	if t := strings.TrimSpace(ini.Load(vtvibeIniPath()).GetString("general", "github_token", "")); t != "" {
		return t, i18n.Msg("AI.TokenFromSettings")
	}
	return "", ""
}

// aiTokenCommand is ai:token: it tells where the dialog's GitHub token comes
// from and asks for one bound to this dialog alone; ai:token clear unbinds it.
func aiTokenCommand(pf *panel.PanelsFrame, arg string) {
	session := aiSession()
	unbind := strings.EqualFold(strings.TrimSpace(arg), "clear")
	if unbind {
		session.SetGitHubToken("")
	}
	_, source := aiGitHubToken(session)
	if source == "" {
		source = i18n.Msg("AI.TokenNone")
	}
	if unbind {
		vtui.ShowMessage(i18n.Msg("AI.Title"), fmt.Sprintf(i18n.Msg("AI.TokenSource"), source), []string{i18n.Msg("vtui.Ok")})
		return
	}
	// The token is typed into a dialog box, not the command line, so it does
	// not land in the command history.
	vtui.InputBox(i18n.Msg("AI.Title"), fmt.Sprintf(i18n.Msg("AI.TokenPrompt"), source), "", func(token string) {
		if token = strings.TrimSpace(token); token == "" {
			return
		}
		session.SetGitHubToken(token)
		if err := session.StoreError(); err != nil {
			aiShowError(err)
			return
		}
		vtui.ShowMessage(i18n.Msg("AI.Title"), fmt.Sprintf(i18n.Msg("AI.TokenSource"), i18n.Msg("AI.TokenFromDialog")), []string{i18n.Msg("vtui.Ok")})
	})
}

// vtvibeNonstopDefault is the mode of the dialogs that did not choose their
// own (Settings → AI, f4#1842 stage H6): question-and-answer unless set.
func vtvibeNonstopDefault() bool {
	return ini.Load(vtvibeIniPath()).GetString("general", "nonstop", "false") == "true"
}

// aiNonstop reports the mode the current dialog works in.
func aiNonstop(session *vtvibe.Session) bool {
	switch session.Mode() {
	case vtvibe.ModeNonstop:
		return true
	case vtvibe.ModeQA:
		return false
	}
	return vtvibeNonstopDefault()
}

// aiModeCommand is ai:mode: alone it tells the current dialog's mode, with
// nonstop, qa or default it chooses one for this dialog (f4#1842, stage H6).
func aiModeCommand(pf *panel.PanelsFrame, arg string) {
	session := aiSession()
	if arg = strings.TrimSpace(arg); arg != "" {
		mode, err := vtvibe.ParseMode(arg)
		if err != nil {
			vtui.ShowMessage(i18n.Msg("AI.Title"), i18n.Msg("AI.ModeUsage"), []string{i18n.Msg("vtui.Ok")})
			return
		}
		session.SetMode(mode)
		aiBotRefresh(pf)
	}
	name := i18n.Msg("AI.ModeQA")
	if aiNonstop(session) {
		name = i18n.Msg("AI.ModeNonstop")
	}
	text := fmt.Sprintf(i18n.Msg("AI.ModeIs"), name)
	if session.Mode() == vtvibe.ModeDefault {
		text += "\n" + i18n.Msg("AI.ModeFromSettings")
	}
	vtui.ShowMessage(i18n.Msg("AI.Title"), text+"\n\n"+i18n.Msg("AI.ModeUsage"), []string{i18n.Msg("vtui.Ok")})
}

// aiDialogsMenu lists the dialogs ai:new put aside, newest first; Enter makes
// the chosen one current, the current one going to the archive in its place
// (f4#1842, stage H3).
func aiDialogsMenu(pf *panel.PanelsFrame) {
	dialogs, err := vtvibe.ListArchive(vtvibeArchiveDir())
	if err != nil {
		aiShowError(err)
		return
	}
	if len(dialogs) == 0 {
		vtui.ShowMessage(i18n.Msg("AI.Title"), i18n.Msg("AI.NoDialogs"), []string{i18n.Msg("vtui.Ok")})
		return
	}
	menu := vtui.NewVMenu(i18n.Msg("AI.DialogsTitle"))
	width := vtui.StringWidth(i18n.Msg("AI.DialogsTitle")) + 6
	for _, d := range dialogs {
		title := d.Title
		if title == "" {
			title = i18n.Msg("AI.DialogUntitled")
		}
		text := fmt.Sprintf("%s  %s (%d)", d.Saved.Format("2006-01-02 15:04"), title, d.Messages)
		width = max(width, vtui.StringWidth(text)+6)
		menu.AddItem(vtui.MenuItem{Text: dialog.EscapeAmpersand(text)})
	}
	menu.OnAction = func(idx int) {
		menu.Close()
		if idx < 0 || idx >= len(dialogs) {
			return
		}
		if err := aiSession().OpenArchived(dialogs[idx].Path, vtvibeArchiveDir()); err != nil {
			aiShowError(err)
			return
		}
		vtvibeConfig()
		aiBotRefresh(pf)
	}
	sw, sh := 80, 25
	if vtui.FrameManager != nil {
		if w := vtui.FrameManager.GetScreenSize(); w > 0 {
			sw = w
		}
		if h := vtui.FrameManager.GetScreenHeight(); h > 0 {
			sh = h
		}
	}
	w := min(width, max(sw-4, 20))
	h := min(len(dialogs)+2, max(sh-4, 3))
	x, y := (sw-w)/2, (sh-h)/2
	menu.SetPosition(x, y, x+w-1, y+h-1)
	vtui.FrameManager.Push(menu)
}

// aiOrdersCommand shows the register of the user's orders, or closes and
// reopens one: "ai:orders", "ai:done N", "ai:undone N" (f4#1842, stage H4).
func aiOrdersCommand(pf *panel.PanelsFrame, verb, arg string) {
	session := aiSession()
	if verb != "orders" {
		id, err := strconv.Atoi(strings.TrimPrefix(strings.TrimSpace(arg), "#"))
		if err == nil {
			err = session.SetOrderDone(id, verb == "done")
		}
		if err != nil {
			vtui.ShowMessage(i18n.Msg("AI.Title"), i18n.Msg("AI.OrderUnknown"), []string{i18n.Msg("vtui.Ok")})
			return
		}
		aiBotRefresh(pf)
	}
	vtui.ShowMessage(i18n.Msg("AI.OrdersTitle"), aiOrdersText(session.Orders()), []string{i18n.Msg("vtui.Ok")})
}

// aiOrdersText lists the open orders first, then the done ones.
func aiOrdersText(orders []vtvibe.Order) string {
	if len(orders) == 0 {
		return i18n.Msg("AI.NoOrders")
	}
	var lines []string
	for _, done := range []bool{false, true} {
		for _, o := range orders {
			if o.Done != done {
				continue
			}
			mark := i18n.Msg("AI.OrderOpen")
			if o.Done {
				mark = i18n.Msg("AI.OrderDone")
			}
			text := []rune(strings.ReplaceAll(strings.TrimSpace(o.Text), "\n", " "))
			if len(text) > 70 {
				text = append(text[:70], '…')
			}
			lines = append(lines, fmt.Sprintf("#%d %s %s", o.ID, mark, string(text)))
		}
	}
	return dialog.EscapeAmpersand(strings.Join(lines, "\n"))
}

// aiWorkers are the workers ai:task starts (f4#1842, stage H5).
var aiWorkers vtvibe.Workers

// aiTaskCommand handles "ai:task" (list), "ai:task stop N" and
// "ai:task <task>": the task goes to a worker in a clean context after a
// confirmation, as a bot does, and enters the register of orders; the
// worker's report comes into the chat and closes the order when it succeeded.
func aiTaskCommand(pf *panel.PanelsFrame, arg string) {
	manager := vtui.FrameManager
	arg = strings.TrimSpace(arg)
	lower := strings.ToLower(arg)
	switch {
	case arg == "":
		vtui.ShowMessage(i18n.Msg("AI.Title"), aiTasksText(aiWorkers.Running()), []string{i18n.Msg("vtui.Ok")})
		return
	case strings.HasPrefix(lower, "stop "):
		id, err := strconv.Atoi(strings.TrimPrefix(strings.TrimSpace(arg[len("stop "):]), "#"))
		if err != nil || !aiWorkers.Stop(id) {
			vtui.ShowMessage(i18n.Msg("AI.Title"), i18n.Msg("AI.TaskUnknown"), []string{i18n.Msg("vtui.Ok")})
		}
		return
	}
	cfg, keySource, provider := vtvibeProviderConfig()
	if cfg.APIKey == "" && keySource == "" && provider.NeedsKey(cfg.BaseURL) {
		aiShowError(vtvibe.ErrNoKey)
		return
	}
	dir := aiBotDir(pf)
	question := fmt.Sprintf(i18n.Msg("AI.TaskConfirm"), arg, dir, cfg.Model)
	dlg := vtui.ShowMessage(i18n.Msg("AI.Title"), question, []string{i18n.Msg("AI.BotStart"), i18n.Msg("vtui.Cancel")})
	dlg.OnResult = func(code int) {
		if code != 0 {
			return
		}
		session := aiSession()
		order := session.AddOrder(arg)
		aiStartWorker(pf, manager, session, arg, dir, order, true, nil)
	}
}

// aiStartWorker gives task, serving order (0: none), to a worker in dir and
// puts its report into the dialog. closeOrder closes the order when the
// worker succeeds: right for a task the user gave with ai:task, while an
// order the manager split up is closed by the manager. done, if set, runs on
// the UI thread after the report.
func aiStartWorker(pf *panel.PanelsFrame, manager interface{ PostTask(func()) }, session *vtvibe.Session, task, dir string, order int, closeOrder bool, done func()) {
	config := aiAgentConfig(session)
	tools := func() []vtvibe.Tool { return vtvibe.WorkTools(dir, config().ToolEnv...) }
	id := aiWorkers.Start(task, dir, config, tools, func(r vtvibe.WorkerResult) {
		text := aiTaskResultText(r, order)
		manager.PostTask(func() {
			if r.Err == nil && closeOrder {
				_ = session.SetOrderDone(order, true)
			}
			session.Note("assistant", text)
			aiBotRefresh(pf)
			if done != nil {
				done()
			}
		})
	})
	if order > 0 {
		session.Note("assistant", fmt.Sprintf(i18n.Msg("AI.TaskStarted"), id, order, task))
	} else {
		session.Note("assistant", fmt.Sprintf(i18n.Msg("AI.WorkerStarted"), id, task))
	}
	aiBotRefresh(pf)
}

// aiDelegate offers the tasks the manager handed out to the user; confirmed,
// each goes to its own worker. When the last report is in and the dialog
// works without stopping, the manager goes on by itself (f4#1842, stage H5).
func aiDelegate(pf *panel.PanelsFrame, session *vtvibe.Session, delegations []vtvibe.Delegation) {
	manager := vtui.FrameManager
	cfg, _ := vtvibeConfig()
	dir := aiBotDir(pf)
	// The whole task is shown, within reason: the user approves what the
	// workers will run.
	lines := make([]string, 0, len(delegations))
	for _, d := range delegations {
		task := []rune(d.Task)
		if len(task) > 400 {
			task = append(task[:400], '…')
		}
		if d.Order > 0 {
			lines = append(lines, fmt.Sprintf("#%d: %s", d.Order, string(task)))
		} else {
			lines = append(lines, "- "+string(task))
		}
	}
	question := fmt.Sprintf(i18n.Msg("AI.DelegateConfirm"), len(delegations), dialog.EscapeAmpersand(strings.Join(lines, "\n")), dir, cfg.Model)
	dlg := vtui.ShowMessage(i18n.Msg("AI.Title"), question, []string{i18n.Msg("AI.BotStart"), i18n.Msg("vtui.Cancel")})
	dlg.OnResult = func(code int) {
		if code != 0 {
			// The manager learns it from the dialog, not by guessing.
			session.Note("assistant", i18n.Msg("AI.DelegateDeclined"))
			aiBotRefresh(pf)
			return
		}
		left := len(delegations)
		for _, d := range delegations {
			aiStartWorker(pf, manager, session, d.Task, dir, d.Order, false, func() {
				if left--; left == 0 && aiNonstop(session) && !session.Busy() {
					aiRunWork(pf, session, func(ctx context.Context) (vtvibe.WorkEnd, error) {
						c, _ := vtvibeConfig()
						return session.Resume(ctx, c)
					})
				}
			})
		}
	}
}

func aiTaskResultText(r vtvibe.WorkerResult, order int) string {
	var sb strings.Builder
	switch {
	case r.Err != nil && order > 0:
		fmt.Fprintf(&sb, i18n.Msg("AI.TaskFailed"), r.ID, order, r.Err)
	case r.Err != nil:
		fmt.Fprintf(&sb, i18n.Msg("AI.WorkerFailed"), r.ID, r.Err)
	default:
		if order > 0 {
			fmt.Fprintf(&sb, i18n.Msg("AI.TaskDone"), r.ID, order, len(r.Steps), r.Usage.In, r.Usage.Out)
		} else {
			fmt.Fprintf(&sb, i18n.Msg("AI.WorkerDone"), r.ID, len(r.Steps), r.Usage.In, r.Usage.Out)
		}
		sb.WriteString("\n\n")
		sb.WriteString(r.Report)
	}
	if r.Restarts > 0 {
		sb.WriteString("\n\n")
		fmt.Fprintf(&sb, i18n.Msg("AI.TaskRestarts"), r.Restarts)
	}
	return sb.String()
}

func aiTasksText(running map[int]string) string {
	if len(running) == 0 {
		return i18n.Msg("AI.NoTasks")
	}
	ids := make([]int, 0, len(running))
	for id := range running {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	lines := make([]string, 0, len(ids))
	for _, id := range ids {
		lines = append(lines, fmt.Sprintf("#%d %s", id, orderLine(running[id])))
	}
	return dialog.EscapeAmpersand(strings.Join(lines, "\n"))
}

func orderLine(text string) string {
	r := []rune(strings.ReplaceAll(strings.TrimSpace(text), "\n", " "))
	if len(r) > 70 {
		r = append(r[:70], '…')
	}
	return string(r)
}
