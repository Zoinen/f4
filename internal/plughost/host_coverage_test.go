package plughost

import (
	msgpack "github.com/vmihailenco/msgpack/v5"
	"testing"
)

type recordingPluginTransport struct {
	calls []string
}

func (r *recordingPluginTransport) Call(method string, params, result any) error {
	r.calls = append(r.calls, method)
	return nil
}

func callHostMethod(t *testing.T, handler func(msgpack.RawMessage) (any, error), value any) any {
	t.Helper()
	data, err := msgpack.Marshal(value)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	result, err := handler(data)
	if err != nil {
		t.Fatalf("handler failed: %v", err)
	}
	return result
}

func TestNewHostMethodsBasicHandlers(t *testing.T) {
	api := newLuaTestHostAPI()
	back := &recordingPluginTransport{}
	methods := newHostMethods(api, back, "test-plugin", nil)

	callHostMethod(t, methods["Host.Log"], "log message")
	callHostMethod(t, methods["Host.Message"], "user message")
	if len(api.logs) != 2 || api.logs[0] != "log message" || api.logs[1] != "user message" {
		t.Fatalf("host messages = %q", api.logs)
	}

	if got := callHostMethod(t, methods["Host.GetVersion"], nil); got != "test-version" {
		t.Fatalf("Host.GetVersion = %v", got)
	}
	if got := callHostMethod(t, methods["Host.RunAction"], "open"); got != true {
		t.Fatalf("Host.RunAction = %v", got)
	}
	if len(api.logs) != 3 || api.logs[2] != "action:open" {
		t.Fatalf("action log = %q", api.logs)
	}

	callHostMethod(t, methods["Host.RegisterHighlighter"], nil)
	callHostMethod(t, methods["Host.RegisterGlobalHotkey"], HotkeyReq{VK: 42})
	callHostMethod(t, methods["Host.UpdateProgress"], ProgressUpdateReq{Msg: "working", Percent: 50})
	if got := callHostMethod(t, methods["Host.IsProgressCancelled"], nil); got != false {
		t.Fatalf("Host.IsProgressCancelled = %v", got)
	}

	oldApp := App
	App = nil
	t.Cleanup(func() { App = oldApp })
	if got := callHostMethod(t, methods["Host.AskOverwrite"], AskOverwriteReq{Path: "file.txt"}); got != (AskOverwriteRes{}) {
		t.Fatalf("Host.AskOverwrite without app = %+v", got)
	}
	if got := callHostMethod(t, methods["Host.AskError"], AskErrorReq{Op: "read", Err: "failed"}); got != 0 {
		t.Fatalf("Host.AskError without app = %v", got)
	}
}
