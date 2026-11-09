package ui

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode"

	"fyne.io/fyne/v2"

	"github.com/mrpoundsign/Hearsay/internal/listener"
)

var (
	avatarMu       sync.RWMutex
	avatarCache    = make(map[string]fyne.Resource)
	avatarInFlight = make(map[string]bool)
	avatarHTTP     = &http.Client{Timeout: 4 * time.Second}
)

// ParseHexColor parses a #RRGGBB hex string into a color.NRGBA, returning fallback if invalid.
func ParseHexColor(hexStr string, fallback color.Color) color.Color {
	raw := strings.TrimPrefix(strings.TrimSpace(hexStr), "#")
	if len(raw) != 6 {
		return fallback
	}
	r, err1 := strconv.ParseUint(raw[0:2], 16, 8)
	g, err2 := strconv.ParseUint(raw[2:4], 16, 8)
	b, err3 := strconv.ParseUint(raw[4:6], 16, 8)
	if err1 != nil || err2 != nil || err3 != nil {
		return fallback
	}
	return color.NRGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: 0xFF}
}

// ChannelAvatarResource returns a cached 48x48 circular channel icon resource for (platform, channel).
// It immediately returns a stylized monogram badge and, outside unit tests, fetches the live channel
// avatar in the background and invokes onLoaded when ready.
func ChannelAvatarResource(platform, channel string, onLoaded func()) fyne.Resource {
	plat := strings.ToLower(strings.TrimSpace(platform))
	ch := strings.TrimPrefix(strings.TrimSpace(channel), "@")
	if ch == "" {
		ch = "?"
	}
	key := plat + ":" + strings.ToLower(ch)

	avatarMu.RLock()
	res, ok := avatarCache[key]
	inFlight := avatarInFlight[key]
	avatarMu.RUnlock()

	if !ok {
		res = generateMonogramAvatar(key, plat, ch)
		avatarMu.Lock()
		if existing, exists := avatarCache[key]; exists {
			res = existing
		} else {
			avatarCache[key] = res
		}
		avatarMu.Unlock()
	}

	if !inFlight && !testing.Testing() {
		avatarMu.Lock()
		if !avatarInFlight[key] {
			avatarInFlight[key] = true
			go fetchRemoteChannelAvatar(key, plat, ch, onLoaded)
		}
		avatarMu.Unlock()
	}

	return res
}

func fetchRemoteChannelAvatar(key, platform, channel string, onLoaded func()) {
	provider := platform
	switch platform {
	case "twitch", "youtube", "kick", "tiktok":
	default:
		return
	}
	u := fmt.Sprintf("https://unavatar.io/%s/%s?fallback=false", url.PathEscape(provider), url.PathEscape(channel))
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return
	}
	req.Header.Set("User-Agent", "Hearsay/1.0")
	resp, err := avatarHTTP.Do(req)
	if err != nil {
		return
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil || len(data) == 0 {
		return
	}
	srcImg, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return
	}
	circPNG := encodeCircularAvatarPNG(srcImg, platformBorderColor(platform))
	if len(circPNG) == 0 {
		return
	}
	staticRes := fyne.NewStaticResource("avatar-"+key+".png", circPNG)

	avatarMu.Lock()
	avatarCache[key] = staticRes
	avatarMu.Unlock()

	if onLoaded != nil {
		fyne.Do(onLoaded)
	}
}

func platformBorderColor(platform string) color.NRGBA {
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case "twitch":
		return color.NRGBA{R: 0x91, G: 0x46, B: 0xFF, A: 0xFF}
	case "youtube":
		return color.NRGBA{R: 0xFF, G: 0x20, B: 0x20, A: 0xFF}
	case "kick":
		return color.NRGBA{R: 0x53, G: 0xFC, B: 0x18, A: 0xFF}
	case "tiktok":
		return color.NRGBA{R: 0x25, G: 0xF4, B: 0xEE, A: 0xFF}
	default:
		return color.NRGBA{R: 0x38, G: 0xBD, B: 0xF8, A: 0xFF}
	}
}

func encodeCircularAvatarPNG(src image.Image, borderCol color.NRGBA) []byte {
	const size = 48
	dst := image.NewNRGBA(image.Rect(0, 0, size, size))
	sb := src.Bounds()
	sw, sh := sb.Dx(), sb.Dy()
	if sw <= 0 || sh <= 0 {
		return nil
	}
	cx, cy := float64(size-1)/2.0, float64(size-1)/2.0
	outerR := float64(size)/2.0 - 1.0
	innerR := outerR - 2.5

	for y := range size {
		for x := range size {
			dx := float64(x) - cx
			dy := float64(y) - cy
			distSq := dx*dx + dy*dy
			if distSq > outerR*outerR {
				continue
			}
			if distSq >= innerR*innerR {
				dst.SetNRGBA(x, y, borderCol)
				continue
			}
			sx := sb.Min.X + (x*sw)/size
			sy := sb.Min.Y + (y*sh)/size
			dst.Set(x, y, src.At(sx, sy))
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return nil
	}
	return buf.Bytes()
}

func generateMonogramAvatar(key, platform, channel string) fyne.Resource {
	const size = 48
	dst := image.NewNRGBA(image.Rect(0, 0, size, size))
	borderCol := platformBorderColor(platform)
	fillHex := listener.ResolveChatterColorHex(platform, channel, "")
	fillCol, ok := ParseHexColor(fillHex, color.NRGBA{R: 0x1E, G: 0x29, B: 0x3B, A: 0xFF}).(color.NRGBA)
	if !ok {
		fillCol = color.NRGBA{R: 0x1E, G: 0x29, B: 0x3B, A: 0xFF}
	}
	// Darken fill slightly for high contrast white initial
	bgCol := color.NRGBA{
		R: fillCol.R / 3,
		G: fillCol.G / 3,
		B: fillCol.B / 3,
		A: 0xFF,
	}

	cx, cy := float64(size-1)/2.0, float64(size-1)/2.0
	outerR := float64(size)/2.0 - 1.0
	innerR := outerR - 2.5

	for y := range size {
		for x := range size {
			dx := float64(x) - cx
			dy := float64(y) - cy
			distSq := dx*dx + dy*dy
			if distSq > outerR*outerR {
				continue
			}
			if distSq >= innerR*innerR {
				dst.SetNRGBA(x, y, borderCol)
			} else {
				dst.SetNRGBA(x, y, bgCol)
			}
		}
	}

	initial := extractInitialRune(channel)
	drawGlyph5x7(dst, initial, (size-5*3)/2, (size-7*3)/2, 3, color.NRGBA{R: 0xF8, G: 0xFA, B: 0xFC, A: 0xFF})

	var buf bytes.Buffer
	_ = png.Encode(&buf, dst)
	return fyne.NewStaticResource("monogram-"+key+".png", buf.Bytes())
}

func extractInitialRune(s string) rune {
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			return unicode.ToUpper(r)
		}
	}
	return '?'
}

