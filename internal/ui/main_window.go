package ui

import (
	"fmt"
	"image/color"
	"slices"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/mrpoundsign/Hearsay/assets"
	"github.com/mrpoundsign/Hearsay/internal/config"
	"github.com/mrpoundsign/Hearsay/internal/listener"
	"github.com/mrpoundsign/TownCrier/pkg/audio"
	"github.com/mrpoundsign/TownCrier/pkg/tts"
)

// MainWindow hosts the Hearsay Radio Deck (Left Pane), Unified Live Log & Transport (Right Pane),
// and the compact Always-on-Top Mini Player view.
type MainWindow struct {
	App              fyne.App
	Window           fyne.Window
	Session          *listener.Session
	deviceEnumerator func() ([]audio.DeviceInfo, error)

	mu         sync.Mutex
	uiMu       sync.Mutex
	isMiniMode bool
	logEntries []listener.LogEntry
	deviceMap  map[string]string

	// Top Header & Accessibility Bar
	activeBadgeLbl *widget.Label
	nowSpeakingLbl *widget.Label
	lastStatusText string
	fontScaleSel   *widget.Select
	highContrastCk *widget.Check
	speakEmotesCk  *widget.Check
	channelIconsCk *widget.Check
	miniToggleBtn  *TooltipButton
	panicBtn       *TooltipButton
	tooltipLayer   *fyne.Container
	tooltipBubble  *fyne.Container
	tooltipText    *canvas.Text
	activeTipOwner *TooltipButton

	// Quick Tune Bar
	PlatformSelect *widget.Select
	QuickTuneEntry *widget.Entry
	TuneInBtn      *TooltipButton
	tuneErrorLbl   *widget.Label

	// Left Pane: Active Streams Mixer & Saved Library
	mixerCardsBox     *fyne.Container
	savedStreamersBox *fyne.Container
	lineupsBox        *fyne.Container

	// Right Pane: Transport & Unified Live Log
	deviceSel    *widget.Select
	masterVolSld *widget.Slider
	masterVolLbl *widget.Label
	speedSld     *widget.Slider
	speedLbl     *widget.Label
	pauseBtn     *TooltipButton
	skipBtn      *TooltipButton
	replayBtn    *TooltipButton
	clearBtn     *TooltipButton
	logList      *widget.List

	// Root containers for full mode vs mini-player mode
	fullRoot fyne.CanvasObject
	miniRoot fyne.CanvasObject

	// Global Hotkey Managers
	hkPause  *GlobalHotkeyManager
	hkSkip   *GlobalHotkeyManager
	hkReplay *GlobalHotkeyManager
	hkPanic  *GlobalHotkeyManager
}

// NewMainWindow constructs and wires the Hearsay MainWindow.
func NewMainWindow(app fyne.App, session *listener.Session, devEnum func() ([]audio.DeviceInfo, error)) *MainWindow {
	if devEnum == nil {
		devEnum = audio.EnumeratePlaybackDevices
	}
	cfg := session.ConfigSnapshot()
	app.Settings().SetTheme(NewHearsayTheme(cfg.Accessibility.FontScale, cfg.Accessibility.HighContrast))

	w := app.NewWindow("Hearsay " + AppVersion + " — Multi-Stream Live Chat Reader & Audio Mixer")
	w.Resize(fyne.NewSize(1180, 740))

	mw := &MainWindow{
		App:              app,
		Window:           w,
		Session:          session,
		deviceEnumerator: devEnum,
		deviceMap:        make(map[string]string),
	}

	mw.buildUI(cfg)
	mw.refreshMixerAndLibrary()
	mw.refreshLog()

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

	mw.registerHotkeys(cfg.Accessibility.Hotkeys)

	w.Canvas().SetOnTypedKey(func(ev *fyne.KeyEvent) {
		if ev.Name == fyne.KeyEscape {
			mw.Session.PanicMute()
			mw.updateNowSpeaking(nil)
		}
	})

	w.SetOnClosed(func() {
		mw.closeHotkeys()
	})

	w.SetContent(mw.fullRoot)
	return mw
}

