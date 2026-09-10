package media

// Application is what the media views need from the application above them.
// One method: the image viewer forwards a frame command it does not own —
// forking a workspace — the same way the file viewer does.
type Application interface {
	// HandleCommand runs a frame command the media views do not own
	// themselves and reports whether it did.
	HandleCommand(cmd int, args any) bool
}

// App is the live application. The composition root sets it once at startup.
var App Application
