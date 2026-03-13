// Package audit provides tamper-evident, hash-chained session logging
// for ztransfer remote shell sessions. Each event in a session references
// the hash of the previous event, forming a verifiable chain that can be
// audited by customers or compliance systems.
//
// Events are append-only and designed for streaming to BigQuery or local
// JSON files for offline verification.
package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// EventType classifies what happened in the session.
type EventType string

const (
	EventSessionStart  EventType = "session_start"
	EventSessionEnd    EventType = "session_end"
	EventCommandExec   EventType = "command_exec"
	EventCommandOutput EventType = "command_output"
	EventFileTransfer  EventType = "file_transfer"
	EventAuthChallenge EventType = "auth_challenge"
	EventAuthResult    EventType = "auth_result"
	EventError         EventType = "error"
	EventHeartbeat     EventType = "heartbeat"
)

// Event is a single audit log entry in a session chain.
type Event struct {
	// Chain fields
	SessionID    string `json:"session_id"`
	Sequence     uint64 `json:"sequence"`
	PreviousHash string `json:"previous_hash"` // hex-encoded SHA-256 of the previous event
	Hash         string `json:"hash"`           // hex-encoded SHA-256 of this event (computed after all other fields are set)

	// Event metadata
	Timestamp   time.Time `json:"timestamp"`
	EventType   EventType `json:"event_type"`
	ActorID     string    `json:"actor_id"`               // who performed the action (operator email or SA)
	TargetID    string    `json:"target_id,omitempty"`     // target machine identifier
	Description string    `json:"description,omitempty"`   // human-readable summary
	Redacted    bool      `json:"redacted,omitempty"`      // true if sensitive content was stripped

	// Payload — type-specific data
	Command    string            `json:"command,omitempty"`     // for command_exec
	ExitCode   *int              `json:"exit_code,omitempty"`   // for command_exec result
	ByteCount  int64             `json:"byte_count,omitempty"`  // for file_transfer or output
	FileName   string            `json:"file_name,omitempty"`   // for file_transfer
	Direction  string            `json:"direction,omitempty"`   // "upload" or "download"
	ErrorMsg   string            `json:"error_msg,omitempty"`   // for error events
	Metadata   map[string]string `json:"metadata,omitempty"`    // extensible key-value pairs
}

// computeHash calculates the SHA-256 hash of the event's content.
// The Hash field is excluded from the hash computation.
func (e *Event) computeHash() string {
	// Create a copy without the Hash field for deterministic hashing
	hashInput := struct {
		SessionID    string            `json:"session_id"`
		Sequence     uint64            `json:"sequence"`
		PreviousHash string            `json:"previous_hash"`
		Timestamp    time.Time         `json:"timestamp"`
		EventType    EventType         `json:"event_type"`
		ActorID      string            `json:"actor_id"`
		TargetID     string            `json:"target_id,omitempty"`
		Description  string            `json:"description,omitempty"`
		Redacted     bool              `json:"redacted,omitempty"`
		Command      string            `json:"command,omitempty"`
		ExitCode     *int              `json:"exit_code,omitempty"`
		ByteCount    int64             `json:"byte_count,omitempty"`
		FileName     string            `json:"file_name,omitempty"`
		Direction    string            `json:"direction,omitempty"`
		ErrorMsg     string            `json:"error_msg,omitempty"`
		Metadata     map[string]string `json:"metadata,omitempty"`
	}{
		SessionID:    e.SessionID,
		Sequence:     e.Sequence,
		PreviousHash: e.PreviousHash,
		Timestamp:    e.Timestamp,
		EventType:    e.EventType,
		ActorID:      e.ActorID,
		TargetID:     e.TargetID,
		Description:  e.Description,
		Redacted:     e.Redacted,
		Command:      e.Command,
		ExitCode:     e.ExitCode,
		ByteCount:    e.ByteCount,
		FileName:     e.FileName,
		Direction:    e.Direction,
		ErrorMsg:     e.ErrorMsg,
		Metadata:     e.Metadata,
	}

	data, err := json.Marshal(hashInput)
	if err != nil {
		// Should never happen with these types
		panic(fmt.Sprintf("audit: failed to marshal event for hashing: %v", err))
	}

	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// Seal computes and sets the Hash field. Must be called after all other
// fields are set. Returns the hash for convenience.
func (e *Event) Seal() string {
	e.Hash = e.computeHash()
	return e.Hash
}

// Verify checks that this event's hash matches its content.
func (e *Event) Verify() bool {
	return e.Hash == e.computeHash()
}
