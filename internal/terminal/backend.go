package terminal

// SelectedTTYBackend holds the user-chosen or auto-detected console renderer
// name ("ansi" or "winapi"). The composition root sets it once at startup.
var SelectedTTYBackend string
