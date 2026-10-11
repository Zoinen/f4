//go:build lite

package history

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestReadFar3HistoryLiteUnavailable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.db")
	got, err := ReadFar3History(t.Context(), path)
	if !errors.Is(err, errFar3ImportUnavailable) {
		t.Fatalf("import error = %v, want unavailable in lite builds", err)
	}
	if !reflect.DeepEqual(got, Far3History{}) {
		t.Fatalf("unavailable import returned data: %+v", got)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("unavailable import created a database: %v", err)
	}
	t.Logf("[FIX:lite-sqlite] unavailable importer: %v", err)
}

func TestReadFar3HistoryLitePreservesSource(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.db")
	want := []byte("existing history database")
	if err := os.WriteFile(path, want, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadFar3History(t.Context(), path); !errors.Is(err, errFar3ImportUnavailable) {
		t.Fatalf("import error = %v, want unavailable", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("import modified the source: %q", got)
	}
}

func TestReadFar3HistoryLiteCancellation(t *testing.T) {
	t.Run("canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if _, err := ReadFar3History(ctx, "unused.db"); !errors.Is(err, context.Canceled) {
			t.Fatalf("import error = %v, want canceled", err)
		}
	})
	t.Run("deadline", func(t *testing.T) {
		ctx, cancel := context.WithDeadline(t.Context(), time.Unix(0, 0))
		defer cancel()
		if _, err := ReadFar3History(ctx, "unused.db"); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("import error = %v, want deadline exceeded", err)
		}
	})
}
