# Changelog

## v0.0.7 — 2026-11-27

### Changed
- **Compressed Setup & Portable Payloads (`appinstaller@v0.2.0`) & Windows Metadata**:
  - Switched build-time embedded payloads to zstd-compressed archives (`Hearsay-Setup.exe` reduced from ~77.2 MB to ~42.4 MB) with `.payload-sha256` fast-path caching for portable launches.
  - Migrated Windows executables to `go-winres` (embedding application icon, GUI DPI manifest, and version metadata) and added `.githooks/pre-commit` quality gates.
  - Upgraded shared core dependency to `TownCrier v0.2.0` (native platform emote & Twitch cheermote parsing improvements).

---

## v0.0.6 — 2026-11-26

### Fixed
- **GitHub Actions Release Publishing**:
  - Added `GH_REPO` and corrected `--clobber` in `.github/workflows/release.yml` so GitHub Actions publishes release assets cleanly without needing a repository checkout in the release job.

### Changed
- **Documentation (`mrpoundsign.com/Hearsay`)**:
  - Updated GitHub Pages (`index.html`) with configuration guides for the in-app GitHub Releases updater, smart installer pack preservation, anti-spam & chat hygiene filters, permitted auto-assign voice curation, and per-chatter voice speed multipliers.

---

## v0.0.5 — 2026-11-26

### Added
- **In-App SHA-256 Self-Updater & Smart Installer**:
  - Added an integrated GitHub Releases updater powered by `appinstaller@v0.1.0` with a top-header **`⬆ Update`** badge, update dialog, and a new **Settings → Updates** tab (supporting startup update checks and SHA-256 verification).
  - Updated `Hearsay-Setup.exe` with `IsInstalled` model-pack detection so existing neural TTS voice packs remain selected across silent updates and reinstalls.
- **Concurrent Vertical & Horizontal YouTube Live Streams**:
  - Automatically detects and joins both horizontal and vertical simultaneous YouTube Live streams on the same channel.
- **macOS Application Icon Bundling**:
  - Added `.icns` app bundle icon generation inside `build.sh` for macOS releases.

