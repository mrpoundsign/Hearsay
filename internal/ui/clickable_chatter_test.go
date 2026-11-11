package ui

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"
)

func TestClickableChatterLabel(t *testing.T) {
	testApp := test.NewApp()
	defer testApp.Quit()

	hovered := false
	unhovered := false
	tapped := false

	onHover := func(obj fyne.CanvasObject, tip string) {
		hovered = true
	}
	onUnhover := func(obj fyne.CanvasObject) {
		unhovered = true
	}

	label := newClickableChatterLabel(onHover, onUnhover)

	label.SetChatter("alice:", color.NRGBA{R: 0xFF, G: 0x00, B: 0x00, A: 0xFF}, "Open @alice profile in browser", func() {
		tapped = true
	})

	if cur := label.Cursor(); cur != desktop.PointerCursor {
		t.Fatalf("expected desktop.PointerCursor, got %v", cur)
	}

	label.Tapped(&fyne.PointEvent{})
	if !tapped {
		t.Fatalf("expected tapped callback to be invoked")
	}

	label.MouseIn(&desktop.MouseEvent{})
	if !hovered {
		t.Fatalf("expected onHover callback to be invoked")
	}
	if !label.canvasTxt.TextStyle.Underline {
		t.Fatalf("expected canvasTxt.TextStyle.Underline=true when hovered with onTap")
	}

	label.MouseOut()
	if !unhovered {
		t.Fatalf("expected onUnhover callback to be invoked")
	}
	if label.canvasTxt.TextStyle.Underline {
		t.Fatalf("expected canvasTxt.TextStyle.Underline=false after MouseOut")
	}

	// Non-clickable chatter
	label.SetChatter("system:", nil, "System info", nil)
	if cur := label.Cursor(); cur != desktop.DefaultCursor {
		t.Fatalf("expected desktop.DefaultCursor for non-clickable chatter, got %v", cur)
	}
}
