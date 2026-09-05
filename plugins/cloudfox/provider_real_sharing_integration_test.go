package cloudfox

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
	"google.golang.org/api/googleapi"
)

const (
	realSharingEnv          = "F4_CLOUDFOX_REAL_SHARING"
	realSharingConfirmed    = "CONFIRMED"
	realSharingFolderMarker = "sharing-"
)

type realSharingTarget struct {
	name            string
	location        string
	isFile          bool
	revokeAttempted bool
}

type realSharingGate struct {
	getenv func(string) string
}

func (gate realSharingGate) confirmed() bool {
	if gate.getenv == nil || gate.getenv(realMutationEnv) != realMutationConfirmed {
		return false
	}
	return gate.getenv(realSharingEnv) == realSharingConfirmed
}

// TestRealSavedCloudSharing exercises public-link sharing through saved Google
// Drive and Yandex.Disk connections. It is deliberately doubly gated: both
// F4_CLOUDFOX_REAL_MUTATION and F4_CLOUDFOX_REAL_SHARING must have the exact
// value CONFIRMED before this test reads configuration, unlocks credentials,
// performs network I/O, or mutates either account.
//
// Each provider gets one UUID-qualified top-level folder containing one small
// file and one child folder. Both children go through private info, Viewer link
// creation, anonymous URL access, revocation, and anonymous-denial checks.
// Google additionally attempts the advertised Commenter and Editor transitions
// on the file, but never performs an anonymous write. No mutation is retried;
// an unknown mutation result is reconciled with a fresh read only. Polling is
// limited to anonymous GETs and Yandex read-only metadata helpers.
//
// Share URLs and provider error text are never written to the test log. Cleanup
// first revokes any freshly observed active links once, then uses the existing
// exact name+canonical-identity workspace cleanup and proves the generated
// folder has left the writable root.
func TestRealSavedCloudSharing(t *testing.T) {
	if !(realSharingGate{getenv: os.Getenv}).confirmed() {
		t.Skip("real CloudFox sharing requires both explicit confirmations")
	}

	// This is intentionally the first config access in the test, after both
	// opt-in gates above have succeeded.
	configDir := strings.TrimSpace(os.Getenv(realConfigDirEnv))
	if configDir == "" || !filepath.IsAbs(configDir) {
		t.Fatal("real CloudFox sharing requires an absolute config directory")
	}
	if info, err := os.Stat(configDir); err != nil || !info.IsDir() { // #nosec G703 -- the doubly opted-in sharing test intentionally loads the operator-supplied absolute config directory.
		t.Fatal("real CloudFox sharing config directory is unavailable")
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
			t.Errorf("close real sharing CloudFox plugin: %s", realSharingErrorClass(err))
		}
	})

	loadCtx, cancelLoad := context.WithTimeout(context.Background(), 30*time.Second)
	connections, err := plugin.Repository().List(loadCtx)
	cancelLoad()
	if err != nil {
		t.Fatalf("load saved sharing connections: %s", realSharingErrorClass(err))
	}

	targets := []struct {
		name        string
		provider    ProviderType
		selectorEnv string
	}{
		{name: "google-drive", provider: ProviderGoogleDrive, selectorEnv: realGoogleSelectorEnv},
		{name: "yandex-disk", provider: ProviderYandexDisk, selectorEnv: realYandexSelectorEnv},
	}
	for _, target := range targets {
		target := target
		t.Run(target.name, func(t *testing.T) {
			connection := selectRealConnection(t, connections, target.provider, target.selectorEnv)
			factory, ok := plugin.Factory(target.provider)
			if !ok {
				t.Fatal("real sharing provider factory is unavailable")
			}
			if target.provider == ProviderYandexDisk {
				factory = realYandexFactoryWithDialRetries(t, factory)
			}

			openCtx, cancelOpen := context.WithTimeout(context.Background(), 2*time.Minute)
			secrets, err := plugin.Repository().Credentials(openCtx, connection)
			if err != nil {
				cancelOpen()
				t.Fatalf("unlock saved sharing credentials: %s", realSharingErrorClass(err))
			}
			backend, err := factory.Open(openCtx, connection.Clone(), secrets.Clone())
			clearSecrets(secrets)
			cancelOpen()
			if err != nil {
				t.Fatalf("open saved sharing connection: %s", realSharingErrorClass(err))
			}
			t.Cleanup(func() {
				if err := backend.Close(); err != nil {
					t.Errorf("close real sharing backend: %s", realSharingErrorClass(err))
				}
			})

			runRealSavedSharing(t, backend, target.provider)
		})
	}
}

