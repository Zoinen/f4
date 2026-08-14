package cloudfox

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path"
	"strings"
)

// credentialScopeSecretKey is intentionally stored with the encrypted/keyring
// credential bundle instead of the plaintext profile. An attacker who can
// rewrite CloudFox.json therefore cannot retarget credentials to another
// endpoint without making this binding fail.
const credentialScopeSecretKey = "_cloudfox_credential_scope_v1"

// credentialScope returns an opaque digest of the settings which decide where
// credentials can be sent. OAuth providers already bind their credentials to
// a provider/client audience, so the explicit profile binding is needed only
// for S3 and WebDAV.
func credentialScope(c Connection) (string, bool, error) {
	return credentialScopeWithCAFingerprint(c, nil)
}

func credentialScopeWithCAFingerprint(c Connection, caFingerprint *string) (string, bool, error) {
	var fields []string
	switch c.Provider {
	case ProviderS3:
		var settings S3Settings
		if err := jsonUnmarshalSettings(c.Settings, &settings, "S3"); err != nil {
			return "", false, err
		}
		settings.Bucket = strings.TrimSpace(settings.Bucket)
		settings.Region = strings.TrimSpace(settings.Region)
		if settings.Region == "" {
			settings.Region = "us-east-1"
		}
		settings.Auth = strings.ToLower(strings.TrimSpace(settings.Auth))
		if settings.Auth == "" {
			settings.Auth = "default"
		}
		// Anonymous access cannot disclose a credential. A profile changed from
		// anonymous to another mode will require a binding before it can open.
		if settings.Auth == "anonymous" {
			return "", false, nil
		}
		endpoint := "aws-default"
		settings.Endpoint = strings.TrimRight(strings.TrimSpace(settings.Endpoint), "/")
		if settings.Endpoint != "" {
			var err error
			endpoint, err = canonicalEndpoint(settings.Endpoint)
			if err != nil {
				return "", false, fmt.Errorf("cloudfox: canonicalize S3 credential endpoint: %w", err)
			}
		}
		customCA := ""
		if caFingerprint != nil {
			customCA = *caFingerprint
		} else {
			var err error
			customCA, err = customCAFingerprint(settings.CustomCA)
			if err != nil {
				return "", false, fmt.Errorf("cloudfox: fingerprint S3 custom CA: %w", err)
			}
		}
		fields = []string{
			"cloudfox-credential-scope-v2", strings.ToLower(c.ID), "s3", endpoint,
			settings.Bucket, strings.ToLower(settings.Region),
			strings.TrimSpace(settings.Profile), settings.Auth,
			fmt.Sprintf("path-style=%t", settings.UsePathStyle),
			"custom-ca=" + customCA,
		}
	case ProviderWebDAV:
		var settings WebDAVSettings
		if err := jsonUnmarshalSettings(c.Settings, &settings, "WebDAV"); err != nil {
			return "", false, err
		}
		settings.Auth = strings.ToLower(strings.TrimSpace(settings.Auth))
		if settings.Auth == "" {
			settings.Auth = "basic"
		}
		if settings.Auth == "anonymous" {
			return "", false, nil
		}
		origin := "invalid-origin:" + strings.TrimSpace(settings.BaseURL)
		davPath := "/"
		if base, err := url.Parse(strings.TrimSpace(settings.BaseURL)); err == nil && base.Host != "" {
			origin, err = canonicalOrigin(base)
			if err != nil {
				return "", false, fmt.Errorf("cloudfox: canonicalize WebDAV credential origin: %w", err)
			}
			root := strings.TrimSpace(strings.ReplaceAll(settings.Root, "\\", "/"))
			if root == "" {
				root = "/"
			}
			root = path.Clean("/" + strings.TrimPrefix(root, "/"))
			davPath = path.Clean(path.Join(strings.TrimSuffix(base.Path, "/"), root))
		}
		customCA := ""
		if caFingerprint != nil {
			customCA = *caFingerprint
		} else {
			var err error
			customCA, err = customCAFingerprint(settings.CustomCA)
			if err != nil {
				return "", false, fmt.Errorf("cloudfox: fingerprint WebDAV custom CA: %w", err)
			}
		}
		// Bind credentials to the configured DAV subtree as well as its origin.
		// Otherwise editing only CloudFox.json could retarget a saved password to
		// a different application mounted on the same host.
		fields = []string{
			"cloudfox-credential-scope-v2", strings.ToLower(c.ID), "webdav", origin,
			davPath, settings.Username, settings.Auth,
			"custom-ca=" + customCA,
		}
	default:
		return "", false, nil
	}

	digest := sha256.Sum256([]byte(strings.Join(fields, "\x00")))
	return "sha256:" + hex.EncodeToString(digest[:]), true, nil
}

