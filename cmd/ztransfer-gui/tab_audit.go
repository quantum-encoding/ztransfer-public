//go:build darwin || linux

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/quantum-encoding/ztransfer/pkg/audit"
)

// BuildAuditTab creates the session audit log viewer and verifier.
func (c *Controller) BuildAuditTab(w fyne.Window) fyne.CanvasObject {
	// === Verification result ===
	verifyResult := widget.NewLabelWithStyle("No log loaded", fyne.TextAlignLeading, fyne.TextStyle{Monospace: true})
	sessionID := widget.NewLabel("—")
	operatorLabel := widget.NewLabel("—")
	targetLabel := widget.NewLabel("—")
	eventCount := widget.NewLabel("—")
	startLabel := widget.NewLabel("—")
	endLabel := widget.NewLabel("—")

	// === Event list ===
	var events []audit.Event

	eventList := widget.NewList(
		func() int { return len(events) },
		func() fyne.CanvasObject {
			return container.NewHBox(
				widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Monospace: true}),
				widget.NewLabel(""),
				widget.NewLabel(""),
			)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			if id >= len(events) {
				return
			}
			evt := events[id]
			box := obj.(*fyne.Container)
			box.Objects[0].(*widget.Label).SetText(fmt.Sprintf("#%d", evt.Sequence))
			box.Objects[1].(*widget.Label).SetText(string(evt.EventType))

			desc := evt.Description
			if desc == "" {
				if evt.Command != "" {
					desc = evt.Command
				} else if evt.FileName != "" {
					desc = fmt.Sprintf("%s %s", evt.Direction, evt.FileName)
				} else if evt.ErrorMsg != "" {
					desc = evt.ErrorMsg
				}
			}
			box.Objects[2].(*widget.Label).SetText(desc)
		},
	)

	// === Event detail ===
	detailText := widget.NewMultiLineEntry()
	detailText.Disable()
	detailText.TextStyle = fyne.TextStyle{Monospace: true}
	detailText.SetMinRowsVisible(8)

	eventList.OnSelected = func(id widget.ListItemID) {
		if id >= len(events) {
			return
		}
		evt := events[id]
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Sequence:      %d\n", evt.Sequence))
		sb.WriteString(fmt.Sprintf("Type:          %s\n", evt.EventType))
		sb.WriteString(fmt.Sprintf("Timestamp:     %s\n", evt.Timestamp.Format("2006-01-02 15:04:05 UTC")))
		sb.WriteString(fmt.Sprintf("Actor:         %s\n", evt.ActorID))
		if evt.TargetID != "" {
			sb.WriteString(fmt.Sprintf("Target:        %s\n", evt.TargetID))
		}
		if evt.Description != "" {
			sb.WriteString(fmt.Sprintf("Description:   %s\n", evt.Description))
		}
		if evt.Command != "" {
			sb.WriteString(fmt.Sprintf("Command:       %s\n", evt.Command))
		}
		if evt.ExitCode != nil {
			sb.WriteString(fmt.Sprintf("Exit Code:     %d\n", *evt.ExitCode))
		}
		if evt.ByteCount > 0 {
			sb.WriteString(fmt.Sprintf("Bytes:         %s\n", formatBytes(evt.ByteCount)))
		}
		if evt.FileName != "" {
			sb.WriteString(fmt.Sprintf("File:          %s (%s)\n", evt.FileName, evt.Direction))
		}
		if evt.ErrorMsg != "" {
			sb.WriteString(fmt.Sprintf("Error:         %s\n", evt.ErrorMsg))
		}
		sb.WriteString(fmt.Sprintf("\nHash:          %s\n", evt.Hash))
		sb.WriteString(fmt.Sprintf("Previous Hash: %s\n", evt.PreviousHash))
		if evt.Verify() {
			sb.WriteString("Integrity:     ✓ VALID")
		} else {
			sb.WriteString("Integrity:     ✗ TAMPERED")
		}

		detailText.SetText(sb.String())
	}

	// === Load and verify ===
	loadAndVerify := func(path string) {
		data, err := os.ReadFile(path)
		if err != nil {
			verifyResult.SetText("Error: " + err.Error())
			return
		}

		parsed, err := audit.ParseEvents(data)
		if err != nil {
			verifyResult.SetText("Parse error: " + err.Error())
			return
		}

		events = parsed
		eventList.Refresh()

		if len(events) == 0 {
			verifyResult.SetText("Empty log file")
			return
		}

		idx, verifyErr := audit.VerifyChain(events)
		if verifyErr != nil {
			verifyResult.SetText(fmt.Sprintf("✗ CHAIN BROKEN at event %d: %v", idx, verifyErr))
		} else {
			verifyResult.SetText(fmt.Sprintf("✓ VERIFIED — %d events, chain intact", len(events)))
		}

		sessionID.SetText(events[0].SessionID)
		operatorLabel.SetText(events[0].ActorID)
		targetLabel.SetText(events[0].TargetID)
		eventCount.SetText(fmt.Sprintf("%d", len(events)))
		startLabel.SetText(events[0].Timestamp.Format("2006-01-02 15:04:05"))
		if len(events) > 1 {
			endLabel.SetText(events[len(events)-1].Timestamp.Format("2006-01-02 15:04:05"))
		}

		// Summary stats
		var cmds, files, errors int
		for _, evt := range events {
			switch evt.EventType {
			case audit.EventCommandExec:
				cmds++
			case audit.EventFileTransfer:
				files++
			case audit.EventError:
				errors++
			}
		}
		c.SetStatus(fmt.Sprintf("Audit: %d events, %d commands, %d files, %d errors — %s",
			len(events), cmds, files, errors, filepath.Base(path)))
	}

	openBtn := widget.NewButtonWithIcon("Open Log File", theme.FolderOpenIcon(), func() {
		fd := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
			if err != nil || reader == nil {
				return
			}
			reader.Close()
			path := reader.URI().Path()
			loadAndVerify(path)
		}, w)
		fd.Show()
	})
	openBtn.Importance = widget.HighImportance

	// === Layout ===
	summaryPanel := panelWithTitle("Verification", container.NewVBox(
		verifyResult,
		widget.NewSeparator(),
		container.New(layout.NewFormLayout(),
			widget.NewLabel("Session:"), sessionID,
			widget.NewLabel("Operator:"), operatorLabel,
			widget.NewLabel("Target:"), targetLabel,
			widget.NewLabel("Events:"), eventCount,
			widget.NewLabel("Start:"), startLabel,
			widget.NewLabel("End:"), endLabel,
		),
	))

	topBar := container.NewHBox(
		openBtn,
		layout.NewSpacer(),
		widget.NewLabelWithStyle("Session Audit Log Viewer", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		layout.NewSpacer(),
	)

	// Event list panel
	listPanel := panelWithTitle("Events", eventList)

	// Detail panel
	detailPanel := panelWithTitle("Event Detail", container.NewVScroll(detailText))

	// Left: event list, Right: summary + detail
	bottomRight := container.NewVSplit(summaryPanel, detailPanel)
	bottomRight.SetOffset(0.4)

	mainSplit := container.NewHSplit(listPanel, bottomRight)
	mainSplit.SetOffset(0.4)

	return container.NewBorder(topBar, nil, nil, nil, mainSplit)
}
