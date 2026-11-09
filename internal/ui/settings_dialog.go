package ui

import (
	"fmt"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/mrpoundsign/Hearsay/internal/config"
	"github.com/mrpoundsign/TownCrier/pkg/tts"
)

func (mw *MainWindow) showSettingsDialog() {
	cfg := mw.Session.ConfigSnapshot()

	// Identity & Mentions
	myUsernamesEntry := widget.NewEntry()
	myUsernamesEntry.SetPlaceHolder("Your Twitch/YouTube/Kick/TikTok username(s), comma-separated")
	myUsernamesEntry.SetText(strings.Join(cfg.Identity.MyUsernames, ", "))

	priorityEntry := widget.NewEntry()
	priorityEntry.SetPlaceHolder("Priority VIP/Friend usernames, comma-separated")
	priorityEntry.SetText(strings.Join(cfg.Identity.PriorityChatters, ", "))

	ignoredEntry := widget.NewEntry()
	ignoredEntry.SetPlaceHolder("Muted usernames, comma-separated")
	ignoredEntry.SetText(strings.Join(cfg.Identity.IgnoredChatters, ", "))

	// Audio & Voice Differentiation
	hashVoicesCk := widget.NewCheck("Assign a unique neural voice to each chatter (FNV-1a hash)", nil)
	hashVoicesCk.SetChecked(cfg.Audio.HashChatterVoices)

	speakSwitchCk := widget.NewCheck("Announce channel name only when active speaker switches streams", nil)
	speakSwitchCk.SetChecked(cfg.Audio.SpeakChannelOnSwitch)

	speakNamesCk := widget.NewCheck("Speak chatter usernames before messages", nil)
	speakNamesCk.SetChecked(cfg.Audio.SpeakNames)

	speakEmotesCk := widget.NewCheck("Speak emote names aloud", nil)
	speakEmotesCk.SetChecked(cfg.Audio.SpeakEmotes)

	earconsCk := widget.NewCheck("Play spatial earcons (Stream Live/Offline & @Mention chimes)", nil)
	earconsCk.SetChecked(cfg.Audio.EarconsEnabled)

	// Crowd Control
	cooldownEntry := widget.NewEntry()
	cooldownEntry.SetText(strconv.Itoa(cfg.Crowd.ChatterCooldownSec))

	pacingEntry := widget.NewEntry()
	pacingEntry.SetText(strconv.Itoa(cfg.Crowd.GlobalPacingMs))

	skipSpamCk := widget.NewCheck("Skip 1-word hype spam (LUL, GG, W, F, KEKW)", nil)
	skipSpamCk.SetChecked(cfg.Crowd.SkipShortSpam)

	speakNoTextCk := widget.NewCheck("Speak no-text events (follows, shares, gift streaks)", nil)
	speakNoTextCk.SetChecked(cfg.Crowd.SpeakNoTextEvents)

	// Translation
	transEnabledCk := widget.NewCheck("Automatically translate foreign-language chat messages", nil)
	transEnabledCk.SetChecked(cfg.Translation.Enabled)

	targetLangEntry := widget.NewEntry()
	targetLangEntry.SetPlaceHolder("en, es, fr, de, ja...")
	targetLangEntry.SetText(cfg.Translation.TargetLang)

	// Global Hotkeys
	hkPauseEntry := widget.NewEntry()
	hkPauseEntry.SetText(cfg.Accessibility.Hotkeys.PauseResume)
	hkSkipEntry := widget.NewEntry()
	hkSkipEntry.SetText(cfg.Accessibility.Hotkeys.SkipCurrent)
	hkReplayEntry := widget.NewEntry()
	hkReplayEntry.SetText(cfg.Accessibility.Hotkeys.ReplayLast)
	hkPanicEntry := widget.NewEntry()
	hkPanicEntry.SetText(cfg.Accessibility.Hotkeys.PanicMute)

	// Shared Neural TTS Model Pack Status
	modelDir := tts.DefaultModelDir()
	supertonicStatus := "Not Installed"
	if tts.IsModelFamilyInstalled(modelDir, tts.ModelSupertonic, false) {
		supertonicStatus = "Installed (Shared with TownCrier)"
	}
	kokoroStatus := "Not Installed"
	if tts.IsKokoroInstalledForMode(modelDir, false) {
		kokoroStatus = "Installed (Shared with TownCrier)"
	}
	modelStatusLbl := widget.NewLabel(fmt.Sprintf("Shared Models Dir: %s\n- Supertonic: %s\n- Kokoro-82M: %s", modelDir, supertonicStatus, kokoroStatus))

	tabs := container.NewAppTabs(
		container.NewTabItem("My Identity & Friends", container.NewVBox(
			widget.NewLabel("My Username(s) (@mentions play a spatial chime and bypass Crowd Control):"),
			myUsernamesEntry,
			widget.NewLabel("Priority Friends (always spoken):"),
			priorityEntry,
			widget.NewLabel("Muted Chatters (never spoken):"),
			ignoredEntry,
		)),
		container.NewTabItem("Crowd Control", container.NewVBox(
			skipSpamCk,
			speakNoTextCk,
			widget.NewLabel("Per-Chatter Cooldown (seconds, 0 = off):"),
			cooldownEntry,
			widget.NewLabel("Minimum Global Pacing Gap (milliseconds, 0 = off):"),
			pacingEntry,
		)),
		container.NewTabItem("Voices & Audio", container.NewVBox(
			hashVoicesCk,
			speakSwitchCk,
			speakNamesCk,
			speakEmotesCk,
			earconsCk,
			widget.NewSeparator(),
			modelStatusLbl,
		)),
		container.NewTabItem("Translation & Hotkeys", container.NewVBox(
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
		)),
	)

	d := dialog.NewCustomConfirm("Hearsay Settings", "Save Changes", "Cancel", tabs, func(save bool) {
		if !save {
			return
		}
		_ = mw.Session.UpdateConfig(func(c *config.Config) {
			c.Identity.MyUsernames = parseCSVList(myUsernamesEntry.Text)
			c.Identity.PriorityChatters = parseCSVList(priorityEntry.Text)
			c.Identity.IgnoredChatters = parseCSVList(ignoredEntry.Text)

			c.Audio.HashChatterVoices = hashVoicesCk.Checked
			c.Audio.SpeakChannelOnSwitch = speakSwitchCk.Checked
			c.Audio.SpeakNames = speakNamesCk.Checked
			c.Audio.SpeakEmotes = speakEmotesCk.Checked
			c.Audio.EarconsEnabled = earconsCk.Checked

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
		mw.registerHotkeys(updated.Accessibility.Hotkeys)
	}, mw.Window)
	d.Resize(fyne.NewSize(640, 480))
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
