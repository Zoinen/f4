package vfs

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

// --- ValidateInPlacePieces -------------------------------------------------

func TestValidateInPlacePieces(t *testing.T) {
	cases := []struct {
		name    string
		pieces  []PatchPiece
		wantErr bool
	}{
		{"empty pieces", nil, false},
		{"single unchanged piece at offset zero", []PatchPiece{{Offset: 0, Length: 10}}, false},
		{
			"new data piece ignores its own Offset field",
			[]PatchPiece{{Offset: 999, Length: 5, Data: []byte("hello")}},
			false,
		},
		{
			"sequential unchanged pieces line up",
			[]PatchPiece{
				{Offset: 0, Length: 4},
				{Offset: 4, Length: 6},
			},
			false,
		},
		{
			"new data followed by an unchanged piece that lines up",
			[]PatchPiece{
				{Length: 5, Data: []byte("hello")},
				{Offset: 5, Length: 3},
			},
			false,
		},
		{
			"unchanged piece with a gap fails",
			[]PatchPiece{
				{Offset: 0, Length: 4},
				{Offset: 10, Length: 6},
			},
			true,
		},
		{
			"unchanged piece after new data at the wrong offset fails",
			[]PatchPiece{
				{Length: 5, Data: []byte("hello")},
				{Offset: 0, Length: 3},
			},
			true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateInPlacePieces(tc.pieces)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ValidateInPlacePieces(%+v) error = %v, wantErr %v", tc.pieces, err, tc.wantErr)
			}
		})
	}
}

// --- ReadFileHead ------------------------------------------------------

// fakeHeadReaderVFS is a minimal VFS that only implements HeadReader, for
// pinning ReadFileHead's own logic in isolation from any real provider.
// Embedding the VFS interface (left nil) rather than implementing every
// method mirrors the pattern already used by session_identity_test.go in
// this package: ReadFileHead only ever calls the HeadReader methods below.
type fakeHeadReaderVFS struct {
	VFS
	read func(ctx context.Context, path string, p []byte) (int, error)
}

func (f *fakeHeadReaderVFS) ReadHead(ctx context.Context, path string, p []byte) (int, error) {
	return f.read(ctx, path, p)
}

func TestReadFileHeadNotAHeadReader(t *testing.T) {
	v := NewNullVFS(0) // a real, full VFS that does not implement HeadReader
	got, err := ReadFileHead(context.Background(), v, "/1KB.bin", 16)
	if err != nil || got != nil {
		t.Fatalf("ReadFileHead on a non-HeadReader VFS = (%v, %v), want (nil, nil)", got, err)
	}
}

func TestReadFileHeadNonPositiveLimit(t *testing.T) {
	v := &fakeHeadReaderVFS{read: func(ctx context.Context, path string, p []byte) (int, error) {
		t.Fatal("ReadHead must not be called when limit <= 0")
		return 0, nil
	}}
	for _, limit := range []int{0, -1} {
		got, err := ReadFileHead(context.Background(), v, "/f", limit)
		if err != nil || got != nil {
			t.Fatalf("ReadFileHead with limit=%d = (%v, %v), want (nil, nil)", limit, got, err)
		}
	}
}

