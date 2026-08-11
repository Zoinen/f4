package vfs

import (
	"errors"
	"net/url"
	"strings"
	"time"
	"unicode"
)

var (
	ErrInvalidShareURL    = errors.New("invalid share link URL")
	ErrInvalidShareResult = errors.New("invalid share link result")
)

func unsafeShareRune(r rune) bool {
	return unicode.IsControl(r) || unicode.In(r, unicode.Cf, unicode.Zl, unicode.Zp)
}

// ValidateShareURL applies the host trust boundary for links returned by any
// ShareLinkProvider. A share URL can be a bearer credential, so userinfo,
// control characters, non-web schemes and unbounded values are rejected before
// a link is displayed or copied. The raw value is deliberately absent from the
// returned error.
func ValidateShareURL(raw string) error {
	if raw == "" || len(raw) > 64<<10 || strings.IndexFunc(raw, unsafeShareRune) >= 0 {
		return ErrInvalidShareURL
	}
	target, err := url.Parse(raw)
	if err != nil || !target.IsAbs() || target.Host == "" || target.User != nil ||
		(!strings.EqualFold(target.Scheme, "https") && !strings.EqualFold(target.Scheme, "http")) {
		return ErrInvalidShareURL
	}
	return nil
}

// ValidateCreatedShareLink verifies the provider result against the exact
// access and lifetime requested by the user. issuedAfter must be captured
// immediately before the provider call. A mismatch is security-relevant: the
// caller must not display or copy the URL as a successful result.
func ValidateCreatedShareLink(link ShareLink, request ShareLinkRequest, issuedAfter, now time.Time) error {
	if err := ValidateShareURL(link.URL); err != nil || link.Role != request.Role || link.Role < ShareRoleViewer || link.Role > ShareRoleServerControlled {
		return ErrInvalidShareResult
	}
	if link.ExpiresAtIsMaximum && link.ExpiresAt.IsZero() {
		return ErrInvalidShareResult
	}
	if request.ExpiresIn == 0 {
		if !link.ExpiresAt.IsZero() {
			return ErrInvalidShareResult
		}
		return nil
	}
	if request.ExpiresIn < 0 || link.ExpiresAt.IsZero() || !link.ExpiresAt.After(now) {
		return ErrInvalidShareResult
	}
	// Allow a small scheduling/clock granularity margin, while rejecting a
	// provider that returns a materially longer-lived bearer URL than selected.
	elapsed := now.Sub(issuedAfter)
	if elapsed < 0 {
		elapsed = 0
	}
	if link.ExpiresAt.After(issuedAfter.Add(elapsed + request.ExpiresIn + time.Minute)) {
		return ErrInvalidShareResult
	}
	return nil
}
