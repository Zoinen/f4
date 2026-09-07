package vtui

import (
	"encoding/json"
	"os"
	"testing"
)

func TestFactory_NewByType(t *testing.T) {
	data, err := os.ReadFile("vocabulary.json")
	if err != nil {
		t.Fatalf("Failed to read vocabulary.json: %v", err)
	}

	var vocab struct {
		Widgets map[string]any `json:"widgets"`
	}
	if err := json.Unmarshal(data, &vocab); err != nil {
		t.Fatalf("Failed to unmarshal vocabulary.json: %v", err)
	}

	for typeName := range vocab.Widgets {
		if typeName == "Widget" {
			continue // abstract base type
		}
		t.Run(typeName, func(t *testing.T) {
			el, err := NewByType(typeName)
			if err != nil {
				t.Fatalf("NewByType(%q) failed: %v", typeName, err)
			}
			if el == nil {
				t.Fatalf("NewByType(%q) returned nil element", typeName)
			}
			pa, ok := el.(PropertyAccess)
			if !ok {
				t.Fatalf("Type %q does not implement PropertyAccess", typeName)
			}
			// Verify basic property access
			if err := pa.SetProperty("id", PropValString("test_id")); err != nil {
				t.Errorf("SetProperty(id) failed on %q: %v", typeName, err)
			}
			if v, ok := pa.GetProperty("id"); !ok || v.S != "test_id" {
				t.Errorf("GetProperty(id) mismatch on %q: got %v, ok=%v", typeName, v, ok)
			}
		})
	}

	// Test unknown type name
	if _, err := NewByType("NonExistentWidget"); err == nil {
		t.Error("Expected error for unknown type name, got nil")
	}
}
