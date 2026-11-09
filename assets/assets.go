package assets

import (
	_ "embed"

	"fyne.io/fyne/v2"
)

//go:embed icon.png
var iconPNG []byte

// AppIcon is the embedded Hearsay application icon resource.
var AppIcon = fyne.NewStaticResource("icon.png", iconPNG)
