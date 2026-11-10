package ui

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/mrpoundsign/Hearsay/internal/config"
	"github.com/mrpoundsign/TownCrier/pkg/chat"
	"github.com/mrpoundsign/TownCrier/pkg/tts"
)

var coalesceOptions = []string{
	"Off (Always speak username)",
	"10 seconds",
	"15 seconds",
	"30 seconds (Default)",
	"60 seconds",
	"120 seconds",
}

func coalesceSecToLabel(sec int) string {
	switch sec {
	case 0:
		return "Off (Always speak username)"
	case 10:
		return "10 seconds"
	case 15:
		return "15 seconds"
	case 60:
		return "60 seconds"
	case 120:
		return "120 seconds"
	default:
		return "30 seconds (Default)"
	}
}

func labelToCoalesceSec(lbl string) int {
	switch lbl {
	case "Off (Always speak username)":
		return 0
	case "10 seconds":
		return 10
	case "15 seconds":
		return 15
	case "60 seconds":
		return 60
	case "120 seconds":
		return 120
	default:
		return 30
	}
}

var duplicateWordsOptions = []string{
	"Unlimited",
	"1 word",
	"2 words (Default)",
	"3 words",
}

func dupWordsToLabel(maxWords int) string {
	switch maxWords {
	case 0:
		return "Unlimited"
	case 1:
		return "1 word"
	case 3:
		return "3 words"
	default:
		return "2 words (Default)"
	}
}

func labelToDupWords(lbl string) int {
	switch lbl {
	case "Unlimited":
		return 0
	case "1 word":
		return 1
	case "3 words":
		return 3
	default:
		return 2
	}
}

var unknownGenderOptions = []string{
	"Balanced 50/50 (Feminine & Masculine)",
	"Prefer Feminine Voices",
	"Prefer Masculine Voices",
	"Use Default Voice",
}

func unknownGenderPolicyToLabel(policy string) string {
	switch strings.ToLower(strings.TrimSpace(policy)) {
	case "prefer_feminine":
		return "Prefer Feminine Voices"
	case "prefer_masculine":
		return "Prefer Masculine Voices"
	case "default_voice":
		return "Use Default Voice"
	default:
		return "Balanced 50/50 (Feminine & Masculine)"
	}
}

func labelToUnknownGenderPolicy(lbl string) string {
	switch lbl {
	case "Prefer Feminine Voices":
		return "prefer_feminine"
	case "Prefer Masculine Voices":
		return "prefer_masculine"
	case "Use Default Voice":
		return "default_voice"
	default:
		return "balanced"
	}
}

func availableVoiceOptions(cfg *config.Config, eng tts.Engine) ([]string, map[string]string) {
	families := cfg.Audio.EffectiveEnabledModels()
	rawLabels := tts.VoiceDisplayLabelsForModels(families)
	labels := make([]string, 0, len(rawLabels))
	labelToID := make(map[string]string, len(rawLabels))
	for _, lbl := range rawLabels {
		labels = append(labels, lbl)
		labelToID[lbl] = tts.VoiceIDFromLabel(lbl)
	}
	if len(labels) == 0 && eng != nil {
		for _, id := range eng.Voices() {
			lbl := tts.VoiceDisplayName(id)
			labels = append(labels, lbl)
			labelToID[lbl] = id
		}
	}
	return labels, labelToID
}

