package ui

import (
	"context"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"

	"github.com/mrpoundsign/appinstaller/pkg/updater"
	"github.com/mrpoundsign/appinstaller/pkg/updaterui"

	"github.com/mrpoundsign/Hearsay/internal/config"
	"github.com/mrpoundsign/Hearsay/internal/listener"
)

// NewHearsayUpdater returns a GitHub Releases Checker configured for mrpoundsign/Hearsay.
func NewHearsayUpdater() *updater.Checker {
	return updater.NewChecker("mrpoundsign", "Hearsay")
}

// ShowHearsayUpdateDialog opens the Fyne Update Available dialog for a discovered release.
func ShowHearsayUpdateDialog(parent fyne.Window, session *listener.Session, rel *updater.ReleaseInfo) {
	if parent == nil || rel == nil {
		return
	}
	checker := NewHearsayUpdater()
	updaterui.ShowUpdateDialog(updaterui.DialogConfig{
		Parent:         parent,
		AppName:        "Hearsay",
		CurrentVersion: AppVersion,
		Release:        rel,
		Checker:        checker,
		OnSkipVersion: func(ver string) {
			if session != nil {
				_ = session.UpdateConfig(func(c *config.Config) {
					c.Update.SkippedVersion = ver
				})
			}
		},
		OnQuitApp: func() {
			if session != nil {
				_ = session.Close()
			}
			if a := fyne.CurrentApp(); a != nil {
				a.Quit()
			}
		},
	})
}

// TriggerManualUpdateCheck queries GitHub Releases asynchronously and either notifies the user
// that Hearsay is up to date or opens the Update Available dialog.
func TriggerManualUpdateCheck(parent fyne.Window, session *listener.Session, onStatus func(msg string)) {
	if testing.Testing() {
		return
	}
	if onStatus != nil {
		onStatus("Checking GitHub Releases for updates...")
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()

		checker := NewHearsayUpdater()
		rel, isNewer, err := checker.CheckLatest(ctx, AppVersion)
		fyne.Do(func() {
			if err != nil {
				msg := "⚠️ Update check failed: " + err.Error()
				if onStatus != nil {
					onStatus(msg)
				}
				return
			}
			if !isNewer || rel == nil {
				msg := "✓ Hearsay " + AppVersion + " is up to date."
				if onStatus != nil {
					onStatus(msg)
				} else if parent != nil {
					dialog.ShowInformation("Up to Date", msg, parent)
				}
				return
			}

			if onStatus != nil {
				onStatus("⬆ Update available: Hearsay v" + rel.Version)
			}
			ShowHearsayUpdateDialog(parent, session, rel)
		})
	}()
}
