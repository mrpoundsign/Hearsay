package ui

import (
	"fmt"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// LaunchStateTracker manages the visual checklist and progress during Hearsay startup.
type LaunchStateTracker struct {
	window       fyne.Window
	statusLabel  *widget.Label
	stepConfig   *widget.Label
	stepAudio    *widget.Label
	stepTTS      *widget.Label
	progress     *widget.ProgressBarInfinite
	downloadProg *widget.ProgressBar
	actionBox    *fyne.Container
	retryBtn     *widget.Button
	quitBtn      *widget.Button

	mu            sync.Mutex
	lastUpdate    time.Time
	downloadModel string
	onRetryFn     func()
	onQuitFn      func()
}

// NewLaunchWindow creates a centered launch status window for Hearsay.
func NewLaunchWindow(a fyne.App) (*LaunchStateTracker, fyne.Window) {
	w := a.NewWindow("Hearsay " + AppVersion + " — Launching")
	w.Resize(fyne.NewSize(480, 300))
	w.SetFixedSize(true)
	w.CenterOnScreen()

	title := widget.NewLabelWithStyle("🎧 Hearsay "+AppVersion, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	sub := widget.NewLabelWithStyle("Multi-Stream Accessibility Chat Reader", fyne.TextAlignCenter, fyne.TextStyle{Italic: true})

	dlProg := widget.NewProgressBar()
	dlProg.Hide()

	tracker := &LaunchStateTracker{
		window:       w,
		statusLabel:  widget.NewLabelWithStyle("Starting up subsystems...", fyne.TextAlignCenter, fyne.TextStyle{}),
		stepConfig:   widget.NewLabel("⏳ Reading configuration & saved library..."),
		stepAudio:    widget.NewLabel("⏳ Enumerating WASAPI stereo playback devices..."),
		stepTTS:      widget.NewLabel("⏳ Loading shared neural TTS engine..."),
		progress:     widget.NewProgressBarInfinite(),
		downloadProg: dlProg,
	}

	tracker.retryBtn = widget.NewButtonWithIcon("Retry Download", theme.ViewRefreshIcon(), func() {
		tracker.mu.Lock()
		retry := tracker.onRetryFn
		tracker.mu.Unlock()
		if retry != nil {
			fyne.Do(func() {
				tracker.actionBox.Hide()
				tracker.downloadProg.Hide()
				tracker.progress.Show()
				tracker.progress.Start()
				tracker.stepTTS.SetText("⏳ Loading shared neural TTS engine...")
			})
			go retry()
		}
	})
	tracker.retryBtn.Importance = widget.HighImportance

	tracker.quitBtn = widget.NewButtonWithIcon("Quit", theme.CancelIcon(), func() {
		tracker.mu.Lock()
		quit := tracker.onQuitFn
		tracker.mu.Unlock()
		if quit != nil {
			quit()
		}
	})

	tracker.actionBox = container.NewCenter(container.NewHBox(tracker.retryBtn, tracker.quitBtn))
	tracker.actionBox.Hide()

	content := container.NewVBox(
		title,
		sub,
		tracker.progress,
		tracker.downloadProg,
		tracker.statusLabel,
		widget.NewSeparator(),
		tracker.stepConfig,
		tracker.stepAudio,
		tracker.stepTTS,
		tracker.actionBox,
	)

	w.SetContent(content)
	return tracker, w
}

func (t *LaunchStateTracker) SetConfigStatus(done bool, detail string) {
	fyne.Do(func() {
		if done {
			t.stepConfig.SetText("✓ Configuration: " + detail)
		} else {
			t.stepConfig.SetText("⚠️ Configuration: " + detail)
		}
	})
}

func (t *LaunchStateTracker) SetAudioStatus(done bool, detail string) {
	fyne.Do(func() {
		if done {
			t.stepAudio.SetText("✓ Audio Playback: " + detail)
		} else {
			t.stepAudio.SetText("⚠️ Audio Playback: " + detail)
		}
	})
}

func (t *LaunchStateTracker) SetDownloadModel(name string) {
	t.mu.Lock()
	t.downloadModel = name
	t.mu.Unlock()
}

func (t *LaunchStateTracker) SetTTSStatus(done bool, detail string) {
	fyne.Do(func() {
		t.downloadProg.Hide()
		if !t.progress.Visible() {
			t.progress.Show()
		}
		if done {
			t.stepTTS.SetText("✓ Neural TTS: " + detail)
		} else {
			t.stepTTS.SetText("❌ Neural TTS: " + detail)
		}
	})
}

func (t *LaunchStateTracker) SetDownloadProgress(current, total int64) {
	now := time.Now()
	t.mu.Lock()
	if current < total && now.Sub(t.lastUpdate) < 50*time.Millisecond {
		t.mu.Unlock()
		return
	}
	t.lastUpdate = now
	modelName := t.downloadModel
	if modelName == "" {
		modelName = "Voice"
	}
	t.mu.Unlock()

	curMB := float64(current) / (1024 * 1024)
	totMB := float64(total) / (1024 * 1024)
	ratio := 0.0
	if total > 0 {
		ratio = min(float64(current)/float64(total), 1.0)
	}

	fyne.Do(func() {
		if t.progress.Visible() {
			t.progress.Stop()
			t.progress.Hide()
		}
		if !t.downloadProg.Visible() {
			t.downloadProg.Show()
		}
		t.downloadProg.SetValue(ratio)
		msg := fmt.Sprintf("Downloading & extracting %s models... %.1f / %.1f MB", modelName, curMB, totMB)
		t.statusLabel.SetText(msg)
		t.stepTTS.SetText(fmt.Sprintf("⏳ TTS (%s): Downloading (%.0f%%)...", modelName, ratio*100))
	})
}

func (t *LaunchStateTracker) ShowFatalError(statusMsg string, ttsDetail string, onRetry func(), onQuit func()) {
	t.mu.Lock()
	t.onRetryFn = onRetry
	t.onQuitFn = onQuit
	t.mu.Unlock()

	fyne.Do(func() {
		t.progress.Stop()
		t.progress.Hide()
		t.downloadProg.Hide()
		t.statusLabel.SetText(statusMsg)
		t.stepTTS.SetText("❌ Neural TTS: " + ttsDetail)
		t.actionBox.Show()
	})
}

func (t *LaunchStateTracker) SetStatus(msg string) {
	fyne.Do(func() {
		t.statusLabel.SetText(msg)
	})
}
