package fishplus

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestCompactDropsCommentsAndIndentation(t *testing.T) {
	src := "# a comment\n\n  echo hello\n\t# indented comment\n  \n  case $x in\n   foo ) echo bar;;\n  esac\n"
	want := "echo hello\ncase $x in\nfoo ) echo bar;;\nesac\n"
	if got := Compact(src); got != want {
		t.Errorf("Compact() = %q, want %q", got, want)
	}
}

func TestCompactNormalizesCRLF(t *testing.T) {
	src := "# a comment\r\n\r\n  echo hello\r\n  case $x in\r\n   foo ) echo bar;;\r\n  esac\r\n"
	want := "echo hello\ncase $x in\nfoo ) echo bar;;\nesac\n"
	if got := Compact(src); got != want {
		t.Errorf("Compact(CRLF) = %q, want %q", got, want)
	}
}

func TestHelperScriptSubstitutesToken(t *testing.T) {
	const token = "0123456789abcdef"
	script := HelperScript(token)
	if strings.Contains(script, "\r") {
		t.Error("helper script contains a carriage return after compaction")
	}
	if strings.Contains(script, tokenPlaceholder) {
		t.Error("token placeholder survived substitution")
	}
	if !strings.Contains(script, "F4TOKEN="+token) {
		t.Error("session token was not substituted into the helper")
	}
	if strings.Contains(script, "\n#") || strings.HasPrefix(script, "#") {
		t.Error("comments survived compaction")
	}
	for _, needle := range []string{"FISHPLUS", "f4_dec", "ping", "noop"} {
		if !strings.Contains(script, needle) {
			t.Errorf("helper script lost %q during compaction", needle)
		}
	}
}

func TestHelperScriptHasNoHeredocs(t *testing.T) {
	// Compact() strips indentation, which would corrupt here-documents and
	// multi-line literals, so the helper must not contain any.
	if strings.Contains(HelperSource(), "<<") {
		t.Error("helper script uses a here-document, which Compact() would break")
	}
}

func TestHelperConsidersAndroidShellTempDirectory(t *testing.T) {
	if !strings.Contains(HelperSource(), "/data/local/tmp") {
		t.Fatal("helper does not consider Android's shell-writable temporary directory")
	}
}

func TestBase64BootstrapLineIsOneBinaryCleanLine(t *testing.T) {
	const token = "0123456789abcdef"
	line := Base64BootstrapLine(token)
	if !strings.HasSuffix(line, "\n") || strings.Count(line, "\n") != 1 {
		t.Fatalf("bootstrap must be exactly one newline-terminated line, got %d newlines", strings.Count(line, "\n"))
	}
	for i, b := range []byte(strings.TrimSuffix(line, "\n")) {
		if b < 0x20 || b > 0x7e {
			t.Fatalf("bootstrap byte %d is not printable ASCII: %#x", i, b)
		}
	}
	if strings.Contains(line, ReadyMarker(token)) {
		t.Fatal("bootstrap source contains its readiness marker verbatim; a terminal echo could satisfy the client")
	}
	if strings.Contains(line, "F4TOKEN=") {
		t.Fatal("raw helper source leaked into the base64 bootstrap")
	}
}

func TestBase64BootstrapSplitsCoincidentalReadyMarker(t *testing.T) {
	const token = "0123456789abcdef"
	// Exercise the shell-literal transformation directly: such a collision
	// is vanishingly unlikely in the real payload, but protocol safety must
	// not depend on luck.
	encoded := "before" + ReadyMarker(token) + "after"
	encoded = splitReadyMarker(encoded, token)
	if strings.Contains(encoded, ReadyMarker(token)) {
		t.Fatal("readiness marker survived shell-literal splitting")
	}
	if strings.ReplaceAll(encoded, "''", "") != "before"+ReadyMarker(token)+"after" {
		t.Fatal("shell-literal splitting changed the assigned value")
	}
}

func TestBase64BootstrapLineContainsExactHelper(t *testing.T) {
	const token = "0123456789abcdef"
	line := Base64BootstrapLine(token)
	const start = "F4B='"
	at := strings.Index(line, start)
	if at < 0 {
		t.Fatal("bootstrap has no encoded payload assignment")
	}
	rest := line[at+len(start):]
	end := strings.IndexByte(rest, '\'')
	if end < 0 {
		t.Fatal("encoded payload assignment is not terminated")
	}
	raw, err := base64.StdEncoding.DecodeString(rest[:end])
	if err != nil {
		t.Fatalf("payload is not standard base64: %v", err)
	}
	want := ": F4B64" + token + ";\n" + HelperScript(token)
	if string(raw) != want {
		t.Fatalf("decoded bootstrap payload differs from helper: got %d bytes, want %d", len(raw), len(want))
	}
}

func TestBase64BootstrapLineCoversDecoderDialects(t *testing.T) {
	line := Base64BootstrapLine("0123456789abcdef")
	for _, decoder := range []string{"base64 -d", "base64 -D", "base64 --decode", "openssl base64 -A -d"} {
		if !strings.Contains(line, "'"+decoder+"'") {
			t.Errorf("bootstrap does not try decoder %q", decoder)
		}
	}
	if strings.Contains(line, "/tmp") || strings.Contains(line, ">F4") {
		t.Fatal("base64 bootstrap appears to use a temporary file")
	}
}
