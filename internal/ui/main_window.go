package ui

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
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

	// Header & status widgets
	activeBadgeLbl *widget.Label
	nowSpeakingLbl *widget.Label
	fontScaleSel   *widget.Select
	highContrastCk *widget.Check
	miniToggleBtn  *widget.Button
	panicBtn       *widget.Button

	// Quick-Tune widgets
	PlatformSelect *widget.Select
	QuickTuneEntry *widget.Entry
	TuneInBtn      *widget.Button
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
	pauseBtn     *widget.Button
	skipBtn      *widget.Button
	replayBtn    *widget.Button
	clearQBtn    *widget.Button
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

func (mw *MainWindow) buildWidgets(cfg *config.Config) {
	// 1. Header Bar
	mw.activeBadgeLbl = widget.NewLabelWithStyle("🎧 Hearsay — 0 Active Streams", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	mw.nowSpeakingLbl = widget.NewLabelWithStyle("🔇 Idle — Ready to tune in", fyne.TextAlignLeading, fyne.TextStyle{Italic: true})
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

	mw.miniToggleBtn = widget.NewButton("🗗 Mini Player", func() {
		mw.ToggleMiniMode()
	})

	settingsBtn := widget.NewButtonWithIcon("Settings & Voices", theme.SettingsIcon(), func() {
		mw.showSettingsDialog()
	})

	mw.panicBtn = widget.NewButton("🛑 PANIC MUTE", func() {
		mw.Session.PanicMute()
		mw.updateNowSpeaking(nil)
	})
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
	mw.QuickTuneEntry.SetPlaceHolder("Paste stream URL or type channel (e.g. twitch:piratesoftware, @handle)...")
	mw.QuickTuneEntry.OnSubmitted = func(_ string) {
		mw.handleTuneIn()
	}

	mw.TuneInBtn = widget.NewButtonWithIcon("Tune In", theme.MediaPlayIcon(), func() {
		mw.handleTuneIn()
	})
	mw.TuneInBtn.Importance = widget.HighImportance

	mw.tuneErrorLbl = widget.NewLabel("")
	mw.tuneErrorLbl.Hide()

	quickTuneBox := container.NewVBox(
		widget.NewLabelWithStyle("📻 Quick Tune — Add Live Stream", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		container.NewBorder(nil, nil, mw.PlatformSelect, mw.TuneInBtn, mw.QuickTuneEntry),
		mw.tuneErrorLbl,
	)

	mw.mixerCardsBox = container.NewVBox()
	mixerScroll := container.NewVScroll(mw.mixerCardsBox)
	mixerScroll.SetMinSize(fyne.NewSize(420, 260))

	mw.savedStreamersBox = container.NewVBox()
	mw.lineupsBox = container.NewVBox()

	addSavedStreamerBtn := widget.NewButtonWithIcon("Add Saved Streamer", theme.ContentAddIcon(), func() {
		mw.showAddSavedStreamerDialog()
	})
	saveLineupBtn := widget.NewButtonWithIcon("Save Active Streams as Lineup", theme.DocumentSaveIcon(), func() {
		mw.showSaveLineupDialog()
	})

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
		container.NewTabItem("⭐ Saved Streamers", savedStreamersTab),
		container.NewTabItem("🎛 Lineups / Co-Streams", lineupsTab),
	)

	leftSplit := container.NewVSplit(
		container.NewBorder(
			container.NewVBox(quickTuneBox, widget.NewSeparator(), widget.NewLabelWithStyle("🎚 Active Streams Mixer (Volume, Stereo Pan & Crowd Filter)", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})),
			nil, nil, nil,
			mixerScroll,
		),
		libraryTabs,
	)
	leftSplit.Offset = 0.58

	// 3. Right Pane — Transport & Unified Live Log
	mw.buildTransportWidgets(cfg)
	mw.buildLiveLogWidget()

	rightPane := container.NewBorder(
		container.NewVBox(
			mw.buildTransportBar(),
			widget.NewSeparator(),
			widget.NewLabelWithStyle("📜 Unified Live Log (Click Replay, Friend, Voice, or Mute on any message)", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		),
		nil, nil, nil,
		mw.logList,
	)

	mainSplit := container.NewHSplit(leftSplit, rightPane)
	mainSplit.Offset = 0.42

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

	mw.pauseBtn = widget.NewButton("⏸ Pause", func() {
		paused := mw.Session.TogglePause()
		if paused {
			mw.pauseBtn.SetText("▶ Resume")
		} else {
			mw.pauseBtn.SetText("⏸ Pause")
		}
	})

	mw.skipBtn = widget.NewButton("⏭ Skip", func() {
		mw.Session.SkipCurrent()
	})

	mw.replayBtn = widget.NewButton("🔁 Replay Last", func() {
		mw.Session.ReplayLast()
	})

	mw.clearQBtn = widget.NewButton("🧹 Clear Queue", func() {
		mw.Session.ClearQueue()
	})
}

func (mw *MainWindow) buildTransportBar() fyne.CanvasObject {
	volBox := container.NewBorder(nil, nil, mw.masterVolLbl, nil, mw.masterVolSld)
	speedBox := container.NewBorder(nil, nil, mw.speedLbl, nil, mw.speedSld)
	slidersRow := container.NewGridWithColumns(3,
		container.NewBorder(nil, nil, widget.NewLabel("Output:"), nil, mw.deviceSelect),
		volBox,
		speedBox,
	)
	buttonsRow := container.NewHBox(
		mw.pauseBtn,
		mw.skipBtn,
		mw.replayBtn,
		mw.clearQBtn,
	)
	return container.NewVBox(slidersRow, buttonsRow)
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
			replayBtn := widget.NewButton("🔁", nil)
			friendBtn := widget.NewButton("⭐", nil)
			voiceBtn := widget.NewButton("🎭", nil)
			muteBtn := widget.NewButton("🔇", nil)
			actions := container.NewHBox(replayBtn, friendBtn, voiceBtn, muteBtn)
			return container.NewBorder(nil, nil, nil, actions, msgLbl)
		},
		func(id widget.ListItemID, item fyne.CanvasObject) {
			mw.mu.Lock()
			// Display newest messages at the top (index 0 = newest)
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

			replayBtn := actions.Objects[0].(*widget.Button)
			friendBtn := actions.Objects[1].(*widget.Button)
			voiceBtn := actions.Objects[2].(*widget.Button)
			muteBtn := actions.Objects[3].(*widget.Button)

			badge := formatLogBadge(entry)
			panTag := formatPanBadge(entry.Pan)
			msgLbl.SetText(fmt.Sprintf("%s [%s %s] %s: %s", badge, entry.Channel, panTag, entry.UserName, entry.Text))

			entryID := entry.ID
			chatter := entry.UserName
			replayBtn.OnTapped = func() {
				mw.Session.ReplayEntry(entryID)
			}
			friendBtn.OnTapped = func() {
				mw.Session.TogglePriorityFriend(chatter)
				mw.refreshLog()
			}
			voiceBtn.OnTapped = func() {
				mw.showAssignVoiceDialog(chatter)
			}
			muteBtn.OnTapped = func() {
				mw.Session.MuteChatter(chatter)
			}
		},
	)
}