func TestReadFileHeadNilContextDefaultsToBackground(t *testing.T) {
	v := &fakeHeadReaderVFS{read: func(ctx context.Context, path string, p []byte) (int, error) {
		if ctx == nil {
			t.Fatal("expected ReadFileHead to substitute context.Background() for a nil ctx")
		}
		n := copy(p, "hi")
		return n, nil
	}}
	var nilCtx context.Context // typed nil: this test exercises the nil-ctx fallback on purpose
	got, err := ReadFileHead(nilCtx, v, "/f", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "hi" {
		t.Fatalf("got %q, want %q", got, "hi")
	}
}

func TestReadFileHeadUnavailableIsNotAnError(t *testing.T) {
	v := &fakeHeadReaderVFS{read: func(ctx context.Context, path string, p []byte) (int, error) {
		return 0, ErrHeadUnavailable
	}}
	got, err := ReadFileHead(context.Background(), v, "/f", 10)
	if err != nil || got != nil {
		t.Fatalf("ReadFileHead with ErrHeadUnavailable = (%v, %v), want (nil, nil)", got, err)
	}
}

func TestReadFileHeadWrappedUnavailableIsNotAnError(t *testing.T) {
	v := &fakeHeadReaderVFS{read: func(ctx context.Context, path string, p []byte) (int, error) {
		return 0, fmt.Errorf("archive member: %w", ErrHeadUnavailable)
	}}
	got, err := ReadFileHead(context.Background(), v, "/f", 10)
	if err != nil || got != nil {
		t.Fatalf("ReadFileHead with a wrapped ErrHeadUnavailable = (%v, %v), want (nil, nil)", got, err)
	}
}

func TestReadFileHeadPropagatesOtherErrors(t *testing.T) {
	boom := errors.New("boom")
	v := &fakeHeadReaderVFS{read: func(ctx context.Context, path string, p []byte) (int, error) {
		return 0, boom
	}}
	got, err := ReadFileHead(context.Background(), v, "/f", 10)
	if !errors.Is(err, boom) {
		t.Fatalf("expected the underlying error to propagate, got %v", err)
	}
	if got != nil {
		t.Fatalf("expected no bytes on error, got %v", got)
	}
}

func TestReadFileHeadTruncatesToBytesActuallyRead(t *testing.T) {
	v := &fakeHeadReaderVFS{read: func(ctx context.Context, path string, p []byte) (int, error) {
		// Short read: the file is shorter than the requested limit.
		n := copy(p, "abc")
		return n, nil
	}}
	got, err := ReadFileHead(context.Background(), v, "/f", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "abc" {
		t.Fatalf("got %q, want %q", got, "abc")
	}
	if len(got) != 3 {
		t.Fatalf("expected the result truncated to the bytes actually read, got len=%d", len(got))
	}
}

// --- Provider registry: RegisterProvider/UnregisterProvider/FindProvider/
//     FindStandaloneProvider --------------------------------------------

// fakeVFSProvider is a minimal, closure-configurable VFSProvider used only
// to exercise the package-level provider registry. Each test registers its
// own instances and unregisters them via t.Cleanup so the shared registry
// never leaks state into other tests in this package.
type fakeVFSProvider struct {
	canOpen func(ctx context.Context, parent VFS, path string) bool
	open    func(ctx context.Context, parent VFS, path string) (VFS, error)
}

func (p *fakeVFSProvider) Name() string { return "fake" }

func (p *fakeVFSProvider) Priority() int { return 0 }

func (p *fakeVFSProvider) CanOpen(ctx context.Context, parent VFS, path string) bool {
	if p.canOpen == nil {
		return false
	}
	return p.canOpen(ctx, parent, path)
}

func (p *fakeVFSProvider) Open(ctx context.Context, parent VFS, path string) (VFS, error) {
	if p.open == nil {
		return nil, nil
	}
	return p.open(ctx, parent, path)
}

// fakeStandaloneProvider additionally implements StandalonePathProvider,
// with the opt-in controlled explicitly by the standalone field.
type fakeStandaloneProvider struct {
	fakeVFSProvider
	standalone bool
}

func (p *fakeStandaloneProvider) OpensStandalonePaths() bool { return p.standalone }

func TestFindProviderReturnsFirstRegisteredMatch(t *testing.T) {
	nonMatching := &fakeVFSProvider{canOpen: func(ctx context.Context, parent VFS, path string) bool { return false }}
	first := &fakeVFSProvider{canOpen: func(ctx context.Context, parent VFS, path string) bool { return path == "/match" }}
	second := &fakeVFSProvider{canOpen: func(ctx context.Context, parent VFS, path string) bool { return path == "/match" }}

	RegisterProvider(nonMatching)
	t.Cleanup(func() { UnregisterProvider(nonMatching) })
	RegisterProvider(first)
	t.Cleanup(func() { UnregisterProvider(first) })
	RegisterProvider(second)
	t.Cleanup(func() { UnregisterProvider(second) })

	got := FindProvider(context.Background(), nil, "/match")
	if got != first {
		t.Fatalf("FindProvider returned %v, want the first registered match %v", got, first)
	}
}

func TestFindProviderNoMatch(t *testing.T) {
	p := &fakeVFSProvider{canOpen: func(ctx context.Context, parent VFS, path string) bool { return false }}
	RegisterProvider(p)
	t.Cleanup(func() { UnregisterProvider(p) })

	if got := FindProvider(context.Background(), nil, "/anything"); got != nil {
		t.Fatalf("expected no provider match, got %v", got)
	}
}

func TestUnregisterProviderRemovesExactInstance(t *testing.T) {
	p := &fakeVFSProvider{canOpen: func(ctx context.Context, parent VFS, path string) bool { return path == "/x" }}
	RegisterProvider(p)

	if FindProvider(context.Background(), nil, "/x") != p {
		t.Fatal("expected the provider to be registered and findable")
	}
	if !UnregisterProvider(p) {
		t.Fatal("expected the first UnregisterProvider call to report success")
	}
	if UnregisterProvider(p) {
		t.Fatal("expected a second UnregisterProvider call on an already-removed provider to report failure")
	}
	if FindProvider(context.Background(), nil, "/x") != nil {
		t.Fatal("expected the provider to no longer be findable after Unregister")
	}
}

func TestUnregisterProviderNil(t *testing.T) {
	if UnregisterProvider(nil) {
		t.Fatal("expected UnregisterProvider(nil) to report failure")
	}
}

// nonComparableProvider deliberately carries a slice field, making its type
// non-comparable. UnregisterProvider must reject it via reflect before ever
// reaching the "==" comparison, which would otherwise panic at runtime.
type nonComparableProvider struct {
	values []int
}

func (nonComparableProvider) Name() string { return "fake-non-comparable" }

func (nonComparableProvider) Priority() int { return 0 }

func (nonComparableProvider) CanOpen(ctx context.Context, parent VFS, path string) bool {
	return false
}
func (nonComparableProvider) Open(ctx context.Context, parent VFS, path string) (VFS, error) {
	return nil, nil
}

func TestUnregisterProviderNonComparableType(t *testing.T) {
	target := nonComparableProvider{values: []int{1, 2, 3}}
	if UnregisterProvider(target) {
		t.Fatal("expected a non-comparable provider value to be rejected, not matched")
	}
}

func TestUnregisterProviderSkipsOtherEntries(t *testing.T) {
	decoy := &fakeStandaloneProvider{standalone: true}
	target := &fakeVFSProvider{}
	other := &fakeVFSProvider{}

	RegisterProvider(decoy)
	t.Cleanup(func() { UnregisterProvider(decoy) })
	RegisterProvider(target)
	RegisterProvider(other)
	t.Cleanup(func() { UnregisterProvider(other) })

	if !UnregisterProvider(target) {
		t.Fatal("expected the target provider, registered among others, to be removed")
	}
	if UnregisterProvider(target) {
		t.Fatal("target should already be gone")
	}
	// other has the exact same type as target but is a different instance:
	// type equality alone must not be enough for a match, so it must still
	// be registered and independently removable.
	if !UnregisterProvider(other) {
		t.Fatal("expected the other same-type provider to still be registered and removable")
	}
}

func TestFindStandaloneProvider(t *testing.T) {
	standaloneMatch := &fakeStandaloneProvider{standalone: true}
	standaloneMatch.canOpen = func(ctx context.Context, parent VFS, path string) bool { return path == "/acct/foo" }

	standaloneButOptedOut := &fakeStandaloneProvider{standalone: false}
	standaloneButOptedOut.canOpen = func(ctx context.Context, parent VFS, path string) bool { return path == "/acct/foo" }

	plain := &fakeVFSProvider{canOpen: func(ctx context.Context, parent VFS, path string) bool { return path == "/acct/foo" }}

	for _, p := range []VFSProvider{standaloneButOptedOut, plain, standaloneMatch} {
		RegisterProvider(p)
	}
	t.Cleanup(func() {
		UnregisterProvider(standaloneButOptedOut)
		UnregisterProvider(plain)
		UnregisterProvider(standaloneMatch)
	})

	got := FindStandaloneProvider(context.Background(), nil, "/acct/foo")
	if got != standaloneMatch {
		t.Fatalf("FindStandaloneProvider = %v, want the provider that both implements StandalonePathProvider and opts in", got)
	}

	if got := FindStandaloneProvider(context.Background(), nil, "/nowhere"); got != nil {
		t.Fatalf("expected no standalone provider for an unmatched path, got %v", got)
	}
}
