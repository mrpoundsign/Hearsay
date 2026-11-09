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
	Tooltip  string
	onHover  func(btn *TooltipButton, text string)
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
