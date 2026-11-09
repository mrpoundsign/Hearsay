package assets

import (
	_ "embed"
	"strings"

	"fyne.io/fyne/v2"
)

//go:embed icon.png
var iconPNG []byte

// AppIcon is the embedded Hearsay application icon resource.
var AppIcon = fyne.NewStaticResource("icon.png", iconPNG)

var twitchSVG = []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" width="24" height="24">
  <rect x="1" y="1" width="22" height="22" rx="5" fill="#9146FF"/>
  <path d="M6.5 5L5 8v10h3.5v2.5L11 18h3l4.5-4.5V5H6.5zm10.5 7.8L14.5 15.2H11l-2.2 2.2V15.2H6.5V6.5H17v6.3z" fill="#FFFFFF"/>
  <rect x="13.5" y="8.5" width="1.6" height="4.2" fill="#FFFFFF"/>
  <rect x="10" y="8.5" width="1.6" height="4.2" fill="#FFFFFF"/>
</svg>`)

var youtubeSVG = []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" width="24" height="24">
  <rect x="1" y="3.5" width="22" height="17" rx="5" fill="#FF0000"/>
  <polygon points="9.5,8 16.5,12 9.5,16" fill="#FFFFFF"/>
</svg>`)

var kickSVG = []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" width="24" height="24">
  <rect x="1" y="1" width="22" height="22" rx="5" fill="#53FC18"/>
  <path d="M6.5 5.5h3.5v4.5h1.8V7.8h1.8V5.5h3.9v4.5h-2v1.8h-1.9v1.4h1.9v1.8h2v3.5h-3.9v-2.3h-1.8v-2.2h-1.8v4.5H6.5V5.5z" fill="#0B0E0F"/>
</svg>`)

var tiktokSVG = []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" width="24" height="24">
  <rect x="1" y="1" width="22" height="22" rx="5" fill="#111827"/>
  <path d="M15.8 5.2c.6 1.6 1.8 2.7 3.4 2.9v2.6c-1.3 0-2.5-.4-3.4-1.1v5.1c0 2.9-2.4 5.1-5.3 5.1-2.8 0-5-2.2-5-5s2.2-5 5-5c.3 0 .7 0 1 .1v2.7c-.3-.1-.6-.2-1-.2-1.3 0-2.3 1-2.3 2.3s1 2.3 2.3 2.3c1.4 0 2.5-1 2.5-2.5V5.2h2.8z" fill="#25F4EE"/>
  <path d="M15.2 5.8c.6 1.6 1.8 2.7 3.4 2.9v2.2c-1.3 0-2.5-.4-3.4-1.1v5.1c0 2.9-2.4 5.1-5.3 5.1-2.4 0-4.4-1.6-4.9-3.8.6 1.7 2.2 2.9 4.2 2.9 2.9 0 5.2-2.2 5.2-5.1V5.8h.8z" fill="#FE2C55"/>
</svg>`)

var genericStreamSVG = []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" width="24" height="24">
  <rect x="1" y="1" width="22" height="22" rx="5" fill="#38BDF8"/>
  <circle cx="12" cy="12" r="4" fill="#0F172A"/>
</svg>`)

var (
	TwitchIcon        = fyne.NewStaticResource("twitch.svg", twitchSVG)
	YouTubeIcon       = fyne.NewStaticResource("youtube.svg", youtubeSVG)
	KickIcon          = fyne.NewStaticResource("kick.svg", kickSVG)
	TikTokIcon        = fyne.NewStaticResource("tiktok.svg", tiktokSVG)
	GenericStreamIcon = fyne.NewStaticResource("stream.svg", genericStreamSVG)
)

// PlatformIcon returns the embedded vector icon resource for a streaming platform.
func PlatformIcon(platform string) fyne.Resource {
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case "twitch":
		return TwitchIcon
	case "youtube":
		return YouTubeIcon
	case "kick":
		return KickIcon
	case "tiktok":
		return TikTokIcon
	default:
		return GenericStreamIcon
	}
}

// PlatformDisplayName returns the human-friendly capitalized platform name.
func PlatformDisplayName(platform string) string {
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case "twitch":
		return "Twitch"
	case "youtube":
		return "YouTube"
	case "kick":
		return "Kick"
	case "tiktok":
		return "TikTok"
	default:
		return "Stream"
	}
}
