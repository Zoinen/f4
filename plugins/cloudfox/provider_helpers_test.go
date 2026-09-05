package cloudfox

import (
	"reflect"
	"testing"

	"github.com/unxed/f4/vfs"
)

type progressTestReporter struct {
	updates []int
}

func (*progressTestReporter) UpdateScan(string, int64, int64) {}
func (r *progressTestReporter) UpdateTransfer(_ string, _ string, currentPct int, _ string, _ int, _ string) {
	r.updates = append(r.updates, currentPct)
}
func (*progressTestReporter) IsCancelled() bool { return false }

var _ vfs.TaskReporter = (*progressTestReporter)(nil)

func TestProviderUploadProgressSuppressesLateBodyUpdates(t *testing.T) {
	reporter := &progressTestReporter{}
	progress := &providerUploadProgress{reporter: reporter, action: "Uploading", name: "item"}

	progress.update(99)
	progress.complete()
	progress.update(99)

	if !reflect.DeepEqual(reporter.updates, []int{99, 100}) {
		t.Fatalf("progress updates = %v, want [99 100]", reporter.updates)
	}
}

func TestStrongETagAcceptsOnlyOneRFCEntityTag(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		value string
		want  string
	}{
		{value: `"version-one"`, want: `"version-one"`},
		{value: ` "comma,inside" `, want: `"comma,inside"`},
		{value: "\"obs-\x80\"", want: "\"obs-\x80\""},
		{value: `W/"weak"`},
		{value: `w/"weak"`},
		{value: `*`},
		{value: `unquoted`},
		{value: `"one", "two"`},
		{value: "\"control\x7f\""},
		{value: "\"space inside\""},
		{value: `"embedded"quote"`},
	} {
		if got := strongETag(test.value); got != test.want {
			t.Errorf("strongETag(%q)=%q, want %q", test.value, got, test.want)
		}
	}
}

func TestCacheETagAcceptsValidStrongAndWeakValidatorsOnly(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		value string
		want  string
	}{
		{value: `"strong"`, want: `"strong"`},
		{value: ` W/"weak" `, want: `W/"weak"`},
		{value: `*`},
		{value: `garbage`},
		{value: `w/"lowercase-invalid"`},
		{value: `W/ "space-invalid"`},
		{value: `W/"one", "two"`},
	} {
		if got := cacheETag(test.value); got != test.want {
			t.Errorf("cacheETag(%q)=%q, want %q", test.value, got, test.want)
		}
	}
}

func TestWeakETagComparisonUsesOpaqueTag(t *testing.T) {
	t.Parallel()
	if !weakETagEqual(`W/"same"`, `"same"`) {
		t.Fatal("weak and strong forms of the same opaque tag did not match")
	}
	for _, pair := range [][2]string{{`"one"`, `"two"`}, {`*`, `*`}, {`garbage`, `garbage`}} {
		if weakETagEqual(pair[0], pair[1]) {
			t.Fatalf("weakETagEqual(%q,%q)=true", pair[0], pair[1])
		}
	}
}
