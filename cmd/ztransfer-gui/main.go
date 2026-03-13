package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
)

const appVersion = "0.2.0"

func init() {
	app.SetMetadata(fyne.AppMetadata{
		ID:      "com.quantumencoding.ztransfer",
		Name:    "ztransfer",
		Version: appVersion,
		Build:   1,
		Migrations: map[string]bool{
			"fyneDo": true,
		},
	})
}

func main() {
	a := app.NewWithID("com.quantumencoding.ztransfer")
	a.Settings().SetTheme(&ztransferTheme{variant: theme.VariantDark})

	w := a.NewWindow("ztransfer")
	w.Resize(fyne.NewSize(1100, 720))

	ctrl := NewController()

	tabs := container.NewAppTabs(
		container.NewTabItemWithIcon("Transfer", theme.DownloadIcon(), ctrl.BuildTransferTab(w)),
		container.NewTabItemWithIcon("Files", theme.FolderIcon(), ctrl.BuildFilesTab(w)),
		container.NewTabItemWithIcon("Remote", theme.MediaPlayIcon(), ctrl.BuildRemoteTab(w)),
		container.NewTabItemWithIcon("Server", theme.ComputerIcon(), ctrl.BuildServerTab(w)),
		container.NewTabItemWithIcon("Peers", theme.AccountIcon(), ctrl.BuildPeersTab()),
		container.NewTabItemWithIcon("Audit", theme.VisibilityIcon(), ctrl.BuildAuditTab(w)),
		container.NewTabItemWithIcon("Tokens", theme.LoginIcon(), ctrl.BuildTokensTab()),
		container.NewTabItemWithIcon("Settings", theme.SettingsIcon(), ctrl.BuildSettingsTab(a)),
	)
	tabs.SetTabLocation(container.TabLocationTop)

	w.SetContent(container.NewBorder(
		nil,
		ctrl.BuildStatusBar(),
		nil, nil,
		tabs,
	))

	w.SetOnDropped(ctrl.HandleDrop)

	w.SetOnClosed(func() {
		ctrl.Shutdown()
	})

	w.ShowAndRun()
}
