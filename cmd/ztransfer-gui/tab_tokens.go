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

	"github.com/quantum-encoding/ztransfer/pkg/auth"
)

// BuildTokensTab creates the authentication and token minting interface.
func (c *Controller) BuildTokensTab() fyne.CanvasObject {
	// =================================================================
	// Google Login section
	// =================================================================
	loginStatusLabel := widget.NewLabel("")
	loginStatusLabel.TextStyle = fyne.TextStyle{Monospace: true}

	emailLabel := widget.NewLabel("Not logged in")
	emailLabel.TextStyle = fyne.TextStyle{Bold: true}

	expiryLabel := widget.NewLabel("")

	// Check for existing credentials on load.
	if creds, err := auth.LoadCredentials(); err == nil {
		emailLabel.SetText(creds.Email)
		if !creds.Expiry.IsZero() {
			expiryLabel.SetText("Token expires: " + creds.Expiry.Format("15:04 02 Jan 2006"))
		}
		loginStatusLabel.SetText("✓ Logged in")
	}

	var loginBtn *widget.Button
	var logoutBtn *widget.Button

	loginBtn = widget.NewButtonWithIcon("Sign in with Google", theme.LoginIcon(), func() {
		loginStatusLabel.SetText("Opening browser...")
		loginBtn.Disable()

		go func() {
			creds, err := auth.RunLoginFlow()

			fyne.Do(func() {
				loginBtn.Enable()
				if err != nil {
					loginStatusLabel.SetText("Error: " + err.Error())
					return
				}

				emailLabel.SetText(creds.Email)
				if !creds.Expiry.IsZero() {
					expiryLabel.SetText("Token expires: " + creds.Expiry.Format("15:04 02 Jan 2006"))
				}
				loginStatusLabel.SetText("✓ Logged in as " + creds.Email)
				logoutBtn.Enable()
			})
		}()
	})
	loginBtn.Importance = widget.HighImportance

	logoutBtn = widget.NewButtonWithIcon("Sign out", theme.LogoutIcon(), func() {
		if creds, err := auth.LoadCredentials(); err == nil {
			auth.DeleteCredentials()
			loginStatusLabel.SetText("Signed out (" + creds.Email + ")")
		} else {
			loginStatusLabel.SetText("Not logged in")
		}
		emailLabel.SetText("Not logged in")
		expiryLabel.SetText("")
	})

	// Disable logout if not logged in.
	if _, err := auth.LoadCredentials(); err != nil {
		logoutBtn.Disable()
	}

	loginPanel := panelWithTitle("Google Login", container.NewVBox(
		container.New(layout.NewFormLayout(),
			widget.NewLabel("Account:"), emailLabel,
			widget.NewLabel(""), expiryLabel,
		),
		container.NewHBox(loginBtn, logoutBtn),
		loginStatusLabel,
	))

	// =================================================================
	// Token minting section (admin)
	// =================================================================
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

	tokenOutput := widget.NewMultiLineEntry()
	tokenOutput.SetMinRowsVisible(4)
	tokenOutput.TextStyle = fyne.TextStyle{Monospace: true}
	tokenOutput.Wrapping = fyne.TextWrapBreak
	tokenOutput.Disable()

	mintStatusLabel := widget.NewLabel("")
	mintStatusLabel.TextStyle = fyne.TextStyle{Monospace: true}

	sourceLabel := widget.NewLabel("—")
	saLabel := widget.NewLabel("—")
	audienceLabel := widget.NewLabel("—")

	mintBtn := widget.NewButtonWithIcon("Mint Token", theme.ConfirmIcon(), func() {
		scope := scopeSelect.Selected
		tokenType := typeSelect.Selected

		mintStatusLabel.SetText("Minting...")
		tokenOutput.SetText("")

		go func() {
			args := []string{"--scope", scope, "--type", tokenType, "-v"}
			cmd := exec.Command("ztransfer-mint", args...)
			out, err := cmd.CombinedOutput()
			output := string(out)

			fyne.Do(func() {
				if err != nil {
					mintStatusLabel.SetText("Error: " + err.Error())
					tokenOutput.SetText(output)
					return
				}

				lines := strings.Split(strings.TrimSpace(output), "\n")
				if len(lines) == 0 {
					mintStatusLabel.SetText("No output")
					return
				}

				token := lines[len(lines)-1]
				tokenOutput.SetText(token)
				mintStatusLabel.SetText(fmt.Sprintf("✓ Token minted (%d chars)", len(token)))

				for _, line := range lines[:len(lines)-1] {
					parts := strings.SplitN(line, ": ", 2)
					if len(parts) != 2 {
						continue
					}
					key := strings.TrimSpace(parts[0])
					val := strings.TrimSpace(parts[1])
					switch key {
					case "service_account":
						saLabel.SetText(val)
					case "audience":
						audienceLabel.SetText(val)
					}
				}

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

	copyBtn := widget.NewButtonWithIcon("Copy to Clipboard", theme.ContentCopyIcon(), func() {
		if tokenOutput.Text != "" {
			w := fyne.CurrentApp().Driver().AllWindows()
			if len(w) > 0 {
				w[0].Clipboard().SetContent(tokenOutput.Text)
				mintStatusLabel.SetText("✓ Copied to clipboard")
			}
		}
	})

	mintForm := panelWithTitle("Mint Token (Admin)", container.NewVBox(
		container.New(layout.NewFormLayout(),
			widget.NewLabel("Scope:"), scopeSelect,
			widget.NewLabel("Type:"), typeSelect,
		),
		container.NewHBox(mintBtn, copyBtn),
		mintStatusLabel,
	))

	tokenPanel := panelWithTitle("Token", tokenOutput)

	sourceInfo := panelWithTitle("Token Source", container.New(layout.NewFormLayout(),
		widget.NewLabel("Source:"), sourceLabel,
		widget.NewLabel("Service Account:"), saLabel,
		widget.NewLabel("Audience:"), audienceLabel,
	))

	// =================================================================
	// Reference
	// =================================================================
	scopeDesc := widget.NewRichTextFromMarkdown(
		"**Google Login** signs you in with your Google account.\n" +
			"Once logged in, relay connections are automatic.\n\n" +
			"**Token Minting** is for admin/automated workflows.\n\n" +
			"**Scopes:**\n" +
			"- `relay` — authenticate to Cloud Run relay\n" +
			"- `diagnostic` — read-only system inspection\n" +
			"- `repair` — full repair session access\n" +
			"- `full` — all permissions\n\n" +
			"**Token types:**\n" +
			"- `identity` — OIDC token for Cloud Run auth\n" +
			"- `access` — OAuth2 token for direct API calls",
	)

	// =================================================================
	// Layout
	// =================================================================
	leftPanel := container.NewVBox(
		loginPanel,
		widget.NewSeparator(),
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