func runRealSavedSharing(t *testing.T, backend Backend, provider ProviderType) {
	t.Helper()
	linker, ok := backend.(BackendShareLinker)
	if !ok {
		t.Fatal("real provider does not expose share-link support")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	root := backend.Root()
	writeRoot := realWritableRoot(t, ctx, backend, root, readRealDirectory(t, ctx, backend, root))
	uuid, err := newUUID()
	if err != nil {
		t.Fatal("generate real sharing workspace identity")
	}
	folderName := realFolderPrefix + realSharingFolderMarker + string(provider) + "-" + strings.ReplaceAll(uuid, "-", "")
	if !strings.HasPrefix(folderName, realFolderPrefix+realSharingFolderMarker) || strings.ContainsAny(folderName, "/\\") {
		t.Fatal("generated an unsafe real sharing workspace name")
	}
	folderCandidate := backend.Join(writeRoot, folderName)
	if folderCandidate == "" || folderCandidate == writeRoot || backend.IsRoot(folderCandidate) {
		t.Fatal("provider produced an unsafe real sharing workspace location")
	}

	workspace := ""
	creationTried := false
	// Register exact cleanup before the first mutation. If MkDir returns an
	// unknown state, cleanup can still discover only this UUID-qualified name.
	t.Cleanup(func() {
		if creationTried {
			cleanupRealWorkspace(t, backend, writeRoot, workspace, folderName)
		}
	})
	creationTried = true
	if err := backend.MkDir(ctx, folderCandidate); err != nil {
		t.Fatalf("create real sharing workspace: %s", realSharingErrorClass(err))
	}
	workspaceEntry, err := statRealReadOnly(ctx, backend, folderCandidate)
	if err != nil || !workspaceEntry.IsDir || workspaceEntry.Name != folderName || workspaceEntry.Location == "" {
		t.Fatalf("resolve real sharing workspace: %s", realSharingErrorClass(err))
	}
	workspace = workspaceEntry.Location
	assertRealWorkspaceTarget(t, ctx, backend, writeRoot, workspace, folderName, workspaceEntry)

	fileName := "public-link-file.txt"
	filePayload := []byte("CloudFox anonymous sharing integration marker\n")
	fileCandidate := backend.Join(workspace, fileName)
	writeRealBytes(t, ctx, backend, fileCandidate, filePayload)
	fileEntry := statRealFile(t, ctx, backend, fileCandidate, int64(len(filePayload)))

	childFolderName := "public-link-folder"
	childFolderCandidate := backend.Join(workspace, childFolderName)
	if err := backend.MkDir(ctx, childFolderCandidate); err != nil {
		t.Fatalf("create real sharing child folder: %s", realSharingErrorClass(err))
	}
	childFolderEntry, err := statRealReadOnly(ctx, backend, childFolderCandidate)
	if err != nil || !childFolderEntry.IsDir || childFolderEntry.Location == "" || childFolderEntry.Name != childFolderName {
		t.Fatalf("resolve real sharing child folder: %s", realSharingErrorClass(err))
	}

	shareTargets := []realSharingTarget{
		{name: "file", location: fileEntry.Location, isFile: true},
		{name: "folder", location: childFolderEntry.Location},
	}
	// This cleanup runs before workspace deletion (LIFO). Each active link is
	// freshly observed and revoked at most once; it is not a mutation retry.
	t.Cleanup(func() {
		cleanupRealSharingLinks(t, backend, linker, shareTargets)
	})

	for index := range shareTargets {
		target := &shareTargets[index]
		t.Run(target.name, func(t *testing.T) {
			exerciseRealShareTarget(t, ctx, backend, linker, provider, target)
		})
	}
}

func exerciseRealShareTarget(t *testing.T, ctx context.Context, backend Backend, linker BackendShareLinker, provider ProviderType, target *realSharingTarget) {
	t.Helper()
	location := target.location
	info := mustRealShareInfo(t, ctx, backend, linker, location)
	if info.Link != nil || info.CanRevoke || info.LinksUnenumerable || info.LinkInherited {
		t.Fatal("new isolated sharing target unexpectedly has public-link state")
	}
	if !info.CanCreate || !realShareRoleAvailable(info.Roles, vfs.ShareRoleViewer) || !realShareExpirationAvailable(info.ExpirationOptions, 0) {
		t.Fatal("provider does not expose a persistent Viewer link for the isolated target")
	}

	t.Log("phase: create Viewer link and verify anonymous GET access")
	link, err := createRealShareOnce(ctx, backend, linker, location, vfs.ShareRoleViewer)
	if err != nil {
		t.Fatalf("create Viewer share link: %s", realSharingErrorClass(err))
	}
	assertRealShareLink(t, link, vfs.ShareRoleViewer)
	active := mustRealShareInfo(t, ctx, backend, linker, location)
	if active.Link == nil || active.Link.Role != vfs.ShareRoleViewer || !active.CanRevoke || !active.Link.Revocable || active.Link.URL != link.URL {
		t.Fatal("provider did not report the newly created Viewer link consistently")
	}
	awaitRealAnonymousShareState(t, ctx, link.URL, true)

	if provider == ProviderGoogleDrive && target.isFile {
		link = exerciseRealGoogleShareRoleTransitions(t, ctx, backend, linker, location, link)
	}

	t.Log("phase: revoke link once and verify anonymous GET access stops")
	target.revokeAttempted = true
	if err := revokeRealShareOnce(ctx, backend, linker, location); err != nil {
		t.Fatalf("revoke share link: %s", realSharingErrorClass(err))
	}
	revoked := mustRealShareInfo(t, ctx, backend, linker, location)
	if revoked.Link != nil || revoked.CanRevoke {
		t.Fatal("provider still reports an active link after revocation")
	}
	awaitRealAnonymousShareState(t, ctx, link.URL, false)
}

func exerciseRealGoogleShareRoleTransitions(t *testing.T, ctx context.Context, backend Backend, linker BackendShareLinker, location string, current vfs.ShareLink) vfs.ShareLink {
	t.Helper()
	currentRole := vfs.ShareRoleViewer
	transitioned := false
	for _, role := range []vfs.ShareRole{vfs.ShareRoleCommenter, vfs.ShareRoleEditor} {
		info := mustRealShareInfo(t, ctx, backend, linker, location)
		if !realShareRoleAvailable(info.Roles, role) {
			t.Logf("phase: Google %s transition is not exposed for this item", realSharingRoleName(role))
			continue
		}
		t.Logf("phase: transition Google link to %s without anonymous writes", realSharingRoleName(role))
		link, err := createRealShareOnce(ctx, backend, linker, location, role)
		if err != nil {
			if realGoogleOptionalShareRoleUnavailable(err) {
				t.Logf("Google account policy did not permit the optional %s transition", realSharingRoleName(role))
				continue
			}
			t.Fatalf("transition Google share role to %s: %s", realSharingRoleName(role), realSharingErrorClass(err))
		}
		assertRealShareLink(t, link, role)
		verified := mustRealShareInfo(t, ctx, backend, linker, location)
		if verified.Link == nil || verified.Link.Role != role || verified.Link.URL != link.URL {
			t.Fatalf("Google did not report the requested %s role", realSharingRoleName(role))
		}
		current = link
		currentRole = role
		transitioned = true
	}
	if transitioned && currentRole != vfs.ShareRoleViewer {
		t.Log("phase: restore Google link to Viewer before revocation")
		viewer, err := createRealShareOnce(ctx, backend, linker, location, vfs.ShareRoleViewer)
		if err != nil {
			t.Fatalf("restore Google Viewer role: %s", realSharingErrorClass(err))
		}
		assertRealShareLink(t, viewer, vfs.ShareRoleViewer)
		verified := mustRealShareInfo(t, ctx, backend, linker, location)
		if verified.Link == nil || verified.Link.Role != vfs.ShareRoleViewer || verified.Link.URL != viewer.URL {
			t.Fatal("Google did not restore the Viewer role")
		}
		current = viewer
	}
	return current
}

// createRealShareOnce performs exactly one provider mutation. An unknown result
// is reconciled only by ShareLinkInfo; the mutation itself is never retried.
func createRealShareOnce(ctx context.Context, backend Backend, linker BackendShareLinker, location string, role vfs.ShareRole) (vfs.ShareLink, error) {
	request := vfs.ShareLinkRequest{Role: role}
	issuedAfter := time.Now()
	link, err := linker.CreateShareLink(ctx, location, request)
	if err == nil {
		if validationErr := vfs.ValidateCreatedShareLink(link, request, issuedAfter, time.Now()); validationErr != nil {
			return vfs.ShareLink{}, &vfs.UnknownOperationStateError{Operation: "real sharing create validation", Err: validationErr}
		}
		return link, nil
	}
	if !errors.Is(err, vfs.ErrOperationStateUnknown) {
		return vfs.ShareLink{}, err
	}
	info, reconcileErr := realShareInfoReadOnly(ctx, backend, linker, location)
	if reconcileErr != nil {
		return vfs.ShareLink{}, &vfs.UnknownOperationStateError{Operation: "real sharing create reconciliation", Err: errors.New("read-only reconciliation failed")}
	}
	if info.Link == nil || info.Link.Role != role {
		return vfs.ShareLink{}, err
	}
	if validationErr := vfs.ValidateCreatedShareLink(*info.Link, request, issuedAfter, time.Now()); validationErr != nil {
		return vfs.ShareLink{}, &vfs.UnknownOperationStateError{Operation: "real sharing create reconciliation", Err: validationErr}
	}
	return *info.Link, nil
}

// revokeRealShareOnce performs exactly one revoke. As with creation, an
// unknown result is resolved by a read and never by replaying DELETE/PUT.
func revokeRealShareOnce(ctx context.Context, backend Backend, linker BackendShareLinker, location string) error {
	err := linker.RevokeShareLink(ctx, location)
	if err == nil || !errors.Is(err, vfs.ErrOperationStateUnknown) {
		return err
	}
	info, reconcileErr := realShareInfoReadOnly(ctx, backend, linker, location)
	if reconcileErr != nil {
		return &vfs.UnknownOperationStateError{Operation: "real sharing revoke reconciliation", Err: errors.New("read-only reconciliation failed")}
	}
	if info.Link == nil {
		return nil
	}
	return err
}

func mustRealShareInfo(t *testing.T, ctx context.Context, backend Backend, linker BackendShareLinker, location string) vfs.ShareLinkInfo {
	t.Helper()
	info, err := realShareInfoReadOnly(ctx, backend, linker, location)
	if err != nil {
		t.Fatalf("read share-link info: %s", realSharingErrorClass(err))
	}
	if info.Link != nil {
		if err := vfs.ValidateShareURL(info.Link.URL); err != nil {
			t.Fatal("provider returned an invalid share URL")
		}
	}
	return info
}

func realShareInfoReadOnly(ctx context.Context, backend Backend, linker BackendShareLinker, location string) (vfs.ShareLinkInfo, error) {
	var info vfs.ShareLinkInfo
	err := retryRealYandexRead(ctx, backend, func() error {
		var err error
		info, err = linker.ShareLinkInfo(ctx, location)
		return err
	})
	return info, err
}

func assertRealShareLink(t *testing.T, link vfs.ShareLink, role vfs.ShareRole) {
	t.Helper()
	if err := vfs.ValidateShareURL(link.URL); err != nil {
		t.Fatal("provider returned an invalid share URL")
	}
	if link.Role != role || !link.ExpiresAt.IsZero() || !link.Revocable {
		t.Fatal("provider returned unexpected share-link capabilities")
	}
}

func cleanupRealSharingLinks(t *testing.T, backend Backend, linker BackendShareLinker, targets []realSharingTarget) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	for _, target := range targets {
		if target.location == "" {
			continue
		}
		info, err := realShareInfoReadOnly(ctx, backend, linker, target.location)
		if err != nil {
			t.Errorf("inspect %s link during exact cleanup: %s", target.name, realSharingErrorClass(err))
			continue
		}
		if info.Link == nil {
			continue
		}
		// Never replay a revoke whose result was uncertain. The exact parent
		// workspace deletion below is the cleanup for that case.
		if target.revokeAttempted {
			continue
		}
		if !info.CanRevoke || !info.Link.Revocable {
			t.Errorf("cannot revoke active %s link during exact cleanup", target.name)
			continue
		}
		// One cleanup mutation only. Never replay it on an error.
		if err := linker.RevokeShareLink(ctx, target.location); err != nil {
			t.Errorf("revoke active %s link during exact cleanup: %s", target.name, realSharingErrorClass(err))
		}
	}
}

