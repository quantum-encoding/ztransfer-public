// Command ztransfer-mint generates scoped tokens for automated ztransfer
// sessions. Designed for headless environments (GCP VMs, CI/CD, Claude
// operator workflows) where interactive gcloud login isn't possible.
//
// Token sources (tried in order):
//  1. GCP metadata server (on GCP VMs — zero config)
//  2. Application Default Credentials (ADC — gcloud auth application-default login)
//  3. Service account key file (GOOGLE_APPLICATION_CREDENTIALS env var)
//
// Usage:
//
//	ztransfer-mint --scope relay
//	ztransfer-mint --scope diagnostic --audience https://ztransfer-relay-xxx.run.app
//	ztransfer-mint --scope repair --sa ztransfer-agent@proj.iam.gserviceaccount.com
//
// The token is printed to stdout. Set it as:
//
//	export ZTRANSFER_RELAY_TOKEN=$(ztransfer-mint --scope relay)
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Scopes define what the minted token allows.
var scopeMap = map[string][]string{
	"relay": {
		"https://www.googleapis.com/auth/cloud-platform",
	},
	"diagnostic": {
		"https://www.googleapis.com/auth/cloud-platform.read-only",
	},
	"repair": {
		"https://www.googleapis.com/auth/cloud-platform",
	},
	"full": {
		"https://www.googleapis.com/auth/cloud-platform",
	},
}

const (
	defaultServiceAccount = "ztransfer-agent@ztransfer-gcp-relay.iam.gserviceaccount.com"
	defaultAudience       = "https://ztransfer-relay-967904281608.europe-west1.run.app"
	metadataURL           = "http://metadata.google.internal/computeMetadata/v1"
	iamCredentialsURL     = "https://iamcredentials.googleapis.com/v1"
)

func main() {
	scope := flag.String("scope", "relay", "Token scope: relay, diagnostic, repair, full")
	sa := flag.String("sa", defaultServiceAccount, "Service account to impersonate")
	audience := flag.String("audience", defaultAudience, "Audience for identity token")
	tokenType := flag.String("type", "identity", "Token type: identity (for Cloud Run) or access (for APIs)")
	verbose := flag.Bool("v", false, "Verbose output to stderr")
	flag.Parse()

	if _, ok := scopeMap[*scope]; !ok {
		fmt.Fprintf(os.Stderr, "error: unknown scope %q (valid: relay, diagnostic, repair, full)\n", *scope)
		os.Exit(1)
	}

	if *verbose {
		fmt.Fprintf(os.Stderr, "scope: %s\n", *scope)
		fmt.Fprintf(os.Stderr, "service_account: %s\n", *sa)
		fmt.Fprintf(os.Stderr, "audience: %s\n", *audience)
		fmt.Fprintf(os.Stderr, "type: %s\n", *tokenType)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var token string
	var err error

	switch *tokenType {
	case "identity":
		token, err = mintIdentityToken(ctx, *sa, *audience, *verbose)
	case "access":
		token, err = mintAccessToken(ctx, *sa, scopeMap[*scope], *verbose)
	default:
		fmt.Fprintf(os.Stderr, "error: unknown token type %q\n", *tokenType)
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Print(token)
}

// mintIdentityToken produces an OIDC identity token suitable for
// authenticating to Cloud Run services.
func mintIdentityToken(ctx context.Context, sa, audience string, verbose bool) (string, error) {
	// Try 1: GCP metadata server (on a VM)
	if token, err := mintIdentityFromMetadata(ctx, sa, audience); err == nil {
		if verbose {
			fmt.Fprintln(os.Stderr, "source: metadata server")
		}
		return token, nil
	}

	// Try 2: IAM Credentials API with ADC
	token, err := mintIdentityFromIAM(ctx, sa, audience, verbose)
	if err == nil {
		if verbose {
			fmt.Fprintln(os.Stderr, "source: IAM Credentials API")
		}
		return token, nil
	}

	// Try 3: gcloud fallback
	if verbose {
		fmt.Fprintf(os.Stderr, "IAM failed: %v, trying gcloud\n", err)
	}
	return mintFromGcloud(audience)
}

// mintAccessToken produces an OAuth2 access token with specific scopes.
func mintAccessToken(ctx context.Context, sa string, scopes []string, verbose bool) (string, error) {
	// Try 1: Metadata server
	if token, err := mintAccessFromMetadata(ctx, scopes); err == nil {
		if verbose {
			fmt.Fprintln(os.Stderr, "source: metadata server")
		}
		return token, nil
	}

	// Try 2: IAM Credentials API
	token, err := mintAccessFromIAM(ctx, sa, scopes, verbose)
	if err == nil {
		if verbose {
			fmt.Fprintln(os.Stderr, "source: IAM Credentials API")
		}
		return token, nil
	}

	return "", fmt.Errorf("all token sources failed: %w", err)
}

// --------------------------------------------------------------------------
// Metadata server (GCP VMs — zero config)
// --------------------------------------------------------------------------

func mintIdentityFromMetadata(ctx context.Context, sa, audience string) (string, error) {
	// On a GCP VM, the metadata server can mint identity tokens for the
	// VM's default SA or any SA it can impersonate.
	u := fmt.Sprintf("%s/instance/service-accounts/%s/identity?audience=%s&format=full",
		metadataURL, url.QueryEscape(sa), url.QueryEscape(audience))

	return metadataGet(ctx, u)
}

func mintAccessFromMetadata(ctx context.Context, scopes []string) (string, error) {
	u := fmt.Sprintf("%s/instance/service-accounts/default/token?scopes=%s",
		metadataURL, url.QueryEscape(strings.Join(scopes, ",")))

	body, err := metadataGet(ctx, u)
	if err != nil {
		return "", err
	}

	var resp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		return "", fmt.Errorf("parse metadata token: %w", err)
	}
	return resp.AccessToken, nil
}

func metadataGet(ctx context.Context, u string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Metadata-Flavor", "Google")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("metadata server unreachable: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("metadata server %d: %s", resp.StatusCode, body)
	}
	return string(body), nil
}

