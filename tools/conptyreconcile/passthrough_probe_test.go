package main

import (
	"strings"
	"testing"
)

func TestPassthroughResultsSeesWhatReachedTheOutput(t *testing.T) {
	var raw strings.Builder
	for _, seq := range passthroughSequences {
		if seq.Name == "sixel" || seq.Name == "osc-7-cwd" {
			continue // these two are swallowed
		}
		raw.WriteString("noise" + seq.Data + "noise")
	}
	got := passthroughResults([]byte(raw.String()))
	if len(got) != len(passthroughSequences) {
		t.Fatalf("%d results for %d sequences", len(got), len(passthroughSequences))
	}
	for _, seq := range passthroughSequences {
		want := seq.Name != "sixel" && seq.Name != "osc-7-cwd"
		if got[seq.Name] != want {
			t.Errorf("%s = %v, want %v", seq.Name, got[seq.Name], want)
		}
	}
}

func TestPassthroughWorkloadCarriesEverySequenceBetweenTheMarkers(t *testing.T) {
	w := semanticProbeWorkload("passthrough", "BEGIN", "END")
	if !strings.Contains(w, "BEGIN") || !strings.Contains(w, "END") {
		t.Fatal("markers missing")
	}
	for _, seq := range passthroughSequences {
		if !strings.Contains(w, seq.Data) {
			t.Errorf("workload lacks %s", seq.Name)
		}
	}
}