func realShareRoleAvailable(roles []vfs.ShareRole, wanted vfs.ShareRole) bool {
	for _, role := range roles {
		if role == wanted {
			return true
		}
	}
	return false
}

func realShareExpirationAvailable(options []time.Duration, wanted time.Duration) bool {
	for _, option := range options {
		if option == wanted {
			return true
		}
	}
	return false
}

func realGoogleOptionalShareRoleUnavailable(err error) bool {
	if err == nil || errors.Is(err, vfs.ErrOperationStateUnknown) {
		return false
	}
	if errors.Is(err, os.ErrPermission) || errors.Is(err, os.ErrInvalid) {
		return true
	}
	var apiErr *googleapi.Error
	return errors.As(err, &apiErr) && (apiErr.Code == http.StatusBadRequest || apiErr.Code == http.StatusForbidden)
}

func realSharingRoleName(role vfs.ShareRole) string {
	switch role {
	case vfs.ShareRoleViewer:
		return "Viewer"
	case vfs.ShareRoleCommenter:
		return "Commenter"
	case vfs.ShareRoleEditor:
		return "Editor"
	default:
		return "unsupported"
	}
}

// realSharingErrorClass intentionally never calls err.Error. Share URLs are
// bearer credentials and must not enter go test output even when an HTTP or
// SDK error embeds the complete request URL.
func realSharingErrorClass(err error) string {
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
		{name: "unknown-state", err: vfs.ErrOperationStateUnknown},
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

var errRealAnonymousShareProbe = errors.New("anonymous share probe failed")

func newRealAnonymousShareClient(transport http.RoundTripper) *http.Client {
	if transport == nil {
		transport = http.DefaultTransport
	}
	return &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
		CheckRedirect: func(next *http.Request, via []*http.Request) error {
			if len(via) >= 10 || !safeRealAnonymousProbeURL(next.URL) {
				return errRealAnonymousShareProbe
			}
			// Do not forward or synthesize credentials and do not disclose a
			// bearer-like source URL through the Referer header.
			for _, header := range []string{"Authorization", "Cookie", "Proxy-Authorization", "Referer"} {
				next.Header.Del(header)
			}
			return nil
		},
	}
}

