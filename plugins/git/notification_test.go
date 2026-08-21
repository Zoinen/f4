package gitplugin

import (
	"testing"

	"github.com/unxed/f4/vfs"
)

type recordedNotification struct {
	title   string
	message string
}

type notificationTestApp struct {
	logControllerTestApp
	notifications []recordedNotification
}

func (app *notificationTestApp) Notify(title, message string) {
	app.notifications = append(app.notifications, recordedNotification{title: title, message: message})
}

func TestNotifyPrefersAsynchronousHostCapability(t *testing.T) {
	app := &notificationTestApp{}

	notify(app, " Git ", "background discovery")

	if got := len(app.messages); got != 0 {
		t.Fatalf("synchronous Message calls = %d, want 0", got)
	}
	if got, want := app.notifications, []recordedNotification{{title: " Git ", message: "background discovery"}}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("notifications = %#v, want %#v", got, want)
	}
}

func TestNotifyFallsBackToSynchronousMessageForOlderHosts(t *testing.T) {
	app := &logControllerTestApp{}

	notify(app, " Git ", "legacy host")

	if got, want := app.messages, []string{"legacy host"}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("fallback messages = %#v, want %#v", got, want)
	}
}

var _ vfs.NotificationHost = (*notificationTestApp)(nil)
