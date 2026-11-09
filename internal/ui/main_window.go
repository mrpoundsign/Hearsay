package ui

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/mrpoundsign/Hearsay/internal/config"
	"github.com/mrpoundsign/Hearsay/internal/listener"
	"github.com/mrpoundsign/TownCrier/pkg/audio"
)

// DeviceEnumerator lists available playback audio devices.
type DeviceEnumerator func() ([]audio.DeviceInfo, error)

// MainWindow manages the Hearsay desktop GUI, including the Full Radio Deck and Always-on-Top Mini Player.
type MainWindow struct {
	App              fyne.App
	Window           fyne.Window
	Session          *listener.Session
	deviceEnumerator DeviceEnumerator

	mu             sync.Mutex
	uiMu           sync.Mutex
	isMiniMode     bool
	suppressEvents bool
	lastStatusText string
	activeTipPopup *widget.PopUp
	activeTipBtn   *TooltipButton

	// Header & status widgets
	activeBadgeLbl *widget.Label
	nowSpeakingLbl *widget.Label
	fontScaleSel   *widget.Select
	highContrastCk *widget.Check
	miniToggleBtn  *TooltipButton
	panicBtn       *TooltipButton

	// Quick-Tune widgets
	PlatformSelect *widget.Select
	QuickTuneEntry *widget.Entry
	TuneInBtn      *TooltipButton
	tuneErrorLbl   *widget.Label

	// Active Mixer & Saved Library containers
	mixerCardsBox     *fyne.Container
	savedStreamersBox *fyne.Container
	lineupsBox        *fyne.Container

	// Transport & Live Log widgets
	deviceSelect *widget.Select
	deviceMap    map[string]string
	masterVolSld *widget.Slider
	masterVolLbl *widget.Label
	speedSld     *widget.Slider
	speedLbl     *widget.Label
	pauseBtn     *TooltipButton
	skipBtn      *TooltipButton
	replayBtn    *TooltipButton
	clearQBtn    *TooltipButton
	logEntries   []listener.LogEntry
	logList      *widget.List

	// Layouts for switching between Full Deck and Mini Player
	fullRoot fyne.CanvasObject
	miniRoot fyne.CanvasObject

	// Global Hotkey Managers
	hkPause  *GlobalHotkeyManager
	hkSkip   *GlobalHotkeyManager
	hkReplay *GlobalHotkeyManager
	hkPanic  *GlobalHotkeyManager
}

// NewMainWindow constructs and wires the Hearsay MainWindow.
func NewMainWindow(a fyne.App, session *listener.Session, enumFn ...DeviceEnumerator) *MainWindow {
	cfg := session.ConfigSnapshot()
	a.Settings().SetTheme(NewHearsayTheme(cfg.Accessibility.FontScale, cfg.Accessibility.HighContrast))

	w := a.NewWindow("Hearsay " + AppVersion + " — Stream Accessibility Chat Reader")
	w.Resize(fyne.NewSize(1140, 740))

	devEnum := audio.EnumeratePlaybackDevices
	if len(enumFn) > 0 && enumFn[0] != nil {
		devEnum = enumFn[0]
	}

	mw := &MainWindow{
		App:              a,
		Window:           w,
		Session:          session,
		deviceEnumerator: devEnum,
		deviceMap:        make(map[string]string),
		lastStatusText:   "Idle — Ready to tune in",
	}

	mw.buildWidgets(cfg)
	mw.registerHotkeys(cfg.Accessibility.Hotkeys)

	session.SetCallbacks(
		func() {
			fyne.Do(func() {
				mw.refreshLog()
			})
		},
		func() {
			fyne.Do(func() {
				mw.refreshMixerAndLibrary()
			})
		},
		func(entry *listener.LogEntry) {
			fyne.Do(func() {
				mw.updateNowSpeaking(entry)
			})
		},
	)

	w.SetOnClosed(func() {
		mw.closeHotkeys()
	})

	mw.refreshMixerAndLibrary()
	mw.refreshLog()
	w.SetContent(mw.fullRoot)
	return mw
}

func (mw *MainWindow) newTipBtn(label string, icon fyne.Resource, tooltip string, tapped func()) *TooltipButton {
	return NewTooltipButton(label, icon, tooltip, mw.showTooltip, mw.hideTooltip, func() {
		mw.hideTooltip(nil)
		if tapped != nil {
			tapped()
		}
	})
}