func (mw *MainWindow) buildUI(cfg *config.Config) {
	// 1. Top Header & Quick Controls Bar
	mw.activeBadgeLbl = widget.NewLabelWithStyle("Hearsay — 0 Active Streams", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	mw.lastStatusText = "Idle — Listening for chat..."
	mw.nowSpeakingLbl = widget.NewLabelWithStyle(mw.lastStatusText, fyne.TextAlignLeading, fyne.TextStyle{Italic: true})
	mw.nowSpeakingLbl.Truncation = fyne.TextTruncateEllipsis

	scaleOptions := []string{"100%", "125%", "150%", "175%"}
	mw.fontScaleSel = widget.NewSelect(scaleOptions, func(selected string) {
		var scale float32 = 1.0
		switch selected {
		case "125%":
			scale = 1.25
		case "150%":
			scale = 1.50
		case "175%":
			scale = 1.75
		}
		_ = mw.Session.UpdateConfig(func(c *config.Config) {
			c.Accessibility.FontScale = scale
		})
		sc := mw.Session.ConfigSnapshot()
		mw.App.Settings().SetTheme(NewHearsayTheme(sc.Accessibility.FontScale, sc.Accessibility.HighContrast))
		mw.refreshLog()
	})
	mw.fontScaleSel.SetSelected(formatScalePercent(cfg.Accessibility.FontScale))

	mw.highContrastCk = widget.NewCheck("High Contrast", func(checked bool) {
		_ = mw.Session.UpdateConfig(func(c *config.Config) {
			c.Accessibility.HighContrast = checked
		})
		sc := mw.Session.ConfigSnapshot()
		mw.App.Settings().SetTheme(NewHearsayTheme(sc.Accessibility.FontScale, sc.Accessibility.HighContrast))
	})
	mw.highContrastCk.SetChecked(cfg.Accessibility.HighContrast)

	mw.miniToggleBtn = mw.newTipBtn(
		"Mini Player",
		theme.ViewRestoreIcon(),
		"Switch to compact Always-on-Top Mini Player window",
		func() {
			mw.ToggleMiniMode()
		},
	)

	settingsBtn := mw.newTipBtn(
		"Settings & Voices",
		theme.SettingsIcon(),
		"Configure Voice Packs, Coalescence, Usernames, Crowd Control & Hotkeys",
		func() {
			mw.showSettingsDialog()
		},
	)

	mw.panicBtn = mw.newTipBtn(
		"PANIC MUTE",
		theme.VolumeMuteIcon(),
		"Emergency Mute: Immediately stop audio and flush speech queue (Ctrl+Shift+M / Esc)",
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

	// 2. Left Pane — Radio Deck (Platform Selector + Channel Name)
	mw.PlatformSelect = widget.NewSelect([]string{"Twitch", "YouTube", "Kick", "TikTok"}, nil)
	mw.PlatformSelect.SetSelected("Twitch")

	mw.QuickTuneEntry = widget.NewEntry()
	mw.QuickTuneEntry.SetPlaceHolder("Channel name or paste stream URL (e.g. MoCoMade)...")
	mw.QuickTuneEntry.OnSubmitted = func(_ string) {
		mw.handleTuneIn()
	}

	mw.TuneInBtn = mw.newTipBtn(
		"Tune In",
		theme.MediaPlayIcon(),
		"Connect to the selected platform and channel (or pasted stream URL)",
		func() {
			mw.handleTuneIn()
		},
	)
	mw.TuneInBtn.Importance = widget.HighImportance

	mw.tuneErrorLbl = widget.NewLabel("")
	mw.tuneErrorLbl.Hide()

	quickTuneHeader := widget.NewLabelWithStyle("Quick Tune — Select Platform & Enter Channel", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
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
			mw.showSavedStreamerDialog(nil)
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

	logHeader := widget.NewLabelWithStyle("Unified Live Log", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	logHeader.Truncation = fyne.TextTruncateEllipsis

	mw.speakEmotesCk = widget.NewCheck("Speak Emotes", func(checked bool) {
		_ = mw.Session.UpdateConfig(func(c *config.Config) {
			c.Audio.SpeakEmotes = checked
		})
	})
	mw.speakEmotesCk.SetChecked(cfg.Audio.SpeakEmotes)

	mw.channelIconsCk = widget.NewCheck("Channel Icons", func(checked bool) {
		_ = mw.Session.UpdateConfig(func(c *config.Config) {
			c.Accessibility.ShowChannelIcons = checked
		})
		mw.refreshLog()
	})
	mw.channelIconsCk.SetChecked(cfg.Accessibility.ShowChannelIcons)

	logHeaderRow := container.NewBorder(
		nil, nil, nil,
		container.NewHBox(mw.speakEmotesCk, mw.channelIconsCk),
		logHeader,
	)

	rightPane := container.NewBorder(
		container.NewVBox(
			mw.buildTransportBar(),
			widget.NewSeparator(),
			logHeaderRow,
		),
		nil, nil, nil,
		mw.logList,
	)

	mainSplit := container.NewHSplit(leftSplit, rightPane)
	mainSplit.Offset = 0.50

	mw.tooltipBubble, mw.tooltipText = buildTooltipBubble()
	mw.tooltipLayer = container.NewWithoutLayout(mw.tooltipBubble)

	baseFullRoot := container.NewBorder(
		container.NewVBox(headerTop, widget.NewSeparator()),
		nil, nil, nil,
		mainSplit,
	)
	mw.fullRoot = container.NewStack(baseFullRoot, mw.tooltipLayer)

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

	mw.deviceSel = widget.NewSelect(devOptions, func(selected string) {
		devID := mw.deviceMap[selected]
		_ = mw.Session.UpdateConfig(func(c *config.Config) {
			c.Audio.OutputDeviceID = devID
		})
	})
	mw.deviceSel.SetSelected(selectedDevLabel)

	mw.masterVolLbl = widget.NewLabel(fmt.Sprintf("Vol: %.0f%%", cfg.Audio.MasterVolume*100))
	mw.masterVolSld = widget.NewSlider(0, 200)
	mw.masterVolSld.Step = 5
	mw.masterVolSld.Value = cfg.Audio.MasterVolume * 100
	mw.masterVolSld.OnChanged = func(v float64) {
		mw.masterVolLbl.SetText(fmt.Sprintf("Vol: %.0f%%", v))
		_ = mw.Session.UpdateConfig(func(c *config.Config) {
			c.Audio.MasterVolume = v / 100.0
		})
	}

	mw.speedLbl = widget.NewLabel(fmt.Sprintf("Speed: %.1fx", cfg.Audio.DefaultSpeed))
	mw.speedSld = widget.NewSlider(0.7, 2.0)
	mw.speedSld.Step = 0.1
	mw.speedSld.Value = cfg.Audio.DefaultSpeed
	mw.speedSld.OnChanged = func(v float64) {
		mw.speedLbl.SetText(fmt.Sprintf("Speed: %.1fx", v))
		_ = mw.Session.UpdateConfig(func(c *config.Config) {
			c.Audio.DefaultSpeed = v
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
		"Replay",
		theme.MediaReplayIcon(),
		"Replay the last spoken chat message aloud (Ctrl+Shift+R)",
		func() {
			mw.Session.ReplayLast()
		},
	)

	mw.clearBtn = mw.newTipBtn(
		"Clear",
		theme.DeleteIcon(),
		"Clear all pending messages waiting in the speech queue",
		func() {
			mw.Session.ClearQueue()
		},
	)
}

func (mw *MainWindow) buildTransportBar() fyne.CanvasObject {
	outputRow := container.NewBorder(nil, nil, widget.NewLabel("Output:"), nil, mw.deviceSel)
	slidersRow := container.NewGridWithColumns(2,
		container.NewBorder(nil, nil, mw.masterVolLbl, nil, mw.masterVolSld),
		container.NewBorder(nil, nil, mw.speedLbl, nil, mw.speedSld),
	)
	buttonsRow := container.NewHBox(
		mw.pauseBtn,
		mw.skipBtn,
		mw.replayBtn,
		mw.clearBtn,
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
			statusLbl := widget.NewLabel("[OK]")
			platImg := canvas.NewImageFromResource(assets.TwitchIcon)
			platImg.FillMode = canvas.ImageFillContain
			platImg.SetMinSize(fyne.NewSize(18, 18))

			chanImg := canvas.NewImageFromResource(assets.GenericStreamIcon)
			chanImg.FillMode = canvas.ImageFillContain
			chanImg.SetMinSize(fyne.NewSize(20, 20))
			chanImg.Hide()

			chanLbl := widget.NewLabel("[Channel C]")
			userTxt := canvas.NewText("User:", color.NRGBA{R: 0x38, G: 0xBD, B: 0xF8, A: 0xFF})
			userTxt.TextStyle = fyne.TextStyle{Bold: true}

			prefixBox := container.NewHBox(statusLbl, platImg, chanImg, chanLbl, userTxt)

			msgLbl := widget.NewLabel("Message template")
			msgLbl.Wrapping = fyne.TextWrapWord

			replayBtn := mw.newTipBtn("", theme.MediaReplayIcon(), "Replay this message aloud", nil)
			friendBtn := mw.newTipBtn("VIP", nil, "Toggle Priority VIP/Friend (always spoken, bypasses Crowd Control)", nil)
			voiceBtn := mw.newTipBtn("Voice", nil, "Assign a specific neural voice to this chatter", nil)
			muteBtn := mw.newTipBtn("", theme.VolumeMuteIcon(), "Mute / ignore all future messages from this chatter", nil)
			actions := container.NewHBox(replayBtn, friendBtn, voiceBtn, muteBtn)

			return container.NewBorder(nil, nil, prefixBox, actions, msgLbl)
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

			cfg := mw.Session.ConfigSnapshot()
			showChanIcons := cfg.Accessibility.ShowChannelIcons

			border := item.(*fyne.Container)
			msgLbl := border.Objects[0].(*widget.Label)
			prefixBox := border.Objects[1].(*fyne.Container)
			actions := border.Objects[2].(*fyne.Container)

			statusLbl := prefixBox.Objects[0].(*widget.Label)
			platImg := prefixBox.Objects[1].(*canvas.Image)
			chanImg := prefixBox.Objects[2].(*canvas.Image)
			chanLbl := prefixBox.Objects[3].(*widget.Label)
			userTxt := prefixBox.Objects[4].(*canvas.Text)

			replayBtn := actions.Objects[0].(*TooltipButton)
			friendBtn := actions.Objects[1].(*TooltipButton)
			voiceBtn := actions.Objects[2].(*TooltipButton)
			muteBtn := actions.Objects[3].(*TooltipButton)

			statusLbl.SetText(formatLogBadge(entry))
			platImg.Resource = assets.PlatformIcon(entry.Platform)
			platImg.Refresh()

			panTag := formatPanBadge(entry.Pan)
			if showChanIcons {
				chanLbl.Hide()
				chanImg.Resource = ChannelAvatarResource(entry.Platform, entry.Channel, func() {
					mw.refreshLog()
				})
				chanImg.Show()
				chanImg.Refresh()
			} else {
				chanImg.Hide()
				chanLbl.SetText(fmt.Sprintf("[%s %s]", entry.Channel, panTag))
				chanLbl.Show()
			}

			colHex := entry.UserColor
			if colHex == "" {
				colHex = listener.ResolveChatterColorHex(entry.Platform, entry.UserName, "")
			}
			userTxt.Text = entry.UserName + ":"
			userTxt.Color = ParseHexColor(colHex, color.NRGBA{R: 0x38, G: 0xBD, B: 0xF8, A: 0xFF})
			userTxt.TextSize = theme.TextSize()
			userTxt.Refresh()

			msgLbl.SetText(entry.Text)

			// Compute dynamic word-wrapped row height so multi-line messages never get truncated
			listWidth := mw.logList.Size().Width
			if listWidth <= 0 {
				listWidth = 460
			}
			availMsgWidth := listWidth - prefixBox.MinSize().Width - actions.MinSize().Width - 20
			if availMsgWidth < 110 {
				availMsgWidth = 110
			}
			msgLbl.Resize(fyne.NewSize(availMsgWidth, msgLbl.MinSize().Height))
			rowHeight := max(float32(38), msgLbl.MinSize().Height+8)
			mw.logList.SetItemHeight(id, rowHeight)

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

	if mini {
		mw.Window.SetContent(mw.miniRoot)
		mw.Window.Resize(fyne.NewSize(480, 320))
	} else {
		mw.Window.SetContent(mw.fullRoot)
		mw.Window.Resize(fyne.NewSize(1180, 740))
	}
}

// IsMiniMode reports whether the window is currently in compact Mini Player mode.
func (mw *MainWindow) IsMiniMode() bool {
	mw.mu.Lock()
	defer mw.mu.Unlock()
	return mw.isMiniMode
}

func (mw *MainWindow) handleTuneIn() {
	raw := strings.TrimSpace(mw.QuickTuneEntry.Text)
	if raw == "" {
		mw.tuneErrorLbl.SetText("Please enter a channel name or paste a live stream URL.")
		mw.tuneErrorLbl.Show()
		return
	}
	fallback := strings.ToLower(strings.TrimSpace(mw.PlatformSelect.Selected))
	if fallback == "" || fallback == "auto / url" {
		fallback = "twitch"
	}

	slotCfg, err := mw.Session.TuneIn(raw, fallback)
	if err != nil {
		mw.tuneErrorLbl.SetText("Warning: " + err.Error())
		mw.tuneErrorLbl.Show()
		return
	}
	mw.PlatformSelect.SetSelected(assets.PlatformDisplayName(slotCfg.Platform))
	mw.tuneErrorLbl.Hide()
	mw.QuickTuneEntry.SetText("")
	mw.refreshMixerAndLibrary()
}

func (mw *MainWindow) refreshMixerAndLibrary() {
	mw.uiMu.Lock()
	defer mw.uiMu.Unlock()

	slots := mw.Session.SlotsSnapshot()
	mw.activeBadgeLbl.SetText(fmt.Sprintf("Hearsay — %d Active Stream(s)", len(slots)))

	mw.mixerCardsBox.RemoveAll()
	if len(slots) == 0 {
		emptyLbl := widget.NewLabelWithStyle(
			"No active streams. Select a platform and type a channel name above, or launch a Saved Streamer / Lineup below.",
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
			iconsBox := container.NewHBox()
			platNames := make([]string, 0, len(streamer.Channels))
			for _, c := range streamer.Channels {
				img := canvas.NewImageFromResource(assets.PlatformIcon(c.Platform))
				img.FillMode = canvas.ImageFillContain
				img.SetMinSize(fyne.NewSize(16, 16))
				iconsBox.Add(img)
				pName := assets.PlatformDisplayName(c.Platform)
				if !strings.EqualFold(c.Channel, streamer.DisplayName) {
					platNames = append(platNames, fmt.Sprintf("%s: %s", pName, c.Channel))
				} else {
					platNames = append(platNames, pName)
				}
			}
			lbl := widget.NewLabel(fmt.Sprintf("%s [%s]", streamer.DisplayName, strings.Join(platNames, ", ")))
			lbl.Truncation = fyne.TextTruncateEllipsis

			tuneBtn := mw.newTipBtn("Tune In", theme.MediaPlayIcon(), "Connect to "+streamer.DisplayName, func() {
				_ = mw.Session.ActivateStreamer(streamer)
			})
			editBtn := mw.newTipBtn("", theme.DocumentCreateIcon(), "Edit "+streamer.DisplayName, func() {
				cp := streamer
				mw.showSavedStreamerDialog(&cp)
			})
			delBtn := mw.newTipBtn("", theme.DeleteIcon(), "Remove "+streamer.DisplayName+" from Saved Streamers", func() {
				_ = mw.Session.UpdateConfig(func(c *config.Config) {
					c.RemoveSavedStreamer(streamer.ID)
				})
				mw.refreshMixerAndLibrary()
			})
			row := container.NewBorder(nil, nil, iconsBox, container.NewHBox(tuneBtn, editBtn, delBtn), lbl)
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

	platIcon := canvas.NewImageFromResource(assets.PlatformIcon(st.Config.Platform))
	platIcon.FillMode = canvas.ImageFillContain
	platIcon.SetMinSize(fyne.NewSize(18, 18))

	titleLbl := widget.NewLabelWithStyle(
		fmt.Sprintf("%s %s", statusBadge, st.Config.EffectiveLabel()),
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
				ID:          strings.ToLower(st.Config.EffectiveLabel()),
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

	headerRow := container.NewBorder(nil, nil, platIcon, container.NewHBox(muteBtn, saveBtn, discBtn), titleLbl)

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
	content := container.NewVBox(
		widget.NewLabel("Save all currently active streams and their stereo panning as a 1-click preset:"),
		nameEntry,
	)
	d := dialog.NewCustomConfirm("Save Active Streams as Lineup", "Save Lineup", "Cancel", content, func(ok bool) {
		if !ok || strings.TrimSpace(nameEntry.Text) == "" {
			return
		}
		_, _ = mw.Session.SaveCurrentAsLineup(nameEntry.Text)
		mw.refreshMixerAndLibrary()
	}, mw.Window)
	d.Resize(fyne.NewSize(440, 190))
	d.Show()
}

type streamerChannelRow struct {
	platSel   *widget.Select
	chanEntry *widget.Entry
	rowObj    fyne.CanvasObject
}

func (mw *MainWindow) showSavedStreamerDialog(existing *config.SavedStreamer) {
	nameEntry := widget.NewEntry()
	nameEntry.SetPlaceHolder("Optional — defaults to first channel name (e.g. MoCoMade)")
	if existing != nil {
		nameEntry.SetText(existing.DisplayName)
	}

	rowsBox := container.NewVBox()
	var rows []*streamerChannelRow

	var rebuildRows func()
	rebuildRows = func() {
		rowsBox.RemoveAll()
		for i, r := range rows {
			idx := i
			row := r
			delBtn := mw.newTipBtn("", theme.DeleteIcon(), "Remove this platform channel row", func() {
				if len(rows) <= 1 {
					row.chanEntry.SetText("")
					return
				}
				rows = slices.Delete(rows, idx, idx+1)
				rebuildRows()
			})
			row.rowObj = container.NewBorder(nil, nil, row.platSel, delBtn, row.chanEntry)
			rowsBox.Add(row.rowObj)
		}
		rowsBox.Refresh()
	}

	addRow := func(plat string, ch string) {
		sel := widget.NewSelect([]string{"Twitch", "YouTube", "Kick", "TikTok"}, nil)
		sel.SetSelected(assets.PlatformDisplayName(plat))
		ent := widget.NewEntry()
		ent.SetPlaceHolder("Channel name or URL (e.g. MoCoMade)")
		ent.SetText(ch)
		rows = append(rows, &streamerChannelRow{
			platSel:   sel,
			chanEntry: ent,
		})
		rebuildRows()
	}

	if existing != nil && len(existing.Channels) > 0 {
		for _, ch := range existing.Channels {
			addRow(ch.Platform, ch.Channel)
		}
	} else {
		addRow("twitch", "")
	}

	addPlatformBtn := mw.newTipBtn("+ Add Another Platform", theme.ContentAddIcon(), "Add another streaming service for this creator (e.g. YouTube + Twitch)", func() {
		addRow("youtube", "")
	})

	rowsScroll := container.NewVScroll(rowsBox)
	rowsScroll.SetMinSize(fyne.NewSize(460, 150))

	content := container.NewVBox(
		widget.NewLabelWithStyle("Display Name / Nickname (Optional):", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		nameEntry,
		widget.NewSeparator(),
		widget.NewLabelWithStyle("Platform & Channel Name(s):", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		rowsScroll,
		addPlatformBtn,
	)

	title := "Add Saved Streamer"
	if existing != nil {
		title = "Edit Saved Streamer — " + existing.DisplayName
	}

	d := dialog.NewCustomConfirm(title, "Save Streamer", "Cancel", content, func(ok bool) {
		if !ok {
			return
		}
		var channels []config.StreamSlotConfig
		for _, r := range rows {
			rawCh := strings.TrimSpace(r.chanEntry.Text)
			if rawCh == "" {
				continue
			}
			fallbackPlat := strings.ToLower(strings.TrimSpace(r.platSel.Selected))
			plat, ch, err := listener.ParseTargetInput(rawCh, fallbackPlat)
			if err == nil && plat != "" && ch != "" {
				channels = append(channels, config.StreamSlotConfig{
					Platform: plat,
					Channel:  ch,
					Volume:   1.0,
				})
			}
		}
		if len(channels) == 0 {
			return
		}
		dispName := strings.TrimSpace(nameEntry.Text)
		if dispName == "" {
			dispName = channels[0].Channel
		}
		for i := range channels {
			channels[i].DisplayName = dispName
		}
		id := strings.ToLower(dispName)
		if existing != nil && strings.TrimSpace(existing.ID) != "" {
			id = existing.ID
		}
		_ = mw.Session.UpdateConfig(func(c *config.Config) {
			c.AddOrUpdateSavedStreamer(config.SavedStreamer{
				ID:          id,
				DisplayName: dispName,
				Channels:    channels,
			})
		})
		mw.refreshMixerAndLibrary()
	}, mw.Window)
	d.Resize(fyne.NewSize(520, 390))
	d.Show()
}

func (mw *MainWindow) showAssignVoiceDialog(username string) {
	cfg := mw.Session.ConfigSnapshot()
	voiceLabels, labelToID := availableVoiceOptions(cfg, mw.Session.Engine())
	options := append([]string{"(Auto Hash Voice)"}, voiceLabels...)
	voiceSel := widget.NewSelect(options, nil)

	currentVoice := cfg.Identity.ChatterVoices[strings.ToLower(strings.TrimPrefix(strings.TrimSpace(username), "@"))]
	if currentVoice != "" {
		voiceSel.SetSelected(tts.VoiceDisplayName(currentVoice))
	} else {
		voiceSel.SetSelected("(Auto Hash Voice)")
	}

	previewBtn := mw.newTipBtn("Preview Voice", theme.MediaPlayIcon(), "Hear a sample of this voice", func() {
		choice := voiceSel.Selected
		vid := labelToID[choice]
		if vid == "" && choice != "(Auto Hash Voice)" {
			vid = tts.VoiceIDFromLabel(choice)
		}
		mw.Session.PreviewVoice(vid, fmt.Sprintf("Hello, this is %s speaking in chat!", username))
	})

	content := container.NewVBox(
		widget.NewLabel(fmt.Sprintf("Select a neural TTS voice for %s:", username)),
		voiceSel,
		previewBtn,
	)

	d := dialog.NewCustomConfirm("Assign Voice to "+username, "Save Voice", "Cancel", content, func(ok bool) {
		if !ok {
			return
		}
		choice := voiceSel.Selected
		vid := ""
		if choice != "(Auto Hash Voice)" {
			vid = labelToID[choice]
			if vid == "" {
				vid = tts.VoiceIDFromLabel(choice)
			}
		}
		mw.Session.AssignChatterVoice(username, vid)
	}, mw.Window)
	d.Resize(fyne.NewSize(460, 220))
	d.Show()
}

func formatScalePercent(scale float32) string {
	switch {
	case scale >= 1.70:
		return "175%"
	case scale >= 1.45:
		return "150%"
	case scale >= 1.20:
		return "125%"
	default:
		return "100%"
	}
}

func formatPanBadge(pan float64) string {
	switch {
	case pan <= -0.15:
		return fmt.Sprintf("L%.0f", -pan*100)
	case pan >= 0.15:
		return fmt.Sprintf("R%.0f", pan*100)
	default:
		return "C"
	}
}

func formatLogBadge(entry listener.LogEntry) string {
	switch entry.Status {
	case listener.StatusSpeaking:
		return "[SPEAKING]"
	case listener.StatusQueued:
		return "[QUEUED]"
	case listener.StatusSpoken:
		if entry.IsMention {
			return "[@MENTION]"
		}
		return "[OK]"
	case listener.StatusSkipped:
		return formatSkipReason(entry.SkipReason)
	default:
		return "[CHAT]"
	}
}

func formatSkipReason(reason string) string {
	switch reason {
	case "empty_message":
		return "[SKIP: Emote]"
	case "short_hype_spam":
		return "[SKIP: Spam]"
	case "slot_muted":
		return "[SKIP: Muted]"
	case "muted_chatter":
		return "[SKIP: Ignored]"
	case "chatter_cooldown":
		return "[SKIP: Cooldown]"
	case "global_pacing":
		return "[SKIP: Pacing]"
	case "queue_cleared":
		return "[SKIP: Cleared]"
	case "stale_ttl":
		return "[SKIP: Stale]"
	default:
		if reason != "" {
			return "[SKIP]"
		}
		return "[SKIP]"
	}
}
