package main

import (
	"context"
	"errors"
	"testing"

	"github.com/unxed/f4/vfs"
	lua "github.com/yuin/gopher-lua"
)

func TestMacroCallProviderAliasesAndCleanup(t *testing.T) {
	api := &coreAPI{}
	wantContext := vfs.MacroCallContext{Current: vfs.FileRef{Dir: "/media", Name: "clip.mkv", Path: "/media/clip.mkv"}}
	registration, err := api.RegisterMacroCallProvider(vfs.MacroCallProvider{
		IDs: []string{"test.macro-provider", "{AABBCCDD-0000-0000-0000-000000000001}"},
		Call: func(_ context.Context, got vfs.MacroCallContext, args []any) ([]any, error) {
			if got.Current.Path != wantContext.Current.Path {
				t.Fatalf("call context = %#v", got)
			}
			return []any{true, int64(len(args))}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(registration.Unregister)

	results, err := dispatchMacroPluginCall(context.Background(), "aabbccdd-0000-0000-0000-000000000001", wantContext, []any{"file"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || results[0] != true || results[1] != int64(1) {
		t.Fatalf("results = %#v", results)
	}

	registration.Unregister()
	registration.Unregister()
	if _, err := dispatchMacroPluginCall(context.Background(), "test.macro-provider", wantContext, nil); !errors.Is(err, errMacroCallProviderNotFound) {
		t.Fatalf("error after unregister = %v", err)
	}
}

func TestLuaPluginCallSupportsMultipleResultsAndArrays(t *testing.T) {
	host := newFakeMacroHost()
	host.pluginCall = func(ctx context.Context, id string, args []any) ([]any, error) {
		if ctx == nil || id != "test.provider" {
			t.Errorf("Plugin.Call id/context = %q/%v", id, ctx)
			return nil, errors.New("unexpected Plugin.Call target")
		}
		if len(args) != 3 || args[0] != true || args[1] != int64(7) {
			t.Errorf("Plugin.Call args = %#v", args)
			return nil, errors.New("unexpected Plugin.Call arguments")
		}
		nested, ok := args[2].([]any)
		if !ok || len(nested) != 2 || nested[0] != "Format" {
			t.Errorf("nested argument = %#v", args[2])
			return nil, errors.New("unexpected nested Plugin.Call argument")
		}
		return []any{true, int64(2), []string{"Format", "Duration"}, []string{"Matroska", "1 min"}}, nil
	}
	engine := newTestMacroEngine(t, host, `
		ok_result = false
		count_result = 0
		first_key = ""
		second_value = ""
		Macro { area = "Shell"; key = "CtrlP"; action = function()
			local ok, count, keys, values = Plugin.Call("test.provider", true, 7, {"Format", "Duration"})
			ok_result = ok
			count_result = count
			first_key = keys[1]
			second_value = values[2]
		end }
	`)
	if !fireMacro(t, engine, "CtrlP") {
		t.Fatal("macro was not consumed")
	}
	values := macroGlobals(t, engine, "ok_result", "count_result", "first_key", "second_value")
	if values["ok_result"] != lua.LTrue || values["count_result"] != lua.LNumber(2) ||
		values["first_key"] != lua.LString("Format") || values["second_value"] != lua.LString("1 min") {
		t.Fatalf("Plugin.Call globals = %#v", values)
	}
}
