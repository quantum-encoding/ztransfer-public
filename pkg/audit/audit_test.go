package audit

import (
	"bytes"
	"testing"
)

func TestChainIntegrity(t *testing.T) {
	var buf bytes.Buffer
	sink := NewWriterSink(&buf)
	chain := NewChain("test-session-001", "operator@example.com", "target-machine", sink)

	// Record a session
	if err := chain.SessionStart(map[string]string{"relay": "eu-west1"}); err != nil {
		t.Fatalf("SessionStart: %v", err)
	}
	if err := chain.CommandExec("uname -a"); err != nil {
		t.Fatalf("CommandExec: %v", err)
	}
	if err := chain.CommandResult("uname -a", 0, 85); err != nil {
		t.Fatalf("CommandResult: %v", err)
	}
	if err := chain.FileTransfer("config.yaml", "upload", 1024); err != nil {
		t.Fatalf("FileTransfer: %v", err)
	}
	if err := chain.SessionEnd(map[string]string{"duration_s": "45"}); err != nil {
		t.Fatalf("SessionEnd: %v", err)
	}
	chain.Close()

	// Parse and verify the chain
	events, err := ParseEvents(buf.Bytes())
	if err != nil {
		t.Fatalf("ParseEvents: %v", err)
	}

	if len(events) != 5 {
		t.Fatalf("expected 5 events, got %d", len(events))
	}

	idx, err := VerifyChain(events)
	if err != nil {
		t.Fatalf("VerifyChain failed at index %d: %v", idx, err)
	}

	// Verify specific event types
	expectedTypes := []EventType{
		EventSessionStart, EventCommandExec, EventCommandOutput,
		EventFileTransfer, EventSessionEnd,
	}
	for i, et := range expectedTypes {
		if events[i].EventType != et {
			t.Errorf("event %d: expected type %s, got %s", i, et, events[i].EventType)
		}
	}

	// Verify chain linkage
	if events[0].PreviousHash != GenesisHash {
		t.Error("first event should reference genesis hash")
	}
	for i := 1; i < len(events); i++ {
		if events[i].PreviousHash != events[i-1].Hash {
			t.Errorf("event %d: previous_hash doesn't match event %d hash", i, i-1)
		}
	}
}

func TestTamperDetection(t *testing.T) {
	var buf bytes.Buffer
	sink := NewWriterSink(&buf)
	chain := NewChain("tamper-test", "op@test.com", "target", sink)

	chain.SessionStart(nil)
	chain.CommandExec("whoami")
	chain.SessionEnd(nil)
	chain.Close()

	events, _ := ParseEvents(buf.Bytes())

	// Tamper with the middle event
	events[1].Command = "rm -rf /"

	idx, err := VerifyChain(events)
	if err == nil {
		t.Fatal("expected tamper detection, got no error")
	}
	if idx != 1 {
		t.Errorf("expected tamper at index 1, got %d", idx)
	}
}

func TestVerifyIndividualEvent(t *testing.T) {
	evt := &Event{
		SessionID: "test",
		Sequence:  0,
		EventType: EventCommandExec,
		Command:   "ls -la",
	}
	evt.PreviousHash = GenesisHash
	evt.Seal()

	if !evt.Verify() {
		t.Error("sealed event should verify")
	}

	evt.Command = "cat /etc/passwd"
	if evt.Verify() {
		t.Error("tampered event should not verify")
	}
}
