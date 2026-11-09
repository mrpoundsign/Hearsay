# Hearsay — Agent & Project Knowledge Base (AGENTS.md)

## 1. Project Overview

**Hearsay** (`github.com/mrpoundsign/Hearsay`) is a standalone, zero-login accessibility desktop application (Go + Fyne) that reads livestream chats aloud for viewers across Twitch, YouTube, Kick, and TikTok. It imports the core TTS, audio, translation, and platform libraries from [`github.com/mrpoundsign/TownCrier/pkg/...`](https://github.com/mrpoundsign/TownCrier).

- **Visual Identity**: Obsidian Slate (`#0F172A`), Chrome White (`#F8FAFC`), and Emerald Mint (`#34D399`).
- **Shared Model & Cache Directories**: Uses `tts.DefaultModelDir()` and `tts.DefaultSharedCacheDir()` (`%LOCALAPPDATA%\com.github.mrpoundsign.towncrier\...`) so models downloaded in either TownCrier or Hearsay work in both without duplication.

## 2. Quality Gate & Build Rules

1. **Pre-Commit Quality Gate** (run inside WSL via `./build.sh lint` and `./build.sh test`):
   - `go fix --diff ./...` (must report 0 diff)
   - `golangci-lint-v2 run ./...` (must report 0 issues)
   - `go test -race -count=1 -v ./...` (all tests must pass)
2. **Always Build via WSL (`./build.sh win`)**:
   - Never run `go build` natively on Windows.
   - All build outputs belong in `build/bin/` (`Hearsay.exe`, `Hearsay-Setup.exe`, `hearsay-core.exe`).
3. **Strict Test Isolation**:
   - Tests must NEVER touch real `%APPDATA%` or `~/.config` directories. Always use `config.NewMemoryStore()` or `config.TestLocations(t.TempDir())`.
   - Always inject mock audio device enumerators in headless UI tests.