func safeRealAnonymousProbeURL(target *url.URL) bool {
	if target == nil || target.User != nil || target.Host == "" {
		return false
	}
	if strings.EqualFold(target.Scheme, "https") {
		return true
	}
	if !strings.EqualFold(target.Scheme, "http") {
		return false
	}
	host := target.Hostname()
	return strings.EqualFold(host, "localhost") || net.ParseIP(host) != nil && net.ParseIP(host).IsLoopback()
}

func probeRealAnonymousShare(ctx context.Context, client *http.Client, rawURL string) (bool, error) {
	if err := vfs.ValidateShareURL(rawURL); err != nil {
		return false, errRealAnonymousShareProbe
	}
	target, err := url.Parse(rawURL)
	if err != nil || !safeRealAnonymousProbeURL(target) {
		return false, errRealAnonymousShareProbe
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return false, errRealAnonymousShareProbe
	}
	request.Header.Set("Accept", "text/html,application/xhtml+xml,application/octet-stream;q=0.8,*/*;q=0.5")
	request.Header.Set("Cache-Control", "no-cache")
	request.Header.Set("Pragma", "no-cache")
	request.Header.Set("User-Agent", "f4-cloudfox-real-sharing-test")
	response, err := client.Do(request)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return false, ctxErr
		}
		return false, errRealAnonymousShareProbe
	}
	defer func() {
		_ = response.Body.Close() // response body cleanup only
	}()
	body, err := io.ReadAll(io.LimitReader(response.Body, 512<<10))
	if err != nil {
		return false, errRealAnonymousShareProbe
	}
	return classifyRealAnonymousShareResponse(response, body)
}

