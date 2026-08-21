package gitplugin

import "github.com/unxed/f4/vfs"

// notify presents a result-free Git message without ever waiting on hosts that
// expose the asynchronous notification capability. The App.Message fallback
// preserves compatibility with older hosts and lightweight test applications.
func notify(app vfs.App, title, message string) {
	if app == nil {
		return
	}
	if host, ok := app.(vfs.NotificationHost); ok {
		host.Notify(title, message)
		return
	}
	app.Message(title, message, []string{"&Ok"})
}