// --------------------------------------------------------------------------
// IAM Credentials API (works from any machine with ADC)
// --------------------------------------------------------------------------

func mintIdentityFromIAM(ctx context.Context, sa, audience string, verbose bool) (string, error) {
	// Get an access token for the calling identity first (ADC)
	callerToken, err := getADCToken(ctx, verbose)
	if err != nil {
		return "", fmt.Errorf("get ADC token: %w", err)
	}

	// Use the caller's token to call IAM Credentials API to generate
	// an identity token for the target service account.
	u := fmt.Sprintf("%s/projects/-/serviceAccounts/%s:generateIdToken",
		iamCredentialsURL, sa)

	payload := fmt.Sprintf(`{"audience":%q,"includeEmail":true}`, audience)

	req, err := http.NewRequestWithContext(ctx, "POST", u, strings.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+callerToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("IAM API call: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("IAM API %d: %s", resp.StatusCode, body)
	}

	var result struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("parse IAM response: %w", err)
	}
	return result.Token, nil
}

func mintAccessFromIAM(ctx context.Context, sa string, scopes []string, verbose bool) (string, error) {
	callerToken, err := getADCToken(ctx, verbose)
	if err != nil {
		return "", fmt.Errorf("get ADC token: %w", err)
	}

	u := fmt.Sprintf("%s/projects/-/serviceAccounts/%s:generateAccessToken",
		iamCredentialsURL, sa)

	scopeJSON, _ := json.Marshal(scopes)
	payload := fmt.Sprintf(`{"scope":%s,"lifetime":"3600s"}`, scopeJSON)

	req, err := http.NewRequestWithContext(ctx, "POST", u, strings.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+callerToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("IAM API call: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("IAM API %d: %s", resp.StatusCode, body)
	}

	var result struct {
		AccessToken string `json:"accessToken"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("parse IAM response: %w", err)
	}
	return result.AccessToken, nil
}

// --------------------------------------------------------------------------
// Application Default Credentials
// --------------------------------------------------------------------------

// getADCToken gets an access token from ADC. This works when:
// - On a GCP VM (metadata server)
// - gcloud auth application-default login has been run
// - GOOGLE_APPLICATION_CREDENTIALS points to a key file
func getADCToken(ctx context.Context, verbose bool) (string, error) {
	// Try metadata server first (fastest, no file I/O)
	if token, err := mintAccessFromMetadata(ctx, []string{"https://www.googleapis.com/auth/cloud-platform"}); err == nil {
		if verbose {
			fmt.Fprintln(os.Stderr, "ADC source: metadata server")
		}
		return token, nil
	}

	// Try well-known ADC file
	adcPaths := []string{}

	if p := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"); p != "" {
		adcPaths = append(adcPaths, p)
	}

	// Default ADC location
	if home, err := os.UserHomeDir(); err == nil {
		adcPaths = append(adcPaths, home+"/.config/gcloud/application_default_credentials.json")
	}

	for _, path := range adcPaths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var creds struct {
			Type         string `json:"type"`
			ClientID     string `json:"client_id"`
			ClientSecret string `json:"client_secret"`
			RefreshToken string `json:"refresh_token"`
		}
		if err := json.Unmarshal(data, &creds); err != nil {
			continue
		}

		if creds.Type == "authorized_user" && creds.RefreshToken != "" {
			token, err := refreshOAuthToken(ctx, creds.ClientID, creds.ClientSecret, creds.RefreshToken)
			if err == nil {
				if verbose {
					fmt.Fprintf(os.Stderr, "ADC source: %s\n", path)
				}
				return token, nil
			}
		}
	}

	return "", fmt.Errorf("no ADC credentials found")
}

func refreshOAuthToken(ctx context.Context, clientID, clientSecret, refreshToken string) (string, error) {
	params := url.Values{
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"refresh_token": {refreshToken},
		"grant_type":    {"refresh_token"},
	}

	req, err := http.NewRequestWithContext(ctx, "POST",
		"https://oauth2.googleapis.com/token",
		strings.NewReader(params.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("token refresh failed %d: %s", resp.StatusCode, body)
	}

	var result struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}
	return result.AccessToken, nil
}

// --------------------------------------------------------------------------
// gcloud fallback
// --------------------------------------------------------------------------

func mintFromGcloud(audience string) (string, error) {
	cmd := exec.Command("gcloud", "auth", "print-identity-token",
		"--impersonate-service-account="+defaultServiceAccount,
		"--audiences="+audience)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("gcloud fallback failed: %s", exitErr.Stderr)
		}
		return "", fmt.Errorf("gcloud fallback failed: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}
