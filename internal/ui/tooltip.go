package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

// TooltipButton extends widget.Button with desktop hover callbacks to display
// a non-modal floating tooltip bubble and status hint when hovered.
type TooltipButton struct {
	widget.Button
	Tooltip   string
	onHover   func(btn *TooltipButton, text string)
	onUnhover func(btn *TooltipButton)
}

// NewTooltipButton creates a new button with a hover tooltip.
func NewTooltipButton(label string, icon fyne.Resource, tooltip string, onHover func(*TooltipButton, string), onUnhover func(*TooltipButton), tapped func()) *TooltipButton {
	b := &TooltipButton{
		Tooltip:   tooltip,
		onHover:   onHover,
		onUnhover: onUnhover,
	}
	b.Text = label
	b.Icon = icon
	b.OnTapped = tapped
	b.ExtendBaseWidget(b)
	return b
}

// SetTooltip updates the tooltip string for this button.
func (b *TooltipButton) SetTooltip(tooltip string) {
	b.Tooltip = tooltip
}

// MouseIn is called when the desktop cursor enters the button.
func (b *TooltipButton) MouseIn(ev *desktop.MouseEvent) {
	b.Button.MouseIn(ev)
	if b.onHover != nil && b.Tooltip != "" {
		b.onHover(b, b.Tooltip)
	}
}

// MouseMoved is called when the desktop cursor moves within the button.
func (b *TooltipButton) MouseMoved(ev *desktop.MouseEvent) {
	b.Button.MouseMoved(ev)
}

// MouseOut is called when the desktop cursor leaves the button.
func (b *TooltipButton) MouseOut() {
	b.Button.MouseOut()
	if b.onUnhover != nil {
		b.onUnhover(b)
	}
}

func buildTooltipBubble() (*fyne.Container, *canvas.Text) {
	bg := canvas.NewRectangle(color.NRGBA{R: 15, G: 23, B: 42, A: 245})
	bg.StrokeColor = color.NRGBA{R: 56, G: 189, B: 248, A: 220}
	bg.StrokeWidth = 1
	bg.CornerRadius = 5

	txt := canvas.NewText("", color.NRGBA{R: 248, G: 250, B: 252, A: 255})
	txt.TextSize = 12

	bubble := container.NewStack(bg, container.NewPadded(txt))
	bubble.Hide()
	return bubble, txt
}

func (mw *MainWindow) newTipBtn(label string, icon fyne.Resource, tooltip string, tapped func()) *TooltipButton {
	return NewTooltipButton(label, icon, tooltip, mw.showTooltip, mw.hideTooltip, tapped)
}

func (mw *MainWindow) showTooltip(btn *TooltipButton, text string) {
	mw.showTooltipForObject(btn, text)
}

func (mw *MainWindow) hideTooltip(btn *TooltipButton) {
	mw.hideTooltipForObject(btn)
}

func (mw *MainWindow) showTooltipForObject(obj fyne.CanvasObject, text string) {
	mw.uiMu.Lock()
	defer mw.uiMu.Unlock()

	if text == "" || mw.Window == nil {
		return
	}
	if mw.nowSpeakingLbl != nil {
		mw.nowSpeakingLbl.SetText("Tip: " + text)
	}
	if mw.tooltipBubble == nil || mw.tooltipText == nil || mw.tooltipLayer == nil || obj == nil {
		return
	}
	cnv := mw.Window.Canvas()
	if cnv == nil || len(cnv.Overlays().List()) > 0 {
		mw.tooltipBubble.Hide()
		return
	}
	appInstance := fyne.CurrentApp()
	if appInstance == nil || appInstance.Driver() == nil {
		return
	}
	pos := appInstance.Driver().AbsolutePositionForObject(obj)
	if pos.X <= 0 && pos.Y <= 0 {
		return
	}
	layerPos := appInstance.Driver().AbsolutePositionForObject(mw.tooltipLayer)
	relX := pos.X - layerPos.X
	relY := pos.Y - layerPos.Y

	mw.tooltipText.Text = text
	mw.tooltipText.Refresh()
	bubbleSize := mw.tooltipBubble.MinSize()
	mw.tooltipBubble.Resize(bubbleSize)

	cnvSize := cnv.Size()
	popY := relY + obj.Size().Height + 6
	if popY+bubbleSize.Height > cnvSize.Height-6 {
		popY = max(4, relY-bubbleSize.Height-6)
	}
	popX := max(4, min(relX, cnvSize.Width-bubbleSize.Width-8))
	mw.tooltipBubble.Move(fyne.NewPos(popX, popY))
	mw.tooltipBubble.Show()
	mw.tooltipBubble.Refresh()
	mw.activeTipOwner = obj
}

func (mw *MainWindow) hideTooltipForObject(obj fyne.CanvasObject) {
	mw.uiMu.Lock()
	defer mw.uiMu.Unlock()

	if obj != nil && mw.activeTipOwner != nil && mw.activeTipOwner != obj {
		return
	}
	if mw.tooltipBubble != nil {
		mw.tooltipBubble.Hide()
	}
	mw.activeTipOwner = nil
	if mw.nowSpeakingLbl != nil && mw.lastStatusText != "" {
		mw.nowSpeakingLbl.SetText(mw.lastStatusText)
	}
}