// customCAFingerprint binds credentials to the trust roots used for their
// transport. Without this binding, changing only CloudFox.json could install
// an attacker-controlled CA while leaving the endpoint origin unchanged.
func customCAFingerprint(filename string) (string, error) {
	filename = strings.TrimSpace(filename)
	if filename == "" {
		return "system", nil
	}
	contents, err := os.ReadFile(filename)
	if err != nil {
		return "", err
	}
	return customCAContentsFingerprint(contents), nil
}

func customCAContentsFingerprint(contents []byte) string {
	if contents == nil {
		return "system"
	}
	digest := sha256.Sum256(contents)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func canonicalEndpoint(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	origin, err := canonicalOrigin(u)
	if err != nil {
		return "", err
	}
	escapedPath := strings.TrimRight(u.EscapedPath(), "/")
	query := u.RawQuery
	if query != "" {
		query = "?" + query
	}
	return origin + escapedPath + query, nil
}

func canonicalOrigin(u *url.URL) (string, error) {
	if u == nil {
		return "", errors.New("missing URL")
	}
	scheme := strings.ToLower(u.Scheme)
	hostname := strings.ToLower(u.Hostname())
	if scheme == "" || hostname == "" {
		return "", errors.New("URL has no origin")
	}
	port := u.Port()
	if (scheme == "https" && port == "443") || (scheme == "http" && port == "80") {
		port = ""
	}
	host := hostname
	if port != "" {
		host = net.JoinHostPort(hostname, port)
	} else if strings.Contains(hostname, ":") {
		host = "[" + hostname + "]"
	}
	return scheme + "://" + host, nil
}

func bindCredentialScope(c Connection, values SecretValues) (SecretValues, error) {
	bound := values.Clone()
	if bound == nil {
		bound = SecretValues{}
	}
	scope, required, err := credentialScope(c)
	if err != nil {
		return nil, err
	}
	if !required {
		delete(bound, credentialScopeSecretKey)
		return bound, nil
	}
	bound[credentialScopeSecretKey] = scope
	return bound, nil
}

func verifyCredentialScope(c Connection, values SecretValues, requireBinding bool) error {
	return verifyCredentialScopeWithCAFingerprint(c, values, requireBinding, nil)
}

func verifyCredentialScopeWithCAFingerprint(c Connection, values SecretValues, requireBinding bool, caFingerprint *string) error {
	expected, required, err := credentialScopeWithCAFingerprint(c, caFingerprint)
	if err != nil {
		return err
	}
	if !required {
		return nil
	}
	actual := values[credentialScopeSecretKey]
	if actual == "" {
		if requireBinding {
			return ErrCredentialScopeUnbound
		}
		return nil
	}
	if actual != expected {
		return ErrCredentialScopeMismatch
	}
	return nil
}

func googleOAuthAudienceChanged(original *Connection, updated Connection) (bool, error) {
	if original == nil {
		return false, nil
	}
	if original.Provider != ProviderGoogleDrive && updated.Provider != ProviderGoogleDrive {
		return false, nil
	}
	if original.Provider != updated.Provider {
		return true, nil
	}
	factory := GoogleDriveFactory{}
	before, err := factory.settings(*original)
	if err != nil {
		return false, err
	}
	after, err := factory.settings(updated)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(before.ClientID) != strings.TrimSpace(after.ClientID), nil
}

func credentialScopeChanged(original *Connection, updated Connection) (bool, error) {
	if changed, err := googleOAuthAudienceChanged(original, updated); err != nil || changed {
		return changed, err
	}
	if original == nil {
		return false, nil
	}
	before, beforeRequired, err := credentialScope(*original)
	if err != nil {
		return false, err
	}
	after, afterRequired, err := credentialScope(updated)
	if err != nil {
		return false, err
	}
	return beforeRequired != afterRequired || before != after, nil
}

func credentialScopeNeedsStoredSecret(c Connection) bool {
	switch c.Provider {
	case ProviderWebDAV:
		var settings WebDAVSettings
		if jsonUnmarshalSettings(c.Settings, &settings, "WebDAV") != nil {
			return false
		}
		return !strings.EqualFold(strings.TrimSpace(settings.Auth), "anonymous")
	case ProviderS3:
		var settings S3Settings
		if jsonUnmarshalSettings(c.Settings, &settings, "S3") != nil {
			return false
		}
		return strings.EqualFold(strings.TrimSpace(settings.Auth), "static")
	default:
		return false
	}
}
