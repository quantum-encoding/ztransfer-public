package main

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/quantum-encoding/ztransfer/pkg/auth"
)

// BuildPeersTab creates the peer management interface.
func (c *Controller) BuildPeersTab() fyne.CanvasObject {
	// Detail labels
	nameValue := widget.NewLabel("—")
	addressValue := widget.NewLabel("—")
	fingerprintValue := widget.NewLabel("—")
	pairedValue := widget.NewLabel("—")

	// Peer list
	peerList := widget.NewList(
		func() int { return len(c.peerStore.ListPeers()) },
		func() fyne.CanvasObject {
			return container.NewHBox(
				widget.NewIcon(theme.AccountIcon()),
				container.NewVBox(
					widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
					widget.NewLabel(""),
				),
			)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			peers := c.peerStore.ListPeers()
			if id >= len(peers) {
				return
			}
			p := peers[id]
			box := obj.(*fyne.Container)
			inner := box.Objects[1].(*fyne.Container)
			inner.Objects[0].(*widget.Label).SetText(p.Name)
			inner.Objects[1].(*widget.Label).SetText(p.Address)
		},
	)

	peerList.OnSelected = func(id widget.ListItemID) {
		peers := c.peerStore.ListPeers()
		if id >= len(peers) {
			return
		}
		p := peers[id]
		nameValue.SetText(p.Name)
		addressValue.SetText(p.Address)
		fingerprintValue.SetText(p.Fingerprint)
		pairedValue.SetText(p.PairedAt.Format("2006-01-02 15:04"))
	}

	// Pair new peer form
	addrEntry := widget.NewEntry()
	addrEntry.SetPlaceHolder("192.168.1.50:9876")
	tokenEntry := widget.NewEntry()
	tokenEntry.SetPlaceHolder("ABC123")

	pairStatus := widget.NewLabel("")

	pairButton := widget.NewButtonWithIcon("Pair", theme.ConfirmIcon(), func() {
		addr := addrEntry.Text
		token := tokenEntry.Text
		if addr == "" || token == "" {
			pairStatus.SetText("Address and token required")
			return
		}

		pairStatus.SetText("Pairing...")
		go func() {
			err := auth.RequestPair(addr, token, c.identity, c.peerStore)
			fyne.Do(func() {
				if err != nil {
					pairStatus.SetText("Failed: " + err.Error())
				} else {
					pairStatus.SetText("Paired successfully!")
					peerList.Refresh()
					addrEntry.SetText("")
					tokenEntry.SetText("")
				}
			})
		}()
	})

	// Remove peer
	removeButton := widget.NewButtonWithIcon("Remove Selected", theme.DeleteIcon(), func() {
		selected := nameValue.Text
		if selected == "" || selected == "—" {
			return
		}
		c.peerStore.RemovePeer(selected)
		peerList.Refresh()
		nameValue.SetText("—")
		addressValue.SetText("—")
		fingerprintValue.SetText("—")
		pairedValue.SetText("—")
	})

	// === Layout ===
	detailSection := panelWithTitle("Peer Details", container.NewVBox(
		container.New(layout.NewFormLayout(),
			widget.NewLabel("Name:"), nameValue,
			widget.NewLabel("Address:"), addressValue,
			widget.NewLabel("Fingerprint:"), fingerprintValue,
			widget.NewLabel("Paired:"), pairedValue,
		),
		widget.NewSeparator(),
		removeButton,
	))

	pairSection := panelWithTitle("Pair New Peer", container.NewVBox(
		container.New(layout.NewFormLayout(),
			widget.NewLabel("Address:"), addrEntry,
			widget.NewLabel("Token:"), tokenEntry,
		),
		container.NewHBox(pairButton, pairStatus),
	))

	rightPanel := container.NewVBox(
		detailSection,
		layout.NewSpacer(),
		pairSection,
	)

	peerPanel := panelWithTitle("Known Peers", peerList)

	split := container.NewHSplit(peerPanel, rightPanel)
	split.SetOffset(0.35)

	// Suppress unused import
	_ = fmt.Sprintf
	_ = dialog.NewInformation

	return split
}
