package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// clickableChatterLabel is a colored, bold chatter name label that displays a native
// typography underline on hover and launches the user's platform profile in the default browser on click.
type clickableChatterLabel struct {
	widget.BaseWidget
	text      string
	textColor color.Color
	tooltip   string
	onHover   func(obj fyne.CanvasObject, tip string)
	onUnhover func(obj fyne.CanvasObject)
	onTap     func()

	hovered   bool
	canvasTxt *canvas.Text
}

var (
	_ fyne.Widget        = (*clickableChatterLabel)(nil)
	_ desktop.Hoverable  = (*clickableChatterLabel)(nil)
	_ desktop.Cursorable = (*clickableChatterLabel)(nil)
	_ fyne.Tappable      = (*clickableChatterLabel)(nil)
)

func newClickableChatterLabel(onHover func(fyne.CanvasObject, string), onUnhover func(fyne.CanvasObject)) *clickableChatterLabel {
	defaultCol := color.NRGBA{R: 0x38, G: 0xBD, B: 0xF8, A: 0xFF}
	w := &clickableChatterLabel{
		textColor: defaultCol,
		onHover:   onHover,
		onUnhover: onUnhover,
	}
	w.canvasTxt = canvas.NewText("", defaultCol)
	w.canvasTxt.TextStyle = fyne.TextStyle{Bold: true}
	w.ExtendBaseWidget(w)
	return w
}

func (w *clickableChatterLabel) SetChatter(name string, col color.Color, tooltip string, onTap func()) {
	w.text = name
	if col != nil {
		w.textColor = col
	}
	w.tooltip = tooltip
	w.onTap = onTap

	w.canvasTxt.Text = name
	w.canvasTxt.Color = w.textColor
	w.canvasTxt.TextSize = theme.TextSize()
	w.canvasTxt.TextStyle = fyne.TextStyle{Bold: true, Underline: w.hovered && w.onTap != nil}
	w.canvasTxt.Refresh()
	w.Refresh()
}

func (w *clickableChatterLabel) Cursor() desktop.Cursor {
	if w.onTap != nil {
		return desktop.PointerCursor
	}
	return desktop.DefaultCursor
}

func (w *clickableChatterLabel) Tapped(*fyne.PointEvent) {
	if w.onTap != nil {
		w.onTap()
	}
}

func (w *clickableChatterLabel) MouseIn(*desktop.MouseEvent) {
	w.hovered = true
	if w.onHover != nil && w.tooltip != "" {
		w.onHover(w, w.tooltip)
	}
	if w.onTap != nil {
		w.canvasTxt.TextStyle = fyne.TextStyle{Bold: true, Underline: true}
		w.canvasTxt.Refresh()
		w.Refresh()
	}
}

func (w *clickableChatterLabel) MouseMoved(*desktop.MouseEvent) {}

func (w *clickableChatterLabel) MouseOut() {
	w.hovered = false
	if w.onUnhover != nil {
		w.onUnhover(w)
	}
	if w.onTap != nil {
		w.canvasTxt.TextStyle = fyne.TextStyle{Bold: true, Underline: false}
		w.canvasTxt.Refresh()
		w.Refresh()
	}
}

func (w *clickableChatterLabel) CreateRenderer() fyne.WidgetRenderer {
	if w.canvasTxt == nil {
		w.canvasTxt = canvas.NewText(w.text, w.textColor)
		w.canvasTxt.TextStyle = fyne.TextStyle{Bold: true, Underline: w.hovered && w.onTap != nil}
	}
	return &clickableChatterRenderer{w: w}
}

type clickableChatterRenderer struct {
	w *clickableChatterLabel
}

func (r *clickableChatterRenderer) Destroy() {}

func (r *clickableChatterRenderer) Layout(size fyne.Size) {
	r.w.canvasTxt.Resize(size)
	r.w.canvasTxt.Move(fyne.NewPos(0, 0))
}

func (r *clickableChatterRenderer) MinSize() fyne.Size {
	return r.w.canvasTxt.MinSize()
}

func (r *clickableChatterRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.w.canvasTxt}
}

func (r *clickableChatterRenderer) Refresh() {
	r.w.canvasTxt.Text = r.w.text
	r.w.canvasTxt.Color = r.w.textColor
	r.w.canvasTxt.TextSize = theme.TextSize()
	r.w.canvasTxt.TextStyle = fyne.TextStyle{Bold: true, Underline: r.w.hovered && r.w.onTap != nil}
	r.w.canvasTxt.Refresh()
}
