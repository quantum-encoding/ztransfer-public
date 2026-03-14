// Firestore-backed email allowlist for the relay server.
//
// Checks authorized_users collection in Firestore to determine if a
// connecting user's email is allowed. Uses the Firestore REST API
// directly (no client library) to avoid pulling in gRPC dependencies.
//
// On Cloud Run, authentication uses the default service account via
// the metadata server. The cache is refreshed every 5 minutes.
package relay

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// FirestoreAllowlist checks user emails against a Firestore collection.
type FirestoreAllowlist struct {
	projectID  string
	collection string

	mu      sync.RWMutex
	emails  map[string]AuthorizedUser
	expiry  time.Time
	cacheTTL time.Duration
}

// AuthorizedUser represents a document in the authorized_users collection.
type AuthorizedUser struct {
	Email        string    `json:"email"`
	Name         string    `json:"name"`
	AuthorizedAt time.Time `json:"authorized_at"`
	AuthorizedBy string    `json:"authorized_by"`
	Scope        string    `json:"scope"` // relay, diagnostic, repair, full
}

// NewFirestoreAllowlist creates a new Firestore-backed allowlist.
func NewFirestoreAllowlist(projectID string) *FirestoreAllowlist {
	return &FirestoreAllowlist{
		projectID:  projectID,
		collection: "authorized_users",
		emails:     make(map[string]AuthorizedUser),
		cacheTTL:   5 * time.Minute,
	}
}

// IsAuthorized checks if the given email is in the allowlist.
// Returns true if authorized, false otherwise.
func (f *FirestoreAllowlist) IsAuthorized(email string) bool {
	email = strings.ToLower(strings.TrimSpace(email))

	// Check cache first.
	f.mu.RLock()
	if time.Now().Before(f.expiry) {
		_, ok := f.emails[email]
		f.mu.RUnlock()
		return ok
	}
	f.mu.RUnlock()

	// Cache expired — refresh.
	if err := f.refresh(); err != nil {
		// On error, use stale cache rather than blocking everyone.
		f.mu.RLock()
		_, ok := f.emails[email]
		f.mu.RUnlock()
		return ok
	}

	f.mu.RLock()
	defer f.mu.RUnlock()
	_, ok := f.emails[email]
	return ok
}

// refresh fetches all documents from the authorized_users collection.
func (f *FirestoreAllowlist) refresh() error {
	token, err := getMetadataAccessToken()
	if err != nil {
		return fmt.Errorf("get access token: %w", err)
	}

	// Firestore REST API: list all documents in the collection.
	u := fmt.Sprintf(
		"https://firestore.googleapis.com/v1/projects/%s/databases/(default)/documents/%s?pageSize=1000",
		url.PathEscape(f.projectID),
		url.PathEscape(f.collection),
	)

	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("firestore request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("firestore error (status %d): %s", resp.StatusCode, body)
	}

	var result firestoreListResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("parse firestore response: %w", err)
	}

	// Parse documents into the cache.
	emails := make(map[string]AuthorizedUser)
	for _, doc := range result.Documents {
		user := parseAuthorizedUser(doc.Fields)
		if user.Email != "" {
			emails[strings.ToLower(user.Email)] = user
		}
	}

	f.mu.Lock()
	f.emails = emails
	f.expiry = time.Now().Add(f.cacheTTL)
	f.mu.Unlock()

	return nil
}

// getMetadataAccessToken fetches an access token from the GCP metadata server.
// This works on Cloud Run, GCE, GKE, and any GCP environment.
func getMetadataAccessToken() (string, error) {
	req, err := http.NewRequest("GET",
		"http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/token",
		nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Metadata-Flavor", "Google")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("metadata server: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("metadata server %d: %s", resp.StatusCode, body)
	}

	var result struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("parse token: %w", err)
	}
	return result.AccessToken, nil
}

// --------------------------------------------------------------------------
// Firestore REST API response types
// --------------------------------------------------------------------------

type firestoreListResponse struct {
	Documents []firestoreDocument `json:"documents"`
}

type firestoreDocument struct {
	Name   string                       `json:"name"`
	Fields map[string]firestoreValue    `json:"fields"`
}

type firestoreValue struct {
	StringValue    *string `json:"stringValue,omitempty"`
	TimestampValue *string `json:"timestampValue,omitempty"`
}

func parseAuthorizedUser(fields map[string]firestoreValue) AuthorizedUser {
	user := AuthorizedUser{}
	if v, ok := fields["email"]; ok && v.StringValue != nil {
		user.Email = *v.StringValue
	}
	if v, ok := fields["name"]; ok && v.StringValue != nil {
		user.Name = *v.StringValue
	}
	if v, ok := fields["authorized_by"]; ok && v.StringValue != nil {
		user.AuthorizedBy = *v.StringValue
	}
	if v, ok := fields["scope"]; ok && v.StringValue != nil {
		user.Scope = *v.StringValue
	}
	if v, ok := fields["authorized_at"]; ok && v.TimestampValue != nil {
		if t, err := time.Parse(time.RFC3339Nano, *v.TimestampValue); err == nil {
			user.AuthorizedAt = t
		}
	}
	return user
}
