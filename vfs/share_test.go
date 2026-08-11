package vfs

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestValidateShareURL(t *testing.T) {
	for _, valid := range []string{
		"https://drive.example/item?id=opaque#view",
		"http://127.0.0.1:8080/dav/folder/",
	} {
		if err := ValidateShareURL(valid); err != nil {
			t.Errorf("ValidateShareURL(%q) = %v", valid, err)
		}
	}
	for _, invalid := range []string{
		"", "relative/path", "ftp://example.test/file", "https://user:secret@example.test/file",
		"https://example.test/file\nX-Fake: value", "https://example.test/file\u202eexe", "https://example.test/file\u0085next",
		"https://", strings.Repeat("x", (64<<10)+1),
	} {
		if err := ValidateShareURL(invalid); !errors.Is(err, ErrInvalidShareURL) || (invalid != "" && strings.Contains(err.Error(), invalid)) {
			t.Errorf("ValidateShareURL(%q) = %v", invalid, err)
		}
	}
}

func TestValidateCreatedShareLink(t *testing.T) {
	issued := time.Now()
	now := issued.Add(time.Second)
	request := ShareLinkRequest{Role: ShareRoleViewer, ExpiresIn: time.Hour}
	valid := ShareLink{URL: "https://share.example/item?signature=opaque", Role: ShareRoleViewer, ExpiresAt: issued.Add(time.Hour), ExpiresAtIsMaximum: true}
	if err := ValidateCreatedShareLink(valid, request, issued, now); err != nil {
		t.Fatalf("valid result rejected: %v", err)
	}

	tests := map[string]func(*ShareLink, *ShareLinkRequest){
		"stronger role":      func(link *ShareLink, _ *ShareLinkRequest) { link.Role = ShareRoleEditor },
		"missing expiration": func(link *ShareLink, _ *ShareLinkRequest) { link.ExpiresAt = time.Time{} },
		"expired":            func(link *ShareLink, _ *ShareLinkRequest) { link.ExpiresAt = now },
		"too long":           func(link *ShareLink, _ *ShareLinkRequest) { link.ExpiresAt = issued.Add(2 * time.Hour) },
		"permanent mismatch": func(link *ShareLink, req *ShareLinkRequest) { req.ExpiresIn = 0 },
		"maximum without time": func(link *ShareLink, req *ShareLinkRequest) {
			req.ExpiresIn = 0
			link.ExpiresAt = time.Time{}
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			link, req := valid, request
			mutate(&link, &req)
			if err := ValidateCreatedShareLink(link, req, issued, now); !errors.Is(err, ErrInvalidShareResult) {
				t.Fatalf("invalid result error = %v", err)
			}
		})
	}

	permanent := ShareLink{URL: "https://share.example/item", Role: ShareRoleViewer}
	if err := ValidateCreatedShareLink(permanent, ShareLinkRequest{Role: ShareRoleViewer}, issued, now); err != nil {
		t.Fatalf("valid permanent result rejected: %v", err)
	}
}