func classifyRealAnonymousShareResponse(response *http.Response, body []byte) (bool, error) {
	if response == nil {
		return false, errRealAnonymousShareProbe
	}
	if response.Request != nil && response.Request.URL != nil {
		host := strings.ToLower(response.Request.URL.Hostname())
		if host == "accounts.google.com" || host == "passport.yandex.com" || host == "passport.yandex.ru" || strings.HasPrefix(host, "passport.yandex.") {
			return false, nil
		}
	}
	switch response.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusGone:
		return false, nil
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return false, fmt.Errorf("%w: HTTP %d", errRealAnonymousShareProbe, response.StatusCode)
	}
	lower := strings.ToLower(string(body))
	for _, denied := range []string{
		"you need access", "request access", "access denied", "you don't have access",
		"file you have requested does not exist", "access to this file is restricted",
		"\u043d\u0435\u0442 \u0434\u043e\u0441\u0442\u0443\u043f\u0430",
		"\u0434\u043e\u0441\u0442\u0443\u043f \u043a \u0444\u0430\u0439\u043b\u0443 \u0437\u0430\u043a\u0440\u044b\u0442",
		"\u0444\u0430\u0439\u043b \u043d\u0435 \u043d\u0430\u0439\u0434\u0435\u043d",
		"\u043d\u0438\u0447\u0435\u0433\u043e \u043d\u0435 \u043d\u0430\u0439\u0434\u0435\u043d\u043e",
	} {
		if strings.Contains(lower, denied) {
			return false, nil
		}
	}
	return true, nil
}