func (mw *MainWindow) showSettingsDialog() {
	cfg := mw.Session.ConfigSnapshot()

	// 1. Identity & Mentions
	myUsernamesEntry := widget.NewEntry()
	myUsernamesEntry.SetPlaceHolder("Your Twitch/YouTube/Kick/TikTok username(s), comma-separated")
	myUsernamesEntry.SetText(strings.Join(cfg.Identity.MyUsernames, ", "))

	priorityEntry := widget.NewEntry()
	priorityEntry.SetPlaceHolder("Priority VIP/Friend usernames, comma-separated")
	priorityEntry.SetText(strings.Join(cfg.Identity.PriorityChatters, ", "))

	ignoredEntry := widget.NewEntry()
	ignoredEntry.SetPlaceHolder("Muted usernames, comma-separated")
	ignoredEntry.SetText(strings.Join(cfg.Identity.IgnoredChatters, ", "))

	// 2. Audio Output & Voice Packs
	dlgDevOptions, dlgDevMap, dlgSelectedDev := mw.enumerateDeviceOptions(cfg.Audio.OutputDeviceID)
	dlgDeviceSel := widget.NewSelect(dlgDevOptions, nil)
	dlgDeviceSel.SetSelected(dlgSelectedDev)
	dlgDeviceRefreshBtn := mw.newTipBtn("Refresh", theme.ViewRefreshIcon(), "Refresh audio output device list", func() {
		opts, m, sel := mw.enumerateDeviceOptions(dlgDevMap[dlgDeviceSel.Selected])
		dlgDevMap = m
		dlgDeviceSel.Options = opts
		dlgDeviceSel.SetSelected(sel)
		dlgDeviceSel.Refresh()
	})

	modelDir := tts.DefaultModelDir()
	enabledSet := make(map[string]bool)
	for _, m := range cfg.Audio.EffectiveEnabledModels() {
		enabledSet[m] = true
	}

	modelChecks := make(map[string]*widget.Check, len(tts.AllModelFamilies))
	modelRows := container.NewVBox()
	modelUpdateStatuses := tts.InspectAllLocalModelStatuses(modelDir, cfg.Audio.UseGPU)

	formatPackLabel := func(spec tts.ModelFamilySpec) string {
		st, hasSt := modelUpdateStatuses[spec.ID]
		if !hasSt {
			st = tts.InspectLocalModelStatus(modelDir, spec.ID, cfg.Audio.UseGPU)
		}
		installed := tts.IsModelFamilyInstalled(modelDir, spec.ID, cfg.Audio.UseGPU)
		if spec.ID == tts.ModelKokoro || spec.ID == tts.ModelKokoroIntl {
			installed = tts.IsKokoroInstalledForMode(modelDir, cfg.Audio.UseGPU)
		}
		status := fmt.Sprintf("Download ~%d MB", spec.DownloadSizeMB)
		if st.UpdateAvailable {
			if spec.ID == tts.ModelKokoroIntl {
				status = "⬆ Update Available (Shares Kokoro)"
			} else {
				status = fmt.Sprintf("⬆ Update Available (~%d MB)", spec.DownloadSizeMB)
			}
		} else if installed {
			status = "Installed"
		}
		return fmt.Sprintf("%s [%s]", spec.Name, status)
	}

	for _, spec := range tts.AllModelFamilies {
		famSpec := spec
		chk := widget.NewCheck(formatPackLabel(famSpec), nil)
		chk.SetChecked(enabledSet[famSpec.ID])
		modelChecks[famSpec.ID] = chk

		summaryLbl := widget.NewLabelWithStyle("      "+famSpec.Summary, fyne.TextAlignLeading, fyne.TextStyle{Italic: true})
		summaryLbl.Wrapping = fyne.TextWrapWord

		modelRows.Add(container.NewVBox(chk, summaryLbl))
	}

	packStatusLbl := widget.NewLabel("Shared Models Dir: " + modelDir)
	packStatusLbl.Wrapping = fyne.TextWrapBreak
	packProgress := widget.NewProgressBar()
	packProgress.Hide()

	voiceLabels, labelToVoiceID := availableVoiceOptions(cfg, mw.Session.Engine())
	defaultVoiceSel := widget.NewSelect(voiceLabels, nil)
	defaultVoiceSel.SetSelected(tts.VoiceDisplayName(cfg.Audio.DefaultVoice))

	previewDefaultBtn := mw.newTipBtn("Preview Voice", theme.MediaPlayIcon(), "Synthesize and play a sample with the selected default voice", func() {
		vid := labelToVoiceID[defaultVoiceSel.Selected]
		if vid == "" {
			vid = tts.VoiceIDFromLabel(defaultVoiceSel.Selected)
		}
		mw.Session.PreviewVoice(vid, "Hello from Hearsay! Ready to read live chat aloud.")
	})

	chatterVoiceMap := make(map[string]string)
	if cfg.Identity.ChatterVoices != nil {
		maps.Copy(chatterVoiceMap, cfg.Identity.ChatterVoices)
	}
	chatterAssignUserEntry := widget.NewEntry()
	chatterAssignUserEntry.SetPlaceHolder("Username (e.g. mocomade)")
	chatterAssignVoiceSel := widget.NewSelect(voiceLabels, nil)
	if len(voiceLabels) > 0 {
		chatterAssignVoiceSel.SetSelected(voiceLabels[0])
	}
	chatterListBox := container.NewVBox()

	var rebuildChatterVoiceRows func()
	rebuildChatterVoiceRows = func() {
		chatterListBox.RemoveAll()
		keys := slices.Sorted(maps.Keys(chatterVoiceMap))
		if len(keys) == 0 {
			chatterListBox.Add(widget.NewLabelWithStyle("No custom chatter voice assignments yet.", fyne.TextAlignLeading, fyne.TextStyle{Italic: true}))
		} else {
			for _, u := range keys {
				uname := u
				vid := chatterVoiceMap[uname]
				lbl := widget.NewLabel(fmt.Sprintf("%s → %s", uname, tts.VoiceDisplayName(vid)))
				lbl.Truncation = fyne.TextTruncateEllipsis
				testBtn := mw.newTipBtn("", theme.MediaPlayIcon(), "Preview voice for "+uname, func() {
					mw.Session.PreviewVoice(vid, fmt.Sprintf("Hello, this is %s speaking in chat!", uname))
				})
				delBtn := mw.newTipBtn("", theme.DeleteIcon(), "Remove custom voice for "+uname, func() {
					delete(chatterVoiceMap, uname)
					rebuildChatterVoiceRows()
				})
				chatterListBox.Add(container.NewBorder(nil, nil, nil, container.NewHBox(testBtn, delBtn), lbl))
			}
		}
		chatterListBox.Refresh()
	}
	rebuildChatterVoiceRows()

	addChatterVoiceBtn := mw.newTipBtn("Assign", theme.ContentAddIcon(), "Assign selected voice to this username", func() {
		u := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(chatterAssignUserEntry.Text), "@"))
		if u == "" {
			return
		}
		vid := labelToVoiceID[chatterAssignVoiceSel.Selected]
		if vid == "" {
			vid = tts.VoiceIDFromLabel(chatterAssignVoiceSel.Selected)
		}
		chatterVoiceMap[u] = vid
		chatterAssignUserEntry.SetText("")
		rebuildChatterVoiceRows()
	})

	disabledChatterVoices := slices.Clone(cfg.Audio.DisabledChatterVoices)
	permittedVoicesLbl := widget.NewLabel(formatPermittedVoicesSummary(cfg.Audio.EffectiveEnabledModels(), disabledChatterVoices))

	checkPackUpdatesBtn := mw.newTipBtn("Check for Voice Updates", theme.ViewRefreshIcon(), "Check GitHub Releases for updated neural TTS voice model weights", nil)
	updatePacksBtn := mw.newTipBtn("Update Voice Models", theme.UploadIcon(), "Download and install updated voice model packs", nil)
	updatePacksBtn.Importance = widget.HighImportance

	syncUpdateBtnState := func() {
		outdated := tts.OutdatedModelFamilies(modelUpdateStatuses)
		if len(outdated) > 0 {
			updatePacksBtn.SetText(fmt.Sprintf("Update Voice Models (%d)", len(outdated)))
			updatePacksBtn.Enable()
		} else {
			updatePacksBtn.SetText("Update Voice Models")
			updatePacksBtn.Disable()
		}
	}
	syncUpdateBtnState()

	checkPackUpdatesBtn.OnTapped = func() {
		checkPackUpdatesBtn.Disable()
		packStatusLbl.SetText("Checking GitHub Releases for voice model updates...")
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			statuses, err := tts.CheckModelUpdates(ctx, modelDir, mw.Session.ConfigSnapshot().Audio.UseGPU)
			fyne.Do(func() {
				checkPackUpdatesBtn.Enable()
				modelUpdateStatuses = statuses
				outdated := tts.OutdatedModelFamilies(statuses)
				for _, spec := range tts.AllModelFamilies {
					if c, ok := modelChecks[spec.ID]; ok {
						c.SetText(formatPackLabel(spec))
						c.Refresh()
					}
				}
				syncUpdateBtnState()
				if err != nil && len(outdated) == 0 {
					packStatusLbl.SetText("Could not check remote voice updates: " + err.Error())
					return
				}
				if len(outdated) > 0 {
					names := make([]string, 0, len(outdated))
					for _, id := range outdated {
						if spec, ok := tts.ModelSpecByID(id); ok {
							names = append(names, spec.ShortName)
						} else {
							names = append(names, id)
						}
					}
					packStatusLbl.SetText(fmt.Sprintf("⬆ %d voice model update(s) available: %s", len(outdated), strings.Join(names, ", ")))
				} else {
					packStatusLbl.SetText("✓ All installed voice model packs are up to date.")
				}
			})
		}()
	}

	updatePacksBtn.OnTapped = func() {
		outdated := tts.OutdatedModelFamilies(modelUpdateStatuses)
		if len(outdated) == 0 {
			return
		}
		updatePacksBtn.Disable()
		checkPackUpdatesBtn.Disable()
		packProgress.SetValue(0)
		packProgress.Show()
		packStatusLbl.SetText("Updating outdated voice packs...")

		go func() {
			err := mw.Session.UpdateModelFamilies(modelDir, outdated, func(modelName string, pct float64) {
				fyne.Do(func() {
					packStatusLbl.SetText(fmt.Sprintf("Updating %s (%.0f%%)...", modelName, pct*100))
					packProgress.SetValue(pct)
				})
			})
			fyne.Do(func() {
				checkPackUpdatesBtn.Enable()
				packProgress.Hide()
				if err != nil {
					syncUpdateBtnState()
					packStatusLbl.SetText("Error updating voice packs: " + err.Error())
					return
				}
				updatedCfg := mw.Session.ConfigSnapshot()
				modelUpdateStatuses = tts.InspectAllLocalModelStatuses(modelDir, updatedCfg.Audio.UseGPU)
				for _, spec := range tts.AllModelFamilies {
					if c, ok := modelChecks[spec.ID]; ok {
						c.SetText(formatPackLabel(spec))
						c.Refresh()
					}
				}
				syncUpdateBtnState()
				voiceLabels, labelToVoiceID = availableVoiceOptions(updatedCfg, mw.Session.Engine())
				defaultVoiceSel.Options = voiceLabels
				defaultVoiceSel.Refresh()
				chatterAssignVoiceSel.Options = voiceLabels
				if len(voiceLabels) > 0 {
					chatterAssignVoiceSel.SetSelected(voiceLabels[0])
				}
				chatterAssignVoiceSel.Refresh()
				permittedVoicesLbl.SetText(formatPermittedVoicesSummary(updatedCfg.Audio.EffectiveEnabledModels(), disabledChatterVoices))
				packStatusLbl.SetText("✓ Voice model packs updated to latest release!")
			})
		}()
	}

	applyPacksBtn := mw.newTipBtn("Download & Load Selected Voice Packs", theme.DownloadIcon(), "Download any missing model packs and hot-reload the TTS engine", nil)
	applyPacksBtn.Importance = widget.HighImportance
	applyPacksBtn.OnTapped = func() {
		var selectedModels []string
		for _, spec := range tts.AllModelFamilies {
			if c, ok := modelChecks[spec.ID]; ok && c.Checked {
				selectedModels = append(selectedModels, spec.ID)
			}
		}
		if len(selectedModels) == 0 {
			selectedModels = []string{tts.ModelSupertonic}
			if c, ok := modelChecks[tts.ModelSupertonic]; ok {
				c.SetChecked(true)
			}
		}
		prevModels := slices.Clone(mw.Session.ConfigSnapshot().Audio.EffectiveEnabledModels())
		_ = mw.Session.UpdateConfig(func(c *config.Config) {
			c.Audio.OutputDeviceID = dlgDevMap[dlgDeviceSel.Selected]
			c.Audio.EnabledModels = selectedModels
		})
		applyPacksBtn.Disable()
		packProgress.SetValue(0)
		packProgress.Show()
		packStatusLbl.SetText("Downloading / initializing voice packs...")

		go func() {
			err := mw.Session.ReloadEngine(modelDir, func(modelName string, pct float64) {
				fyne.Do(func() {
					packStatusLbl.SetText(fmt.Sprintf("Downloading %s (%.0f%%)...", modelName, pct*100))
					packProgress.SetValue(pct)
				})
			})
			fyne.Do(func() {
				applyPacksBtn.Enable()
				packProgress.Hide()
				if err != nil {
					_ = mw.Session.UpdateConfig(func(c *config.Config) {
						c.Audio.EnabledModels = prevModels
					})
					packStatusLbl.SetText("Error loading voice packs: " + err.Error())
					return
				}
				updatedCfg := mw.Session.ConfigSnapshot()
				for _, spec := range tts.AllModelFamilies {
					if c, ok := modelChecks[spec.ID]; ok {
						c.SetText(formatPackLabel(spec))
						c.Refresh()
					}
				}
				voiceLabels, labelToVoiceID = availableVoiceOptions(updatedCfg, mw.Session.Engine())
				defaultVoiceSel.Options = voiceLabels
				defaultVoiceSel.Refresh()
				chatterAssignVoiceSel.Options = voiceLabels
				if len(voiceLabels) > 0 {
					chatterAssignVoiceSel.SetSelected(voiceLabels[0])
				}
				chatterAssignVoiceSel.Refresh()
				permittedVoicesLbl.SetText(formatPermittedVoicesSummary(updatedCfg.Audio.EffectiveEnabledModels(), disabledChatterVoices))
				packStatusLbl.SetText(fmt.Sprintf("Loaded %d total voices across %d pack(s)!", len(voiceLabels), len(selectedModels)))
			})
		}()
	}

	// 3. Coalescence, Chat Rules & Crowd Control
	currentSelectedModels := func() []string {
		var sel []string
		for _, spec := range tts.AllModelFamilies {
			if c, ok := modelChecks[spec.ID]; ok && c.Checked {
				sel = append(sel, spec.ID)
			}
		}
		if len(sel) == 0 {
			return cfg.Audio.EffectiveEnabledModels()
		}
		return sel
	}

	permittedVoicesBtn := mw.newTipBtn("Select Voices...", theme.ListIcon(), "Choose which voices are permitted for automatic chatter voice assignment", func() {
		showPermittedVoicesDialog(
			mw.Window,
			currentSelectedModels(),
			disabledChatterVoices,
			func(vid string) {
				mw.Session.PreviewVoice(vid, "Hello from Hearsay! Ready to read live chat aloud.")
			},
			func(updatedDisabled []string) {
				disabledChatterVoices = slices.Clone(updatedDisabled)
				permittedVoicesLbl.SetText(formatPermittedVoicesSummary(currentSelectedModels(), disabledChatterVoices))
			},
		)
	})

	unknownGenderSel := widget.NewSelect(unknownGenderOptions, nil)
	unknownGenderSel.SetSelected(unknownGenderPolicyToLabel(cfg.Audio.UnknownGenderPolicy))

	smartGenderCk := widget.NewCheck("Smart gender voice matching (chat pronouns + username analysis)", func(b bool) {
		if b {
			unknownGenderSel.Enable()
		} else {
			unknownGenderSel.Disable()
		}
	})
	smartGenderCk.SetChecked(cfg.Audio.SmartGenderVoices)

	hashVoicesCk := widget.NewCheck("Assign a unique neural voice to each chatter automatically (never pollutes custom list)", func(b bool) {
		if b {
			smartGenderCk.Enable()
			permittedVoicesBtn.Enable()
			if smartGenderCk.Checked {
				unknownGenderSel.Enable()
			} else {
				unknownGenderSel.Disable()
			}
		} else {
			smartGenderCk.Disable()
			unknownGenderSel.Disable()
			permittedVoicesBtn.Disable()
		}
	})
	hashVoicesCk.SetChecked(cfg.Audio.HashChatterVoices)
	if !cfg.Audio.HashChatterVoices {
		smartGenderCk.Disable()
		unknownGenderSel.Disable()
		permittedVoicesBtn.Disable()
	} else if !cfg.Audio.SmartGenderVoices {
		unknownGenderSel.Disable()
	}

	speakSwitchCk := widget.NewCheck("Announce channel name only when active speaker switches streams", nil)
	speakSwitchCk.SetChecked(cfg.Audio.SpeakChannelOnSwitch)

	speakNamesCk := widget.NewCheck("Speak chatter usernames before messages", nil)
	speakNamesCk.SetChecked(cfg.Audio.SpeakNames)

	naturalizeNamesCk := widget.NewCheck("Naturalize chatter usernames (split CamelCase/snake_case, strip wrappers & trailing numbers)", nil)
	naturalizeNamesCk.SetChecked(cfg.Audio.NaturalizeNames)

	coalesceSel := widget.NewSelect(coalesceOptions, nil)
	coalesceSel.SetSelected(coalesceSecToLabel(cfg.Audio.CoalesceSeconds))

	speakEmotesCk := widget.NewCheck("Speak emote & emoji names aloud (unchecked = strip emotes)", nil)
	speakEmotesCk.SetChecked(cfg.Audio.SpeakEmotes)

	dedupeEmotesCk := widget.NewCheck("Deduplicate repeated emotes & emojis (speak at most once per message)", nil)
	dedupeEmotesCk.SetChecked(cfg.Audio.DeduplicateEmotes)

	dupWordsSel := widget.NewSelect(duplicateWordsOptions, nil)
	dupWordsSel.SetSelected(dupWordsToLabel(cfg.Audio.MaxDuplicateWords))

	earconsCk := widget.NewCheck("Play spatial earcons (Stream Live/Offline & @Mention chimes)", nil)
	earconsCk.SetChecked(cfg.Audio.EarconsEnabled)

	showChannelIconsCk := widget.NewCheck("Show compact channel avatar icons instead of channel names in Live Log", nil)
	showChannelIconsCk.SetChecked(cfg.Accessibility.ShowChannelIcons)

	cooldownEntry := widget.NewEntry()
	cooldownEntry.SetText(strconv.Itoa(cfg.Crowd.ChatterCooldownSec))

	pacingEntry := widget.NewEntry()
	pacingEntry.SetText(strconv.Itoa(cfg.Crowd.GlobalPacingMs))

	skipSpamCk := widget.NewCheck("Skip 1-word hype spam (LUL, GG, W, F, KEKW)", nil)
	skipSpamCk.SetChecked(cfg.Crowd.SkipShortSpam)

	speakNoTextCk := widget.NewCheck("Speak no-text events (follows, shares, gift streaks)", nil)
	speakNoTextCk.SetChecked(cfg.Crowd.SpeakNoTextEvents)

	// 4. Translation & Hotkeys
	transEnabledCk := widget.NewCheck("Automatically translate foreign-language chat messages", nil)
	transEnabledCk.SetChecked(cfg.Translation.Enabled)

	targetLangEntry := widget.NewEntry()
	targetLangEntry.SetPlaceHolder("en, es, fr, de, ja...")
	targetLangEntry.SetText(cfg.Translation.TargetLang)

	hkPauseEntry := widget.NewEntry()
	hkPauseEntry.SetText(cfg.Accessibility.Hotkeys.PauseResume)
	hkSkipEntry := widget.NewEntry()
	hkSkipEntry.SetText(cfg.Accessibility.Hotkeys.SkipCurrent)
	hkReplayEntry := widget.NewEntry()
	hkReplayEntry.SetText(cfg.Accessibility.Hotkeys.ReplayLast)
	hkPanicEntry := widget.NewEntry()
	hkPanicEntry.SetText(cfg.Accessibility.Hotkeys.PanicMute)

	chatterScroll := container.NewVScroll(chatterListBox)
	chatterScroll.SetMinSize(fyne.NewSize(200, 110))

	myUsersDesc := widget.NewLabel("My Username(s) — your own chat messages are muted, and @mentions to you play a spatial chime and bypass Crowd Control. Tip: use optional platform:username (e.g. twitch:mrpoundsign, youtube:@MrPoundSign) if your handle differs by platform:")
	myUsersDesc.Wrapping = fyne.TextWrapWord

	priorityDesc := widget.NewLabel("Priority VIPs / Friends (always spoken, bypasses Crowd Control):")
	priorityDesc.Wrapping = fyne.TextWrapWord

	ignoredDesc := widget.NewLabel("Muted Chatters (never spoken):")
	ignoredDesc.Wrapping = fyne.TextWrapWord

	tabs := container.NewAppTabs(
		container.NewTabItem("Voices & Voice Packs", container.NewVScroll(container.NewVBox(
			container.NewBorder(nil, nil, widget.NewLabelWithStyle("Audio Output Device:", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), dlgDeviceRefreshBtn, dlgDeviceSel),
			widget.NewSeparator(),
			widget.NewLabelWithStyle("Neural TTS Voice Model Packs (Shared with TownCrier):", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			modelRows,
			container.NewGridWithColumns(3, applyPacksBtn, checkPackUpdatesBtn, updatePacksBtn),
			packProgress,
			packStatusLbl,
			widget.NewSeparator(),
			hashVoicesCk,
			smartGenderCk,
			container.NewBorder(nil, nil, widget.NewLabel("Unknown Gender Voice Pool:"), nil, unknownGenderSel),
			container.NewBorder(nil, nil, widget.NewLabel("Permitted Auto-Assign Voices:"), permittedVoicesBtn, permittedVoicesLbl),
			container.NewBorder(nil, nil, widget.NewLabel("Default Voice:"), previewDefaultBtn, defaultVoiceSel),
			widget.NewSeparator(),
			widget.NewLabelWithStyle("Custom Chatter Voice Assignments:", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			container.NewBorder(nil, nil, nil, addChatterVoiceBtn, container.NewGridWithColumns(2, chatterAssignUserEntry, chatterAssignVoiceSel)),
			chatterScroll,
		))),
		container.NewTabItem("Coalescence & Crowd Control", container.NewVScroll(container.NewVBox(
			widget.NewLabelWithStyle("Speech Coalescence & Formatting:", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			speakNamesCk,
			naturalizeNamesCk,
			container.NewBorder(nil, nil, widget.NewLabel("Coalesce Consecutive Messages (Same Speaker):"), nil, coalesceSel),
			speakSwitchCk,
			speakEmotesCk,
			dedupeEmotesCk,
			container.NewBorder(nil, nil, widget.NewLabel("Max Consecutive Duplicate Words:"), nil, dupWordsSel),
			earconsCk,
			showChannelIconsCk,
			widget.NewSeparator(),
			widget.NewLabelWithStyle("Crowd Control & Pacing:", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			skipSpamCk,
			speakNoTextCk,
			container.NewGridWithColumns(2,
				widget.NewLabel("Per-Chatter Cooldown (sec, 0=off):"), cooldownEntry,
				widget.NewLabel("Min Global Pacing Gap (ms, 0=off):"), pacingEntry,
			),
		))),
		container.NewTabItem("Identity & Friends", container.NewVScroll(container.NewVBox(
			myUsersDesc,
			myUsernamesEntry,
			priorityDesc,
			priorityEntry,
			ignoredDesc,
			ignoredEntry,
		))),
		container.NewTabItem("Translation & Hotkeys", container.NewVScroll(container.NewVBox(
			transEnabledCk,
			widget.NewLabel("Target Language (ISO-639-1):"),
			targetLangEntry,
			widget.NewSeparator(),
			widget.NewLabel("Global Hotkeys (e.g. Ctrl+Shift+P):"),
			container.NewGridWithColumns(2,
				widget.NewLabel("Pause / Resume:"), hkPauseEntry,
				widget.NewLabel("Skip Current:"), hkSkipEntry,
				widget.NewLabel("Replay Last:"), hkReplayEntry,
				widget.NewLabel("Panic Mute:"), hkPanicEntry,
			),
		))),
	)

	d := dialog.NewCustomConfirm("Hearsay Settings & Voices", "Save Changes", "Cancel", tabs, func(save bool) {
		if !save {
			return
		}
		var selectedModels []string
		for _, spec := range tts.AllModelFamilies {
			if c, ok := modelChecks[spec.ID]; ok && c.Checked {
				selectedModels = append(selectedModels, spec.ID)
			}
		}
		if len(selectedModels) == 0 {
			selectedModels = []string{tts.ModelSupertonic}
		}
		defVoiceID := labelToVoiceID[defaultVoiceSel.Selected]
		if defVoiceID == "" {
			defVoiceID = tts.VoiceIDFromLabel(defaultVoiceSel.Selected)
		}

		prevModels := slices.Clone(mw.Session.ConfigSnapshot().Audio.EffectiveEnabledModels())
		modelsChanged := !slices.Equal(prevModels, selectedModels)
		needsReload := modelsChanged || mw.Session.Engine() == nil
		if !needsReload {
			for _, m := range selectedModels {
				if !tts.IsModelFamilyInstalled(modelDir, m, cfg.Audio.UseGPU) {
					needsReload = true
					break
				}
			}
		}

		_ = mw.Session.UpdateConfig(func(c *config.Config) {
			c.Identity.MyUsernames = parseCSVList(myUsernamesEntry.Text)
			c.Identity.PriorityChatters = parseCSVList(priorityEntry.Text)
			c.Identity.IgnoredChatters = parseCSVList(ignoredEntry.Text)
			c.Identity.ChatterVoices = maps.Clone(chatterVoiceMap)

			c.Audio.OutputDeviceID = dlgDevMap[dlgDeviceSel.Selected]
			c.Audio.EnabledModels = selectedModels
			if defVoiceID != "" {
				c.Audio.DefaultVoice = defVoiceID
			}
			c.Audio.HashChatterVoices = hashVoicesCk.Checked
			c.Audio.SmartGenderVoices = smartGenderCk.Checked
			c.Audio.UnknownGenderPolicy = labelToUnknownGenderPolicy(unknownGenderSel.Selected)
			c.Audio.DisabledChatterVoices = slices.Clone(disabledChatterVoices)
			c.Audio.SpeakChannelOnSwitch = speakSwitchCk.Checked
			c.Audio.SpeakNames = speakNamesCk.Checked
			c.Audio.NaturalizeNames = naturalizeNamesCk.Checked
			c.Audio.CoalesceSeconds = labelToCoalesceSec(coalesceSel.Selected)
			c.Audio.SpeakEmotes = speakEmotesCk.Checked
			c.Audio.DeduplicateEmotes = dedupeEmotesCk.Checked
			c.Audio.MaxDuplicateWords = labelToDupWords(dupWordsSel.Selected)
			c.Audio.EarconsEnabled = earconsCk.Checked

			c.Accessibility.ShowChannelIcons = showChannelIconsCk.Checked

			if cd, err := strconv.Atoi(strings.TrimSpace(cooldownEntry.Text)); err == nil && cd >= 0 {
				c.Crowd.ChatterCooldownSec = cd
			}
			if pg, err := strconv.Atoi(strings.TrimSpace(pacingEntry.Text)); err == nil && pg >= 0 {
				c.Crowd.GlobalPacingMs = pg
			}
			c.Crowd.SkipShortSpam = skipSpamCk.Checked
			c.Crowd.SpeakNoTextEvents = speakNoTextCk.Checked

			c.Translation.Enabled = transEnabledCk.Checked
			c.Translation.TargetLang = strings.TrimSpace(targetLangEntry.Text)

			c.Accessibility.Hotkeys.PauseResume = strings.TrimSpace(hkPauseEntry.Text)
			c.Accessibility.Hotkeys.SkipCurrent = strings.TrimSpace(hkSkipEntry.Text)
			c.Accessibility.Hotkeys.ReplayLast = strings.TrimSpace(hkReplayEntry.Text)
			c.Accessibility.Hotkeys.PanicMute = strings.TrimSpace(hkPanicEntry.Text)
		})
		updated := mw.Session.ConfigSnapshot()
		mw.refreshOutputDeviceSelect()
		if mw.speakEmotesCk != nil && mw.speakEmotesCk.Checked != updated.Audio.SpeakEmotes {
			mw.speakEmotesCk.SetChecked(updated.Audio.SpeakEmotes)
		}
		if mw.channelIconsCk != nil && mw.channelIconsCk.Checked != updated.Accessibility.ShowChannelIcons {
			mw.channelIconsCk.SetChecked(updated.Accessibility.ShowChannelIcons)
		}
		mw.registerHotkeys(updated.Accessibility.Hotkeys)
		mw.refreshLog()

		if needsReload {
			go func() {
				err := mw.Session.ReloadEngine(modelDir, func(modelName string, pct float64) {
					fyne.Do(func() {
						if mw.nowSpeakingLbl != nil {
							mw.nowSpeakingLbl.SetText(fmt.Sprintf("Downloading %s voice pack (%.0f%%)...", modelName, pct*100))
						}
					})
				})
				fyne.Do(func() {
					if err != nil {
						_ = mw.Session.UpdateConfig(func(c *config.Config) {
							c.Audio.EnabledModels = prevModels
						})
						if mw.nowSpeakingLbl != nil {
							mw.nowSpeakingLbl.SetText("Voice pack download failed: " + err.Error())
						}
						return
					}
					if mw.nowSpeakingLbl != nil {
						mw.nowSpeakingLbl.SetText(fmt.Sprintf("Loaded %d voice pack(s) ready!", len(selectedModels)))
					}
				})
			}()
		}
	}, mw.Window)
	d.Resize(fyne.NewSize(720, 560))
	d.Show()
}

func parseCSVList(raw string) []string {
	var out []string
	for part := range strings.SplitSeq(raw, ",") {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func formatPermittedVoicesSummary(enabledModels []string, disabledVoices []string) string {
	all := tts.VoiceIDsForModels(enabledModels)
	if len(all) == 0 {
		all = tts.VoiceIDsForModels([]string{tts.ModelSupertonic})
	}
	permitted := chat.FilterPermittedVoices(all, disabledVoices)
	fem, masc, _ := chat.SplitVoicePoolByGender(permitted)
	if len(permitted) == len(all) {
		return fmt.Sprintf("All %d voices permitted (%dF / %dM)", len(all), len(fem), len(masc))
	}
	return fmt.Sprintf("%d of %d voices permitted (%dF / %dM)", len(permitted), len(all), len(fem), len(masc))
}

func showPermittedVoicesDialog(
	w fyne.Window,
	enabledModels []string,
	initialDisabled []string,
	onPreview func(voiceID string),
	onSave func(disabledVoices []string),
) {
	if len(enabledModels) == 0 {
		enabledModels = []string{tts.ModelSupertonic}
	}
	enabledSet := make(map[string]bool, len(enabledModels))
	for _, m := range enabledModels {
		clean := strings.ToLower(strings.TrimSpace(m))
		if clean != "" {
			enabledSet[clean] = true
		}
	}

	disabledSet := make(map[string]bool, len(initialDisabled))
	for _, d := range initialDisabled {
		if k := chat.CanonicalVoicePoolKey(d); k != "" {
			disabledSet[k] = true
		}
	}

	type checkItem struct {
		voiceID string
		check   *widget.Check
	}
	var items []checkItem
	var syncingChecks bool

	summaryLbl := widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	updateSummary := func() {
		var curDisabled []string
		allIDs := make([]string, 0, len(items))
		for _, it := range items {
			allIDs = append(allIDs, it.voiceID)
			if !it.check.Checked {
				curDisabled = append(curDisabled, it.voiceID)
			}
		}
		permitted := chat.FilterPermittedVoices(allIDs, curDisabled)
		fem, masc, _ := chat.SplitVoicePoolByGender(permitted)
		summaryLbl.SetText(fmt.Sprintf("Permitted: %d of %d voices (%d Feminine, %d Masculine)", len(permitted), len(allIDs), len(fem), len(masc)))
	}

	listBox := container.NewVBox()
	for _, spec := range tts.AllModelFamilies {
		if !enabledSet[spec.ID] {
			continue
		}
		hdr := widget.NewLabelWithStyle(spec.ShortName, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
		listBox.Add(hdr)

		for _, vo := range spec.Voices {
			vid := vo.ID
			key := chat.CanonicalVoicePoolKey(vid)
			genderTag := "Neutral"
			switch chat.ClassifyVoiceGender(vid) {
			case chat.GenderFeminine:
				genderTag = "Feminine"
			case chat.GenderMasculine:
				genderTag = "Masculine"
			}

			ck := widget.NewCheck(fmt.Sprintf("%s  [%s]", vo.Label, genderTag), func(_ bool) {
				if syncingChecks {
					return
				}
				updateSummary()
			})
			ck.SetChecked(!disabledSet[key])
			items = append(items, checkItem{voiceID: vid, check: ck})

			prevBtn := widget.NewButtonWithIcon("", theme.MediaPlayIcon(), func() {
				if onPreview != nil {
					onPreview(vid)
				}
			})
			row := container.NewBorder(nil, nil, nil, prevBtn, ck)
			listBox.Add(row)
		}
		listBox.Add(widget.NewSeparator())
	}

	updateSummary()

	selectAllBtn := widget.NewButton("Select All", func() {
		syncingChecks = true
		for _, it := range items {
			it.check.SetChecked(true)
		}
		syncingChecks = false
		updateSummary()
	})
	deselectAllBtn := widget.NewButton("Deselect All", func() {
		syncingChecks = true
		for _, it := range items {
			it.check.SetChecked(false)
		}
		syncingChecks = false
		updateSummary()
	})

	topBar := container.NewBorder(
		nil,
		nil,
		nil,
		container.NewHBox(selectAllBtn, deselectAllBtn),
		summaryLbl,
	)

	scroll := container.NewVScroll(listBox)
	scroll.SetMinSize(fyne.NewSize(520, 360))

	desc := widget.NewLabel("Uncheck any voices you do not want automatically assigned to random chatters:")
	desc.Wrapping = fyne.TextWrapWord

	content := container.NewBorder(
		container.NewVBox(desc, topBar, widget.NewSeparator()),
		nil,
		nil,
		nil,
		scroll,
	)

	dlg := dialog.NewCustomConfirm("Permitted Chatter Voices", "Save", "Cancel", content, func(confirmed bool) {
		if !confirmed {
			return
		}
		var disabled []string
		for _, it := range items {
			if !it.check.Checked {
				disabled = append(disabled, it.voiceID)
			}
		}
		if onSave != nil {
			onSave(disabled)
		}
	}, w)
	dlg.Resize(fyne.NewSize(580, 500))
	dlg.Show()
}
