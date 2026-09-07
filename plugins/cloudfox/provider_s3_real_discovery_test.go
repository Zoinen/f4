package cloudfox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
)

const (
	realS3DiscoveryReadOnlyEnv       = "F4_CLOUDFOX_REAL_S3_READONLY"
	realS3DiscoveryPathEnv           = "F4_CLOUDFOX_REAL_S3_PATH"
	realS3DiscoveryConnectionEnv     = "F4_CLOUDFOX_REAL_S3_CONNECTION"
	realS3DiscoveryReadOnlyConfirmed = "CONFIRMED"
)

// TestRealSavedS3DiscoveryVisualHistoryReadOnly verifies that a persisted,
// user-facing S3 discovery path can be restored by the real connection
// provider in a completely fresh plugin/session and listed without first
// leaking an internal location into history.
//
// This harness is permanently read-only and deliberately gated independently
// from the mutation harness. It performs no config, keyring, vault or network
// access unless F4_CLOUDFOX_REAL_S3_READONLY has the exact value CONFIRMED.
// F4_CLOUDFOX_REAL_CONFIG_DIR must name an existing absolute directory, and
// F4_CLOUDFOX_REAL_S3_PATH must be a native-separator visual path containing a
// discovered bucket and at least one child directory. If several saved S3
// profiles exist, F4_CLOUDFOX_REAL_S3_CONNECTION selects one by ID or display
// name. F4_CLOUDFOX_REAL_VAULT_PASSWORD follows the other real harnesses: an
// unset/empty value intentionally unlocks an empty-password vault, while a
// non-empty value supplies that password without placing it on a command line.
//
// Profile selectors, paths, buckets, endpoints, settings, secret references,
// credentials and provider error strings are never written to test output.
func TestRealSavedS3DiscoveryVisualHistoryReadOnly(t *testing.T) {
	// This check must remain the first operation. In particular, do not inspect
	// the config/path/password environment before explicit confirmation.
	if os.Getenv(realS3DiscoveryReadOnlyEnv) != realS3DiscoveryReadOnlyConfirmed {
		t.Skip("real read-only S3 discovery test requires explicit confirmation")
	}

	configDir := strings.TrimSpace(os.Getenv(realConfigDirEnv))
	if configDir == "" {
		t.Fatal("real read-only S3 config directory is required")
	}
	if !filepath.IsAbs(configDir) {
		t.Fatal("real read-only S3 config directory must be absolute")
	}
	info, err := os.Stat(configDir) // #nosec G703 -- the opted-in read-only test intentionally loads the operator-supplied absolute config directory.
	if err != nil || !info.IsDir() {
		t.Fatal("real read-only S3 config directory is unavailable")
	}
	visualPath := os.Getenv(realS3DiscoveryPathEnv)
	if strings.TrimSpace(visualPath) == "" {
		t.Fatal("real read-only S3 visual path is required")
	}

	prompt := MasterPasswordPromptFunc(func(ctx context.Context, _ bool) (string, error) {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		return os.Getenv(realVaultPasswordEnv), nil
	})
	plugin := NewPlugin(Options{
		ConfigDir:      configDir,
		Keyring:        NewKeyringStore(),
		PasswordPrompt: prompt,
	})
	t.Cleanup(func() {
		if err := plugin.Close(); err != nil {
			t.Errorf("close real read-only S3 plugin: %s", realS3DiscoveryReadOnlyErrorClass(err))
		}
	})

	loadCtx, cancelLoad := context.WithTimeout(context.Background(), 30*time.Second)
	connections, err := plugin.Repository().List(loadCtx)
	cancelLoad()
	if err != nil {
		t.Fatalf("load saved read-only S3 connections: %s", realS3DiscoveryReadOnlyErrorClass(err))
	}
	connection := selectRealConnection(t, connections, ProviderS3, realS3DiscoveryConnectionEnv)
	var settings S3Settings
	if err := json.Unmarshal(connection.Settings, &settings); err != nil {
		t.Fatal("saved read-only S3 profile settings are invalid")
	}
	if strings.TrimSpace(settings.Bucket) != "" {
		t.Fatal("saved read-only S3 profile must use bucket discovery mode")
	}
	if err := validateRealS3DiscoveryVisualPath(connection, visualPath); err != nil {
		t.Fatal("real read-only S3 path is not a canonical native visual history path")
	}

	provider := &connectionProvider{plugin: plugin}
	parent := vfs.NewOSVFS(configDir)
	probeCtx, cancelProbe := context.WithTimeout(context.Background(), 15*time.Second)
	canOpen := provider.CanOpen(probeCtx, parent, visualPath)
	cancelProbe()
	if !canOpen {
		t.Fatal("real read-only S3 connection provider rejected the visual history path")
	}

	openCtx, cancelOpen := context.WithTimeout(context.Background(), 2*time.Minute)
	opened, err := provider.Open(openCtx, parent, visualPath)
	cancelOpen()
	if err != nil {
		t.Fatalf("restore real read-only S3 visual history path: %s", realS3DiscoveryReadOnlyErrorClass(err))
	}
	t.Cleanup(func() {
		if err := opened.Close(); err != nil {
			t.Errorf("close restored real read-only S3 filesystem: %s", realS3DiscoveryReadOnlyErrorClass(err))
		}
	})
	if unwrapCloudVFS(opened) == nil {
		t.Fatal("real read-only S3 connection provider returned an unexpected filesystem")
	}
	if opened.GetPath() != visualPath {
		t.Fatal("restored real read-only S3 path differs from the requested visual history path")
	}

	readCtx, cancelRead := context.WithTimeout(context.Background(), 2*time.Minute)
	err = opened.ReadDir(readCtx, opened.GetPath(), func([]vfs.VFSItem) {})
	cancelRead()
	if err != nil {
		t.Fatalf("list restored real read-only S3 directory: %s", realS3DiscoveryReadOnlyErrorClass(err))
	}
}