var glyph5x7 = map[rune][7]uint8{
	'A': {0x0E, 0x11, 0x11, 0x1F, 0x11, 0x11, 0x11},
	'B': {0x1E, 0x11, 0x11, 0x1E, 0x11, 0x11, 0x1E},
	'C': {0x0E, 0x11, 0x10, 0x10, 0x10, 0x11, 0x0E},
	'D': {0x1C, 0x12, 0x11, 0x11, 0x11, 0x12, 0x1C},
	'E': {0x1F, 0x10, 0x10, 0x1E, 0x10, 0x10, 0x1F},
	'F': {0x1F, 0x10, 0x10, 0x1E, 0x10, 0x10, 0x10},
	'G': {0x0E, 0x11, 0x10, 0x17, 0x11, 0x11, 0x0F},
	'H': {0x11, 0x11, 0x11, 0x1F, 0x11, 0x11, 0x11},
	'I': {0x0E, 0x04, 0x04, 0x04, 0x04, 0x04, 0x0E},
	'J': {0x07, 0x02, 0x02, 0x02, 0x12, 0x12, 0x0C},
	'K': {0x11, 0x12, 0x14, 0x18, 0x14, 0x12, 0x11},
	'L': {0x10, 0x10, 0x10, 0x10, 0x10, 0x10, 0x1F},
	'M': {0x11, 0x1B, 0x15, 0x15, 0x11, 0x11, 0x11},
	'N': {0x11, 0x19, 0x15, 0x13, 0x11, 0x11, 0x11},
	'O': {0x0E, 0x11, 0x11, 0x11, 0x11, 0x11, 0x0E},
	'P': {0x1E, 0x11, 0x11, 0x1E, 0x10, 0x10, 0x10},
	'Q': {0x0E, 0x11, 0x11, 0x11, 0x15, 0x12, 0x0D},
	'R': {0x1E, 0x11, 0x11, 0x1E, 0x14, 0x12, 0x11},
	'S': {0x0F, 0x10, 0x10, 0x0E, 0x01, 0x01, 0x1E},
	'T': {0x1F, 0x04, 0x04, 0x04, 0x04, 0x04, 0x04},
	'U': {0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x0E},
	'V': {0x11, 0x11, 0x11, 0x11, 0x11, 0x0A, 0x04},
	'W': {0x11, 0x11, 0x11, 0x15, 0x15, 0x15, 0x0A},
	'X': {0x11, 0x11, 0x0A, 0x04, 0x0A, 0x11, 0x11},
	'Y': {0x11, 0x11, 0x0A, 0x04, 0x04, 0x04, 0x04},
	'Z': {0x1F, 0x01, 0x02, 0x04, 0x08, 0x10, 0x1F},
	'0': {0x0E, 0x11, 0x13, 0x15, 0x19, 0x11, 0x0E},
	'1': {0x04, 0x0C, 0x04, 0x04, 0x04, 0x04, 0x0E},
	'2': {0x0E, 0x11, 0x01, 0x02, 0x04, 0x08, 0x1F},
	'3': {0x1F, 0x02, 0x04, 0x02, 0x01, 0x11, 0x0E},
	'4': {0x02, 0x06, 0x0A, 0x12, 0x1F, 0x02, 0x02},
	'5': {0x1F, 0x10, 0x1E, 0x01, 0x01, 0x11, 0x0E},
	'6': {0x06, 0x08, 0x10, 0x1E, 0x11, 0x11, 0x0E},
	'7': {0x1F, 0x01, 0x02, 0x04, 0x08, 0x08, 0x08},
	'8': {0x0E, 0x11, 0x11, 0x0E, 0x11, 0x11, 0x0E},
	'9': {0x0E, 0x11, 0x11, 0x0F, 0x01, 0x02, 0x0C},
	'?': {0x0E, 0x11, 0x01, 0x02, 0x04, 0x00, 0x04},
}

func drawGlyph5x7(dst *image.NRGBA, r rune, startX, startY, scale int, col color.NRGBA) {
	rows, ok := glyph5x7[r]
	if !ok {
		rows = glyph5x7['?']
	}
	for rowIdx, mask := range rows {
		for colIdx := range 5 {
			if (mask & (1 << (4 - colIdx))) != 0 {
				for dy := range scale {
					for dx := range scale {
						dst.SetNRGBA(startX+colIdx*scale+dx, startY+rowIdx*scale+dy, col)
					}
				}
			}
		}
	}
}
