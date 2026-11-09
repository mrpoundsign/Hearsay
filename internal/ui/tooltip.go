package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

// TooltipButton extends widget.Button with desktop hover callbacks to display
// a floating tooltip popup and status hint when hovered.
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

func (mw *MainWindow) newTipBtn(label string, icon fyne.Resource, tooltip string, tapped func()) *TooltipButton {
	return NewTooltipButton(label, icon, tooltip, mw.showTooltip, mw.hideTooltip, tapped)
}

func (mw *MainWindow) showTooltip(btn *TooltipButton, text string) {
	mw.uiMu.Lock()
	defer mw.uiMu.Unlock()

	if text == "" || mw.Window == nil {
		return
	}
	if mw.nowSpeakingLbl != nil {
		mw.nowSpeakingLbl.SetText("Tip: " + text)
	}
	if mw.activeTipPop != nil {
		mw.activeTipPop.Hide()
		mw.activeTipPop = nil
	}
	cnv := mw.Window.Canvas()
	if cnv == nil || btn == nil {
		return
	}
	pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(btn)
	if pos.X <= 0 && pos.Y <= 0 {
		return
	}
	lbl := widget.NewLabel(text)
	pop := widget.NewPopUp(lbl, cnv)
	popY := pos.Y + btn.Size().Height + 4
	if popY+pop.MinSize().Height > cnv.Size().Height {
		popY = max(4, pos.Y-pop.MinSize().Height-4)
	}
	popX := min(pos.X, max(4, cnv.Size().Width-pop.MinSize().Width-8))
	pop.ShowAtPosition(fyne.NewPos(popX, popY))
	mw.activeTipPop = pop
	mw.activeTipOwner = btn
}

func (mw *MainWindow) hideTooltip(btn *TooltipButton) {
	mw.uiMu.Lock()
	defer mw.uiMu.Unlock()

	if btn != nil && mw.activeTipOwner != nil && mw.activeTipOwner != btn {
		return
	}
	if mw.activeTipPop != nil {
		mw.activeTipPop.Hide()
		mw.activeTipPop = nil
	}
	mw.activeTipOwner = nil
	if mw.nowSpeakingLbl != nil && mw.lastStatusText != "" {
		mw.nowSpeakingLbl.SetText(mw.lastStatusText)
	}
}