func (mw *MainWindow) showTooltip(btn *TooltipButton, text string) {
	mw.uiMu.Lock()
	defer mw.uiMu.Unlock()

	if mw.Window == nil || text == "" {
		return
	}
	mw.hideTooltipLocked(nil)
	mw.activeTipBtn = btn
	if mw.nowSpeakingLbl != nil {
		mw.nowSpeakingLbl.SetText("Tip: " + text)
	}

	c := mw.Window.Canvas()
	if c == nil {
		return
	}
	tipLbl := widget.NewLabelWithStyle(text, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	bg := canvas.NewRectangle(colorSlateSurface)
	bg.StrokeColor = colorEmeraldMint
	bg.StrokeWidth = 1
	bg.CornerRadius = 4
	box := container.NewStack(bg, container.NewPadded(tipLbl))

	pop := widget.NewPopUp(box, c)
	btnPos := fyne.CurrentApp().Driver().AbsolutePositionForObject(btn)
	popSize := pop.MinSize()
	canvasSize := c.Size()

	x := btnPos.X
	if x+popSize.Width > canvasSize.Width-8 {
		x = max(8, canvasSize.Width-popSize.Width-8)
	}
	y := btnPos.Y - popSize.Height - 4
	if y < 4 {
		y = btnPos.Y + btn.Size().Height + 4
	}
	pop.ShowAtPosition(fyne.NewPos(x, y))
	mw.activeTipPopup = pop
}

func (mw *MainWindow) hideTooltip(btn *TooltipButton) {
	mw.uiMu.Lock()
	defer mw.uiMu.Unlock()
	mw.hideTooltipLocked(btn)
}

func (mw *MainWindow) hideTooltipLocked(btn *TooltipButton) {
	if btn != nil && mw.activeTipBtn != nil && mw.activeTipBtn != btn {
		return
	}
	if mw.activeTipPopup != nil {
		mw.activeTipPopup.Hide()
		mw.activeTipPopup = nil
	}
	mw.activeTipBtn = nil
	if mw.nowSpeakingLbl != nil {
		mw.nowSpeakingLbl.SetText(mw.lastStatusText)
	}
}

func (mw *MainWindow) buildWidgets(cfg *config.Config) {
	// 1. Header Bar
	mw.activeBadgeLbl = widget.NewLabelWithStyle("Hearsay — 0 Active Streams", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	mw.nowSpeakingLbl = widget.NewLabelWithStyle(mw.lastStatusText, fyne.TextAlignLeading, fyne.TextStyle{Italic: true})
	mw.nowSpeakingLbl.Truncation = fyne.TextTruncateEllipsis

	scaleOptions := []string{"100%", "125%", "150%", "175%", "200%"}
	mw.fontScaleSel = widget.NewSelect(scaleOptions, func(val string) {
		if mw.suppressEvents {
			return
		}
		scale := parseFontScaleOption(val)
		_ = mw.Session.UpdateConfig(func(c *config.Config) {
			c.Accessibility.FontScale = scale
		})
		snap := mw.Session.ConfigSnapshot()
		mw.App.Settings().SetTheme(NewHearsayTheme(snap.Accessibility.FontScale, snap.Accessibility.HighContrast))
	})
	mw.suppressEvents = true
	mw.fontScaleSel.SetSelected(formatFontScaleOption(cfg.Accessibility.FontScale))
	mw.suppressEvents = false

	mw.highContrastCk = widget.NewCheck("High Contrast", func(checked bool) {
		if mw.suppressEvents {
			return
		}
		_ = mw.Session.UpdateConfig(func(c *config.Config) {
			c.Accessibility.HighContrast = checked
		})
		snap := mw.Session.ConfigSnapshot()
		mw.App.Settings().SetTheme(NewHearsayTheme(snap.Accessibility.FontScale, snap.Accessibility.HighContrast))
	})
	mw.suppressEvents = true
	mw.highContrastCk.SetChecked(cfg.Accessibility.HighContrast)
	mw.suppressEvents = false

	mw.miniToggleBtn = mw.newTipBtn(
		"Mini Player",
		theme.ViewRestoreIcon(),
		"Collapse into compact Always-on-Top Mini Player overlay",
		func() {
			mw.ToggleMiniMode()
		},
	)

	settingsBtn := mw.newTipBtn(
		"Settings & Voices",
		theme.SettingsIcon(),
		"Configure My Username(s), Crowd Control, Voices, Translation & Hotkeys",
		func() {
			mw.showSettingsDialog()
		},
	)

	mw.panicBtn = mw.newTipBtn(
		"PANIC MUTE",
		theme.VolumeMuteIcon(),
		"Emergency Mute: Immediately stop audio and flush speech queue (Ctrl+Shift+M)",
		func() {
			mw.Session.PanicMute()
			mw.updateNowSpeaking(nil)
		},
	)
	mw.panicBtn.Importance = widget.DangerImportance

	headerTop := container.NewBorder(
		nil, nil,
		mw.activeBadgeLbl,
		container.NewHBox(
			widget.NewLabel("Text:"),
			mw.fontScaleSel,
			mw.highContrastCk,
			mw.miniToggleBtn,
			settingsBtn,
			mw.panicBtn,
		),
		mw.nowSpeakingLbl,
	)

	// 2. Left Pane — Radio Deck
	mw.PlatformSelect = widget.NewSelect([]string{"Auto / URL", "Twitch", "YouTube", "Kick", "TikTok"}, nil)
	mw.PlatformSelect.SetSelected("Auto / URL")

	mw.QuickTuneEntry = widget.NewEntry()
	mw.QuickTuneEntry.SetPlaceHolder("Paste stream URL or type channel (e.g. twitch:mocomade, @handle)...")
	mw.QuickTuneEntry.OnSubmitted = func(_ string) {
		mw.handleTuneIn()
	}

	mw.TuneInBtn = mw.newTipBtn(
		"Tune In",
		theme.MediaPlayIcon(),
		"Connect to the entered stream URL or channel handle",
		func() {
			mw.handleTuneIn()
		},
	)
	mw.TuneInBtn.Importance = widget.HighImportance

	mw.tuneErrorLbl = widget.NewLabel("")
	mw.tuneErrorLbl.Hide()

	quickTuneHeader := widget.NewLabelWithStyle("Quick Tune — Add Live Stream", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	quickTuneHeader.Truncation = fyne.TextTruncateEllipsis

	quickTuneBox := container.NewVBox(
		quickTuneHeader,
		container.NewBorder(nil, nil, mw.PlatformSelect, mw.TuneInBtn, mw.QuickTuneEntry),
		mw.tuneErrorLbl,
	)

	mw.mixerCardsBox = container.NewVBox()
	mixerScroll := container.NewVScroll(mw.mixerCardsBox)
	mixerScroll.SetMinSize(fyne.NewSize(260, 200))

	mw.savedStreamersBox = container.NewVBox()
	mw.lineupsBox = container.NewVBox()

	addSavedStreamerBtn := mw.newTipBtn(
		"Add Saved Streamer",
		theme.ContentAddIcon(),
		"Bookmark a creator with one or more platform channels",
		func() {
			mw.showAddSavedStreamerDialog()
		},
	)
	saveLineupBtn := mw.newTipBtn(
		"Save Active Streams as Lineup",
		theme.DocumentSaveIcon(),
		"Save all currently active streams and their Left/Right stereo panning as a 1-click preset",
		func() {
			mw.showSaveLineupDialog()
		},
	)

	savedStreamersTab := container.NewBorder(
		container.NewPadded(addSavedStreamerBtn),
		nil, nil, nil,
		container.NewVScroll(mw.savedStreamersBox),
	)
	lineupsTab := container.NewBorder(
		container.NewPadded(saveLineupBtn),
		nil, nil, nil,
		container.NewVScroll(mw.lineupsBox),
	)

	libraryTabs := container.NewAppTabs(
		container.NewTabItemWithIcon("Saved Streamers", theme.AccountIcon(), savedStreamersTab),
		container.NewTabItemWithIcon("Lineups / Co-Streams", theme.ListIcon(), lineupsTab),
	)

	mixerHeader := widget.NewLabelWithStyle("Active Streams Mixer (Volume, Stereo Pan & Crowd Filter)", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	mixerHeader.Truncation = fyne.TextTruncateEllipsis

	leftSplit := container.NewVSplit(
		container.NewBorder(
			container.NewVBox(quickTuneBox, widget.NewSeparator(), mixerHeader),
			nil, nil, nil,
			mixerScroll,
		),
		libraryTabs,
	)
	leftSplit.Offset = 0.58

	// 3. Right Pane — Transport & Unified Live Log (compact min-width so HSplit is freely resizable)
	mw.buildTransportWidgets(cfg)
	mw.buildLiveLogWidget()

	logHeader := widget.NewLabelWithStyle("Unified Live Log (Hover any action button for tooltip)", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	logHeader.Truncation = fyne.TextTruncateEllipsis

	rightPane := container.NewBorder(
		container.NewVBox(
			mw.buildTransportBar(),
			widget.NewSeparator(),
			logHeader,
		),
		nil, nil, nil,
		mw.logList,
	)

	mainSplit := container.NewHSplit(leftSplit, rightPane)
	mainSplit.Offset = 0.52

	mw.fullRoot = container.NewBorder(
		container.NewVBox(headerTop, widget.NewSeparator()),
		nil, nil, nil,
		mainSplit,
	)

	mw.miniRoot = mw.buildMiniPlayerRoot()
}

func (mw *MainWindow) buildTransportWidgets(cfg *config.Config) {
	devOptions := []string{"System Default"}
	mw.deviceMap["System Default"] = ""
	selectedDevLabel := "System Default"

	if mw.deviceEnumerator != nil {
		if devs, err := mw.deviceEnumerator(); err == nil {
			for _, d := range devs {
				label := d.Name
				if d.IsDefault {
					label += " (Default)"
				}
				devOptions = append(devOptions, label)
				mw.deviceMap[label] = d.ID
				if cfg.Audio.OutputDeviceID != "" && d.ID == cfg.Audio.OutputDeviceID {
					selectedDevLabel = label
				}
			}
		}
	}

	mw.deviceSelect = widget.NewSelect(devOptions, func(label string) {
		if mw.suppressEvents {
			return
		}
		devID := mw.deviceMap[label]
		_ = mw.Session.UpdateConfig(func(c *config.Config) {
			c.Audio.OutputDeviceID = devID
		})
		if p := mw.Session.Player(); p != nil {
			_ = p.SwitchDevice(devID)
		}
	})
	mw.suppressEvents = true
	mw.deviceSelect.SetSelected(selectedDevLabel)
	mw.suppressEvents = false

	mw.masterVolLbl = widget.NewLabel(fmt.Sprintf("Vol: %.0f%%", cfg.Audio.MasterVolume*100))
	mw.masterVolSld = widget.NewSlider(0, 200)
	mw.masterVolSld.Step = 5
	mw.masterVolSld.Value = cfg.Audio.MasterVolume * 100
	mw.masterVolSld.OnChanged = func(val float64) {
		vol := val / 100.0
		mw.masterVolLbl.SetText(fmt.Sprintf("Vol: %.0f%%", val))
		_ = mw.Session.UpdateConfig(func(c *config.Config) {
			c.Audio.MasterVolume = vol
		})
	}

	mw.speedLbl = widget.NewLabel(fmt.Sprintf("Speed: %.1fx", cfg.Audio.DefaultSpeed))
	mw.speedSld = widget.NewSlider(0.5, 2.0)
	mw.speedSld.Step = 0.1
	mw.speedSld.Value = cfg.Audio.DefaultSpeed
	mw.speedSld.OnChanged = func(val float64) {
		mw.speedLbl.SetText(fmt.Sprintf("Speed: %.1fx", val))
		_ = mw.Session.UpdateConfig(func(c *config.Config) {
			c.Audio.DefaultSpeed = val
		})
	}

	mw.pauseBtn = mw.newTipBtn(
		"Pause",
		theme.MediaPauseIcon(),
		"Pause or resume TTS speech queue (Ctrl+Shift+P)",
		func() {
			paused := mw.Session.TogglePause()
			if paused {
				mw.pauseBtn.SetText("Resume")
				mw.pauseBtn.SetIcon(theme.MediaPlayIcon())
			} else {
				mw.pauseBtn.SetText("Pause")
				mw.pauseBtn.SetIcon(theme.MediaPauseIcon())
			}
		},
	)

	mw.skipBtn = mw.newTipBtn(
		"Skip",
		theme.MediaSkipNextIcon(),
		"Skip the currently speaking message (Ctrl+Shift+S)",
		func() {
			mw.Session.SkipCurrent()
		},
	)

	mw.replayBtn = mw.newTipBtn(
		"Replay Last",
		theme.MediaReplayIcon(),
		"Replay the last spoken message aloud (Ctrl+Shift+R)",
		func() {
			mw.Session.ReplayLast()
		},
	)

	mw.clearQBtn = mw.newTipBtn(
		"Clear Queue",
		theme.DeleteIcon(),
		"Clear all pending messages waiting in the speech queue",
		func() {
			mw.Session.ClearQueue()
		},
	)
}

func (mw *MainWindow) buildTransportBar() fyne.CanvasObject {
	outputRow := container.NewBorder(nil, nil, widget.NewLabel("Output:"), nil, mw.deviceSelect)
	volBox := container.NewBorder(nil, nil, mw.masterVolLbl, nil, mw.masterVolSld)
	speedBox := container.NewBorder(nil, nil, mw.speedLbl, nil, mw.speedSld)
	slidersRow := container.NewGridWithColumns(2, volBox, speedBox)
	buttonsRow := container.NewHBox(
		mw.pauseBtn,
		mw.skipBtn,
		mw.replayBtn,
		mw.clearQBtn,
	)
	return container.NewVBox(outputRow, slidersRow, buttonsRow)
}

func (mw *MainWindow) buildLiveLogWidget() {
	mw.logList = widget.NewList(
		func() int {
			mw.mu.Lock()
			defer mw.mu.Unlock()
			return len(mw.logEntries)
		},
		func() fyne.CanvasObject {
			msgLbl := widget.NewLabel("Message template")
			msgLbl.Truncation = fyne.TextTruncateEllipsis
			replayBtn := mw.newTipBtn("", theme.MediaReplayIcon(), "Replay this message aloud", nil)
			friendBtn := mw.newTipBtn("VIP", nil, "Toggle Priority Friend (always spoken, bypasses Crowd Control)", nil)
			voiceBtn := mw.newTipBtn("Voice", nil, "Assign a specific neural voice to this chatter", nil)
			muteBtn := mw.newTipBtn("", theme.VolumeMuteIcon(), "Mute / ignore all future messages from this chatter", nil)
			actions := container.NewHBox(replayBtn, friendBtn, voiceBtn, muteBtn)
			return container.NewBorder(nil, nil, nil, actions, msgLbl)
		},
		func(id widget.ListItemID, item fyne.CanvasObject) {
			mw.mu.Lock()
			idx := len(mw.logEntries) - 1 - id
			if idx < 0 || idx >= len(mw.logEntries) {
				mw.mu.Unlock()
				return
			}
			entry := mw.logEntries[idx]
			mw.mu.Unlock()

			border := item.(*fyne.Container)
			msgLbl := border.Objects[0].(*widget.Label)
			actions := border.Objects[1].(*fyne.Container)

			replayBtn := actions.Objects[0].(*TooltipButton)
			friendBtn := actions.Objects[1].(*TooltipButton)
			voiceBtn := actions.Objects[2].(*TooltipButton)
			muteBtn := actions.Objects[3].(*TooltipButton)

			badge := formatLogBadge(entry)
			panTag := formatPanBadge(entry.Pan)
			msgLbl.SetText(fmt.Sprintf("%s [%s %s] %s: %s", badge, entry.Channel, panTag, entry.UserName, entry.Text))

			entryID := entry.ID
			chatter := entry.UserName
			replayBtn.SetTooltip(fmt.Sprintf("Replay message from %s aloud", chatter))
			replayBtn.OnTapped = func() {
				mw.hideTooltip(nil)
				mw.Session.ReplayEntry(entryID)
			}
			friendBtn.SetTooltip(fmt.Sprintf("Toggle %s as Priority VIP/Friend (bypasses Crowd Control)", chatter))
			friendBtn.OnTapped = func() {
				mw.hideTooltip(nil)
				mw.Session.TogglePriorityFriend(chatter)
				mw.refreshLog()
			}
			voiceBtn.SetTooltip("Assign a custom neural TTS voice to " + chatter)
			voiceBtn.OnTapped = func() {
				mw.hideTooltip(nil)
				mw.showAssignVoiceDialog(chatter)
			}
			muteBtn.SetTooltip("Mute / ignore " + chatter)
			muteBtn.OnTapped = func() {
				mw.hideTooltip(nil)
				mw.Session.MuteChatter(chatter)
			}
		},
	)
}

func (mw *MainWindow) buildMiniPlayerRoot() fyne.CanvasObject {
	expandBtn := mw.newTipBtn("Full Deck", theme.ViewFullScreenIcon(), "Expand back to Full Radio Deck view", func() {
		mw.ToggleMiniMode()
	})
	miniPauseBtn := mw.newTipBtn("", theme.MediaPauseIcon(), "Pause or resume TTS speech queue", func() {
		paused := mw.Session.TogglePause()
		if paused {
			mw.pauseBtn.SetText("Resume")
			mw.pauseBtn.SetIcon(theme.MediaPlayIcon())
		} else {
			mw.pauseBtn.SetText("Pause")
			mw.pauseBtn.SetIcon(theme.MediaPauseIcon())
		}
	})
	miniSkipBtn := mw.newTipBtn("", theme.MediaSkipNextIcon(), "Skip currently speaking message", func() {
		mw.Session.SkipCurrent()
	})
	miniReplayBtn := mw.newTipBtn("", theme.MediaReplayIcon(), "Replay last spoken message", func() {
		mw.Session.ReplayLast()
	})
	miniPanicBtn := mw.newTipBtn("MUTE", theme.VolumeMuteIcon(), "Emergency Mute & clear speech queue", func() {
		mw.Session.PanicMute()
	})
	miniPanicBtn.Importance = widget.DangerImportance

	topBar := container.NewBorder(
		nil, nil,
		widget.NewLabelWithStyle("Hearsay Mini", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		container.NewHBox(miniPauseBtn, miniSkipBtn, miniReplayBtn, miniPanicBtn, expandBtn),
		nil,
	)

	return container.NewBorder(
		container.NewVBox(topBar, mw.nowSpeakingLbl, widget.NewSeparator()),
		nil, nil, nil,
		mw.logList,
	)
}

// ToggleMiniMode switches the window between the Full Radio Deck and the compact Always-on-Top Mini Player.
func (mw *MainWindow) ToggleMiniMode() {
	mw.uiMu.Lock()
	defer mw.uiMu.Unlock()

	mw.mu.Lock()
	mw.isMiniMode = !mw.isMiniMode
	mini := mw.isMiniMode
	mw.mu.Unlock()

	cfg := mw.Session.ConfigSnapshot()
	if mini {
		mw.Window.SetContent(mw.miniRoot)
		mw.Window.Resize(fyne.NewSize(500, 280))
		if cfg.Accessibility.AlwaysOnTopMini {
			SetWindowAlwaysOnTop(mw.Window.Title(), true)
		}
	} else {
		SetWindowAlwaysOnTop(mw.Window.Title(), false)
		mw.Window.SetContent(mw.fullRoot)
		mw.Window.Resize(fyne.NewSize(1140, 740))
	}
}

// IsMiniMode reports whether the window is currently collapsed into the Mini Player.
func (mw *MainWindow) IsMiniMode() bool {
	mw.mu.Lock()
	defer mw.mu.Unlock()
	return mw.isMiniMode
}

func (mw *MainWindow) handleTuneIn() {
	raw := strings.TrimSpace(mw.QuickTuneEntry.Text)
	fallback := "twitch"
	switch mw.PlatformSelect.Selected {
	case "Twitch":
		fallback = "twitch"
	case "YouTube":
		fallback = "youtube"
	case "Kick":
		fallback = "kick"
	case "TikTok":
		fallback = "tiktok"
	}

	_, err := mw.Session.TuneIn(raw, fallback)
	if err != nil {
		mw.tuneErrorLbl.SetText("Error: " + err.Error())
		mw.tuneErrorLbl.Show()
		return
	}
	mw.tuneErrorLbl.Hide()
	mw.QuickTuneEntry.SetText("")
}

func (mw *MainWindow) refreshMixerAndLibrary() {
	mw.uiMu.Lock()
	defer mw.uiMu.Unlock()

	slots := mw.Session.SlotsSnapshot()
	mw.activeBadgeLbl.SetText(fmt.Sprintf("Hearsay — %d Active Stream(s)", len(slots)))

	mw.mixerCardsBox.RemoveAll()
	if len(slots) == 0 {
		emptyLbl := widget.NewLabelWithStyle(
			"No active streams. Paste a stream URL above or launch a Saved Streamer / Lineup below.",
			fyne.TextAlignCenter,
			fyne.TextStyle{Italic: true},
		)
		emptyLbl.Wrapping = fyne.TextWrapWord
		mw.mixerCardsBox.Add(container.NewPadded(emptyLbl))
	} else {
		for _, st := range slots {
			mw.mixerCardsBox.Add(mw.buildSlotCard(st))
		}
	}
	mw.mixerCardsBox.Refresh()

	cfg := mw.Session.ConfigSnapshot()
	mw.savedStreamersBox.RemoveAll()
	if len(cfg.SavedStreamers) == 0 {
		emptySaved := widget.NewLabelWithStyle("No saved streamers yet. Click Save on any active stream or + Add Saved Streamer.", fyne.TextAlignLeading, fyne.TextStyle{Italic: true})
		emptySaved.Wrapping = fyne.TextWrapWord
		mw.savedStreamersBox.Add(emptySaved)
	} else {
		for _, st := range cfg.SavedStreamers {
			streamer := st
			chLabels := make([]string, 0, len(streamer.Channels))
			for _, c := range streamer.Channels {
				chLabels = append(chLabels, fmt.Sprintf("%s:%s", c.Platform, c.Channel))
			}
			lbl := widget.NewLabel(fmt.Sprintf("%s (%s)", streamer.DisplayName, strings.Join(chLabels, ", ")))
			lbl.Truncation = fyne.TextTruncateEllipsis
			tuneBtn := mw.newTipBtn("Tune In", theme.MediaPlayIcon(), "Connect to "+streamer.DisplayName, func() {
				_ = mw.Session.ActivateStreamer(streamer)
			})
			delBtn := mw.newTipBtn("", theme.DeleteIcon(), "Remove "+streamer.DisplayName+" from Saved Streamers", func() {
				_ = mw.Session.UpdateConfig(func(c *config.Config) {
					c.RemoveSavedStreamer(streamer.ID)
				})
				mw.refreshMixerAndLibrary()
			})
			row := container.NewBorder(nil, nil, nil, container.NewHBox(tuneBtn, delBtn), lbl)
			mw.savedStreamersBox.Add(row)
		}
	}
	mw.savedStreamersBox.Refresh()

	mw.lineupsBox.RemoveAll()
	if len(cfg.Lineups) == 0 {
		emptyLineups := widget.NewLabelWithStyle("No saved lineups yet. Tune in to multiple streams, set their Left/Right stereo pan, and click Save Active Streams as Lineup.", fyne.TextAlignLeading, fyne.TextStyle{Italic: true})
		emptyLineups.Wrapping = fyne.TextWrapWord
		mw.lineupsBox.Add(emptyLineups)
	} else {
		for _, l := range cfg.Lineups {
			lineup := l
			lbl := widget.NewLabel(fmt.Sprintf("%s (%d streams)", lineup.Name, len(lineup.Slots)))
			lbl.Truncation = fyne.TextTruncateEllipsis
			launchBtn := mw.newTipBtn("Launch", theme.MediaPlayIcon(), "Launch lineup "+lineup.Name, func() {
				_ = mw.Session.ActivateLineup(lineup, true)
			})
			delBtn := mw.newTipBtn("", theme.DeleteIcon(), "Delete lineup "+lineup.Name, func() {
				_ = mw.Session.UpdateConfig(func(c *config.Config) {
					c.RemoveLineup(lineup.ID)
				})
				mw.refreshMixerAndLibrary()
			})
			row := container.NewBorder(nil, nil, nil, container.NewHBox(launchBtn, delBtn), lbl)
			mw.lineupsBox.Add(row)
		}
	}
	mw.lineupsBox.Refresh()
}

func (mw *MainWindow) buildSlotCard(st listener.SlotState) fyne.CanvasObject {
	slotID := st.Config.SlotID()
	statusBadge := "[OFFLINE]"
	switch st.Status {
	case "connected":
		statusBadge = "[LIVE]"
	case "waiting", "connecting":
		statusBadge = "[WAITING]"
	}

	titleLbl := widget.NewLabelWithStyle(
		fmt.Sprintf("%s [%s] %s", statusBadge, strings.ToUpper(st.Config.Platform), st.Config.EffectiveLabel()),
		fyne.TextAlignLeading,
		fyne.TextStyle{Bold: true},
	)
	titleLbl.Truncation = fyne.TextTruncateEllipsis

	muteText := "On"
	muteIcon := theme.VolumeUpIcon()
	if st.Config.Muted {
		muteText = "Muted"
		muteIcon = theme.VolumeMuteIcon()
	}
	var muteBtn *TooltipButton
	muteBtn = mw.newTipBtn(muteText, muteIcon, "Toggle TTS audio for "+st.Config.EffectiveLabel(), func() {
		var nowMuted bool
		mw.Session.UpdateSlot(slotID, func(sc *config.StreamSlotConfig) {
			sc.Muted = !sc.Muted
			nowMuted = sc.Muted
		})
		if nowMuted {
			muteBtn.SetText("Muted")
			muteBtn.SetIcon(theme.VolumeMuteIcon())
		} else {
			muteBtn.SetText("On")
			muteBtn.SetIcon(theme.VolumeUpIcon())
		}
	})

	saveBtn := mw.newTipBtn("Save", theme.DocumentSaveIcon(), "Bookmark "+st.Config.EffectiveLabel()+" to Saved Streamers", func() {
		_ = mw.Session.UpdateConfig(func(c *config.Config) {
			c.AddOrUpdateSavedStreamer(config.SavedStreamer{
				ID:          slotID,
				DisplayName: st.Config.EffectiveLabel(),
				Channels:    []config.StreamSlotConfig{st.Config},
			})
		})
		mw.refreshMixerAndLibrary()
	})

	discBtn := mw.newTipBtn("", theme.CancelIcon(), "Disconnect and remove "+st.Config.EffectiveLabel(), func() {
		_ = mw.Session.RemoveSlot(slotID)
	})
	discBtn.Importance = widget.DangerImportance

	headerRow := container.NewBorder(nil, nil, nil, container.NewHBox(muteBtn, saveBtn, discBtn), titleLbl)

	// Volume & Stereo Pan controls
	volLbl := widget.NewLabel(fmt.Sprintf("Vol: %.0f%%", st.Config.Volume*100))
	volSld := widget.NewSlider(0, 200)
	volSld.Step = 5
	volSld.Value = st.Config.Volume * 100
	volSld.OnChanged = func(v float64) {
		volLbl.SetText(fmt.Sprintf("Vol: %.0f%%", v))
		mw.Session.UpdateSlot(slotID, func(sc *config.StreamSlotConfig) {
			sc.Volume = v / 100.0
		})
	}

	panLbl := widget.NewLabel("Pan: " + formatPanBadge(st.Config.Pan))
	panSld := widget.NewSlider(-100, 100)
	panSld.Step = 5
	panSld.Value = st.Config.Pan * 100
	panSld.OnChanged = func(v float64) {
		panVal := v / 100.0
		panLbl.SetText("Pan: " + formatPanBadge(panVal))
		mw.Session.UpdateSlot(slotID, func(sc *config.StreamSlotConfig) {
			sc.Pan = panVal
		})
	}

	panLBtn := mw.newTipBtn("L", nil, "Pan stream audio 75% Left", func() { panSld.SetValue(-75) })
	panCBtn := mw.newTipBtn("C", nil, "Center stream audio (Both ears)", func() { panSld.SetValue(0) })
	panRBtn := mw.newTipBtn("R", nil, "Pan stream audio 75% Right", func() { panSld.SetValue(75) })

	mixerRow := container.NewGridWithColumns(2,
		container.NewBorder(nil, nil, volLbl, nil, volSld),
		container.NewBorder(nil, nil, panLbl, container.NewHBox(panLBtn, panCBtn, panRBtn), panSld),
	)

	// Crowd Filter selector
	filterOptions := []string{
		"Inherit Global",
		"All Chatters",
		"Subs + VIPs + Mods",
		"Mods + Streamer + @Mentions",
	}
	filterSel := widget.NewSelect(filterOptions, nil)
	switch st.Config.CrowdFilter {
	case config.FilterAll:
		filterSel.SetSelected("All Chatters")
	case config.FilterSubsVIPsMods:
		filterSel.SetSelected("Subs + VIPs + Mods")
	case config.FilterModsStreamerMentions:
		filterSel.SetSelected("Mods + Streamer + @Mentions")
	default:
		filterSel.SetSelected("Inherit Global")
	}
	filterSel.OnChanged = func(choice string) {
		mode := ""
		switch choice {
		case "All Chatters":
			mode = config.FilterAll
		case "Subs + VIPs + Mods":
			mode = config.FilterSubsVIPsMods
		case "Mods + Streamer + @Mentions":
			mode = config.FilterModsStreamerMentions
		}
		mw.Session.UpdateSlot(slotID, func(sc *config.StreamSlotConfig) {
			sc.CrowdFilter = mode
		})
	}

	filterRow := container.NewBorder(nil, nil, widget.NewLabel("Crowd Filter:"), nil, filterSel)

	return container.NewVBox(
		headerRow,
		mixerRow,
		filterRow,
		widget.NewSeparator(),
	)
}

func (mw *MainWindow) refreshLog() {
	mw.uiMu.Lock()
	defer mw.uiMu.Unlock()

	entries := mw.Session.LogSnapshot()
	mw.mu.Lock()
	mw.logEntries = slices.Clone(entries)
	mw.mu.Unlock()
	if mw.logList != nil {
		mw.logList.Refresh()
	}
}

func (mw *MainWindow) updateNowSpeaking(entry *listener.LogEntry) {
	mw.uiMu.Lock()
	defer mw.uiMu.Unlock()

	if entry == nil {
		mw.lastStatusText = "Idle — Listening for chat..."
		mw.nowSpeakingLbl.SetText(mw.lastStatusText)
		return
	}
	mw.lastStatusText = fmt.Sprintf("Speaking [%s %s] %s: %s", entry.Channel, formatPanBadge(entry.Pan), entry.UserName, entry.Text)
	mw.nowSpeakingLbl.SetText(mw.lastStatusText)
}

func (mw *MainWindow) registerHotkeys(hk config.HotkeyConfig) {
	mw.closeHotkeys()

	mw.hkPause = NewGlobalHotkeyManagerWithID(HotkeyIDPauseResume, func() {
		fyne.Do(func() {
			paused := mw.Session.TogglePause()
			if paused {
				mw.pauseBtn.SetText("Resume")
				mw.pauseBtn.SetIcon(theme.MediaPlayIcon())
			} else {
				mw.pauseBtn.SetText("Pause")
				mw.pauseBtn.SetIcon(theme.MediaPauseIcon())
			}
		})
	})
	_ = mw.hkPause.Register(hk.PauseResume)

	mw.hkSkip = NewGlobalHotkeyManagerWithID(HotkeyIDSkipCurrent, func() {
		mw.Session.SkipCurrent()
	})
	_ = mw.hkSkip.Register(hk.SkipCurrent)

	mw.hkReplay = NewGlobalHotkeyManagerWithID(HotkeyIDReplayLast, func() {
		mw.Session.ReplayLast()
	})
	_ = mw.hkReplay.Register(hk.ReplayLast)

	mw.hkPanic = NewGlobalHotkeyManagerWithID(HotkeyIDPanicMute, func() {
		fyne.Do(func() {
			mw.Session.PanicMute()
			mw.updateNowSpeaking(nil)
		})
	})
	_ = mw.hkPanic.Register(hk.PanicMute)
}

func (mw *MainWindow) closeHotkeys() {
	if mw.hkPause != nil {
		mw.hkPause.Close()
	}
	if mw.hkSkip != nil {
		mw.hkSkip.Close()
	}
	if mw.hkReplay != nil {
		mw.hkReplay.Close()
	}
	if mw.hkPanic != nil {
		mw.hkPanic.Close()
	}
}

func (mw *MainWindow) showSaveLineupDialog() {
	nameEntry := widget.NewEntry()
	nameEntry.SetPlaceHolder("e.g. Friday Co-Stream (Left & Right)")
	dialog.ShowForm("Save Active Streams as Lineup", "Save", "Cancel",
		[]*widget.FormItem{
			widget.NewFormItem("Lineup Name", nameEntry),
		},
		func(ok bool) {
			if !ok || strings.TrimSpace(nameEntry.Text) == "" {
				return
			}
			_, _ = mw.Session.SaveCurrentAsLineup(nameEntry.Text)
			mw.refreshMixerAndLibrary()
		},
		mw.Window,
	)
}

func (mw *MainWindow) showAddSavedStreamerDialog() {
	nameEntry := widget.NewEntry()
	nameEntry.SetPlaceHolder("Creator Display Name (e.g. MoCoMade)")
	targetsEntry := widget.NewMultiLineEntry()
	targetsEntry.SetPlaceHolder("One channel or URL per line:\ntwitch:mocomade\nyoutube:@MoCoMade")

	dialog.ShowForm("Add Saved Streamer (Single or Multi-Platform)", "Save", "Cancel",
		[]*widget.FormItem{
			widget.NewFormItem("Display Name", nameEntry),
			widget.NewFormItem("Channels / URLs", targetsEntry),
		},
		func(ok bool) {
			if !ok || strings.TrimSpace(nameEntry.Text) == "" {
				return
			}
			var channels []config.StreamSlotConfig
			for line := range strings.SplitSeq(targetsEntry.Text, "\n") {
				plat, ch, err := listener.ParseTargetInput(line, "twitch")
				if err == nil {
					channels = append(channels, config.StreamSlotConfig{
						Platform:    plat,
						Channel:     ch,
						DisplayName: strings.TrimSpace(nameEntry.Text),
						Volume:      1.0,
					})
				}
			}
			if len(channels) == 0 {
				return
			}
			_ = mw.Session.UpdateConfig(func(c *config.Config) {
				c.AddOrUpdateSavedStreamer(config.SavedStreamer{
					ID:          strings.ToLower(strings.TrimSpace(nameEntry.Text)),
					DisplayName: strings.TrimSpace(nameEntry.Text),
					Channels:    channels,
				})
			})
			mw.refreshMixerAndLibrary()
		},
		mw.Window,
	)
}

func (mw *MainWindow) showAssignVoiceDialog(username string) {
	voices := listener.DefaultSupertonicVoicePool
	if eng := mw.Session.Engine(); eng != nil && len(eng.Voices()) > 0 {
		voices = eng.Voices()
	}
	options := append([]string{"(Auto Hash Voice)"}, voices...)
	voiceSel := widget.NewSelect(options, nil)
	voiceSel.SetSelected("(Auto Hash Voice)")

	dialog.ShowForm("Assign Voice to "+username, "Save", "Cancel",
		[]*widget.FormItem{
			widget.NewFormItem("Voice", voiceSel),
		},
		func(ok bool) {
			if !ok {
				return
			}
			choice := voiceSel.Selected
			if choice == "(Auto Hash Voice)" {
				choice = ""
			}
			mw.Session.AssignChatterVoice(username, choice)
		},
		mw.Window,
	)
}

func formatLogBadge(entry listener.LogEntry) string {
	prefix := ""
	if entry.IsMention {
		prefix = "[@MENTION] "
	} else if entry.IsPriority {
		prefix = "[VIP] "
	}
	switch entry.Status {
	case listener.StatusSpeaking:
		return prefix + "[SPEAKING]"
	case listener.StatusQueued:
		return prefix + "[QUEUED]"
	case listener.StatusSpoken:
		return prefix + "[OK]"
	case listener.StatusSkipped:
		return fmt.Sprintf("[SKIP: %s]", formatSkipReason(entry.SkipReason))
	default:
		return prefix + "[CHAT]"
	}
}

func formatSkipReason(reason string) string {
	switch reason {
	case "empty_message":
		return "Emote"
	case "short_hype_spam":
		return "Spam"
	case "chatter_cooldown":
		return "Cooldown"
	case "global_pacing":
		return "Pacing"
	case "muted_chatter":
		return "Muted"
	case "slot_muted":
		return "Stream Muted"
	case "role_filter_subs_vips_mods", "role_filter_mods_mentions":
		return "Role Filter"
	case "no_text_event":
		return "Event"
	case "queue_cleared":
		return "Cleared"
	case "stale_ttl":
		return "Stale"
	default:
		return reason
	}
}

func formatPanBadge(pan float64) string {
	if math.Abs(pan) < 0.05 {
		return "C"
	}
	if pan < 0 {
		return fmt.Sprintf("L%.0f%%", math.Abs(pan)*100)
	}
	return fmt.Sprintf("R%.0f%%", pan*100)
}

func parseFontScaleOption(opt string) float32 {
	switch strings.TrimSpace(opt) {
	case "125%":
		return 1.25
	case "150%":
		return 1.5
	case "175%":
		return 1.75
	case "200%":
		return 2.0
	default:
		return 1.0
	}
}

func formatFontScaleOption(scale float32) string {
	switch {
	case scale >= 1.9:
		return "200%"
	case scale >= 1.65:
		return "175%"
	case scale >= 1.4:
		return "150%"
	case scale >= 1.15:
		return "125%"
	default:
		return "100%"
	}
}
