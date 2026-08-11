package vfs

import "testing"

type sessionIdentityTestVFS struct {
	VFS
	key   any
	title string
}

type nonComparableSessionTestVFS struct {
	VFS
	values []int
}

type titleOnlySessionTestVFS struct {
	VFS
	title string
}

func (v *sessionIdentityTestVFS) SessionKey() any   { return v.key }
func (v *sessionIdentityTestVFS) GetTitle() string  { return v.title }
func (v *titleOnlySessionTestVFS) GetTitle() string { return v.title }

func TestSameSessionIdentityOverridesMatchingTitle(t *testing.T) {
	first := &sessionIdentityTestVFS{key: new(int), title: "same profile"}
	second := &sessionIdentityTestVFS{key: new(int), title: "same profile"}
	if SameSession(first, second) {
		t.Fatal("different explicit session identities were treated as the same session")
	}
}

func TestSameSessionMatchingIdentity(t *testing.T) {
	key := new(int)
	first := &sessionIdentityTestVFS{key: key, title: "old title"}
	second := &sessionIdentityTestVFS{key: key, title: "new title"}
	if !SameSession(first, second) {
		t.Fatal("matching explicit session identities were treated as different sessions")
	}
}

func TestSameSessionRejectsNonComparableIdentity(t *testing.T) {
	first := &sessionIdentityTestVFS{key: []byte("session"), title: "same profile"}
	second := &sessionIdentityTestVFS{key: []byte("session"), title: "same profile"}
	if SameSession(first, second) {
		t.Fatal("invalid non-comparable identities fell back to a matching title")
	}
}

func TestSameSessionHandlesNonComparableVFSValues(t *testing.T) {
	first := nonComparableSessionTestVFS{values: []int{1}}
	second := nonComparableSessionTestVFS{values: []int{1}}
	if SameSession(first, second) {
		t.Fatal("non-comparable VFS values were treated as the same instance")
	}
}

func TestSameSessionNeverUsesDisplayTitleAsIdentity(t *testing.T) {
	first := &titleOnlySessionTestVFS{title: "user@example"}
	second := &titleOnlySessionTestVFS{title: "user@example"}
	if SameSession(first, second) {
		t.Fatal("matching display titles were treated as a session identity")
	}
}

func TestSameSessionRejectsOneSidedIdentity(t *testing.T) {
	first := &sessionIdentityTestVFS{key: new(int), title: "same"}
	second := &titleOnlySessionTestVFS{title: "same"}
	if SameSession(first, second) {
		t.Fatal("one-sided identity fell back to a matching display title")
	}
}