func awaitRealAnonymousShareState(t *testing.T, parent context.Context, rawURL string, wanted bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(parent, 90*time.Second)
	defer cancel()
	client := newRealAnonymousShareClient(nil)
	var lastClass string
	for {
		accessible, err := probeRealAnonymousShare(ctx, client, rawURL)
		if err == nil && accessible == wanted {
			return
		}
		if err != nil {
			lastClass = realSharingErrorClass(err)
		} else {
			lastClass = "state-mismatch"
		}
		timer := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			t.Fatalf("anonymous share state did not converge to accessible=%t (last=%s)", wanted, lastClass)
		case <-timer.C:
		}
	}
}

func TestRealSharingHarnessIsInertByDefault(t *testing.T) {
	t.Parallel()
	var requested []string
	gate := realSharingGate{getenv: func(name string) string {
		requested = append(requested, name)
		if name == realConfigDirEnv || name == realVaultPasswordEnv {
			t.Fatalf("default-disabled sharing gate accessed %s", name)
		}
		return ""
	}}
	if gate.confirmed() {
		t.Fatal("real sharing harness was enabled without confirmations")
	}
	if len(requested) != 1 || requested[0] != realMutationEnv {
		t.Fatalf("default gate requests = %v, want only the primary mutation gate", requested)
	}
}

func TestRealSharingHarnessRequiresExactDualConfirmation(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name     string
		mutation string
		sharing  string
		want     bool
	}{
		{name: "unset"},
		{name: "mutation only", mutation: "CONFIRMED"},
		{name: "sharing only", sharing: "CONFIRMED"},
		{name: "wrong case", mutation: "confirmed", sharing: "CONFIRMED"},
		{name: "extra whitespace", mutation: "CONFIRMED", sharing: " CONFIRMED"},
		{name: "both exact", mutation: "CONFIRMED", sharing: "CONFIRMED", want: true},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			values := map[string]string{realMutationEnv: test.mutation, realSharingEnv: test.sharing}
			if got := (realSharingGate{getenv: func(name string) string { return values[name] }}).confirmed(); got != test.want {
				t.Fatalf("confirmed = %t, want %t", got, test.want)
			}
		})
	}
}

