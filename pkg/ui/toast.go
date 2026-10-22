//go:build windows

package ui

import (
	"log"

	"github.com/go-toast/toast"
)

// ponytail: Windows drops toasts from unregistered AppIDs, so borrow PowerShell's
// (toast header says "Windows PowerShell"). Register an own AppID via a Start Menu
// shortcut if branding matters.
const appID = `{1AC14E77-02E7-4E5D-B744-2EB1AE5198B7}\WindowsPowerShell\v1.0\powershell.exe`

// Notify shows a toast. It spawns PowerShell (~1s), so call it with `go`.
func Notify(title, message string) {
	n := toast.Notification{AppID: appID, Title: title, Message: message}
	if err := n.Push(); err != nil {
		log.Printf("ui: toast: %v", err)
	}
}
