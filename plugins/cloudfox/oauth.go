package cloudfox

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

const defaultYandexOAuthBase = "https://oauth.yandex.ru"
const defaultYandexRedirectURL = "https://oauth.yandex.ru/verification_code"

var errOAuthDenied = errors.New("cloudfox: authorization was denied")

// browserURLCommandRunner is the native desktop boundary. Tests replace only
// this last step, keeping URL validation and OAuth behavior under test without
// ever launching the user's browser.
var browserURLCommandRunner = openBrowserURLWithOS

func noRedirectHTTPClient(client *http.Client) *http.Client {
	if client == nil {
		client = http.DefaultClient
	}
	clone := *client
	// OAuth POST bodies contain authorization codes, device codes or client
	// credentials. Never let net/http replay them to a redirect target.
	clone.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &clone
}

func noRedirectOAuthContext(ctx context.Context) context.Context {
	client, _ := ctx.Value(oauth2.HTTPClient).(*http.Client)
	return context.WithValue(ctx, oauth2.HTTPClient, noRedirectHTTPClient(client))
}

func randomOAuthState() (string, error) {
	data := make([]byte, 32)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

// AuthorizeGoogleDesktop performs Google's installed-application loopback
// flow with PKCE. openURL must launch the system browser; passing it in keeps
// the OAuth exchange testable and avoids embedding a browser in the terminal.
func AuthorizeGoogleDesktop(ctx context.Context, connection Connection, secrets SecretValues, openURL func(string) error) (SecretValues, error) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("cloudfox: start Google OAuth callback: %w", err)
	}
	defer func() { _ = listener.Close() }() // Listener teardown is best effort.

	state, err := randomOAuthState()
	if err != nil {
		return nil, err
	}
	verifier := oauth2.GenerateVerifier()
	redirectURL := "http://" + listener.Addr().String() + "/oauth2/callback"
	authorizationURL, err := GoogleAuthorizationURL(connection, secrets, redirectURL, state, verifier)
	if err != nil {
		return nil, err
	}

	type callbackResult struct {
		code string
		err  error
	}
	result := make(chan callbackResult, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth2/callback", func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
		if request.URL.Query().Get("state") != state {
			http.Error(writer, "CloudFox rejected an invalid OAuth state.", http.StatusBadRequest)
			select {
			case result <- callbackResult{err: errors.New("cloudfox: Google OAuth state mismatch")}:
			default:
			}
			return
		}
		if oauthErr := request.URL.Query().Get("error"); oauthErr != "" {
			http.Error(writer, "Authorization was not completed. You may close this page.", http.StatusBadRequest)
			select {
			case result <- callbackResult{err: fmt.Errorf("%w: %s", errOAuthDenied, oauthErr)}:
			default:
			}
			return
		}
		code := request.URL.Query().Get("code")
		if code == "" {
			http.Error(writer, "The authorization response contained no code.", http.StatusBadRequest)
			select {
			case result <- callbackResult{err: errors.New("cloudfox: Google OAuth callback contained no code")}:
			default:
			}
			return
		}
		_, _ = io.WriteString(writer, "CloudFox is authorized. You may close this page and return to f4.\n")
		select {
		case result <- callbackResult{code: code}:
		default:
		}
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	serveErr := make(chan error, 1)
	go func() {
		err := server.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		serveErr <- err
	}()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	if openURL == nil {
		openURL = openBrowserURL
	}
	if err := openURL(authorizationURL); err != nil {
		return nil, fmt.Errorf("cloudfox: open Google authorization page: %w", err)
	}

	var code string
	select {
	case callback := <-result:
		if callback.err != nil {
			return nil, callback.err
		}
		code = callback.code
	case err := <-serveErr:
		if err == nil {
			err = errors.New("callback server stopped")
		}
		return nil, fmt.Errorf("cloudfox: Google OAuth callback: %w", err)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	token, err := ExchangeGoogleAuthorizationCode(ctx, connection, secrets, redirectURL, code, verifier)
	if err != nil {
		return nil, err
	}
	updated := secrets.Clone()
	if updated == nil {
		updated = SecretValues{}
	}
	updated["access_token"] = token.AccessToken
	if token.RefreshToken != "" {
		updated["refresh_token"] = token.RefreshToken
	}
	if !token.Expiry.IsZero() {
		updated["expires_at"] = token.Expiry.UTC().Format(time.RFC3339Nano)
	}
	return updated, nil
}

type yandexOAuthResponse struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	TokenType        string `json:"token_type"`
	ExpiresIn        int    `json:"expires_in"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// YandexAuthorizationURL builds the browser URL for Yandex's public-client
// authorization-code flow. PKCE replaces a client secret, which a desktop
// application cannot keep confidential.
func YandexAuthorizationURL(oauthBase, clientID, redirectURL, verifier string) (string, error) {
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		return "", errors.New("cloudfox: Yandex OAuth client ID is required")
	}
	if verifier == "" {
		return "", errors.New("cloudfox: Yandex OAuth PKCE verifier is required")
	}
	if oauthBase == "" {
		oauthBase = defaultYandexOAuthBase
	}
	if redirectURL == "" {
		redirectURL = defaultYandexRedirectURL
	}
	endpoint, err := url.Parse(strings.TrimRight(oauthBase, "/") + "/authorize")
	if err != nil {
		return "", fmt.Errorf("cloudfox: parse Yandex OAuth endpoint: %w", err)
	}
	if endpoint.Scheme != "https" || endpoint.Host == "" {
		return "", errors.New("cloudfox: invalid Yandex OAuth endpoint")
	}
	values := endpoint.Query()
	values.Set("response_type", "code")
	values.Set("client_id", clientID)
	values.Set("redirect_uri", redirectURL)
	values.Set("code_challenge", oauth2.S256ChallengeFromVerifier(verifier))
	values.Set("code_challenge_method", "S256")
	values.Set("force_confirm", "yes")
	endpoint.RawQuery = values.Encode()
	return endpoint.String(), nil
}

// ExchangeYandexAuthorizationCode exchanges the code displayed by Yandex for
// tokens. The PKCE verifier authenticates the authorization attempt, so no
// client secret is sent or stored.
func ExchangeYandexAuthorizationCode(ctx context.Context, client *http.Client, oauthBase, clientID, code, verifier string) (SecretValues, error) {
	clientID = strings.TrimSpace(clientID)
	code = strings.TrimSpace(code)
	if clientID == "" {
		return nil, errors.New("cloudfox: Yandex OAuth client ID is required")
	}
	if code == "" {
		return nil, errors.New("cloudfox: Yandex authorization code is required")
	}
	if verifier == "" {
		return nil, errors.New("cloudfox: Yandex OAuth PKCE verifier is required")
	}
	if client == nil {
		client = http.DefaultClient
	}
	client = noRedirectHTTPClient(client)
	if oauthBase == "" {
		oauthBase = defaultYandexOAuthBase
	}
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"client_id":     {clientID},
		"code_verifier": {verifier},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(oauthBase, "/")+"/token", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }() // Response-body cleanup is best effort.
	data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	var oauthResponse yandexOAuthResponse
	if err := json.Unmarshal(data, &oauthResponse); err != nil {
		return nil, fmt.Errorf("cloudfox: decode Yandex token response: %w", err)
	}
	if response.StatusCode/100 != 2 || oauthResponse.AccessToken == "" {
		message := oauthResponse.ErrorDescription
		if message == "" {
			message = oauthResponse.Error
		}
		if message == "" {
			message = strings.TrimSpace(string(data))
		}
		return nil, mapProviderHTTPError(response, message)
	}
	values := SecretValues{"oauth_token": oauthResponse.AccessToken}
	if oauthResponse.RefreshToken != "" {
		values["refresh_token"] = oauthResponse.RefreshToken
	}
	if oauthResponse.ExpiresIn > 0 {
		values["expires_at"] = time.Now().Add(time.Duration(oauthResponse.ExpiresIn) * time.Second).UTC().Format(time.RFC3339Nano)
	}
	return values, nil
}

func openBrowserURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return errors.New("cloudfox: refusing to open an invalid authorization URL")
	}
	return browserURLCommandRunner(rawURL)
}

func openBrowserURLWithOS(rawURL string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", rawURL)
	case "darwin":
		command = exec.Command("open", rawURL)
	default:
		command = exec.Command("xdg-open", rawURL)
	}
	if err := command.Start(); err != nil {
		return err
	}
	go func() { _ = command.Wait() }()
	return nil
}
