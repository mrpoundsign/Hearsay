package ui

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/mrpoundsign/Hearsay/internal/config"
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

	// 2. Voice Packs & Voices
	modelDir := tts.DefaultModelDir()
	enabledSet := make(map[string]bool)
	for _, m := range cfg.Audio.EffectiveEnabledModels() {
		enabledSet[m] = true
	}

	modelChecks := make(map[string]*widget.Check, len(tts.AllModelFamilies))
	modelRows := container.NewVBox()

	formatPackLabel := func(spec tts.ModelFamilySpec) string {
		installed := tts.IsModelFamilyInstalled(modelDir, spec.ID, cfg.Audio.UseGPU)
		if spec.ID == tts.ModelKokoro || spec.ID == tts.ModelKokoroIntl {
			installed = tts.IsKokoroInstalledForMode(modelDir, cfg.Audio.UseGPU)
		}
		status := fmt.Sprintf("Download ~%d MB", spec.DownloadSizeMB)
		if installed {
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
				packStatusLbl.SetText(fmt.Sprintf("Loaded %d total voices across %d pack(s)!", len(voiceLabels), len(selectedModels)))
			})
		}()
	}

	// 3. Coalescence, Chat Rules & Crowd Control
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
			if smartGenderCk.Checked {
				unknownGenderSel.Enable()
			} else {
				unknownGenderSel.Disable()
			}
		} else {
			smartGenderCk.Disable()
			unknownGenderSel.Disable()
		}
	})
	hashVoicesCk.SetChecked(cfg.Audio.HashChatterVoices)
	if !cfg.Audio.HashChatterVoices {
		smartGenderCk.Disable()
		unknownGenderSel.Disable()
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
			widget.NewLabelWithStyle("Neural TTS Voice Model Packs (Shared with TownCrier):", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			modelRows,
			applyPacksBtn,
			packProgress,
			packStatusLbl,
			widget.NewSeparator(),
			hashVoicesCk,
			smartGenderCk,
			container.NewBorder(nil, nil, widget.NewLabel("Unknown Gender Voice Pool:"), nil, unknownGenderSel),
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

			c.Audio.EnabledModels = selectedModels
			if defVoiceID != "" {
				c.Audio.DefaultVoice = defVoiceID
			}
			c.Audio.HashChatterVoices = hashVoicesCk.Checked
			c.Audio.SmartGenderVoices = smartGenderCk.Checked
			c.Audio.UnknownGenderPolicy = labelToUnknownGenderPolicy(unknownGenderSel.Selected)
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
