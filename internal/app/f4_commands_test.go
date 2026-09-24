package app

import (
	"testing"

	"github.com/unxed/f4/internal/panel"
)

func TestF4CommandPrefixIsRegistered(t *testing.T) {
	panel.CommandPrefixRegistry.RLock()
	registration := panel.CommandPrefixRegistry.ByPrefix[f4CommandPrefix]
	panel.CommandPrefixRegistry.RUnlock()
	if registration == nil || !registration.Active || registration.Id != f4CommandPrefix {
		t.Fatalf("f4: prefix registration = %#v", registration)
	}
}

func TestF4CommandActionResolvesOnlyKnownCommands(t *testing.T) {
	for _, input := range []string{"about", " About ", "ABOUT"} {
		if name, ok := f4CommandAction(input); !ok || name != "App.About" {
			t.Errorf("f4CommandAction(%q) = %q, %v; want App.About", input, name, ok)
		}
	}
	if name, ok := f4CommandAction(" Config"); !ok || name != "App.ConfigEditor" {
		t.Errorf("f4CommandAction(\" Config\") = %q, %v; want App.ConfigEditor", name, ok)
	}
	for _, input := range []string{"", "abou", "about more", "configure"} {
		if name, ok := f4CommandAction(input); ok {
			t.Errorf("f4CommandAction(%q) = %q; want no command", input, name)
		}
	}
	for _, command := range f4Commands {
		if _, ok := GetAction(command.action); !ok {
			t.Errorf("f4:%s runs %s, which is not registered", command.name, command.action)
		}
	}
}

func TestAboutFactsSuppliesTheClipboard(t *testing.T) {
	facts := aboutFacts()
	if facts.Copy == nil {
		t.Fatal("aboutFacts left Copy nil, so Ctrl+C in f4:about would do nothing")
	}
	if facts.Version == "" {
		t.Fatal("aboutFacts has no version")
	}
}
