package listener

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// SupportedPlatforms lists the canonical platform IDs supported by Hearsay.
var SupportedPlatforms = []string{"twitch", "youtube", "kick", "tiktok"}

// ParseTargetInput parses a stream URL, "platform:channel" shorthand, or bare channel handle
// into a canonical (platform, channel) pair.
func ParseTargetInput(raw string, fallbackPlatform string) (string, string, error) {
	input := strings.TrimSpace(raw)
	if input == "" {
		return "", "", errors.New("stream URL or channel name cannot be empty")
	}

	fb := normalizePlatformAlias(fallbackPlatform)
	if fb == "" {
		fb = "twitch"
	}

	// Check if input looks like a URL (with scheme or known domain prefix)
	lower := strings.ToLower(input)
	if strings.HasPrefix(lower, "http://") ||
		strings.HasPrefix(lower, "https://") ||
		strings.HasPrefix(lower, "www.") ||
		strings.HasPrefix(lower, "twitch.tv/") ||
		strings.HasPrefix(lower, "youtube.com/") ||
		strings.HasPrefix(lower, "youtu.be/") ||
		strings.HasPrefix(lower, "kick.com/") ||
		strings.HasPrefix(lower, "tiktok.com/") {
		urlStr := input
		if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
			urlStr = "https://" + input
		}
		return parseStreamURL(urlStr)
	}

	// Check for "platform:channel" shorthand (e.g. "twitch:piratesoftware", "yt:@PirateSoftware")
	if prefix, rest, ok := strings.Cut(input, ":"); ok {
		plat := normalizePlatformAlias(prefix)
		ch := strings.TrimSpace(rest)
		if plat != "" && ch != "" {
			return plat, cleanChannelIdentifier(plat, ch), nil
		}
	}

	ch := cleanChannelIdentifier(fb, input)
	if ch == "" {
		return "", "", errors.New("channel name cannot be empty")
	}
	return fb, ch, nil
}

func parseStreamURL(rawURL string) (string, string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", "", fmt.Errorf("invalid stream URL: %w", err)
	}
	host := strings.ToLower(strings.TrimPrefix(u.Hostname(), "www."))
	host = strings.TrimPrefix(host, "m.")

	pathParts := splitNonEmptyPath(u.Path)

	switch {
	case host == "twitch.tv" || strings.HasSuffix(host, ".twitch.tv"):
		if len(pathParts) == 0 {
			return "", "", errors.New("twitch URL does not contain a channel name")
		}
		return "twitch", cleanChannelIdentifier("twitch", pathParts[0]), nil

	case host == "kick.com" || strings.HasSuffix(host, ".kick.com"):
		if len(pathParts) == 0 {
			return "", "", errors.New("kick URL does not contain a channel name")
		}
		return "kick", cleanChannelIdentifier("kick", pathParts[0]), nil

	case host == "tiktok.com" || strings.HasSuffix(host, ".tiktok.com"):
		if len(pathParts) == 0 {
			return "", "", errors.New("tiktok URL does not contain a creator handle")
		}
		return "tiktok", cleanChannelIdentifier("tiktok", pathParts[0]), nil

	case host == "youtu.be":
		if len(pathParts) == 0 {
			return "", "", errors.New("youtu.be URL does not contain a video ID")
		}
		return "youtube", pathParts[0], nil

	case host == "youtube.com" || strings.HasSuffix(host, ".youtube.com"):
		if v := strings.TrimSpace(u.Query().Get("v")); v != "" {
			return "youtube", v, nil
		}
		if len(pathParts) >= 2 && (pathParts[0] == "live" || pathParts[0] == "watch" || pathParts[0] == "channel" || pathParts[0] == "c" || pathParts[0] == "user") {
			return "youtube", pathParts[1], nil
		}
		if len(pathParts) >= 1 {
			return "youtube", pathParts[0], nil
		}
		return "", "", errors.New("youtube URL does not contain a channel or video ID")

	default:
		return "", "", fmt.Errorf("unsupported stream URL host %q", host)
	}
}

func splitNonEmptyPath(p string) []string {
	var out []string
	for seg := range strings.SplitSeq(p, "/") {
		trimmed := strings.TrimSpace(seg)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func normalizePlatformAlias(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "twitch", "tw":
		return "twitch"
	case "youtube", "yt":
		return "youtube"
	case "kick":
		return "kick"
	case "tiktok", "tt":
		return "tiktok"
	default:
		return ""
	}
}

func cleanChannelIdentifier(platform, raw string) string {
	trimmed := strings.TrimSpace(raw)
	trimmed = strings.Trim(trimmed, "/")
	switch platform {
	case "twitch", "kick", "tiktok":
		return strings.TrimPrefix(trimmed, "@")
	default:
		return trimmed
	}
}
