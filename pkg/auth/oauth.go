// OAuth 2.0 credential management for ztransfer.
//
// Stores and refreshes Google OAuth credentials in ~/.ztransfer/credentials.json.
// Used by the relay client to authenticate without manual token management.
package auth

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Google OAuth 2.0 endpoints.
const (
	GoogleAuthURL  = "https://accounts.google.com/o/oauth2/v2/auth"
	GoogleTokenURL = "https://oauth2.googleapis.com/token"

)

// OAuth client credentials — loaded from environment or ~/.ztransfer/oauth_client.json.
// For installed/desktop apps these are not truly secret per Google's OAuth spec,
// but we keep them out of source for public repos.
var (
	OAuthClientID     string
	OAuthClientSecret string
)

func init() {
	OAuthClientID = os.Getenv("ZTRANSFER_OAUTH_CLIENT_ID")
	OAuthClientSecret = os.Getenv("ZTRANSFER_OAUTH_CLIENT_SECRET")

	// Fall back to ~/.ztransfer/oauth_client.json
	if OAuthClientID == "" {
		if dir, err := ConfigDir(); err == nil {
			data, err := os.ReadFile(filepath.Join(dir, "oauth_client.json"))
			if err == nil {
				var client struct {
					Installed struct {
						ClientID     string `json:"client_id"`
						ClientSecret string `json:"client_secret"`
					} `json:"installed"`
				}
				if json.Unmarshal(data, &client) == nil {
					OAuthClientID = client.Installed.ClientID
					OAuthClientSecret = client.Installed.ClientSecret
				}
			}
		}
	}
}

// ErrNoCredentials is returned when no stored credentials exist.
var ErrNoCredentials = errors.New("no stored credentials — run 'ztransfer login'")

// Credentials holds OAuth tokens persisted to disk.
type Credentials struct {
	IDToken      string    `json:"id_token"`
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	Expiry       time.Time `json:"expiry"`
	Email        string    `json:"email"`
}

// credentialsPath returns the path to ~/.ztransfer/credentials.json.
func credentialsPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "credentials.json"), nil
}

// LoadCredentials reads stored OAuth credentials from disk.
func LoadCredentials() (*Credentials, error) {
	path, err := credentialsPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNoCredentials
		}
		return nil, fmt.Errorf("read credentials: %w", err)
	}

	var creds Credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("parse credentials: %w", err)
	}
	return &creds, nil
}

// SaveCredentials writes OAuth credentials to disk with 0600 permissions.
func SaveCredentials(creds *Credentials) error {
	path, err := credentialsPath()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// DeleteCredentials removes stored credentials (logout).
func DeleteCredentials() error {
	path, err := credentialsPath()
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// GetIDToken returns a valid ID token, refreshing if needed.
func (c *Credentials) GetIDToken() (string, error) {
	if err := c.RefreshIfNeeded(); err != nil {
		return "", err
	}
	return c.IDToken, nil
}

// RefreshIfNeeded checks if the token is expired or about to expire
// (within 5 minutes) and refreshes it using the refresh token.
func (c *Credentials) RefreshIfNeeded() error {
	if c.RefreshToken == "" {
		return fmt.Errorf("no refresh token available")
	}

	// Still valid for more than 5 minutes — no refresh needed.
	if time.Until(c.Expiry) > 5*time.Minute {
		return nil
	}

	params := url.Values{
		"client_id":     {OAuthClientID},
		"client_secret": {OAuthClientSecret},
		"refresh_token": {c.RefreshToken},
		"grant_type":    {"refresh_token"},
	}

	resp, err := http.PostForm(GoogleTokenURL, params)
	if err != nil {
		return fmt.Errorf("refresh token request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("refresh failed (status %d): %s", resp.StatusCode, body)
	}

	var result struct {
		IDToken     string `json:"id_token"`
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("parse refresh response: %w", err)
	}

	c.IDToken = result.IDToken
	c.AccessToken = result.AccessToken
	c.Expiry = time.Now().Add(time.Duration(result.ExpiresIn) * time.Second)

	// Update email from new ID token.
	if email := extractEmailFromIDToken(result.IDToken); email != "" {
		c.Email = email
	}

	// Persist updated credentials.
	return SaveCredentials(c)
}

// extractEmailFromIDToken decodes the JWT payload to get the email claim.
// No signature verification — we just received this token from Google over TLS.
func extractEmailFromIDToken(idToken string) string {
	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		return ""
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}

	var claims struct {
		Email string `json:"email"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return ""
	}
	return claims.Email
}

// extractExpiryFromIDToken decodes the JWT payload to get the exp claim.
func extractExpiryFromIDToken(idToken string) time.Time {
	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		return time.Time{}
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}
	}

	var claims struct {
		Exp float64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return time.Time{}
	}
	if claims.Exp == 0 {
		return time.Time{}
	}
	return time.Unix(int64(claims.Exp), 0)
}