func TestRealAnonymousShareProbeIsCredentiallessAndDoesNotExposeURLsInErrors(t *testing.T) {
	t.Parallel()
	var finalHeaders http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/start":
			http.Redirect(w, r, "/open", http.StatusFound)
		case "/open":
			finalHeaders = r.Header.Clone()
			_, _ = io.WriteString(w, "public content")
		case "/closed":
			http.NotFound(w, r)
		default:
			http.Error(w, "unexpected request", http.StatusBadRequest)
		}
	}))
	defer server.Close()
	client := newRealAnonymousShareClient(server.Client().Transport)

	accessible, err := probeRealAnonymousShare(context.Background(), client, server.URL+"/start?public_url=sensitive")
	if err != nil || !accessible {
		t.Fatalf("open probe = accessible:%t error:%s", accessible, realSharingErrorClass(err))
	}
	for _, header := range []string{"Authorization", "Cookie", "Proxy-Authorization", "Referer"} {
		if finalHeaders.Get(header) != "" {
			t.Fatalf("anonymous redirect forwarded %s", header)
		}
	}
	accessible, err = probeRealAnonymousShare(context.Background(), client, server.URL+"/closed?token=sensitive")
	if err != nil || accessible {
		t.Fatalf("closed probe = accessible:%t error:%s", accessible, realSharingErrorClass(err))
	}

	secretURL := "https://share.invalid/file?token=must-not-leak" // #nosec G101 -- a synthetic secret-bearing URL verifies error redaction.
	class := realSharingErrorClass(fmt.Errorf("request %s: %w", secretURL, errRealAnonymousShareProbe))
	if strings.Contains(class, "share.invalid") || strings.Contains(class, "must-not-leak") || strings.Contains(class, "https://") {
		t.Fatalf("error class leaked a share URL: %q", class)
	}
}

type realSharingMutationHarnessBackend struct {
	*fakeBackend
	info        vfs.ShareLinkInfo
	createErr   error
	revokeErr   error
	infoCalls   int
	createCalls int
	revokeCalls int
}

func (backend *realSharingMutationHarnessBackend) ShareLinkInfo(context.Context, string) (vfs.ShareLinkInfo, error) {
	backend.infoCalls++
	return backend.info, nil
}

func (backend *realSharingMutationHarnessBackend) CreateShareLink(context.Context, string, vfs.ShareLinkRequest) (vfs.ShareLink, error) {
	backend.createCalls++
	return vfs.ShareLink{}, backend.createErr
}

func (backend *realSharingMutationHarnessBackend) RevokeShareLink(context.Context, string) error {
	backend.revokeCalls++
	return backend.revokeErr
}

func TestRealSharingHarnessReconcilesUnknownStateWithoutRetryingMutations(t *testing.T) {
	t.Parallel()
	unknown := &vfs.UnknownOperationStateError{Operation: "test mutation", Err: errors.New("connection lost")}
	link := &vfs.ShareLink{URL: "https://share.invalid/opaque", Role: vfs.ShareRoleViewer, Revocable: true}
	backend := &realSharingMutationHarnessBackend{
		fakeBackend: &fakeBackend{},
		info:        vfs.ShareLinkInfo{Link: link},
		createErr:   unknown,
		revokeErr:   unknown,
	}

	created, err := createRealShareOnce(context.Background(), backend, backend, "/root/item", vfs.ShareRoleViewer)
	if err != nil || created.URL != link.URL {
		t.Fatalf("reconciled create returned class=%s", realSharingErrorClass(err))
	}
	if backend.createCalls != 1 || backend.infoCalls != 1 {
		t.Fatalf("create harness calls: mutation=%d info=%d, want 1/1", backend.createCalls, backend.infoCalls)
	}

	backend.info = vfs.ShareLinkInfo{}
	if err := revokeRealShareOnce(context.Background(), backend, backend, "/root/item"); err != nil {
		t.Fatalf("reconciled revoke returned class=%s", realSharingErrorClass(err))
	}
	if backend.revokeCalls != 1 || backend.infoCalls != 2 {
		t.Fatalf("revoke harness calls: mutation=%d total-info=%d, want 1/2", backend.revokeCalls, backend.infoCalls)
	}

	backend.info = vfs.ShareLinkInfo{Link: link, CanRevoke: true}
	backend.revokeCalls = 0
	cleanupRealSharingLinks(t, backend, backend, []realSharingTarget{{
		name: "item", location: "/root/item", revokeAttempted: true,
	}})
	if backend.revokeCalls != 0 {
		t.Fatalf("cleanup replayed an already-attempted revoke %d time(s)", backend.revokeCalls)
	}
}

var _ BackendShareLinker = (*realSharingMutationHarnessBackend)(nil)
