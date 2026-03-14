// Command ztransfer-audit verifies and inspects session audit logs.
//
// Customers receive NDJSON audit logs of remote sessions performed on
// their systems. This tool verifies the hash chain is intact (proving
// no events were added, removed, or modified) and provides a readable
// summary of the session.
//
// Usage:
//
//	ztransfer-audit verify session.log
//	ztransfer-audit summary session.log
//	ztransfer-audit verify --json session.log
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/quantum-encoding/ztransfer/pkg/audit"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: ztransfer-audit <verify|summary> [--json] <file>")
		os.Exit(1)
	}

	command := os.Args[1]
	var jsonOutput bool
	var filePath string

	if len(os.Args) == 4 && os.Args[2] == "--json" {
		jsonOutput = true
		filePath = os.Args[3]
	} else {
		filePath = os.Args[2]
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	events, err := audit.ParseEvents(data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error parsing events: %v\n", err)
		os.Exit(1)
	}

	switch command {
	case "verify":
		runVerify(events, jsonOutput)
	case "summary":
		runSummary(events, jsonOutput)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q (use verify or summary)\n", command)
		os.Exit(1)
	}
}

func runVerify(events []audit.Event, jsonOut bool) {
	idx, err := audit.VerifyChain(events)

	if jsonOut {
		result := map[string]interface{}{
			"valid":       err == nil,
			"event_count": len(events),
		}
		if err != nil {
			result["error"] = err.Error()
			result["failed_at_index"] = idx
		}
		if len(events) > 0 {
			result["session_id"] = events[0].SessionID
		}
		json.NewEncoder(os.Stdout).Encode(result)
	} else {
		if err != nil {
			fmt.Printf("FAILED — chain broken at event %d: %v\n", idx, err)
			os.Exit(1)
		}
		fmt.Printf("VERIFIED — %d events, chain intact\n", len(events))
		if len(events) > 0 {
			fmt.Printf("  Session:  %s\n", events[0].SessionID)
			fmt.Printf("  Operator: %s\n", events[0].ActorID)
			fmt.Printf("  Target:   %s\n", events[0].TargetID)
			fmt.Printf("  Start:    %s\n", events[0].Timestamp.Format(time.RFC3339))
			fmt.Printf("  End:      %s\n", events[len(events)-1].Timestamp.Format(time.RFC3339))
		}
	}
}

func runSummary(events []audit.Event, jsonOut bool) {
	// Verify first
	idx, err := audit.VerifyChain(events)
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: chain verification failed at event %d: %v\n", idx, err)
	}

	var (
		sessionID    string
		actor        string
		target       string
		startTime    time.Time
		endTime      time.Time
		commands     []string
		files        []string
		errors       []string
		totalBytes   int64
		commandCount int
		fileCount    int
	)

	for _, evt := range events {
		if sessionID == "" {
			sessionID = evt.SessionID
			actor = evt.ActorID
			target = evt.TargetID
		}
		if startTime.IsZero() {
			startTime = evt.Timestamp
		}
		endTime = evt.Timestamp

		switch evt.EventType {
		case audit.EventCommandExec:
			commandCount++
			commands = append(commands, evt.Command)
		case audit.EventCommandOutput:
			totalBytes += evt.ByteCount
		case audit.EventFileTransfer:
			fileCount++
			dir := "↑"
			if evt.Direction == "download" {
				dir = "↓"
			}
			files = append(files, fmt.Sprintf("%s %s (%s)", dir, evt.FileName, humanBytes(evt.ByteCount)))
			totalBytes += evt.ByteCount
		case audit.EventError:
			errors = append(errors, evt.ErrorMsg)
		}
	}

	duration := endTime.Sub(startTime)

	if jsonOut {
		result := map[string]interface{}{
			"session_id":    sessionID,
			"actor":         actor,
			"target":        target,
			"start":         startTime,
			"end":           endTime,
			"duration_s":    duration.Seconds(),
			"event_count":   len(events),
			"command_count": commandCount,
			"file_count":    fileCount,
			"total_bytes":   totalBytes,
			"chain_valid":   err == nil,
			"commands":      commands,
			"files":         files,
			"errors":        errors,
		}
		json.NewEncoder(os.Stdout).Encode(result)
		return
	}

	fmt.Println("═══════════════════════════════════════════════════")
	fmt.Println("  SESSION AUDIT REPORT")
	fmt.Println("═══════════════════════════════════════════════════")
	fmt.Println()
	fmt.Printf("  Session ID:  %s\n", sessionID)
	fmt.Printf("  Operator:    %s\n", actor)
	fmt.Printf("  Target:      %s\n", target)
	fmt.Printf("  Started:     %s\n", startTime.Format("2006-01-02 15:04:05 UTC"))
	fmt.Printf("  Ended:       %s\n", endTime.Format("2006-01-02 15:04:05 UTC"))
	fmt.Printf("  Duration:    %s\n", formatDuration(duration))
	fmt.Println()

	if err == nil {
		fmt.Println("  Chain:       ✓ VERIFIED (all hashes valid)")
	} else {
		fmt.Printf("  Chain:       ✗ BROKEN at event %d\n", idx)
	}
	fmt.Println()

	fmt.Printf("  Events:      %d total\n", len(events))
	fmt.Printf("  Commands:    %d executed\n", commandCount)
	fmt.Printf("  Files:       %d transferred\n", fileCount)
	fmt.Printf("  Data:        %s total\n", humanBytes(totalBytes))
	fmt.Println()

	if len(commands) > 0 {
		fmt.Println("  ─── Commands ──────────────────────────────────")
		for i, cmd := range commands {
			fmt.Printf("  %3d. %s\n", i+1, cmd)
		}
		fmt.Println()
	}

	if len(files) > 0 {
		fmt.Println("  ─── File Transfers ────────────────────────────")
		for _, f := range files {
			fmt.Printf("       %s\n", f)
		}
		fmt.Println()
	}

	if len(errors) > 0 {
		fmt.Println("  ─── Errors ────────────────────────────────────")
		for _, e := range errors {
			fmt.Printf("       %s\n", e)
		}
		fmt.Println()
	}

	fmt.Println("═══════════════════════════════════════════════════")
}

func humanBytes(b int64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(1<<10))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

func formatDuration(d time.Duration) string {
	parts := []string{}
	if h := int(d.Hours()); h > 0 {
		parts = append(parts, fmt.Sprintf("%dh", h))
		d -= time.Duration(h) * time.Hour
	}
	if m := int(d.Minutes()); m > 0 {
		parts = append(parts, fmt.Sprintf("%dm", m))
		d -= time.Duration(m) * time.Minute
	}
	s := int(d.Seconds())
	parts = append(parts, fmt.Sprintf("%ds", s))
	return strings.Join(parts, " ")
}
