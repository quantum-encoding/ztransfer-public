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

const sessionsCollection = "active_sessions"

// ActiveSession represents a machine hosting a remote session.
type ActiveSession struct {
	Email       string    `json:"email"`
	SessionID   string    `json:"session_id"`
	MachineName string    `json:"machine_name"`
	Status      string    `json:"status"` // "hosting", "connected", "offline"
	RelayURL    string    `json:"relay_url"`
	StartedAt   time.Time `json:"started_at"`
	LastSeen    time.Time `json:"last_seen"`
}

// RegisterSession creates or updates an active session in Firestore.
// Called when a machine starts hosting.
func RegisterSession(accessToken string, session *ActiveSession) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)

	doc := map[string]any{
		"fields": map[string]any{
			"email":        map[string]any{"stringValue": session.Email},
			"session_id":   map[string]any{"stringValue": session.SessionID},
			"machine_name": map[string]any{"stringValue": session.MachineName},
			"status":       map[string]any{"stringValue": session.Status},
			"relay_url":    map[string]any{"stringValue": session.RelayURL},
			"started_at":   map[string]any{"timestampValue": now},
			"last_seen":    map[string]any{"timestampValue": now},
		},
	}

	body, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}

	// Document ID = email (one session per user for now)
	u := fmt.Sprintf("%s/projects/%s/databases/(default)/documents/%s/%s",
		firestoreBaseURL,
		url.PathEscape(firestoreProject),
		url.PathEscape(sessionsCollection),
		url.PathEscape(session.Email),
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

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("firestore error (status %d): %s", resp.StatusCode, respBody)
	}

	return nil
}

// HeartbeatSession updates the last_seen timestamp for a session.
func HeartbeatSession(accessToken, email string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)

	doc := map[string]any{
		"fields": map[string]any{
			"last_seen": map[string]any{"timestampValue": now},
		},
	}

	body, err := json.Marshal(doc)
	if err != nil {
		return err
	}

	u := fmt.Sprintf("%s/projects/%s/databases/(default)/documents/%s/%s?updateMask.fieldPaths=last_seen",
		firestoreBaseURL,
		url.PathEscape(firestoreProject),
		url.PathEscape(sessionsCollection),
		url.PathEscape(email),
	)

	req, err := http.NewRequest("PATCH", u, strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// RemoveSession deletes a session from Firestore (when hosting stops).
func RemoveSession(accessToken, email string) error {
	u := fmt.Sprintf("%s/projects/%s/databases/(default)/documents/%s/%s",
		firestoreBaseURL,
		url.PathEscape(firestoreProject),
		url.PathEscape(sessionsCollection),
		url.PathEscape(email),
	)

	req, err := http.NewRequest("DELETE", u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// ListActiveSessions returns all active sessions from Firestore.
func ListActiveSessions(accessToken string) ([]ActiveSession, error) {
	u := fmt.Sprintf("%s/projects/%s/databases/(default)/documents/%s?pageSize=100",
		firestoreBaseURL,
		url.PathEscape(firestoreProject),
		url.PathEscape(sessionsCollection),
	)

	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("firestore request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("firestore error (status %d): %s", resp.StatusCode, body)
	}

	var result struct {
		Documents []struct {
			Fields map[string]struct {
				StringValue    *string `json:"stringValue,omitempty"`
				TimestampValue *string `json:"timestampValue,omitempty"`
			} `json:"fields"`
		} `json:"documents"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse sessions: %w", err)
	}

	var sessions []ActiveSession
	for _, doc := range result.Documents {
		s := ActiveSession{}
		if v, ok := doc.Fields["email"]; ok && v.StringValue != nil {
			s.Email = *v.StringValue
		}
		if v, ok := doc.Fields["session_id"]; ok && v.StringValue != nil {
			s.SessionID = *v.StringValue
		}
		if v, ok := doc.Fields["machine_name"]; ok && v.StringValue != nil {
			s.MachineName = *v.StringValue
		}
		if v, ok := doc.Fields["status"]; ok && v.StringValue != nil {
			s.Status = *v.StringValue
		}
		if v, ok := doc.Fields["relay_url"]; ok && v.StringValue != nil {
			s.RelayURL = *v.StringValue
		}
		if v, ok := doc.Fields["started_at"]; ok && v.TimestampValue != nil {
			if t, err := time.Parse(time.RFC3339Nano, *v.TimestampValue); err == nil {
				s.StartedAt = t
			}
		}
		if v, ok := doc.Fields["last_seen"]; ok && v.TimestampValue != nil {
			if t, err := time.Parse(time.RFC3339Nano, *v.TimestampValue); err == nil {
				s.LastSeen = t
			}
		}

		// Only include sessions seen in the last 2 minutes
		if time.Since(s.LastSeen) < 2*time.Minute {
			sessions = append(sessions, s)
		}
	}

	return sessions, nil
}
