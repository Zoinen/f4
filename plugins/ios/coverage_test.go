package iosfs

import (
	"errors"
	"reflect"
	"testing"

	"github.com/danielpaulus/go-ios/ios/installationproxy"
)

func TestApplicationMetadataHelpersNormalizeValues(t *testing.T) {
	app := map[string]interface{}{
		"name":       "  Example  ",
		"groups":     map[string]interface{}{"one": true},
		"enabled":    true,
		"wrongValue": "not a map",
	}
	nativeApp := installationproxy.AppInfo(app)
	if got := appString(nativeApp, "name"); got != "Example" {
		t.Fatalf("appString = %q, want trimmed value", got)
	}
	if got := appString(nativeApp, "missing"); got != "" {
		t.Fatalf("missing appString = %q, want empty", got)
	}
	if got := appMap(nativeApp, "groups"); !reflect.DeepEqual(got, map[string]interface{}{"one": true}) {
		t.Fatalf("appMap = %#v, want group map", got)
	}
	if got := appMap(nativeApp, "wrongValue"); len(got) != 0 {
		t.Fatalf("wrong-type appMap = %#v, want empty map", got)
	}
	if got := appMap(nativeApp, "missing"); len(got) != 0 {
		t.Fatalf("missing appMap = %#v, want empty map", got)
	}
	if !mapBool(app, "enabled") || mapBool(app, "name") {
		t.Fatal("mapBool must accept bool values and reject other types")
	}
}

func TestCollectAppGroupsMergesAndSortsContainerForms(t *testing.T) {
	entitlements := map[string]interface{}{
		"com.apple.security.application-groups": []interface{}{" group.z ", "", "group.a"},
	}
	containers := map[string]interface{}{
		" group.m ": "container",
		"group.a":   "duplicate",
		"":          "ignored",
	}
	want := []string{"group.a", "group.m", "group.z"}
	if got := collectAppGroups(entitlements, containers); !reflect.DeepEqual(got, want) {
		t.Fatalf("collectAppGroups = %#v, want %#v", got, want)
	}

	entitlements["com.apple.security.application-groups"] = []string{" group.b ", "group.a"}
	if got := collectAppGroups(entitlements, nil); !reflect.DeepEqual(got, []string{"group.a", "group.b"}) {
		t.Fatalf("collectAppGroups with []string = %#v", got)
	}
}

func TestNativeMetadataHelpersClassifyLockdownFailures(t *testing.T) {
	for _, test := range []struct {
		err    error
		state  string
		paired bool
	}{
		{err: errors.New("device is not paired"), state: DeviceStateUnpaired, paired: false},
		{err: errors.New("device is locked by passcode"), state: DeviceStateLocked, paired: true},
		{err: errors.New("connection refused"), state: DeviceStateOffline, paired: true},
	} {
		state, paired := lockdownFailureState(test.err)
		if state != test.state || paired != test.paired {
			t.Errorf("lockdownFailureState(%q) = (%q, %t), want (%q, %t)", test.err, state, paired, test.state, test.paired)
		}
	}
	if got := plistString(map[string]interface{}{"name": "  iPhone  "}, "name"); got != "iPhone" {
		t.Errorf("plistString = %q, want iPhone", got)
	}
	if plistString(map[string]interface{}{"name": 7}, "name") != "" {
		t.Error("plistString must reject non-string values")
	}
	for _, test := range []struct {
		value interface{}
		want  bool
	}{
		{value: true, want: true},
		{value: uint64(1), want: true},
		{value: int64(0), want: false},
		{value: "enabled", want: true},
		{value: "false", want: false},
		{value: 7, want: false},
	} {
		if got := plistBool(map[string]interface{}{"value": test.value}, "value"); got != test.want {
			t.Errorf("plistBool(%#v) = %t, want %t", test.value, got, test.want)
		}
	}
}
