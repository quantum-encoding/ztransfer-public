package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"time"
)

// RunLoginFlow performs a Google OAuth 2.0 PKCE login via the browser.
//
// Flow:
//  1. Start local HTTP server on a random port
//  2. Open browser to Google consent screen
//  3. Receive auth code via callback
//  4. Exchange code for tokens
//  5. Save credentials to ~/.ztransfer/credentials.json
func RunLoginFlow() (*Credentials, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Generate PKCE code verifier and challenge.
	verifier, challenge, err := generatePKCE()
	if err != nil {
		return nil, fmt.Errorf("generate PKCE: %w", err)
	}

	// Generate random state for CSRF protection.
	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		return nil, fmt.Errorf("generate state: %w", err)
	}
	state := hex.EncodeToString(stateBytes)

	// Start local callback server on a random port.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("start callback server: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d/callback", port)

	// Channel to receive the auth code from the callback handler.
	type authResult struct {
		code string
		err  error
	}
	resultCh := make(chan authResult, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		// Validate state.
		if r.URL.Query().Get("state") != state {
			resultCh <- authResult{err: fmt.Errorf("state mismatch")}
			http.Error(w, "State mismatch", http.StatusBadRequest)
			return
		}

		// Check for errors from Google.
		if errParam := r.URL.Query().Get("error"); errParam != "" {
			resultCh <- authResult{err: fmt.Errorf("oauth error: %s", errParam)}
			fmt.Fprintf(w, "<html><body><h2>Login failed: %s</h2><p>You can close this tab.</p></body></html>", errParam)
			return
		}

		code := r.URL.Query().Get("code")
		if code == "" {
			resultCh <- authResult{err: fmt.Errorf("no auth code received")}
			http.Error(w, "No code", http.StatusBadRequest)
			return
		}

		resultCh <- authResult{code: code}

		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><body style="font-family:system-ui;text-align:center;margin-top:80px">
			<h2 style="color:#22c55e">Login successful!</h2>
			<p>You can close this tab and return to the terminal.</p>
		</body></html>`)
	})

	server := &http.Server{Handler: mux}
	go server.Serve(listener)
	defer server.Shutdown(context.Background())

	// Build the Google OAuth authorization URL.
	authURL := fmt.Sprintf("%s?%s", GoogleAuthURL, url.Values{
		"client_id":             {OAuthClientID},
		"redirect_uri":          {redirectURI},
		"response_type":         {"code"},
		"scope":                 {"openid email profile"},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"state":                 {state},
		"access_type":           {"offline"},
		"prompt":                {"consent"},
	}.Encode())

	// Open browser.
	fmt.Println("Opening browser for Google login...")
	fmt.Printf("\nIf the browser doesn't open, visit:\n  %s\n\n", authURL)
	openBrowser(authURL)

	// Wait for callback or timeout.
	var code string
	select {
	case result := <-resultCh:
		if result.err != nil {
			return nil, result.err
		}
		code = result.code
	case <-ctx.Done():
		return nil, fmt.Errorf("login timed out (2 minutes)")
	}

	// Exchange auth code for tokens.
	creds, err := exchangeCode(code, redirectURI, verifier)
	if err != nil {
		return nil, fmt.Errorf("token exchange: %w", err)
	}

	// Save to disk.
	if err := SaveCredentials(creds); err != nil {
		return nil, fmt.Errorf("save credentials: %w", err)
	}

	return creds, nil
}

// exchangeCode exchanges an authorization code for OAuth tokens.
func exchangeCode(code, redirectURI, verifier string) (*Credentials, error) {
	params := url.Values{
		"client_id":     {OAuthClientID},
		"client_secret": {OAuthClientSecret},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"grant_type":    {"authorization_code"},
		"code_verifier": {verifier},
	}

	resp, err := http.PostForm(GoogleTokenURL, params)
	if err != nil {
		return nil, fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token exchange failed (status %d): %s", resp.StatusCode, body)
	}

	var result struct {
		IDToken      string `json:"id_token"`
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse token response: %w", err)
	}

	if result.RefreshToken == "" {
		return nil, fmt.Errorf("no refresh token received — try revoking access at https://myaccount.google.com/permissions")
	}

	email := extractEmailFromIDToken(result.IDToken)
	expiry := time.Now().Add(time.Duration(result.ExpiresIn) * time.Second)

	return &Credentials{
		IDToken:      result.IDToken,
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		Expiry:       expiry,
		Email:        email,
	}, nil
}

// generatePKCE creates a PKCE code verifier and S256 challenge.
func generatePKCE() (verifier, challenge string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	verifier = base64.RawURLEncoding.EncodeToString(buf)
	hash := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(hash[:])
	return verifier, challenge, nil
}

// openBrowser opens the given URL in the default browser.
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	default:
		return
	}
	cmd.Start()
}
