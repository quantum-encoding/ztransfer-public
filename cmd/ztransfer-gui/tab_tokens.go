//go:build darwin || linux

package main

import (
	"fmt"
	"os/exec"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// BuildTokensTab creates the token minting interface.
func (c *Controller) BuildTokensTab() fyne.CanvasObject {
	// === Scope selector ===
	scopeSelect := widget.NewSelect(
		[]string{"relay", "diagnostic", "repair", "full"},
		nil,
	)
	scopeSelect.SetSelected("relay")

	typeSelect := widget.NewSelect(
		[]string{"identity", "access"},
		nil,
	)
	typeSelect.SetSelected("identity")

	// === Token output ===
	tokenOutput := widget.NewMultiLineEntry()
	tokenOutput.SetMinRowsVisible(4)
	tokenOutput.TextStyle = fyne.TextStyle{Monospace: true}
	tokenOutput.Wrapping = fyne.TextWrapBreak
	tokenOutput.Disable()

	statusLabel := widget.NewLabel("")
	statusLabel.TextStyle = fyne.TextStyle{Monospace: true}

	// === Token source info ===
	sourceLabel := widget.NewLabel("—")
	saLabel := widget.NewLabel("—")
	audienceLabel := widget.NewLabel("—")

	// === Mint button ===
	mintBtn := widget.NewButtonWithIcon("Mint Token", theme.ConfirmIcon(), func() {
		scope := scopeSelect.Selected
		tokenType := typeSelect.Selected

		statusLabel.SetText("Minting...")
		tokenOutput.SetText("")

		go func() {
			args := []string{"--scope", scope, "--type", tokenType, "-v"}
			cmd := exec.Command("ztransfer-mint", args...)
			out, err := cmd.CombinedOutput()
			output := string(out)

			fyne.Do(func() {
				if err != nil {
					statusLabel.SetText("Error: " + err.Error())
					tokenOutput.SetText(output)
					return
				}

				// Parse verbose output — last line is the token, lines before are info
				lines := strings.Split(strings.TrimSpace(output), "\n")
				if len(lines) == 0 {
					statusLabel.SetText("No output")
					return
				}

				token := lines[len(lines)-1]
				tokenOutput.SetText(token)
				statusLabel.SetText(fmt.Sprintf("✓ Token minted (%d chars)", len(token)))

				// Parse info lines
				for _, line := range lines[:len(lines)-1] {
					parts := strings.SplitN(line, ": ", 2)
					if len(parts) != 2 {
						continue
					}
					key := strings.TrimSpace(parts[0])
					val := strings.TrimSpace(parts[1])
					switch key {
					case "scope":
						// already shown in selector
					case "service_account":
						saLabel.SetText(val)
					case "audience":
						audienceLabel.SetText(val)
					case "type":
						// already shown
					default:
						if strings.Contains(line, "metadata") {
							sourceLabel.SetText("GCP Metadata Server")
						} else if strings.Contains(line, "ADC") {
							sourceLabel.SetText("Application Default Credentials")
						} else if strings.Contains(line, "gcloud") {
							sourceLabel.SetText("gcloud CLI")
						}
					}
				}

				// Detect source from output
				if strings.Contains(output, "IAM failed") && strings.Contains(output, "gcloud") {
					sourceLabel.SetText("gcloud CLI (IAM fallback)")
				} else if strings.Contains(output, "metadata") {
					sourceLabel.SetText("GCP Metadata Server")
				} else if strings.Contains(output, "ADC") {
					sourceLabel.SetText("Application Default Credentials")
				}
			})
		}()
	})
	mintBtn.Importance = widget.HighImportance

	// === Copy button ===
	copyBtn := widget.NewButtonWithIcon("Copy to Clipboard", theme.ContentCopyIcon(), func() {
		if tokenOutput.Text != "" {
			w := fyne.CurrentApp().Driver().AllWindows()
			if len(w) > 0 {
				w[0].Clipboard().SetContent(tokenOutput.Text)
				statusLabel.SetText("✓ Copied to clipboard")
			}
		}
	})

	// === Scope descriptions ===
	scopeDesc := widget.NewRichTextFromMarkdown(
		"**Scopes:**\n" +
			"- `relay` — authenticate to Cloud Run relay\n" +
			"- `diagnostic` — read-only system inspection\n" +
			"- `repair` — full repair session access\n" +
			"- `full` — all permissions\n\n" +
			"**Token types:**\n" +
			"- `identity` — OIDC token for Cloud Run auth\n" +
			"- `access` — OAuth2 token for direct API calls",
	)

	// === Layout ===
	mintForm := panelWithTitle("Mint Token", container.NewVBox(
		container.New(layout.NewFormLayout(),
			widget.NewLabel("Scope:"), scopeSelect,
			widget.NewLabel("Type:"), typeSelect,
		),
		container.NewHBox(mintBtn, copyBtn),
		statusLabel,
	))

	tokenPanel := panelWithTitle("Token", tokenOutput)

	sourceInfo := panelWithTitle("Token Source", container.New(layout.NewFormLayout(),
		widget.NewLabel("Source:"), sourceLabel,
		widget.NewLabel("Service Account:"), saLabel,
		widget.NewLabel("Audience:"), audienceLabel,
	))

	leftPanel := container.NewVBox(
		mintForm,
		widget.NewSeparator(),
		tokenPanel,
		widget.NewSeparator(),
		sourceInfo,
	)

	rightPanel := panelWithTitle("Reference", scopeDesc)

	split := container.NewHSplit(leftPanel, rightPanel)
	split.SetOffset(0.6)

	return split
}
