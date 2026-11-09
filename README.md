# Hearsay — Multi-Stream Accessibility Chat Reader

**Hearsay** is an open-source, zero-login Windows & Linux desktop application that lets viewers **listen to live stream chats** across **Twitch**, **YouTube**, **Kick**, and **TikTok** using free, local neural Text-to-Speech (TTS).

Built on the [`TownCrier/pkg`](https://github.com/mrpoundsign/TownCrier) neural TTS and platform connector SDK, Hearsay is designed from the ground up for **stream viewers**, **blind & low-vision accessibility**, and **multi-stream co-watch sessions**.

---

## Key Features

- **100% Zero-Login & Read-Only**: Paste any stream URL (`twitch.tv/...`, `youtube.com/watch?v=...`, `kick.com/...`, `tiktok.com/@...`) or `platform:channel` handle and tune in immediately—no OAuth or accounts required.
- **Multi-Stream Audio Mixer & Spatial Stereo Panning**: Listen to multiple streams simultaneously (even multiple channels on the same platform) with independent volume gain and **Left / Center / Right constant-power stereo panning** (`-1.0` to `+1.0`).
- **Deterministic Multi-Voice Chatter Hashing**: Every chatter automatically receives a consistent neural voice (`FNV-1a` hash across the active voice pool) so back-and-forth conversations sound like distinct people.
- **Smart Channel-Switch Announcements**: Announces the channel name (`"MoCoMade: ..."`) only when the active speaker switches from one watched stream to another.
- **Crowd Control for High-Velocity Chats**:
  - Role/Badge filtering (`All Chatters`, `Subs + VIPs + Mods`, or `Mods + Streamer + @Mentions`).
  - Per-chatter cooldown timer & global pacing gap.
  - Hype/emote spam filter (skips 1-word spam like `LUL`, `GG`, `W`, `F`, `KEKW`).
  - Priority VIP/Friend list & `"My Username(s)"` `@mention` spatial chime bypass.
- **Two-Level Saved Library**:
  - **Saved Streamers**: Bookmark favorite creators (including multi-platform creators) with per-channel voice, volume, pan, and crowd filter settings.
  - **Lineups / Presets**: Save multi-stream co-watch groups with pre-configured Left/Right stereo panning and launch them in one click.
- **Accessibility First**:
  - **1-Click Always-on-Top Mini Player**: Collapses into a compact dockable overlay alongside your video player.
  - **Global Hotkeys**: System-wide shortcuts for `Pause/Resume`, `Skip Current`, `Replay Last`, and `<50ms` `Panic Mute`.
  - **High-Contrast Mode & Scalable UI (`100%`–`200%`)**.
  - **Shared Model Storage**: Automatically shares downloaded ONNX neural TTS models (`Supertonic`, `Kokoro-82M`, `Piper`) and audio cache with **TownCrier** in `%LOCALAPPDATA%\com.github.mrpoundsign.towncrier\models`.

---

## Building from Source (WSL Ubuntu -> Windows amd64)

```bash
./build.sh lint
./build.sh test
./build.sh win
```

Outputs `build/bin/Hearsay.exe` (portable single-file launcher) and `build/bin/Hearsay-Setup.exe` (standalone GUI installer).

## License

GNU Affero General Public License v3.0 (AGPL-3.0) — see [LICENSE](LICENSE).