func validateRealS3DiscoveryVisualPath(connection Connection, visualPath string) error {
	separator := string(os.PathSeparator)
	prefix := connection.Name + ":" + separator
	if !strings.HasPrefix(visualPath, prefix) {
		return errors.New("visual path belongs to another connection")
	}
	relative := strings.TrimPrefix(visualPath, prefix)
	if relative == "" {
		return errors.New("visual path does not select a bucket")
	}
	foreignSeparator := "/"
	if os.PathSeparator == '/' {
		foreignSeparator = "\\"
	}
	if strings.Contains(relative, foreignSeparator) {
		return errors.New("visual path uses a non-native separator")
	}
	parts := strings.Split(relative, separator)
	if len(parts) < 2 {
		return errors.New("visual path must include a bucket and child directory")
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return errors.New("visual path is not canonical")
		}
	}
	return nil
}

// realS3DiscoveryReadOnlyErrorClass deliberately never calls err.Error. AWS
// errors can embed endpoints, bucket names, signed URLs and credential-shaped
// query parameters; the real harness reports only stable classes and types.
func realS3DiscoveryReadOnlyErrorClass(err error) string {
	if err == nil {
		return "none"
	}
	classes := make([]string, 0, 4)
	for _, candidate := range []struct {
		name string
		err  error
	}{
		{name: "canceled", err: context.Canceled},
		{name: "deadline", err: context.DeadlineExceeded},
		{name: "permission", err: os.ErrPermission},
		{name: "not-found", err: os.ErrNotExist},
	} {
		if errors.Is(err, candidate.err) {
			classes = append(classes, candidate.name)
		}
	}
	if len(classes) == 0 {
		classes = append(classes, "other")
	}
	return fmt.Sprintf("%T[%s]", err, strings.Join(classes, ","))
}
