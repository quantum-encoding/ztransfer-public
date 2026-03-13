package audit

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"
)

// GenesisHash is the previous_hash for the first event in every chain.
const GenesisHash = "0000000000000000000000000000000000000000000000000000000000000000"

// Chain is an append-only, hash-chained sequence of audit events for a
// single session. It is safe for concurrent use.
type Chain struct {
	mu        sync.Mutex
	sessionID string
	actorID   string
	targetID  string
	sequence  uint64
	lastHash  string
	sinks     []Sink
}

// NewChain creates a new audit chain for a session.
func NewChain(sessionID, actorID, targetID string, sinks ...Sink) *Chain {
	return &Chain{
		sessionID: sessionID,
		actorID:   actorID,
		targetID:  targetID,
		sequence:  0,
		lastHash:  GenesisHash,
		sinks:     sinks,
	}
}

// Append adds an event to the chain, seals it, and writes to all sinks.
// The caller sets event-specific fields; chain fields (session_id, sequence,
// previous_hash, hash, timestamp, actor_id, target_id) are set automatically.
func (c *Chain) Append(evt *Event) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	evt.SessionID = c.sessionID
	evt.ActorID = c.actorID
	evt.TargetID = c.targetID
	evt.Sequence = c.sequence
	evt.PreviousHash = c.lastHash
	evt.Timestamp = time.Now().UTC()

	evt.Seal()

	c.sequence++
	c.lastHash = evt.Hash

	// Write to all sinks
	var firstErr error
	for _, sink := range c.sinks {
		if err := sink.Write(evt); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	return firstErr
}

// SessionStart emits a session_start event. Call this first.
func (c *Chain) SessionStart(meta map[string]string) error {
	return c.Append(&Event{
		EventType:   EventSessionStart,
		Description: "Remote session started",
		Metadata:    meta,
	})
}

// SessionEnd emits a session_end event. Call this last.
func (c *Chain) SessionEnd(meta map[string]string) error {
	return c.Append(&Event{
		EventType:   EventSessionEnd,
		Description: "Remote session ended",
		Metadata:    meta,
	})
}

// CommandExec logs a command execution.
func (c *Chain) CommandExec(command string) error {
	return c.Append(&Event{
		EventType: EventCommandExec,
		Command:   command,
	})
}

// CommandResult logs the result of a command.
func (c *Chain) CommandResult(command string, exitCode int, outputBytes int64) error {
	return c.Append(&Event{
		EventType: EventCommandOutput,
		Command:   command,
		ExitCode:  &exitCode,
		ByteCount: outputBytes,
	})
}

// FileTransfer logs a file transfer event.
func (c *Chain) FileTransfer(fileName, direction string, bytes int64) error {
	return c.Append(&Event{
		EventType: EventFileTransfer,
		FileName:  fileName,
		Direction: direction,
		ByteCount: bytes,
	})
}

// Error logs an error event.
func (c *Chain) Error(msg string) error {
	return c.Append(&Event{
		EventType: EventError,
		ErrorMsg:  msg,
	})
}

// Close flushes and closes all sinks.
func (c *Chain) Close() error {
	var firstErr error
	for _, sink := range c.sinks {
		if err := sink.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// VerifyChain checks that a sequence of events forms a valid hash chain.
// Events must be in order. Returns the index of the first invalid event,
// or -1 if the chain is valid.
func VerifyChain(events []Event) (int, error) {
	expectedPrev := GenesisHash

	for i := range events {
		evt := &events[i]

		if evt.Sequence != uint64(i) {
			return i, fmt.Errorf("event %d: expected sequence %d, got %d", i, i, evt.Sequence)
		}

		if evt.PreviousHash != expectedPrev {
			return i, fmt.Errorf("event %d: previous_hash mismatch", i)
		}

		if !evt.Verify() {
			return i, fmt.Errorf("event %d: hash verification failed (content tampered)", i)
		}

		expectedPrev = evt.Hash
	}

	return -1, nil
}

// ParseEvents parses newline-delimited JSON events (NDJSON).
func ParseEvents(data []byte) ([]Event, error) {
	var events []Event
	decoder := json.NewDecoder(jsonBytesReader(data))
	for decoder.More() {
		var evt Event
		if err := decoder.Decode(&evt); err != nil {
			return events, fmt.Errorf("parse event at position %d: %w", len(events), err)
		}
		events = append(events, evt)
	}
	return events, nil
}

type bytesReader struct {
	data []byte
	pos  int
}

func jsonBytesReader(data []byte) *bytesReader {
	return &bytesReader{data: data}
}

func (r *bytesReader) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n = copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}