func (mw *MainWindow) buildMiniPlayerRoot() fyne.CanvasObject {
	expandBtn := widget.NewButton("🗖 Full Deck", func() {
		mw.ToggleMiniMode()
	})
	miniPauseBtn := widget.NewButton("⏸/▶", func() {
		paused := mw.Session.TogglePause()
		if paused {
			mw.pauseBtn.SetText("▶ Resume")
		} else {
			mw.pauseBtn.SetText("⏸ Pause")
		}
	})
	miniSkipBtn := widget.NewButton("⏭ Skip", func() {
		mw.Session.SkipCurrent()
	})
	miniReplayBtn := widget.NewButton("🔁 Replay", func() {
		mw.Session.ReplayLast()
	})
	miniPanicBtn := widget.NewButton("🛑 MUTE", func() {
		mw.Session.PanicMute()
	})
	miniPanicBtn.Importance = widget.DangerImportance

	topBar := container.NewBorder(
		nil, nil,
		widget.NewLabelWithStyle("🎧 Hearsay Mini", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
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
		mw.tuneErrorLbl.SetText("⚠️ " + err.Error())
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
	mw.activeBadgeLbl.SetText(fmt.Sprintf("🎧 Hearsay — %d Active Stream(s)", len(slots)))

	mw.mixerCardsBox.RemoveAll()
	if len(slots) == 0 {
		emptyLbl := widget.NewLabelWithStyle(
			"No active streams. Paste a stream URL above or launch a Saved Streamer / Lineup below.",
			fyne.TextAlignCenter,
			fyne.TextStyle{Italic: true},
		)
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
		mw.savedStreamersBox.Add(widget.NewLabelWithStyle("No saved streamers yet. Click ⭐ Save on any active stream or + Add Saved Streamer.", fyne.TextAlignLeading, fyne.TextStyle{Italic: true}))
	} else {
		for _, st := range cfg.SavedStreamers {
			streamer := st
			chLabels := make([]string, 0, len(streamer.Channels))
			for _, c := range streamer.Channels {
				chLabels = append(chLabels, fmt.Sprintf("%s:%s", c.Platform, c.Channel))
			}
			lbl := widget.NewLabel(fmt.Sprintf("%s (%s)", streamer.DisplayName, strings.Join(chLabels, ", ")))
			tuneBtn := widget.NewButton("▶ Tune In", func() {
				_ = mw.Session.ActivateStreamer(streamer)
			})
			delBtn := widget.NewButton("🗑", func() {
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
		mw.lineupsBox.Add(widget.NewLabelWithStyle("No saved lineups yet. Tune in to multiple streams, set their Left/Right stereo pan, and click Save Active Streams as Lineup.", fyne.TextAlignLeading, fyne.TextStyle{Italic: true}))
	} else {
		for _, l := range cfg.Lineups {
			lineup := l
			lbl := widget.NewLabel(fmt.Sprintf("%s (%d streams)", lineup.Name, len(lineup.Slots)))
			launchBtn := widget.NewButton("▶ Launch", func() {
				_ = mw.Session.ActivateLineup(lineup, true)
			})
			delBtn := widget.NewButton("🗑", func() {
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
	statusBadge := "⚪ OFFLINE"
	switch st.Status {
	case "connected":
		statusBadge = "🟢 LIVE"
	case "waiting", "connecting":
		statusBadge = "🟡 WAITING"
	}

	titleLbl := widget.NewLabelWithStyle(
		fmt.Sprintf("%s  [%s] %s", statusBadge, strings.ToUpper(st.Config.Platform), st.Config.EffectiveLabel()),
		fyne.TextAlignLeading,
		fyne.TextStyle{Bold: true},
	)

	muteText := "🔊 On"
	if st.Config.Muted {
		muteText = "🔇 Muted"
	}
	var muteBtn *widget.Button
	muteBtn = widget.NewButton(muteText, func() {
		var nowMuted bool
		mw.Session.UpdateSlot(slotID, func(sc *config.StreamSlotConfig) {
			sc.Muted = !sc.Muted
			nowMuted = sc.Muted
		})
		if nowMuted {
			muteBtn.SetText("🔇 Muted")
		} else {
			muteBtn.SetText("🔊 On")
		}
	})

	saveBtn := widget.NewButton("⭐ Save", func() {
		_ = mw.Session.UpdateConfig(func(c *config.Config) {
			c.AddOrUpdateSavedStreamer(config.SavedStreamer{
				ID:          slotID,
				DisplayName: st.Config.EffectiveLabel(),
				Channels:    []config.StreamSlotConfig{st.Config},
			})
		})
		mw.refreshMixerAndLibrary()
	})

	discBtn := widget.NewButton("✕", func() {
		_ = mw.Session.RemoveSlot(slotID)
	})
	discBtn.Importance = widget.DangerImportance

	headerRow := container.NewBorder(nil, nil, titleLbl, container.NewHBox(muteBtn, saveBtn, discBtn), layout.NewSpacer())

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

	panLBtn := widget.NewButton("L", func() { panSld.SetValue(-75) })
	panCBtn := widget.NewButton("C", func() { panSld.SetValue(0) })
	panRBtn := widget.NewButton("R", func() { panSld.SetValue(75) })

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
		mw.nowSpeakingLbl.SetText("🔇 Idle — Listening for chat...")
		return
	}
	mw.nowSpeakingLbl.SetText(fmt.Sprintf("🔊 Speaking [%s %s] %s: %s", entry.Channel, formatPanBadge(entry.Pan), entry.UserName, entry.Text))
}

func (mw *MainWindow) registerHotkeys(hk config.HotkeyConfig) {
	mw.closeHotkeys()

	mw.hkPause = NewGlobalHotkeyManagerWithID(HotkeyIDPauseResume, func() {
		fyne.Do(func() {
			paused := mw.Session.TogglePause()
			if paused {
				mw.pauseBtn.SetText("▶ Resume")
			} else {
				mw.pauseBtn.SetText("⏸ Pause")
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
	nameEntry.SetPlaceHolder("Creator Display Name (e.g. PirateSoftware)")
	targetsEntry := widget.NewMultiLineEntry()
	targetsEntry.SetPlaceHolder("One channel or URL per line:\ntwitch:piratesoftware\nyoutube:@PirateSoftware")

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
		prefix = "🔔 @MENTION "
	} else if entry.IsPriority {
		prefix = "⭐ "
	}
	switch entry.Status {
	case listener.StatusSpeaking:
		return prefix + "🔊"
	case listener.StatusQueued:
		return prefix + "⏳"
	case listener.StatusSpoken:
		return prefix + "✓"
	case listener.StatusSkipped:
		return fmt.Sprintf("⏭ (%s)", entry.SkipReason)
	default:
		return prefix + "•"
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
