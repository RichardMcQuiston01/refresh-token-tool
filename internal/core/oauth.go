package core

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ChromeWebStoreScope is the OAuth scope required by the Chrome Web Store API.
const ChromeWebStoreScope = "https://www.googleapis.com/auth/chromewebstore"

// Endpoints are variables so tests can point them at a local fake server.
var (
	AuthEndpoint  = "https://accounts.google.com/o/oauth2/v2/auth"
	TokenEndpoint = "https://oauth2.googleapis.com/token"
)

// OAuthRequest carries the credentials of a Google "Desktop app" OAuth client.
type OAuthRequest struct {
	ClientID     string
	ClientSecret string
	Scope        string
}

// BrowserOpener opens authURL in the user's browser.
type BrowserOpener func(authURL string) error

type callbackResult struct {
	code      string
	authError string
}

const successPage = `<html><body style="font-family:sans-serif"><h3>Authorized</h3>` +
	`<p>You can close this tab and return to the app.</p></body></html>`

// RunOAuth performs the OAuth 2.0 loopback flow (with PKCE and a state check)
// and returns the refresh token. It blocks until the browser redirects back,
// ctx is cancelled, or ctx's deadline passes.
func RunOAuth(ctx context.Context, request OAuthRequest, openBrowser BrowserOpener) (string, error) {
	if request.ClientID == "" || request.ClientSecret == "" {
		return "", errors.New("both a client ID and a client secret are required to start authorization")
	}
	scope := request.Scope
	if scope == "" {
		scope = ChromeWebStoreScope
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("could not open a loopback port for the OAuth callback: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d", port)

	state, err := randomURLSafeString(24)
	if err != nil {
		listener.Close()
		return "", err
	}
	codeVerifier, err := randomURLSafeString(48)
	if err != nil {
		listener.Close()
		return "", err
	}
	challengeHash := sha256.Sum256([]byte(codeVerifier))
	codeChallenge := base64.RawURLEncoding.EncodeToString(challengeHash[:])

	callbackChannel := make(chan callbackResult, 1)
	server := &http.Server{
		Handler:           newCallbackHandler(state, callbackChannel),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() { _ = server.Serve(listener) }()
	defer func() {
		shutdownContext, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownContext)
	}()

	authParams := url.Values{
		"client_id":             {request.ClientID},
		"redirect_uri":          {redirectURI},
		"response_type":         {"code"},
		"scope":                 {scope},
		"access_type":           {"offline"},
		"prompt":                {"consent"},
		"state":                 {state},
		"code_challenge":        {codeChallenge},
		"code_challenge_method": {"S256"},
	}
	authURL := AuthEndpoint + "?" + authParams.Encode()

	if err := openBrowser(authURL); err != nil {
		return "", fmt.Errorf("could not open the browser for authorization: %w", err)
	}

	var result callbackResult
	select {
	case <-ctx.Done():
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", errors.New("timed out waiting for authorization in the browser; start the request again")
		}
		return "", errors.New("authorization was cancelled")
	case result = <-callbackChannel:
	}

	if result.authError != "" {
		return "", fmt.Errorf("Google returned an authorization error: %s", result.authError)
	}
	return exchangeCodeForRefreshToken(ctx, request, result.code, redirectURI, codeVerifier)
}

func newCallbackHandler(expectedState string, resultChannel chan<- callbackResult) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		code := query.Get("code")
		authError := query.Get("error")
		if code == "" && authError == "" {
			http.NotFound(w, r)
			return
		}
		if subtle.ConstantTimeCompare([]byte(query.Get("state")), []byte(expectedState)) != 1 {
			http.Error(w, "Invalid state parameter; this request was ignored.", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if authError != "" {
			fmt.Fprintf(w, "<html><body>Authorization failed: %s</body></html>", html.EscapeString(authError))
		} else {
			fmt.Fprint(w, successPage)
		}

		select {
		case resultChannel <- callbackResult{code: code, authError: authError}:
		default:
		}
	})
}

func exchangeCodeForRefreshToken(
	ctx context.Context,
	request OAuthRequest,
	code string,
	redirectURI string,
	codeVerifier string,
) (string, error) {
	form := url.Values{
		"client_id":     {request.ClientID},
		"client_secret": {request.ClientSecret},
		"code":          {code},
		"grant_type":    {"authorization_code"},
		"redirect_uri":  {redirectURI},
		"code_verifier": {codeVerifier},
	}

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("could not build the token exchange request: %w", err)
	}
	httpRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	httpClient := &http.Client{Timeout: 30 * time.Second}
	response, err := httpClient.Do(httpRequest)
	if err != nil {
		return "", fmt.Errorf("token exchange request to %s failed: %w", TokenEndpoint, err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("could not read the token exchange response: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token exchange failed with HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}

	var tokenResponse struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.Unmarshal(body, &tokenResponse); err != nil {
		return "", fmt.Errorf("token exchange response was not valid JSON: %w", err)
	}
	if tokenResponse.RefreshToken == "" {
		return "", errors.New(
			"Google's response contained no refresh_token. This usually means this app was already " +
				"authorized: go to https://myaccount.google.com/permissions, remove access for it, and try again")
	}
	return tokenResponse.RefreshToken, nil
}

func randomURLSafeString(byteCount int) (string, error) {
	randomBytes := make([]byte, byteCount)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", fmt.Errorf("could not generate secure random bytes: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(randomBytes), nil
}
