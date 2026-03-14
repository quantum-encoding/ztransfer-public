package auth

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const firestoreBaseURL = "https://firestore.googleapis.com/v1"
const firestoreProject = "warpgate-auth"

// AuthorizeUser adds an email to the authorized_users collection in Firestore.
// Uses the caller's OAuth access token for authentication.
func AuthorizeUser(accessToken, email, name, authorizedBy, scope string) error {
	if scope == "" {
		scope = "relay"
	}
	email = strings.ToLower(strings.TrimSpace(email))

	now := time.Now().UTC().Format(time.RFC3339Nano)

	// Build Firestore document with typed fields.
	doc := map[string]any{
		"fields": map[string]any{
			"email": map[string]any{
				"stringValue": email,
			},
			"name": map[string]any{
				"stringValue": name,
			},
			"authorized_by": map[string]any{
				"stringValue": authorizedBy,
			},
			"scope": map[string]any{
				"stringValue": scope,
			},
			"authorized_at": map[string]any{
				"timestampValue": now,
			},
		},
	}

	body, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("marshal document: %w", err)
	}

	// PATCH creates or overwrites the document. Document ID = email.
	u := fmt.Sprintf("%s/projects/%s/databases/(default)/documents/authorized_users/%s",
		firestoreBaseURL,
		url.PathEscape(firestoreProject),
		url.PathEscape(email),
	)

	req, err := http.NewRequest("PATCH", u, strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("firestore request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("firestore error (status %d): %s", resp.StatusCode, respBody)
	}

	return nil
}

// DeauthorizeUser removes an email from the authorized_users collection.
func DeauthorizeUser(accessToken, email string) error {
	email = strings.ToLower(strings.TrimSpace(email))

	u := fmt.Sprintf("%s/projects/%s/databases/(default)/documents/authorized_users/%s",
		firestoreBaseURL,
		url.PathEscape(firestoreProject),
		url.PathEscape(email),
	)

	req, err := http.NewRequest("DELETE", u, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("firestore request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("firestore error (status %d): %s", resp.StatusCode, respBody)
	}

	return nil
}
