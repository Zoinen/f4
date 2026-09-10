package testutil

import (
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

// RunWithEnvironment executes a test in a fresh process when a package reads
// its environment during initialization. The parent returns after validating
// the child; the initialized child continues with the test's assertions.
func RunWithEnvironment(t *testing.T, key, value string) bool {
	t.Helper()
	return runWithEnvironment(t, key, value, true)
}

// RunWithoutEnvironment validates initialization with a flag absent, rather than set to an empty value.
func RunWithoutEnvironment(t *testing.T, key string) bool {
	t.Helper()
	return runWithEnvironment(t, key, "", false)
}

func runWithEnvironment(t *testing.T, key, value string, present bool) bool {
	t.Helper()
	if actual, exists := os.LookupEnv(key); exists == present && actual == value {
		return true
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(executable, "-test.run=^"+regexp.QuoteMeta(t.Name())+"$", "-test.timeout=60s")
	for _, entry := range os.Environ() {
		if name, _, _ := strings.Cut(entry, "="); !strings.EqualFold(name, key) {
			command.Env = append(command.Env, entry)
		}
	}
	if present {
		command.Env = append(command.Env, key+"="+value)
	}
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("initialized test process: %v\n%s", err, output)
	}
	return false
}