### Changed
- **Per-User Voice Speed Multiplier (#2)**:
  - Changed per-chatter voice speed overrides from absolute values into multipliers applied on top of your global default speech speed so global speed changes scale custom voices proportionally.
- **Core Audio & UI Engine**:
  - Upgraded to `TownCrier v0.1.0` (including Windows WASAPI playback stall auto-recovery and Fyne v2.8.3) and unified GitHub Actions CI & SLSA OIDC release workflows.

### Fixed
- **Punctuated Hype-Spam Filtering**:
  - Trimmed trailing pause punctuation before evaluating single-word hype-spam messages so punctuated hype emotes/words are filtered accurately.

---

## v0.0.4 — 2026-11-10

### Added
- **Anti-Spam & Chat Hygiene Filters**:
  - **Strip Zalgo & Excessive Combining Marks** (`strip_zalgo`): Strips stacked Unicode combining diacritics while preserving normal accented letters.
  - **Strip Braille & ASCII Art Spam** (`strip_ascii_art`): Strips Braille blank/dot art (`U+2800`–`U+28FF`), box-drawing/block art (`U+2500`–`U+259F`), and repeated symbol art before TTS synthesis.
  - **Multi-User Copypasta Deduplication** (`deduplicate_copypasta_window_seconds`: `Off`, `15s`, `30s`, `45s`, `60s`, `90s`, `2m`, `5m`, default `30s`): Suppresses identical normalized chat messages repeated across multiple chatters while never dropping paid or alert events.
  - **Max Duplicate Words & Bot/Command Filtering**: Added configurable **Max Consecutive Duplicate Words** (`max_duplicate_words`), **Ignore Known Chatbots** (`ignore_known_bots`), and **Ignore `!command` Messages** (`ignore_commands`) in **Filters & Moderation**.
- **Clickable Chatter Profile Links**:
  - Made chatter usernames clickable with hover underlines across the Live Feed to open the viewer's Twitch, YouTube, Kick, or TikTok channel profile in the browser.

---

## v0.0.3 — 2026-11-09

### Added
- **Automatic Username Naturalizer (`speak_natural_names`)**:
  - Converts screen-name syntax into natural, human-sounding speech before TTS synthesis (`xX_ShadowWolf_Xx` → `Shadow Wolf`, `DarkKnight_99` → `Dark Knight 99`, `Wolf1998` → `Wolf`, `TVGuy` → `TV Guy`) while preserving meaningful numbers (`Agent007`, `Player1`, `Wolf2`) and custom Pronunciation Dictionary overrides.
  - Added a **Naturalize Usernames** checkbox in **Settings → Speech Formatting & Coalescence** (enabled by default).
- **Permitted Chatter Voices Selector**:
  - Added an interactive **Permitted Auto-Assign Voices** checklist (`AllowedChatterVoices`) in the **Voices & Pronunciations** tab with **All**, **None**, **♀ Feminine**, and **♂ Masculine** quick-filter buttons, search filtering, and inline **🔊** voice preview buttons so listeners can curate the exact pool of voices used for deterministic chatter assignment.
- **Voice Model Update Detection & 1-Click Updating in Settings**:
  - Added upstream GitHub Release SHA-256 digest comparison (`CheckInstalledModelsWithUpdates`) for installed TTS model packs (`k2-fsa/sherpa-onnx@tts-models`).
  - Added an **🔄 Update Available** badge and **🔄 Check for Model Updates / ⬇ Install Model Updates** button in **Settings → Voice Packs & TTS Hardware** to download, verify, extract, and hot-reload updated voice model packs in-place.
- **Hot-Switching Audio Output Devices**:
  - Changing the **Output Device** selector in **Settings → Audio Output & Translation** now immediately switches active WASAPI/miniaudio playback via `player.SetOutputDevice(deviceID)` when clicking **Save Settings** without requiring an app restart.
- **Project Website, Privacy Policy & Terms of Service**:
  - Added GitHub Pages documentation site (`docs/index.html`, `docs/privacy.html`, `docs/terms.html`) with canonical SEO metadata.

### Fixed
- **Latched Panic Mute Toggle (`Ctrl+Shift+M` / `Esc`)**:
  - Fixed the **PANIC MUTE** button in both the main window and Mini-Player so clicking it (or pressing `Ctrl+Shift+M` / `Esc`) latches into an active **🚨 PANIC MUTED (Unmute)** state, flushes and blocks incoming TTS speech, displays **🚨 PANIC MUTED — Click to Unmute** in the Now Speaking banner, and unmutes cleanly when clicked again.
- **WASAPI Audio Output Device Routing**:
  - Upgraded to `TownCrier v0.0.11` to fix a `malgo.DeviceID` double-hex encoding bug in `pkg/audio` where selecting a specific non-default audio output device silently fell back to the OS default playback device.
- **Startup `SetTheme` Crash When Saved `UIScale` Is Configured**:
  - Fixed a startup panic in `ApplyUIScale` (`internal/ui/theme.go`) caused by calling `fyne.CurrentApp().Settings().SetTheme(...)` before the main Fyne window had been created and shown.

---

## v0.0.2 — 2026-11-09

### Added
- **Smart Gender Voice Matching & Two-Tier Pronoun Cache**:
  - Added automatic feminine/masculine/neutral voice matching powered by an asynchronous Alejo.io Twitch Pronoun API lookup cache, self-referential chat pronoun detection, and an offline 360+ common first-name lexicon.
- **Voice Pack Selector & Shared Model Cache**:
  - Added checkboxes in **Settings → Voice Packs & TTS Hardware** for **Kokoro-82M** (54 multi-language voices), **Supertonic** (10 ultra-fast English voices), and **VCTK** (109 English accents), sharing `%LOCALAPPDATA%\com.github.mrpoundsign.towncrier\models` with TownCrier.
- **Message Coalescence & Speech Formatting Controls**:
  - Added configurable **Coalesce Consecutive Messages** window (`Off`, `15s`, `30s`, `45s`, `60s`, `90s`, `2m`, `5m`), **Speak Emotes** toggle, and **Own Username (Mute Self)** filter so your own chat messages are not read back to you.
- **Compact Live Chat Log, Word-Wrap & UI Scaling**:
  - Added word-wrapped colored chat log entries with inline **🔊 Replay**, **👤 Assign Voice**, and **🔇 Mute Chatter** buttons, resizable split layout, non-modal hover tooltips, and configurable **UI Text & Control Size** (`85%`–`150%` or `Auto (OS)`).
- **Launch Skip Fallback & Dynamic Model Digest Verification**:
  - Added **Skip & Launch App** fallback on the Launch Status Window and dynamic GitHub Release SHA-256 verification for `k2-fsa/sherpa-onnx` model archives.
- **macOS `Hearsay.app` Bundle**:
  - Packaged macOS releases as an ad-hoc signed `Hearsay.app` bundle inside `Hearsay-v0.0.2-darwin-arm64.tar.gz`.

---

## v0.0.1 — 2026-11-08

### Added
- **Initial Release of Hearsay**:
  - Zero-login, multi-stream accessibility desktop application (Windows, Linux, and macOS) that reads livestream chats aloud across **Twitch**, **YouTube**, **Kick**, and **TikTok** using 100% local neural TTS.
  - Multi-stream **Streams Mixer** with per-channel volume sliders, stereo panning (`Left` ↔ `Right`), mute/solo toggles, and distinct spatial earcon chimes.
  - Deterministic per-chatter voice assignment, custom **Pronunciation Dictionary**, **Blocked Words & Bots** filtering, and multi-provider **Foreign Chat Translation** (`MyMemory`, `Google Cloud`, `DeepL`, `LibreTranslate`, `Ollama`).
  - Compact **Mini-Player** mode, global hotkeys, standalone portable launcher (`Hearsay.exe`), and Fyne GUI installer (`Hearsay-Setup.exe`) with SHA-256 verified self-updating.
