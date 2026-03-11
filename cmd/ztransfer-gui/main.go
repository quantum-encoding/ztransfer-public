package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
)

const appVersion = "0.1.0"

func main() {
	a := app.NewWithID("com.quantumencoding.ztransfer")
	a.Settings().SetTheme(&ztransferTheme{variant: theme.VariantDark})

	w := a.NewWindow("ztransfer")
	w.Resize(fyne.NewSize(960, 640))

	ctrl := NewController()

	tabs := container.NewAppTabs(
		container.NewTabItemWithIcon("Transfer", theme.DownloadIcon(), ctrl.BuildTransferTab(w)),
		container.NewTabItemWithIcon("Server", theme.ComputerIcon(), ctrl.BuildServerTab(w)),
		container.NewTabItemWithIcon("Peers", theme.AccountIcon(), ctrl.BuildPeersTab()),
		container.NewTabItemWithIcon("Settings", theme.SettingsIcon(), ctrl.BuildSettingsTab(a)),
	)
	tabs.SetTabLocation(container.TabLocationTop)

	w.SetContent(container.NewBorder(
		nil,
		ctrl.BuildStatusBar(),
		nil, nil,
		tabs,
	))

	w.SetOnClosed(func() {
		ctrl.Shutdown()
	})

	w.ShowAndRun()
}
